package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/store"
	"local-print-agent/internal/worker"
)

type retryStore struct {
	mu          sync.Mutex
	jobs        map[string]*jobs.Job
	getFailures int
	notFound    bool
	failures    map[jobs.Status]int
	attempts    []jobs.Job
	entered     chan jobs.Job
	blockStatus jobs.Status
	release     <-chan struct{}
}

func newRetryStore(job *jobs.Job) *retryStore {
	return &retryStore{jobs: map[string]*jobs.Job{job.ID: copyJob(job)}, failures: make(map[jobs.Status]int), entered: make(chan jobs.Job, 32)}
}

func (s *retryStore) Get(ctx context.Context, id string) (*jobs.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notFound {
		return nil, store.ErrNotFound
	}
	if s.getFailures > 0 {
		s.getFailures--
		return nil, errors.New("temporary read failure")
	}
	return copyJob(s.jobs[id]), nil
}

func (s *retryStore) Update(ctx context.Context, job *jobs.Job) error {
	s.mu.Lock()
	block, release := s.blockStatus == job.Status, s.release
	s.attempts = append(s.attempts, *copyJob(job))
	s.mu.Unlock()
	select {
	case s.entered <- *copyJob(job):
	default:
	}
	if block {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures[job.Status] > 0 {
		s.failures[job.Status]--
		return errors.New("temporary write failure")
	}
	s.jobs[job.ID] = copyJob(job)
	return nil
}

func (s *retryStore) Attempts(status jobs.Status) []jobs.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []jobs.Job
	for _, attempt := range s.attempts {
		if attempt.Status == status {
			result = append(result, attempt)
		}
	}
	return result
}

func (s *retryStore) Create(_ context.Context, job *jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return jobs.ErrAlreadyExists
	}
	s.jobs[job.ID] = copyJob(job)
	return nil
}

func (s *retryStore) List(context.Context) ([]*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, copyJob(job))
	}
	return result, nil
}

func runUntilClosed(w *worker.Worker, queue chan string) <-chan struct{} {
	done := make(chan struct{})
	close(queue)
	go func() { w.Run(context.Background()); close(done) }()
	return done
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Worker.Run() did not finish")
	}
}

func TestWorkerRetriesFinalPersistenceWithoutReprinting(t *testing.T) {
	job := queuedJob("final-retry")
	state := newRetryStore(job)
	state.failures[jobs.StatusSucceeded] = 2
	pdf := temporaryPDF(t, "final-retry")
	printerAdapter := printer.NewFake(nil)
	queue := make(chan string, 1)
	queue <- job.ID
	w := worker.New(state, rendererFunc(func(context.Context, *jobs.Job) (string, error) { return pdf, nil }), printerAdapter, queue)
	waitDone(t, runUntilClosed(w, queue))

	if calls := printerAdapter.Calls(); len(calls) != 1 {
		t.Fatalf("Print calls = %#v, want exactly one", calls)
	}
	attempts := state.Attempts(jobs.StatusSucceeded)
	if len(attempts) != 3 || attempts[0].Attempts != 1 || attempts[0].UpdatedAt != attempts[1].UpdatedAt || attempts[1].UpdatedAt != attempts[2].UpdatedAt {
		t.Fatalf("succeeded Update attempts = %#v, want unchanged snapshot retried three times", attempts)
	}
}

func TestWorkerRetriesFailedPersistenceWithoutRerendering(t *testing.T) {
	job := queuedJob("failed-retry")
	state := newRetryStore(job)
	state.failures[jobs.StatusFailed] = 2
	renders := 0
	queue := make(chan string, 1)
	queue <- job.ID
	w := worker.New(state, rendererFunc(func(context.Context, *jobs.Job) (string, error) {
		renders++
		return "", errors.New("bad template")
	}), printer.NewFake(nil), queue)
	waitDone(t, runUntilClosed(w, queue))
	if renders != 1 || len(state.Attempts(jobs.StatusFailed)) != 3 {
		t.Fatalf("renders=%d failed updates=%d, want one render and three unchanged retries", renders, len(state.Attempts(jobs.StatusFailed)))
	}
}

func TestWorkerRetriesTransientGetAndReportsNotFoundWithoutCallingDependencies(t *testing.T) {
	t.Run("transient", func(t *testing.T) {
		job := queuedJob("get-transient")
		state := newRetryStore(job)
		state.getFailures = 2
		queue := make(chan string, 1)
		queue <- job.ID
		w := worker.New(state, rendererFunc(func(context.Context, *jobs.Job) (string, error) { return temporaryPDF(t, "get-transient"), nil }), printer.NewFake(nil), queue)
		waitDone(t, runUntilClosed(w, queue))
		var received int
		for {
			select {
			case _, open := <-w.Errors():
				if !open {
					if received != 2 {
						t.Fatalf("transient Get errors=%d, want 2", received)
					}
					return
				}
				received++
			default:
				if received != 2 {
					t.Fatalf("transient Get errors=%d, want 2", received)
				}
				return
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		job := queuedJob("missing")
		state := newRetryStore(job)
		state.notFound = true
		queue := make(chan string, 1)
		queue <- job.ID
		renders := 0
		printerAdapter := printer.NewFake(nil)
		w := worker.New(state, rendererFunc(func(context.Context, *jobs.Job) (string, error) { renders++; return "", nil }), printerAdapter, queue)
		waitDone(t, runUntilClosed(w, queue))
		select {
		case err := <-w.Errors():
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("Errors() = %v, want ErrNotFound", err)
			}
		default:
			t.Fatal("missing ID was not reported")
		}
		if renders != 0 || len(printerAdapter.Calls()) != 0 {
			t.Fatalf("missing job called renderer=%d printer=%#v", renders, printerAdapter.Calls())
		}
	})
}

func TestWorkerDoesNotProcessFollowingIDUntilCurrentPersistenceSucceeds(t *testing.T) {
	first, second := queuedJob("blocked-first"), queuedJob("blocked-second")
	state := newRetryStore(first)
	state.jobs[second.ID] = copyJob(second)
	release := make(chan struct{})
	state.blockStatus, state.release = jobs.StatusRendering, release
	queue := make(chan string, 2)
	queue <- first.ID
	queue <- second.ID
	renders := make(chan string, 2)
	w := worker.New(state, rendererFunc(func(_ context.Context, job *jobs.Job) (string, error) {
		renders <- job.ID
		return temporaryPDF(t, job.ID), nil
	}), printer.NewFake(nil), queue)
	done := runUntilClosed(w, queue)
	select {
	case update := <-state.entered:
		if update.ID != first.ID || update.Status != jobs.StatusRendering {
			t.Fatalf("first update = %#v, want first rendering", update)
		}
	case <-time.After(time.Second):
		t.Fatal("first rendering update did not start")
	}
	select {
	case id := <-renders:
		t.Fatalf("renderer started %q before rendering state persisted", id)
	default:
	}
	close(release)
	waitDone(t, done)
	if firstID, secondID := <-renders, <-renders; firstID != first.ID || secondID != second.ID {
		t.Fatalf("render order = %q, %q; want FIFO", firstID, secondID)
	}
}

func TestNewPipelineConnectsServiceToWorkerWithoutReturningQueue(t *testing.T) {
	state := newRetryStore(queuedJob("seed"))
	rendered := make(chan string, 1)
	service, pipelineWorker := worker.NewPipeline(state, rendererFunc(func(_ context.Context, job *jobs.Job) (string, error) {
		rendered <- job.ID
		return temporaryPDF(t, "pipeline"), nil
	}), printer.NewFake(nil))
	created, err := service.Create(context.Background(), jobs.CreateJobRequest{Type: jobs.JobTypeSource, PrinterName: "front-desk", Payload: json.RawMessage(`{"language":"go","source_code":"func main() {}"}`)})
	if err != nil {
		t.Fatalf("pipeline Service.Create() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { pipelineWorker.Run(ctx); close(done) }()
	select {
	case id := <-rendered:
		if id != created.ID {
			t.Fatalf("pipeline rendered %q, want %q", id, created.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("private pipeline queue did not reach Worker")
	}
	cancel()
	waitDone(t, done)
}
