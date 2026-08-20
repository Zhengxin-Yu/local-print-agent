package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"local-print-agent/internal/jobs"
)

const (
	browserVersionTimeout = 2 * time.Second
	renderTimeout         = 45 * time.Second
)

var (
	generatedJobID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	versionNumber  = regexp.MustCompile(`(?:^|[^0-9])([0-9]{2,3})\.[0-9]+(?:\.[0-9]+){0,2}(?:$|[^0-9])`)
)

type versionProbe func(context.Context, string) (int, error)
type pathUnsafeFunc func(string, os.FileInfo) (bool, error)
type pdfPrintOptions struct {
	displayHeaderFooter            bool
	headerTemplate, footerTemplate string
	marginTop, marginBottom        float64
	marginLeft, marginRight        float64
}

type pdfPrintFunc func(context.Context, string, string, pdfPrintOptions) ([]byte, error)

// PDFRenderer renders job HTML with an isolated Chromium-family process.
type PDFRenderer struct {
	outputDir   string
	browserPath string
	printToPDF  pdfPrintFunc
	permitOnce  sync.Once
	permit      chan struct{}
	pathUnsafe  pathUnsafeFunc
}

var _ Renderer = (*PDFRenderer)(nil)

// NewPDFRenderer discovers and validates Chrome before accepting work.
func NewPDFRenderer(outputDir, browserPath string) (*PDFRenderer, error) {
	return newPDFRenderer(outputDir, browserPath, probeBrowserMajor)
}

func newPDFRenderer(outputDir, browserPath string, probe versionProbe) (*PDFRenderer, error) {
	if strings.TrimSpace(outputDir) == "" {
		return nil, errors.New("PDF renderer output directory is required")
	}
	resolved, err := discoverBrowser(browserPath)
	if err != nil {
		return nil, err
	}
	if probe == nil {
		probe = probeBrowserMajor
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserVersionTimeout+renderTimeout)
	defer cancel()
	major, err := probe(ctx, resolved)
	if err != nil || major < MinimumChromeMajor {
		return nil, &jobs.JobError{Code: jobs.ErrorCodeRendererUnsupported, Message: fmt.Sprintf("Chromium %d or newer is required", MinimumChromeMajor)}
	}
	renderer := &PDFRenderer{outputDir: outputDir, browserPath: resolved, printToPDF: chromedpPrintToPDF, pathUnsafe: platformPathUnsafe}
	if err := renderer.ensureJobsRoot(); err != nil {
		return nil, fmt.Errorf("prepare PDF jobs directory: %w", err)
	}
	if err := recoverInterruptedPublishes(filepath.Join(outputDir, "jobs"), renderer.pathUnsafeCheck()); err != nil {
		return nil, fmt.Errorf("recover interrupted PDF publish: %w", err)
	}
	return renderer, nil
}

func discoverBrowser(configured string) (string, error) {
	if configured != "" {
		return validateBrowserPath(configured)
	}
	if environment := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_BROWSER_PATH")); environment != "" {
		return validateBrowserPath(environment)
	}
	for _, name := range []string{"chrome", "google-chrome", "chromium", "chromium-browser", "msedge", "microsoft-edge"} {
		if found, err := exec.LookPath(name); err == nil {
			if validated, err := validateBrowserPath(found); err == nil {
				return validated, nil
			}
		}
	}
	for _, candidate := range commonBrowserPaths() {
		if validated, err := validateBrowserPath(candidate); err == nil {
			return validated, nil
		}
	}
	return "", rendererNotFound()
}

func validateBrowserPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", rendererNotFound()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", rendererNotFound()
	}
	return abs, nil
}

func rendererNotFound() error {
	return &jobs.JobError{Code: jobs.ErrorCodeRendererNotFound, Message: "Chromium renderer is unavailable"}
}

func commonBrowserPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}
	}
	return []string{
		"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser",
		"/usr/bin/microsoft-edge", "/usr/bin/microsoft-edge-stable", "/opt/google/chrome/chrome",
	}
}

func probeBrowserMajor(ctx context.Context, browserPath string) (int, error) {
	profile, profileErr := os.MkdirTemp("", "local-print-agent-version-")
	var output []byte
	commandErr := profileErr
	if profileErr == nil {
		versionContext, cancel := context.WithTimeout(ctx, browserVersionTimeout)
		command := exec.CommandContext(versionContext, browserPath, browserVersionArguments(profile)...)
		command.WaitDelay = time.Second
		output, commandErr = command.CombinedOutput()
		cancel()
		_ = os.RemoveAll(profile)
	}
	if commandErr == nil {
		if major, err := parseBrowserMajor(string(output)); err == nil {
			return major, nil
		}

	}
	product, err := chromedpBrowserProduct(ctx, browserPath)
	if err != nil {
		if commandErr != nil {
			return 0, fmt.Errorf("browser version command and startup failed: %w", err)
		}
		return 0, err
	}
	return parseBrowserMajor(product)
}

func browserVersionArguments(profile string) []string {
	return []string{
		"--version",
		"--headless=new",
		"--disable-gpu",
		"--disable-breakpad",
		"--no-first-run",
		"--user-data-dir=" + profile,
	}
}

func parseBrowserMajor(output string) (int, error) {
	match := versionNumber.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, errors.New("browser version is not recognizable")
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, errors.New("browser major version is invalid")
	}
	return major, nil
}

func chromedpBrowserProduct(ctx context.Context, browserPath string) (string, error) {
	var product string
	err := withBrowserContext(ctx, browserPath, func(browserContext context.Context) error {
		return chromedp.Run(browserContext, chromedp.ActionFunc(func(actionContext context.Context) error {
			_, value, _, _, _, err := browser.GetVersion().Do(actionContext)
			product = value
			return err
		}))
	})
	return product, err
}

// Render produces render.html and preview.pdf under outputDir/jobs/<jobID>.
func (r *PDFRenderer) Render(ctx context.Context, job *jobs.Job) (string, error) {
	if ctx == nil {
		return "", context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if r == nil || strings.TrimSpace(r.outputDir) == "" || r.printToPDF == nil {
		return "", errors.New("PDF renderer is not initialized")
	}
	if job == nil || !generatedJobID.MatchString(job.ID) {
		return "", errors.New("PDF renderer requires a generated job ID")
	}
	var html []byte
	var err error
	switch job.Type {
	case jobs.JobTypeBalloon:
		html, err = RenderBalloonHTML(job)
	case jobs.JobTypeSource:
		html, err = RenderSourceHTML(job)
	default:
		return "", fmt.Errorf("unsupported render job type %q", job.Type)
	}
	if err != nil {
		return "", err
	}
	r.permitOnce.Do(func() { r.permit = make(chan struct{}, 1) })
	select {
	case r.permit <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-r.permit }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	options, err := printOptionsForJob(job)
	if err != nil {
		return "", err
	}
	result, err := r.renderLocked(ctx, job.ID, html, options)
	if err == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	var jobError *jobs.JobError
	if errors.As(err, &jobError) {
		return "", err
	}
	return "", jobs.NewJobError(jobs.ErrorCodeRenderFailed, "PDF rendering failed", err)
}

func printOptionsForJob(job *jobs.Job) (pdfPrintOptions, error) {
	if job.Type != jobs.JobTypeSource {
		return pdfPrintOptions{}, nil
	}
	var payload jobs.SourceCodePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return pdfPrintOptions{}, fmt.Errorf("decode source header: %w", err)
	}
	escape := func(value string) string {
		if strings.TrimSpace(value) == "" {
			value = "未提供"
		}
		return html.EscapeString(value)
	}
	header := fmt.Sprintf(`<div style="box-sizing:border-box;width:100%%;margin:0 13mm;border-bottom:1px solid #777;padding:0 0 2mm;font:9px 'Microsoft YaHei',sans-serif;color:#333;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">比赛：%s　队伍：%s　队名：%s　房间：%s　题目：%s　打印编号：%s</div>`, escape(payload.ContestName), escape(payload.TeamID), escape(payload.TeamName), escape(payload.Room), escape(payload.ProblemID), escape(job.ID))
	footer := `<div style="width:100%;text-align:center;font:8px 'Microsoft YaHei',sans-serif;color:#555">第 <span class="pageNumber"></span> / <span class="totalPages"></span> 页</div>`
	return pdfPrintOptions{displayHeaderFooter: true, headerTemplate: header, footerTemplate: footer, marginTop: 0.82, marginBottom: 0.62, marginLeft: 0.51, marginRight: 0.51}, nil
}

func (r *PDFRenderer) renderLocked(ctx context.Context, jobID string, html []byte, options pdfPrintOptions) (result string, err error) {
	jobsRoot := filepath.Join(r.outputDir, "jobs")
	if err := r.ensureJobsRoot(); err != nil {
		return "", fmt.Errorf("prepare PDF jobs directory: %w", err)
	}
	destination := filepath.Join(jobsRoot, jobID)
	if err := r.validatePublishDirectories(destination); err != nil {
		return "", err
	}
	_, statErr := os.Stat(destination)
	createdDestination := os.IsNotExist(statErr)
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("inspect render directory: %w", statErr)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("create render directory: %w", err)
	}
	if err := r.validatePublishDirectories(destination); err != nil {
		return "", err
	}
	var temporaryPaths []string
	defer func() {
		for _, path := range temporaryPaths {
			if path == "" {
				continue
			}
			if cleanupErr := os.Remove(path); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = errors.Join(err, fmt.Errorf("clean render staging file: %w", cleanupErr))
			}
		}
		if err != nil && createdDestination {
			if cleanupErr := os.Remove(destination); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = errors.Join(err, fmt.Errorf("clean failed render directory: %w", cleanupErr))
			}
		}
	}()

	validateDestination := func() error { return r.validatePublishDirectories(destination) }
	htmlPath, err := writeTemporaryFile(destination, ".render-*.html", html, 0o600, validateDestination)
	if err != nil {
		return "", fmt.Errorf("write render HTML: %w", err)
	}
	temporaryPaths = append(temporaryPaths, htmlPath)
	documentURL, err := fileURL(htmlPath)
	if err != nil {
		return "", err
	}
	pdf, err := r.printToPDF(ctx, r.browserPath, documentURL, options)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fmt.Errorf("render PDF: %w", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		return "", errors.New("renderer returned an invalid PDF")
	}
	pdfPath, err := writeTemporaryFile(destination, ".render-*.pdf", pdf, 0o600, validateDestination)
	if err != nil {
		return "", fmt.Errorf("write preview PDF: %w", err)
	}
	temporaryPaths = append(temporaryPaths, pdfPath)
	if err := validateDestination(); err != nil {
		return "", err
	}
	if err := atomicReplaceFile(pdfPath, filepath.Join(destination, "preview.pdf")); err != nil {
		return "", fmt.Errorf("publish preview PDF: %w", err)
	}
	temporaryPaths[1] = ""
	if err := validateDestination(); err != nil {
		return "", err
	}
	if err := atomicReplaceFile(htmlPath, filepath.Join(destination, "render.html")); err != nil {
		return "", fmt.Errorf("publish render HTML: %w", err)
	}
	temporaryPaths[0] = ""
	return filepath.Join(destination, "preview.pdf"), nil
}

func writeTemporaryFile(directory, pattern string, contents []byte, permissions os.FileMode, validateDirectory func() error) (path string, err error) {
	if err := validateDirectory(); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if err != nil {
			if removeErr := os.Remove(temporaryPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if err := validateDirectory(); err != nil {
		return "", err
	}
	if err := temporary.Chmod(permissions); err != nil {
		return "", err
	}
	if _, err := temporary.Write(contents); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	closed = true
	return temporaryPath, nil
}

func recoverInterruptedPublishes(jobsRoot string, check pathUnsafeFunc) error {
	entries, err := os.ReadDir(jobsRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, "-previous") {
			continue
		}
		jobID := strings.TrimSuffix(strings.TrimPrefix(name, "."), "-previous")
		if !generatedJobID.MatchString(jobID) {
			continue
		}
		legacy := filepath.Join(jobsRoot, name)
		destination := filepath.Join(jobsRoot, jobID)
		legacyInfo, infoErr := os.Lstat(legacy)
		if infoErr != nil {
			return infoErr
		}
		if unsafe, unsafeErr := isUnsafeRenderPath(legacy, legacyInfo, check); unsafeErr != nil {
			return unsafeErr
		} else if unsafe {
			return errors.New("interrupted render path is unsafe")
		}
		if !legacyInfo.IsDir() {
			continue
		}
		if _, statErr := os.Stat(destination); os.IsNotExist(statErr) {
			if err := os.Rename(legacy, destination); err != nil {
				return err
			}
			if err := validateExistingRenderDirectory(destination, check); err != nil {
				return err
			}
		} else if statErr != nil {
			return statErr
		} else {
			if err := validateExistingRenderDirectory(destination, check); err != nil {
				return err
			}
			if err := os.RemoveAll(legacy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *PDFRenderer) pathUnsafeCheck() pathUnsafeFunc {
	if r != nil && r.pathUnsafe != nil {
		return r.pathUnsafe
	}
	return platformPathUnsafe
}

func (r *PDFRenderer) ensureJobsRoot() error {
	if r == nil || strings.TrimSpace(r.outputDir) == "" {
		return errors.New("PDF renderer output directory is required")
	}
	if err := validateNoUnsafeExistingComponents(r.outputDir, r.pathUnsafeCheck()); err != nil {
		return err
	}
	if err := validateExistingRenderDirectory(r.outputDir, r.pathUnsafeCheck()); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(r.outputDir, 0o755); err != nil {
			return err
		}
		if err := validateExistingRenderDirectory(r.outputDir, r.pathUnsafeCheck()); err != nil {
			return err
		}
	}
	jobsRoot := filepath.Join(r.outputDir, "jobs")
	if err := validateExistingRenderDirectory(jobsRoot, r.pathUnsafeCheck()); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(jobsRoot, 0o755); err != nil {
			return err
		}
	}
	return r.validatePublishDirectories("")
}

func (r *PDFRenderer) validatePublishDirectories(destination string) error {
	if r == nil {
		return errors.New("PDF renderer is not initialized")
	}
	paths := []string{r.outputDir, filepath.Join(r.outputDir, "jobs")}
	if destination != "" {
		paths = append(paths, destination)
	}
	for index, path := range paths {
		if err := validateNoUnsafeExistingComponents(path, r.pathUnsafeCheck()); err != nil {
			return err
		}
		err := validateExistingRenderDirectory(path, r.pathUnsafeCheck())
		if os.IsNotExist(err) && index == len(paths)-1 && destination != "" {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func validateNoUnsafeExistingComponents(path string, check pathUnsafeFunc) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	var components []string
	for current := absolute; ; current = filepath.Dir(current) {
		components = append(components, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for index := len(components) - 1; index >= 0; index-- {
		component := components[index]
		info, err := os.Lstat(component)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		unsafe, err := isUnsafeRenderPath(component, info, check)
		if err != nil {
			return err
		}
		if unsafe {
			return errors.New("render path contains a link or reparse point")
		}
	}
	return nil
}

func validateExistingRenderDirectory(path string, check pathUnsafeFunc) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	unsafe, err := isUnsafeRenderPath(path, info, check)
	if err != nil {
		return err
	}
	if unsafe {
		return errors.New("render path contains a link or reparse point")
	}
	if !info.IsDir() {
		return errors.New("render path component is not a directory")
	}
	return nil
}

func isUnsafeRenderPath(path string, info os.FileInfo, check pathUnsafeFunc) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	if check == nil {
		check = platformPathUnsafe
	}
	return check(path, info)
}

func fileURL(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve render HTML path: %w", err)
	}
	slashed := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" && !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String(), nil
}

func chromedpPrintToPDF(ctx context.Context, browserPath, documentURL string, options pdfPrintOptions) ([]byte, error) {
	var pdf []byte
	err := withBrowserContext(ctx, browserPath, func(browserContext context.Context) error {
		timedContext, cancel := context.WithTimeout(browserContext, renderTimeout)
		defer cancel()
		return chromedp.Run(timedContext,
			chromedp.Navigate(documentURL),
			chromedp.Evaluate(`(async () => { if (document.readyState !== "complete") { await new Promise(resolve => window.addEventListener("load", resolve, {once:true})); } if (document.fonts && document.fonts.ready) { await document.fonts.ready; } return true; })()`, nil),
			chromedp.ActionFunc(func(actionContext context.Context) error {
				command := page.PrintToPDF().WithPrintBackground(true).WithPreferCSSPageSize(true)
				if options.displayHeaderFooter {
					command = command.WithDisplayHeaderFooter(true).
						WithHeaderTemplate(options.headerTemplate).WithFooterTemplate(options.footerTemplate).
						WithMarginTop(options.marginTop).WithMarginBottom(options.marginBottom).
						WithMarginLeft(options.marginLeft).WithMarginRight(options.marginRight)
				}
				data, _, err := command.Do(actionContext)
				pdf = data
				return err
			}),
		)
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return pdf, nil
}

func withBrowserContext(ctx context.Context, browserPath string, run func(context.Context) error) error {
	if ctx == nil {
		return context.Canceled
	}
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options, chromedp.ExecPath(browserPath))
	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(ctx, options...)
	defer cancelAllocator()
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext)
	defer cancelBrowser()
	return run(browserContext)
}
