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
	"sync"
	"testing"
	"time"

	"local-print-agent/internal/config"
	"local-print-agent/internal/instance"
	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
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

func TestStartWithBuilderRejectsSecondSameDataDirBeforeBuilder(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: dataDir}
	firstContext, cancelFirst := context.WithCancel(context.Background())
	first, err := startWithBuilder(firstContext, cfg, testApplicationBuilder(t))
	if err != nil {
		cancelFirst()
		t.Fatal(err)
	}

	secondContext, cancelSecond := context.WithCancel(context.Background())
	secondBuilderCalled := false
	second, err := startWithBuilder(secondContext, cfg, func(config.Config) (*application, error) {
		secondBuilderCalled = true
		return testApplicationBuilder(t)(cfg)
	})
	if second != nil {
		cancelSecond()
		<-second.Done
	}
	if !errors.Is(err, instance.ErrAlreadyRunning) {
		cancelFirst()
		<-first.Done
		t.Fatalf("second start error = %v, want ErrAlreadyRunning", err)
	}
	if secondBuilderCalled {
		cancelFirst()
		<-first.Done
		t.Fatal("second start invoked the application builder")
	}
	cancelSecond()

	cancelFirst()
	if err := <-first.Done; err != nil {
		t.Fatal(err)
	}
	reacquired, err := instance.Acquire(dataDir)
	if err != nil {
		t.Fatalf("lock remains held after graceful completion: %v", err)
	}
	if err := reacquired.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithBuilderReleasesLockWhenBuilderFails(t *testing.T) {
	dataDir := t.TempDir()
	want := errors.New("builder failed")
	observedLock := false
	_, err := startWithBuilder(context.Background(), config.Config{DataDir: dataDir}, func(config.Config) (*application, error) {
		contender, lockErr := instance.Acquire(dataDir)
		if contender != nil {
			_ = contender.Close()
		}
		observedLock = errors.Is(lockErr, instance.ErrAlreadyRunning)
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("start error = %v, want builder error", err)
	}
	if !observedLock {
		t.Fatal("data directory lock was not held before the builder ran")
	}
	assertInstanceLockAvailable(t, dataDir)
}

func TestStartWithBuilderReleasesLockWhenListenerFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	dataDir := t.TempDir()
	port := occupied.Addr().(*net.TCPAddr).Port
	observedLock := false
	_, err = startWithBuilder(context.Background(), config.Config{Host: "127.0.0.1", FirstPort: port, LastPort: port, DataDir: dataDir}, func(cfg config.Config) (*application, error) {
		contender, lockErr := instance.Acquire(dataDir)
		if contender != nil {
			_ = contender.Close()
		}
		observedLock = errors.Is(lockErr, instance.ErrAlreadyRunning)
		return testApplicationBuilder(t)(cfg)
	})
	if err == nil {
		t.Fatal("start succeeded with its only candidate port occupied")
	}
	if !observedLock {
		t.Fatal("data directory lock was not held while the listener was created")
	}
	assertInstanceLockAvailable(t, dataDir)
}

func TestStartWithBuilderGeneratesDistinctFileOriginCapabilities(t *testing.T) {
	type launch struct {
		running  *runningServer
		cancel   context.CancelFunc
		received string
	}
	launches := make([]launch, 0, 2)
	for index := 0; index < 2; index++ {
		ctx, cancel := context.WithCancel(context.Background())
		cfg := config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: t.TempDir()}
		var received string
		running, err := startWithBuilder(ctx, cfg, func(cfg config.Config) (*application, error) {
			received = cfg.FileOriginToken
			return testApplicationBuilder(t)(cfg)
		})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		launches = append(launches, launch{running: running, cancel: cancel, received: received})
	}
	defer func() {
		for _, current := range launches {
			current.cancel()
			<-current.running.Done
		}
	}()
	for index, current := range launches {
		if current.running.FileOriginToken == "" {
			t.Fatalf("launch %d returned an empty file-origin capability", index)
		}
		if current.received != current.running.FileOriginToken {
			t.Fatalf("launch %d builder capability did not match running server", index)
		}
	}
	if launches[0].running.FileOriginToken == launches[1].running.FileOriginToken {
		t.Fatal("two launches reused the same file-origin capability")
	}
}

func assertInstanceLockAvailable(t *testing.T, dataDir string) {
	t.Helper()
	lock, err := instance.Acquire(dataDir)
	if err != nil {
		t.Fatalf("instance lock remains held: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

type cleanupBlockingRenderer struct {
	entered chan struct{}
	release chan struct{}
	profile string
}

func (r *cleanupBlockingRenderer) Render(ctx context.Context, _ *jobs.Job) (string, error) {
	close(r.entered)
	<-ctx.Done()
	<-r.release
	if err := os.RemoveAll(r.profile); err != nil {
		return "", err
	}
	return "", ctx.Err()
}

func TestRunningDoneWaitsForWorkerAndActiveRendererCleanup(t *testing.T) {
	dataDir := t.TempDir()
	profile := filepath.Join(dataDir, "chromedp-profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	renderer := &cleanupBlockingRenderer{entered: make(chan struct{}), release: make(chan struct{}), profile: profile}
	ctx, cancel := context.WithCancel(context.Background())
	running, err := startWithBuilder(ctx, config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: dataDir}, func(cfg config.Config) (*application, error) {
		return buildApplicationWithRenderer(cfg, renderer)
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	submitDemoJob(t, client, running.URL, jobs.CreateJobRequest{Type: jobs.JobTypeBalloon, PrinterName: fakePrinterName, Payload: json.RawMessage(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`)})
	select {
	case <-renderer.entered:
	case <-time.After(time.Second):
		t.Fatal("renderer did not start")
	}
	cancel()
	select {
	case <-running.Done:
		close(renderer.release)
		t.Fatal("running.Done closed before active renderer cleanup")
	case <-time.After(150 * time.Millisecond):
	}
	close(renderer.release)
	select {
	case err := <-running.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running.Done did not close after renderer cleanup")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("renderer profile remains after Done: %v", err)
	}
	parsed, _ := url.Parse(running.URL)
	listener, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("server port remains occupied after Done: %v", err)
	}
	listener.Close()
}

func TestRunningDoneWaitsForActiveHTTPHandlerAndShutdownCompletion(t *testing.T) {
	dataDir := t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	ctx, cancel := context.WithCancel(context.Background())
	running, err := startWithBuilder(ctx, config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: dataDir}, func(cfg config.Config) (*application, error) {
		built, err := testApplicationBuilder(t)(cfg)
		if err != nil {
			return nil, err
		}
		built.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("handler completed"))
		})
		return built, nil
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	responseDone := make(chan error, 1)
	go func() {
		response, err := (&http.Client{Timeout: 3 * time.Second}).Get(running.URL + "/blocking")
		if err != nil {
			responseDone <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			responseDone <- err
			return
		}
		if response.StatusCode != http.StatusOK || string(body) != "handler completed" {
			responseDone <- errors.New("blocking handler response was incomplete")
			return
		}
		responseDone <- nil
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("blocking handler did not start")
	}

	cancel()
	select {
	case err := <-running.Done:
		unblock()
		t.Fatalf("running.Done closed before HTTP shutdown completed: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-responseDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active handler response did not complete")
	}
	select {
	case err := <-running.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running.Done did not close after HTTP shutdown completed")
	}
	parsed, err := url.Parse(running.URL)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("server port remains occupied after Done: %v", err)
	}
	listener.Close()
	assertInstanceLockAvailable(t, dataDir)
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

func TestConfiguredPrinterDefaultsToNonPrintingDemoAdapter(t *testing.T) {
	called := false
	selected, err := configuredPrinter(config.Config{}, func(printer.PlatformConfig) (printer.Adapter, error) {
		called = true
		return nil, errors.New("platform factory must not be called")
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("default demo mode called the platform adapter factory")
	}
	listed, err := selected.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != fakePrinterName {
		t.Fatalf("demo printers = %#v, want the visibly non-printing adapter", listed)
	}
}

func TestConfiguredPrinterUsesPlatformAdapterOnlyWhenExplicit(t *testing.T) {
	want := printer.NewFake([]printer.Info{{Name: "Controlled OS Queue", IsDefault: true}})
	var received printer.PlatformConfig
	selected, err := configuredPrinter(config.Config{
		DataDir:        "controlled-data",
		SumatraPDFPath: "controlled-sumatra",
		PrinterMode:    config.PrinterModePlatform,
	}, func(cfg printer.PlatformConfig) (printer.Adapter, error) {
		received = cfg
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected != want {
		t.Fatalf("configuredPrinter() = %T, want injected platform adapter", selected)
	}
	if received.DataDir != "controlled-data" || received.SumatraPDFPath != "controlled-sumatra" {
		t.Fatalf("platform config = %#v", received)
	}
}

func TestExplicitPlatformModeExposesPlatformAdapterPrintersAPI(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir(), PrinterMode: config.PrinterModePlatform}
	renderer, err := render.NewFake(filepath.Join(cfg.DataDir, "fake-pdfs"))
	if err != nil {
		t.Fatal(err)
	}
	application, err := buildApplicationWithRendererAndPrinterFactory(cfg, renderer, func(printer.PlatformConfig) (printer.Adapter, error) {
		return printer.NewFake([]printer.Info{{Name: "Platform Queue", IsDefault: true}}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/printers", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Platform Queue") || strings.Contains(response.Body.String(), fakePrinterName) {
		t.Fatalf("printers = %d %s", response.Code, response.Body.String())
	}
}

func TestConfiguredPrinterRejectsUnknownMode(t *testing.T) {
	_, err := configuredPrinter(config.Config{PrinterMode: "automatic"}, func(printer.PlatformConfig) (printer.Adapter, error) {
		t.Fatal("unknown mode called platform factory")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "printer mode") {
		t.Fatalf("configuredPrinter() error = %v, want printer mode error", err)
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
