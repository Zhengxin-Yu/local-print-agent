package worker_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/worker"
)

type recordingStore struct {
	mu      sync.Mutex
	jobs    map[string]*jobs.Job
	updates chan jobs.Job
	history []jobs.Job
}

func newRecordingStore(values ...*jobs.Job) *recordingStore {
	store := &recordingStore{jobs: make(map[string]*jobs.Job), updates: make(chan jobs.Job, 32)}
	for _, job := range values {
		store.jobs[job.ID] = copyJob(job)
	}
	return store
}

func (s *recordingStore) Get(ctx context.Context, id string) (*jobs.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	return copyJob(job), nil
}

func (s *recordingStore) Update(ctx context.Context, job *jobs.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.jobs[job.ID] = copyJob(job)
	s.history = append(s.history, *copyJob(job))
	s.mu.Unlock()
	s.updates <- *copyJob(job)
	return nil
}

func (s *recordingStore) History() []jobs.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]jobs.Job(nil), s.history...)
	return result
}

func (s *recordingStore) Job(id string) *jobs.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyJob(s.jobs[id])
}

func copyJob(job *jobs.Job) *jobs.Job {
	copy := *job
	copy.Payload = append([]byte(nil), job.Payload...)
	if job.Error != nil {
		errCopy := *job.Error
		copy.Error = &errCopy
	}
	return &copy
}

type rendererFunc func(context.Context, *jobs.Job) (string, error)

func (fn rendererFunc) Render(ctx context.Context, job *jobs.Job) (string, error) {
	return fn(ctx, job)
}

type rejectingUpdateStore struct {
	job     *jobs.Job
	entered chan struct{}
}

func (s rejectingUpdateStore) Get(context.Context, string) (*jobs.Job, error) {
	return copyJob(s.job), nil
}
func (s rejectingUpdateStore) Update(context.Context, *jobs.Job) error {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	return errors.New("disk unavailable")
}

func queuedJob(id string) *jobs.Job {
	return &jobs.Job{ID: id, Status: jobs.StatusQueued, PrinterName: "front-desk"}
}

func temporaryPDF(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\n% fake test PDF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func startWorker(t *testing.T, store *recordingStore, renderer rendererFunc, printerAdapter printer.Adapter, queue chan string) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go worker.New(store, renderer, printerAdapter, queue).Run(ctx)
	return cancel
}

func waitForStatus(t *testing.T, store *recordingStore, id string, want jobs.Status) *jobs.Job {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case update := <-store.updates:
			if update.ID == id && update.Status == want {
				return store.Job(id)
			}
		case <-deadline:
			t.Fatalf("job %q did not reach %q; current=%#v", id, want, store.Job(id))
		}
	}
}

// This catches any skipped lifecycle state, lost PDF path, or printer command
// submitted for a value other than the renderer's file.
func TestWorkerProcessesQueuedJobThroughSuccessfulFIFOStates(t *testing.T) {
	first, second := queuedJob("first"), queuedJob("second")
	store := newRecordingStore(first, second)
	firstPDF, secondPDF := temporaryPDF(t, "first"), temporaryPDF(t, "second")
	renderer := rendererFunc(func(_ context.Context, job *jobs.Job) (string, error) {
		if job.ID == first.ID {
			return firstPDF, nil
		}
		return secondPDF, nil
	})
	printerAdapter := printer.NewFake([]printer.Info{{Name: "front-desk", IsDefault: true}})
	queue := make(chan string, 2)
	cancel := startWorker(t, store, renderer, printerAdapter, queue)
	defer cancel()
	queue <- first.ID
	queue <- second.ID

	waitForStatus(t, store, first.ID, jobs.StatusSucceeded)
	secondDone := waitForStatus(t, store, second.ID, jobs.StatusSucceeded)
	if secondDone.Attempts != 1 || secondDone.PDFPath != secondPDF {
		t.Fatalf("second completed job = %#v, want persisted PDF and one attempt", secondDone)
	}
	calls := printerAdapter.Calls()
	if len(calls) != 2 || calls[0].PrinterName != "front-desk" || calls[0].PDFPath != firstPDF || calls[1].PDFPath != secondPDF {
		t.Fatalf("Print calls = %#v, want ordered renderer output on selected printer", calls)
	}
	history := store.History()
	want := []struct {
		id     string
		status jobs.Status
	}{
		{"first", jobs.StatusRendering}, {"first", jobs.StatusPrinting}, {"first", jobs.StatusSucceeded},
		{"second", jobs.StatusRendering}, {"second", jobs.StatusPrinting}, {"second", jobs.StatusSucceeded},
	}
	if len(history) != len(want) {
		t.Fatalf("state updates = %#v, want six ordered updates", history)
	}
	for index, expected := range want {
		if history[index].ID != expected.id || history[index].Status != expected.status {
			t.Fatalf("state update %d = (%q, %q), want (%q, %q)", index, history[index].ID, history[index].Status, expected.id, expected.status)
		}
	}
}

func TestWorkerRecordsRenderFailure(t *testing.T) {
	job := queuedJob("render-failure")
	store := newRecordingStore(job)
	renderer := rendererFunc(func(context.Context, *jobs.Job) (string, error) { return "", errors.New("template invalid") })
	queue := make(chan string, 1)
	cancel := startWorker(t, store, renderer, printer.NewFake(nil), queue)
	defer cancel()
	queue <- job.ID

	failed := waitForStatus(t, store, job.ID, jobs.StatusFailed)
	if failed.Error == nil || failed.Error.Code != jobs.ErrorCodeRenderFailed || failed.Error.Message == "" {
		t.Fatalf("render failure = %#v, want non-empty RENDER_FAILED error", failed.Error)
	}
}

func TestWorkerPreservesRendererJobErrorAndNeverPrints(t *testing.T) {
	job := queuedJob("renderer-unavailable")
	store := newRecordingStore(job)
	renderer := rendererFunc(func(context.Context, *jobs.Job) (string, error) {
		return "", &jobs.JobError{Code: jobs.ErrorCode("RENDERER_NOT_FOUND"), Message: "browser unavailable"}
	})
	printerAdapter := printer.NewFake(nil)
	queue := make(chan string, 1)
	cancel := startWorker(t, store, renderer, printerAdapter, queue)
	defer cancel()
	queue <- job.ID

	failed := waitForStatus(t, store, job.ID, jobs.StatusFailed)
	if failed.Error == nil || failed.Error.Code != jobs.ErrorCode("RENDERER_NOT_FOUND") {
		t.Fatalf("renderer failure = %#v, want preserved RENDERER_NOT_FOUND", failed.Error)
	}
	if calls := printerAdapter.Calls(); len(calls) != 0 {
		t.Fatalf("Print calls = %#v, want none after renderer failure", calls)
	}
	for _, update := range store.History() {
		if update.Status == jobs.StatusPrinting {
			t.Fatalf("job entered printing after renderer failure: %#v", update)
		}
	}
}

func TestWorkerRecordsPrintCommandFailure(t *testing.T) {
	job := queuedJob("print-failure")
	store := newRecordingStore(job)
	pdf := temporaryPDF(t, "print")
	renderer := rendererFunc(func(context.Context, *jobs.Job) (string, error) { return pdf, nil })
	printerAdapter := printer.NewFake(nil)
	printerAdapter.SetPrintError(errors.New("printer offline"))
	queue := make(chan string, 1)
	cancel := startWorker(t, store, renderer, printerAdapter, queue)
	defer cancel()
	queue <- job.ID

	failed := waitForStatus(t, store, job.ID, jobs.StatusFailed)
	if failed.Error == nil || failed.Error.Code != jobs.ErrorCodePrintFailed || failed.Error.Message == "" || failed.PDFPath != pdf {
		t.Fatalf("print failure job = %#v, want persisted PDF and PRINT_COMMAND_FAILED error", failed)
	}
}

func TestWorkerSkipsNonQueuedJob(t *testing.T) {
	job := queuedJob("already-finished")
	job.Status = jobs.StatusSucceeded
	store := newRecordingStore(job)
	queue := make(chan string, 1)
	queue <- job.ID
	close(queue)
	worker.New(store, rendererFunc(func(context.Context, *jobs.Job) (string, error) { return "", fmt.Errorf("must not render") }), printer.NewFake(nil), queue).Run(context.Background())
	if got := store.Job(job.ID); got.Status != jobs.StatusSucceeded {
		t.Fatalf("non-queued job changed to %#v", got)
	}
}

func TestWorkerStopsOnCanceledContextWithoutConsumingQueuedJob(t *testing.T) {
	job := queuedJob("canceled")
	store := newRecordingStore(job)
	queue := make(chan string, 1)
	queue <- job.ID
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.New(store, rendererFunc(func(context.Context, *jobs.Job) (string, error) { return "", nil }), printer.NewFake(nil), queue).Run(ctx)
	if got := store.Job(job.ID); got.Status != jobs.StatusQueued {
		t.Fatalf("canceled worker changed queued job to %#v", got)
	}
}

func TestWorkerReturnsWhenQueueCloses(t *testing.T) {
	queue := make(chan string)
	close(queue)
	done := make(chan struct{})
	go func() {
		worker.New(newRecordingStore(), rendererFunc(func(context.Context, *jobs.Job) (string, error) { return "", nil }), printer.NewFake(nil), queue).Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Worker.Run() did not return after queue closure")
	}
}

func TestWorkerDoesNotRenderAfterStateUpdateError(t *testing.T) {
	called := false
	queue := make(chan string, 1)
	queue <- "cannot-update"
	close(queue)
	entered := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.New(rejectingUpdateStore{job: queuedJob("cannot-update"), entered: entered}, rendererFunc(func(context.Context, *jobs.Job) (string, error) {
			called = true
			return "", nil
		}), printer.NewFake(nil), queue).Run(ctx)
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Update() was not attempted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Worker.Run() did not stop after cancellation")
	}
	if called {
		t.Fatal("Worker rendered despite failing to persist rendering state")
	}
}
