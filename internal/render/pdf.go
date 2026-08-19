package render

import (
	"context"
	"errors"
	"fmt"
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
type pdfPrintFunc func(context.Context, string, string) ([]byte, error)

// PDFRenderer renders job HTML with an isolated Chromium-family process.
type PDFRenderer struct {
	outputDir   string
	browserPath string
	printToPDF  pdfPrintFunc
	mu          sync.Mutex
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
	if err := os.MkdirAll(filepath.Join(outputDir, "jobs"), 0o755); err != nil {
		return nil, fmt.Errorf("create PDF jobs directory: %w", err)
	}
	return &PDFRenderer{outputDir: outputDir, browserPath: resolved, printToPDF: chromedpPrintToPDF}, nil
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.renderLocked(ctx, job.ID, html)
}

func (r *PDFRenderer) renderLocked(ctx context.Context, jobID string, html []byte) (result string, err error) {
	jobsRoot := filepath.Join(r.outputDir, "jobs")
	if err := os.MkdirAll(jobsRoot, 0o755); err != nil {
		return "", fmt.Errorf("create PDF jobs directory: %w", err)
	}
	staging, err := os.MkdirTemp(jobsRoot, "."+jobID+"-")
	if err != nil {
		return "", fmt.Errorf("create render staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	htmlPath := filepath.Join(staging, "render.html")
	if err := writeAtomicFile(htmlPath, html, 0o600); err != nil {
		return "", fmt.Errorf("write render HTML: %w", err)
	}
	documentURL, err := fileURL(htmlPath)
	if err != nil {
		return "", err
	}
	pdf, err := r.printToPDF(ctx, r.browserPath, documentURL)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fmt.Errorf("render PDF: %w", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		return "", errors.New("renderer returned an invalid PDF")
	}
	if err := writeAtomicFile(filepath.Join(staging, "preview.pdf"), pdf, 0o600); err != nil {
		return "", fmt.Errorf("write preview PDF: %w", err)
	}

	destination := filepath.Join(jobsRoot, jobID)
	backup := filepath.Join(jobsRoot, "."+jobID+"-previous")
	_ = os.RemoveAll(backup)
	if _, statErr := os.Stat(destination); statErr == nil {
		if err := os.Rename(destination, backup); err != nil {
			return "", fmt.Errorf("prepare existing render replacement: %w", err)
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.Rename(backup, destination)
		return "", fmt.Errorf("publish rendered job: %w", err)
	}
	committed = true
	_ = os.RemoveAll(backup)
	return filepath.Join(destination, "preview.pdf"), nil
}

func writeAtomicFile(path string, contents []byte, permissions os.FileMode) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".render-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(permissions); err != nil {
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
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

func chromedpPrintToPDF(ctx context.Context, browserPath, documentURL string) ([]byte, error) {
	var pdf []byte
	err := withBrowserContext(ctx, browserPath, func(browserContext context.Context) error {
		timedContext, cancel := context.WithTimeout(browserContext, renderTimeout)
		defer cancel()
		return chromedp.Run(timedContext,
			chromedp.Navigate(documentURL),
			chromedp.Evaluate(`(async () => { if (document.readyState !== "complete") { await new Promise(resolve => window.addEventListener("load", resolve, {once:true})); } if (document.fonts && document.fonts.ready) { await document.fonts.ready; } return true; })()`, nil),
			chromedp.ActionFunc(func(actionContext context.Context) error {
				data, _, err := page.PrintToPDF().WithPrintBackground(true).WithPreferCSSPageSize(true).Do(actionContext)
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
