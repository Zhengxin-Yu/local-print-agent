package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"local-print-agent/internal/httpapi"
	"local-print-agent/internal/jobs"
)

type fixedPreviewJobs struct{ job *jobs.Job }

func (s fixedPreviewJobs) Create(context.Context, jobs.CreateJobRequest) (*jobs.Job, error) {
	return nil, errors.New("unused")
}
func (s fixedPreviewJobs) Get(context.Context, string) (*jobs.Job, error) { return s.job, nil }
func (s fixedPreviewJobs) List(context.Context) ([]*jobs.Job, error) {
	return nil, errors.New("unused")
}
func (s fixedPreviewJobs) Retry(context.Context, string) (*jobs.Job, error) {
	return nil, errors.New("unused")
}

func TestNewPDFRendererRejectsExplicitMissingBrowserWithoutLeakingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "private-browser", "chrome.exe")

	_, err := NewPDFRenderer(t.TempDir(), missing)
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) {
		t.Fatalf("NewPDFRenderer() error = %T %v, want *jobs.JobError", err, err)
	}
	if jobError.Code != jobs.ErrorCode("RENDERER_NOT_FOUND") {
		t.Fatalf("NewPDFRenderer() code = %q, want RENDERER_NOT_FOUND", jobError.Code)
	}
	if strings.Contains(jobError.Message, missing) {
		t.Fatalf("NewPDFRenderer() leaked browser path in message %q", jobError.Message)
	}
}

func TestPDFRendererChromeIntegration(t *testing.T) {
	browser := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_CHROME_E2E"))
	if browser == "" {
		t.Skip("set LOCAL_PRINT_AGENT_CHROME_E2E to run real Chromium PDF rendering")
	}
	root := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_E2E_OUTPUT"))
	if root == "" {
		root = t.TempDir()
	} else if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	major, err := probeBrowserMajor(context.Background(), browser)
	if err != nil {
		t.Fatalf("probe real Chromium version: %v", err)
	}
	t.Logf("Chromium major: %d", major)
	renderer, err := NewPDFRenderer(root, browser)
	if err != nil {
		t.Fatal(err)
	}

	balloon := &jobs.Job{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Type: jobs.JobTypeBalloon, Payload: []byte(`{"contest_name":"比赛名","team_id":"team001","team_name":"Team Atlas","room":"A101","problem_id":"C","balloon_color":"red","solved_at":"2026-08-19T09:30:00+08:00"}`)}
	balloonPDF, err := renderer.Render(context.Background(), balloon)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balloon PDF: %s", balloonPDF)
	if pages := pdfPageCount(t, balloonPDF); pages != 1 {
		t.Fatalf("balloon PDF pages = %d, want 1", pages)
	}

	var source strings.Builder
	for line := 1; line <= 180; line++ {
		fmt.Fprintf(&source, "// 中文注释 第 %03d 行：用于验证行号、分页、页眉和页码\nint value_%03d = %d;\n", line, line, line)
	}
	payload, err := json.Marshal(jobs.SourceCodePayload{Language: "cpp", SourceCode: source.String(), ContestName: "比赛名", TeamID: "team001", TeamName: "Team Atlas", Room: "A101", ProblemID: "C"})
	if err != nil {
		t.Fatal(err)
	}
	sourceJob := &jobs.Job{ID: "cccccccccccccccccccccccccccccccc", Type: jobs.JobTypeSource, Payload: payload}
	sourcePDF, err := renderer.Render(context.Background(), sourceJob)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("source PDF: %s", sourcePDF)
	if pages := pdfPageCount(t, sourcePDF); pages < 2 {
		t.Fatalf("source PDF pages = %d, want at least 2", pages)
	} else {
		t.Logf("source PDF pages: %d", pages)
	}
	if rasterizer := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_PDFTOPPM_E2E")); rasterizer != "" {
		assertSourcePDFGeometry(t, rasterizer, sourcePDF, 1, 2)
	}
}

func assertSourcePDFGeometry(t *testing.T, rasterizer, pdfPath string, pages ...int) {
	t.Helper()
	for _, pageNumber := range pages {
		prefix := filepath.Join(t.TempDir(), fmt.Sprintf("page-%d", pageNumber))
		command := exec.Command(rasterizer, "-png", "-f", strconv.Itoa(pageNumber), "-l", strconv.Itoa(pageNumber), "-singlefile", "-r", "150", pdfPath, prefix)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("rasterize source page %d: %v: %s", pageNumber, err, output)
		}
		file, err := os.Open(prefix + ".png")
		if err != nil {
			t.Fatal(err)
		}
		pageImage, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		assertSourcePageGeometry(t, pageNumber, pageImage)
	}
}

func assertSourcePageGeometry(t *testing.T, pageNumber int, pageImage image.Image) {
	t.Helper()
	bounds := pageImage.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dark := func(x, y int) bool {
		r, g, b, _ := pageImage.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
		return r < 0xb000 && g < 0xb000 && b < 0xb000
	}
	ruleRow, rulePixels := -1, 0
	for y := 0; y < height/8; y++ {
		count := 0
		for x := 0; x < width; x++ {
			if dark(x, y) {
				count++
			}
		}
		if count > rulePixels {
			ruleRow, rulePixels = y, count
		}
	}
	if rulePixels < width/2 {
		t.Fatalf("source page %d lacks the CDP header separator", pageNumber)
	}
	for y := 0; y <= ruleRow+20; y++ {
		for x := 0; x < width/20; x++ {
			if dark(x, y) {
				t.Fatalf("source page %d has body ink above/beside the header separator at (%d,%d)", pageNumber, x, y)
			}
		}
	}
	firstBodyRow := height
	for y := ruleRow + 5; y < height*9/10 && firstBodyRow == height; y++ {
		for x := width / 20; x < width*9/10; x++ {
			if dark(x, y) {
				firstBodyRow = y
				break
			}
		}
	}
	if firstBodyRow-ruleRow < height/40 {
		t.Fatalf("source page %d header/body gap = %dpx, want at least %dpx", pageNumber, firstBodyRow-ruleRow, height/40)
	}
	footFirst := height
	for y := height * 96 / 100; y < height; y++ {
		for x := width * 2 / 5; x < width*3/5; x++ {
			if dark(x, y) {
				footFirst = y
				break
			}
		}
		if footFirst != height {
			break
		}
	}
	if footFirst == height {
		t.Fatalf("source page %d lacks a bottom page-number footer", pageNumber)
	}
	lastBodyRow := firstBodyRow
	for y := firstBodyRow; y < footFirst; y++ {
		for x := width / 20; x < width/8; x++ {
			if dark(x, y) {
				lastBodyRow = y
				break
			}
		}
	}
	if footFirst-lastBodyRow < height/50 {
		t.Fatalf("source page %d body/footer gap = %dpx, want at least %dpx", pageNumber, footFirst-lastBodyRow, height/50)
	}
	t.Logf("source page %d geometry: header rule y=%d, body y=%d, footer y=%d, height=%d", pageNumber, ruleRow, firstBodyRow, footFirst, height)
}

func TestPrintOptionsUseCDPHeaderAndFooterOnlyForSourceJobs(t *testing.T) {
	source := &jobs.Job{ID: "cccccccccccccccccccccccccccccccc", Type: jobs.JobTypeSource, Payload: []byte(`{"contest_name":"比赛<script>","team_id":"team001","team_name":"Team Atlas","room":"A101","problem_id":"C"}`)}
	options, err := printOptionsForJob(source)
	if err != nil {
		t.Fatal(err)
	}
	if !options.displayHeaderFooter || options.marginTop <= 0 || options.marginBottom <= 0 {
		t.Fatalf("source options = %#v, want CDP header/footer and explicit margins", options)
	}
	for _, want := range []string{"比赛&lt;script&gt;", "Team Atlas", "pageNumber", "totalPages"} {
		if !strings.Contains(options.headerTemplate+options.footerTemplate, want) {
			t.Errorf("source header/footer lacks %q", want)
		}
	}
	if strings.Contains(options.headerTemplate, "<script>") {
		t.Fatal("source header template contains unescaped metadata")
	}

	balloon, err := printOptionsForJob(&jobs.Job{Type: jobs.JobTypeBalloon})
	if err != nil {
		t.Fatal(err)
	}
	if balloon.displayHeaderFooter || balloon.headerTemplate != "" || balloon.footerTemplate != "" {
		t.Fatalf("balloon options = %#v, must not include source header/footer", balloon)
	}
}

func pdfPageCount(t *testing.T, path string) int {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(contents), "%PDF-") || !strings.Contains(string(contents), "%%EOF") {
		t.Fatalf("%s is not a complete PDF", path)
	}
	countPattern := regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+([0-9]+)`)
	maxPages := 0
	for _, match := range countPattern.FindAllSubmatch(contents, -1) {
		pages, err := strconv.Atoi(string(match[1]))
		if err == nil && pages > maxPages {
			maxPages = pages
		}
	}
	if maxPages == 0 {
		t.Fatalf("%s has no parseable PDF page tree", path)
	}
	return maxPages
}

func TestNewPDFRendererUsesEnvironmentDiscoveryAndRequiresChrome131(t *testing.T) {
	browser := filepath.Join(t.TempDir(), "chrome.exe")
	if err := os.WriteFile(browser, []byte("browser"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCAL_PRINT_AGENT_BROWSER_PATH", browser)

	var probed string
	renderer, err := newPDFRenderer(t.TempDir(), "", func(_ context.Context, path string) (int, error) {
		probed = path
		return MinimumChromeMajor, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if renderer.browserPath != browser || probed != browser {
		t.Fatalf("browser path = %q, probe = %q, want environment path %q", renderer.browserPath, probed, browser)
	}

	_, err = newPDFRenderer(t.TempDir(), browser, func(context.Context, string) (int, error) {
		return MinimumChromeMajor - 1, nil
	})
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) || jobError.Code != jobs.ErrorCodeRendererUnsupported {
		t.Fatalf("old browser error = %T %v, want RENDERER_VERSION_UNSUPPORTED", err, err)
	}
	if strings.Contains(jobError.Message, browser) {
		t.Fatalf("old browser error leaked path: %q", jobError.Message)
	}
}

func TestParseBrowserMajorAcceptsChromiumFamilyVersions(t *testing.T) {
	for _, test := range []struct {
		output string
		want   int
	}{
		{output: "Google Chrome 143.0.7499.170", want: 143},
		{output: "Chromium 131.0.6778.85 snap", want: 131},
		{output: "Microsoft Edge 142.0.3595.94", want: 142},
	} {
		major, err := parseBrowserMajor(test.output)
		if err != nil || major != test.want {
			t.Errorf("parseBrowserMajor(%q) = %d, %v; want %d", test.output, major, err, test.want)
		}
	}
	if _, err := parseBrowserMajor("browser version unavailable"); err == nil {
		t.Fatal("parseBrowserMajor accepted output without a dotted version")
	}
}

func TestBrowserVersionCommandUsesIsolatedProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "version profile")
	arguments := browserVersionArguments(profile)
	joined := strings.Join(arguments, "\x00")
	for _, want := range []string{"--version", "--headless=new", "--disable-breakpad", "--user-data-dir=" + profile} {
		if !strings.Contains(joined, want) {
			t.Fatalf("browser version arguments %q lack %q", arguments, want)
		}
	}
}

func TestPDFRendererWritesSelectedHTMLAndPDFOnlyInsidePrivateJobDirectory(t *testing.T) {
	root := t.TempDir()
	jobID := "0123456789abcdef0123456789abcdef"
	var openedURL string
	renderer := &PDFRenderer{
		outputDir: root,
		printToPDF: func(_ context.Context, _ string, documentURL string, _ pdfPrintOptions) ([]byte, error) {
			openedURL = documentURL
			parsed, err := url.Parse(documentURL)
			if err != nil || parsed.Scheme != "file" || strings.Contains(documentURL, `\`) {
				t.Fatalf("document URL = %q, parse error = %v", documentURL, err)
			}
			return []byte("%PDF-1.7\nrendered"), nil
		},
	}
	job := &jobs.Job{ID: jobID, Type: jobs.JobTypeBalloon, Payload: []byte(`{"team_name":"Team Atlas","problem_id":"C","solved_at":"2026-08-19T09:30:00+08:00"}`)}

	pdfPath, err := renderer.Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(root, "jobs", jobID)
	if pdfPath != filepath.Join(wantDir, "preview.pdf") {
		t.Fatalf("PDF path = %q, want %q", pdfPath, filepath.Join(wantDir, "preview.pdf"))
	}
	page, err := os.ReadFile(filepath.Join(wantDir, "render.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "气球小票") || strings.Contains(string(page), "源码打印") {
		t.Fatalf("renderer selected wrong HTML: %s", page)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil || string(pdf) != "%PDF-1.7\nrendered" {
		t.Fatalf("PDF contents = %q, %v", pdf, err)
	}
	if openedURL == "" {
		t.Fatal("PDF runner did not receive the render.html file URL")
	}
	if parsed, err := url.Parse(openedURL); err != nil || !strings.HasSuffix(parsed.Path, ".html") {
		t.Fatalf("renderer staging URL = %q, want an .html suffix for Chromium MIME detection", openedURL)
	}
}

func TestPDFRendererSelectsSourceHTMLAndRejectsUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	renderer := &PDFRenderer{
		outputDir: root,
		printToPDF: func(_ context.Context, _ string, documentURL string, _ pdfPrintOptions) ([]byte, error) {
			parsed, err := url.Parse(documentURL)
			if err != nil {
				t.Fatal(err)
			}
			path := parsed.Path
			if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
				path = path[1:]
			}
			path = filepath.FromSlash(path)
			if parsed.Host != "" {
				path = `//` + parsed.Host + parsed.Path
			}
			page, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(page), "源码打印") || !strings.Contains(string(page), "中文注释") {
				t.Fatalf("renderer selected wrong source HTML: %s", page)
			}
			return []byte("%PDF-1.7\nsource"), nil
		},
	}
	source := &jobs.Job{ID: "fedcba9876543210fedcba9876543210", Type: jobs.JobTypeSource, Payload: []byte(`{"language":"cpp","source_code":"// 中文注释\nint main() {}"}`)}
	if _, err := renderer.Render(context.Background(), source); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"", "../escape", `..\escape`, "ABCDEF0123456789ABCDEF0123456789", "short", "0123456789abcdef0123456789abcdef/preview.pdf"} {
		t.Run(id, func(t *testing.T) {
			job := *source
			job.ID = id
			if _, err := renderer.Render(context.Background(), &job); err == nil {
				t.Fatalf("Render accepted unsafe/generated-invalid ID %q", id)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unsafe job created path outside jobs root: %v", err)
	}
}

func TestPDFRendererCleansFailedOutputAndPropagatesContext(t *testing.T) {
	root := t.TempDir()
	jobID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	secretPath := filepath.Join(root, "private-profile", "staging", "render.html")
	runnerError := fmt.Errorf("browser crashed opening %s", secretPath)
	renderer := &PDFRenderer{
		outputDir:  root,
		printToPDF: func(context.Context, string, string, pdfPrintOptions) ([]byte, error) { return nil, runnerError },
	}
	job := &jobs.Job{ID: jobID, Type: jobs.JobTypeBalloon, Payload: []byte(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)}
	if _, err := renderer.Render(context.Background(), job); !errors.Is(err, runnerError) {
		t.Fatalf("Render error = %v, want wrapped runner error", err)
	} else {
		var jobError *jobs.JobError
		if !errors.As(err, &jobError) || jobError.Code != jobs.ErrorCodeRenderFailed || jobError.Message != "PDF rendering failed" {
			t.Fatalf("Render public error = %#v, want stable RENDER_FAILED", jobError)
		}
		if strings.Contains(jobError.Message, secretPath) {
			t.Fatalf("Render public error leaked staging path: %q", jobError.Message)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "jobs", jobID)); !os.IsNotExist(err) {
		t.Fatalf("failed render left job directory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renderer.Render(ctx, job); !errors.Is(err, context.Canceled) {
		t.Fatalf("Render canceled error = %v, want context.Canceled", err)
	}
}

func TestPDFRendererRepublishesFilesWithoutReplacingStableJobDirectory(t *testing.T) {
	root := t.TempDir()
	jobID := "dddddddddddddddddddddddddddddddd"
	destination := filepath.Join(root, "jobs", jobID)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "preview.pdf"), []byte("%PDF-old"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &PDFRenderer{outputDir: root, printToPDF: func(context.Context, string, string, pdfPrintOptions) ([]byte, error) {
		return []byte("%PDF-new"), nil
	}}
	job := &jobs.Job{ID: jobID, Type: jobs.JobTypeBalloon, Payload: []byte(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)}
	if _, err := renderer.Render(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("renderer replaced the public job directory, creating a preview lookup gap")
	}
	contents, err := os.ReadFile(filepath.Join(destination, "preview.pdf"))
	if err != nil || string(contents) != "%PDF-new" {
		t.Fatalf("published PDF = %q, %v", contents, err)
	}
}

func TestPreviewRemainsReadableWhileRetryPublishesReplacement(t *testing.T) {
	root := t.TempDir()
	jobID := "abababababababababababababababab"
	destination := filepath.Join(root, "jobs", jobID)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	previewPath := filepath.Join(destination, "preview.pdf")
	if err := os.WriteFile(previewPath, []byte("%PDF-old"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	renderer := &PDFRenderer{outputDir: root, printToPDF: func(context.Context, string, string, pdfPrintOptions) ([]byte, error) {
		close(entered)
		<-release
		return []byte("%PDF-new"), nil
	}}
	job := &jobs.Job{ID: jobID, Type: jobs.JobTypeBalloon, PDFPath: previewPath, Payload: []byte(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)}
	router := httpapi.NewRouter(httpapi.Dependencies{Jobs: fixedPreviewJobs{job: job}, PreviewRoot: filepath.Join(root, "jobs")})
	requestPreview := func() (int, string) {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/print-jobs/"+jobID+"/preview", nil)
		router.ServeHTTP(response, request)
		return response.Code, response.Body.String()
	}
	done := make(chan error, 1)
	go func() { _, err := renderer.Render(context.Background(), job); done <- err }()
	<-entered
	if status, body := requestPreview(); status != http.StatusOK || body != "%PDF-old" {
		t.Fatalf("preview during retry = %d %q, want old complete PDF", status, body)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if status, body := requestPreview(); status != http.StatusOK || body != "%PDF-new" {
		t.Fatalf("preview after retry = %d %q, want new complete PDF", status, body)
	}
}

func TestNewPDFRendererRecoversLegacyInterruptedDirectoryPublish(t *testing.T) {
	root := t.TempDir()
	jobID := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	jobsRoot := filepath.Join(root, "jobs")
	legacy := filepath.Join(jobsRoot, "."+jobID+"-previous")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "preview.pdf"), []byte("%PDF-recovered"), 0o600); err != nil {
		t.Fatal(err)
	}
	browser := filepath.Join(root, "chrome.exe")
	if err := os.WriteFile(browser, []byte("browser"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newPDFRenderer(root, browser, func(context.Context, string) (int, error) { return MinimumChromeMajor, nil }); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(jobsRoot, jobID, "preview.pdf"))
	if err != nil || string(contents) != "%PDF-recovered" {
		t.Fatalf("recovered PDF = %q, %v", contents, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy backup remains after recovery: %v", err)
	}
}

func TestPDFRendererMutexWaitHonorsContextCancellation(t *testing.T) {
	root := t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int
	renderer := &PDFRenderer{outputDir: root, printToPDF: func(context.Context, string, string, pdfPrintOptions) ([]byte, error) {
		calls++
		if calls == 1 {
			close(entered)
			<-release
		}
		return []byte("%PDF-ok"), nil
	}}
	job := func(id string) *jobs.Job {
		return &jobs.Job{ID: id, Type: jobs.JobTypeBalloon, Payload: []byte(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := renderer.Render(context.Background(), job("11111111111111111111111111111111"))
		firstDone <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { _, err := renderer.Render(ctx, job("22222222222222222222222222222222")); secondDone <- err }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting render error = %v, want context.Canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		close(release)
		<-firstDone
		t.Fatal("waiting render ignored context cancellation")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("print runner calls = %d, want 1", calls)
	}
}
