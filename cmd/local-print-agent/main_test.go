package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-print-agent/internal/config"
	"local-print-agent/internal/jobs"
	"local-print-agent/internal/render"
)

func TestStartServesHealthAndShutsDownWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	running, err := startWithBuilder(ctx, config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: t.TempDir()}, testApplicationBuilder(t))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: time.Second}).Get(running.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) == "" {
		t.Fatalf("health = %d %s", response.StatusCode, body)
	}
	cancel()
	select {
	case err := <-running.Done:
		if err != nil {
			t.Fatalf("server shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

func TestBuildApplicationProvidesAVisibleFakePrinter(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	renderer, err := render.NewFake(filepath.Join(cfg.DataDir, "fake-pdfs"))
	if err != nil {
		t.Fatal(err)
	}
	application, err := buildApplicationWithRenderer(cfg, renderer)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "/api/v1/printers", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Mock Printer（不执行实体打印）") {
		t.Fatalf("printers = %d %s", response.Code, response.Body.String())
	}
}

func TestBuildApplicationFailsClearlyWhenConfiguredBrowserIsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing", "chrome.exe")
	t.Setenv("LOCAL_PRINT_AGENT_BROWSER_PATH", missing)
	_, err := buildApplication(config.Config{DataDir: t.TempDir()})
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) || jobError.Code != jobs.ErrorCodeRendererNotFound {
		t.Fatalf("buildApplication() error = %T %v, want RENDERER_NOT_FOUND", err, err)
	}
	if strings.Contains(jobError.Message, missing) {
		t.Fatalf("startup error leaked browser path: %q", jobError.Message)
	}
}

func TestRealServiceRendersBothJobsServesPreviewAndCleansUp(t *testing.T) {
	browser := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_CHROME_E2E"))
	if browser == "" {
		t.Skip("set LOCAL_PRINT_AGENT_CHROME_E2E to run the real service integration")
	}
	t.Setenv("LOCAL_PRINT_AGENT_BROWSER_PATH", browser)
	profilesBefore, err := browserProfiles()
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string]bool, len(profilesBefore))
	for _, path := range profilesBefore {
		before[path] = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	running, err := start(ctx, config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: t.TempDir()})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	balloon := loadDemoRequest(t, "balloon.json")
	source := loadDemoRequest(t, "source_cpp.json")
	balloonJob := submitDemoJob(t, client, running.URL, balloon)
	sourceJob := submitDemoJob(t, client, running.URL, source)
	balloonDone := waitForDemoJob(t, client, running.URL, balloonJob.ID)
	sourceDone := waitForDemoJob(t, client, running.URL, sourceJob.ID)
	for _, completed := range []*jobs.Job{balloonDone, sourceDone} {
		if completed.Status != jobs.StatusSucceeded || completed.PDFPath == "" {
			t.Fatalf("completed job = %#v, want succeeded with PDF", completed)
		}
		response, err := client.Get(running.URL + "/api/v1/print-jobs/" + completed.ID + "/preview")
		if err != nil {
			t.Fatal(err)
		}
		contents, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(contents, []byte("%PDF-")) {
			t.Fatalf("preview %s = %d %q %v", completed.ID, response.StatusCode, contents[:min(len(contents), 16)], readErr)
		}
	}
	rangeRequest, _ := http.NewRequest(http.MethodGet, running.URL+"/api/v1/print-jobs/"+sourceDone.ID+"/preview", nil)
	rangeRequest.Header.Set("Range", "bytes=0-4")
	rangeResponse, err := client.Do(rangeRequest)
	if err != nil {
		t.Fatal(err)
	}
	rangeContents, _ := io.ReadAll(rangeResponse.Body)
	rangeResponse.Body.Close()
	if rangeResponse.StatusCode != http.StatusPartialContent || string(rangeContents) != "%PDF-" {
		t.Fatalf("preview range = %d %q", rangeResponse.StatusCode, rangeContents)
	}

	cancel()
	select {
	case err := <-running.Done:
		if err != nil {
			t.Fatalf("graceful service shutdown = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not shut down after cancellation")
	}
	parsedURL, err := url.Parse(running.URL)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", parsedURL.Host)
	if err != nil {
		t.Fatalf("service port was not released: %v", err)
	}
	listener.Close()
	profilesAfter, err := browserProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range profilesAfter {
		if !before[path] {
			t.Fatalf("renderer left browser profile %q", path)
		}
	}
	t.Logf("service jobs: balloon=%s source=%s", balloonDone.ID, sourceDone.ID)
}

func browserProfiles() ([]string, error) {
	var result []string
	for _, pattern := range []string{"chromedp-runner*", "local-print-agent-chrome-*", "local-print-agent-version-*"} {
		matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}
	return result, nil
}

func loadDemoRequest(t *testing.T, name string) jobs.CreateJobRequest {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture jobs.Job
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return jobs.CreateJobRequest{Type: fixture.Type, PrinterName: fakePrinterName, Payload: fixture.Payload}
}

func submitDemoJob(t *testing.T, client *http.Client, origin string, input jobs.CreateJobRequest) *jobs.Job {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post(origin+"/api/v1/print-jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data  *jobs.Job `json:"data"`
		Error any       `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted || envelope.Data == nil || envelope.Error != nil {
		t.Fatalf("submit response = %d %#v", response.StatusCode, envelope)
	}
	return envelope.Data
}

func waitForDemoJob(t *testing.T, client *http.Client, origin, id string) *jobs.Job {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(origin + "/api/v1/print-jobs/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data *jobs.Job `json:"data"`
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if envelope.Data != nil && (envelope.Data.Status == jobs.StatusSucceeded || envelope.Data.Status == jobs.StatusFailed) {
			return envelope.Data
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job %s did not complete", id)
	return nil
}

func testApplicationBuilder(t *testing.T) func(config.Config) (*application, error) {
	t.Helper()
	return func(cfg config.Config) (*application, error) {
		renderer, err := render.NewFake(filepath.Join(cfg.DataDir, "fake-pdfs"))
		if err != nil {
			return nil, err
		}
		return buildApplicationWithRenderer(cfg, renderer)
	}
}
