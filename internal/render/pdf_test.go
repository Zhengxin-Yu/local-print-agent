package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"local-print-agent/internal/jobs"
)

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
		printToPDF: func(_ context.Context, _ string, documentURL string) ([]byte, error) {
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
}

func TestPDFRendererSelectsSourceHTMLAndRejectsUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	renderer := &PDFRenderer{
		outputDir: root,
		printToPDF: func(_ context.Context, _ string, documentURL string) ([]byte, error) {
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
	runnerError := errors.New("browser crashed")
	renderer := &PDFRenderer{
		outputDir:  root,
		printToPDF: func(context.Context, string, string) ([]byte, error) { return nil, runnerError },
	}
	job := &jobs.Job{ID: jobID, Type: jobs.JobTypeBalloon, Payload: []byte(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)}
	if _, err := renderer.Render(context.Background(), job); !errors.Is(err, runnerError) {
		t.Fatalf("Render error = %v, want runner error", err)
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
