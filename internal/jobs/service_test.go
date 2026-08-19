package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	. "local-print-agent/internal/jobs"
	"local-print-agent/internal/store"
)

// memoryJobStore is deliberately small: it exercises Service behavior without
// filesystem persistence, which is covered by store's own tests.
type memoryJobStore struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	creates int
}

func newMemoryJobStore() *memoryJobStore { return &memoryJobStore{jobs: make(map[string]*Job)} }

func testTimePointer(value time.Time) *time.Time { return &value }

func (s *memoryJobStore) Create(_ context.Context, job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; ok {
		return fmt.Errorf("duplicate job %q", job.ID)
	}
	s.jobs[job.ID] = copyTestJob(job)
	s.creates++
	return nil
}

func (s *memoryJobStore) Update(_ context.Context, job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return fmt.Errorf("update %q: %w", job.ID, store.ErrNotFound)
	}
	s.jobs[job.ID] = copyTestJob(job)
	return nil
}

func (s *memoryJobStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("get %q: %w", id, store.ErrNotFound)
	}
	return copyTestJob(job), nil
}

func (s *memoryJobStore) List(_ context.Context) ([]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, copyTestJob(job))
	}
	return result, nil
}

func copyTestJob(job *Job) *Job {
	copy := *job
	copy.Payload = append(json.RawMessage(nil), job.Payload...)
	if job.Error != nil {
		errCopy := *job.Error
		copy.Error = &errCopy
	}
	return &copy
}

func validSourceRequest() CreateJobRequest {
	return CreateJobRequest{Type: JobTypeSource, PrinterName: "  front-desk  ", Payload: json.RawMessage(`{"language":" go ","source_code":" func main() {} "}`)}
}

// A regression here catches creation which skips normalization or queues a
// partially initialized job.
func TestServiceCreateNormalizesAndQueuesNewJob(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	service := NewService(jobs, queue)

	job, err := service.Create(context.Background(), validSourceRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if job.ID == "" || job.Status != StatusQueued || job.Attempts != 0 || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		t.Fatalf("new job = %#v, want initialized queued job", job)
	}
	if job.PrinterName != "front-desk" || string(job.Payload) != `{"language":"go","source_code":"func main() {}"}` {
		t.Fatalf("Create() returned unnormalized job %#v", job)
	}
	select {
	case queuedID := <-queue:
		if queuedID != job.ID {
			t.Fatalf("queued ID = %q, want %q", queuedID, job.ID)
		}
	default:
		t.Fatal("Create() did not enqueue job")
	}
}

// This catches persistence of a request before validation rejects it.
func TestServiceCreateInvalidRequestDoesNotPersist(t *testing.T) {
	jobs := newMemoryJobStore()
	service := NewService(jobs, NewQueue())
	request := validSourceRequest()
	request.PrinterName = ""

	_, err := service.Create(context.Background(), request)
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeInvalidRequest {
		t.Fatalf("Create() error = %T %v, want INVALID_REQUEST JobError", err, err)
	}
	if jobs.creates != 0 {
		t.Fatalf("Create() persisted %d jobs for an invalid request", jobs.creates)
	}
}

// This catches the dangerous ordering where a full queue leaves a durable job
// that can never be processed.
func TestServiceCreateQueueFullDoesNotPersist(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	for index := 0; index < QueueCapacity; index++ {
		queue <- fmt.Sprintf("occupied-%d", index)
	}

	_, err := NewService(jobs, queue).Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueFull {
		t.Fatalf("Create() error = %T %v, want QUEUE_FULL JobError", err, err)
	}
	if jobs.creates != 0 {
		t.Fatalf("Create() persisted %d jobs while queue was full", jobs.creates)
	}
}

// This catches retries of successful or active jobs, which would duplicate a
// physical print attempt.
func TestServiceRetryOnlyFailedJobAndResetsLifecycle(t *testing.T) {
	jobs := newMemoryJobStore()
	failed := &Job{ID: "failed", Status: StatusFailed, Attempts: 2, StartedAt: testTimePointer(time.Now().Add(-time.Minute)), FinishedAt: testTimePointer(time.Now()), Error: &JobError{Code: ErrorCodePrintFailed, Message: "offline"}}
	if err := jobs.Create(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	service := NewService(jobs, NewQueue())

	job, err := service.Retry(context.Background(), failed.ID)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if job.Status != StatusQueued || job.Attempts != 2 || job.StartedAt != nil || job.FinishedAt != nil || job.Error != nil {
		t.Fatalf("Retry() = %#v, want reset queued retry", job)
	}
	if _, err := service.Retry(context.Background(), failed.ID); err == nil {
		t.Fatal("Retry() succeeded for a queued job")
	}
}

func TestServiceRetryQueueFullLeavesFailedJobUntouched(t *testing.T) {
	jobs := newMemoryJobStore()
	failed := &Job{ID: "failed", Status: StatusFailed, Attempts: 1, Error: &JobError{Code: ErrorCodeRenderFailed, Message: "renderer failed"}}
	if err := jobs.Create(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	queue := NewQueue()
	for index := 0; index < QueueCapacity; index++ {
		queue <- fmt.Sprintf("occupied-%d", index)
	}

	_, err := NewService(jobs, queue).Retry(context.Background(), failed.ID)
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueFull {
		t.Fatalf("Retry() error = %T %v, want QUEUE_FULL JobError", err, err)
	}
	persisted, getErr := jobs.Get(context.Background(), failed.ID)
	if getErr != nil || persisted.Status != StatusFailed || persisted.Error == nil {
		t.Fatalf("Retry() changed failed job on full queue: job=%#v, err=%v", persisted, getErr)
	}
}

func TestServiceCreateConcurrentIDsAreUnique(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	service := NewService(jobs, queue)
	const creates = 32
	ids := make(chan string, creates)
	errs := make(chan error, creates)
	var group sync.WaitGroup
	for index := 0; index < creates; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			job, err := service.Create(context.Background(), validSourceRequest())
			if err == nil {
				ids <- job.ID
			}
			errs <- err
		}()
	}
	group.Wait()
	close(ids)
	close(errs)
	seen := map[string]bool{}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create() error = %v", err)
		}
	}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate job ID %q", id)
		}
		seen[id] = true
	}
	if len(seen) != creates {
		t.Fatalf("created %d unique IDs, want %d", len(seen), creates)
	}
}

func TestServiceGetPreservesNotFoundCause(t *testing.T) {
	_, err := NewService(newMemoryJobStore(), NewQueue()).Get(context.Background(), "missing")
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeStore {
		t.Fatalf("Get() error = %T %v, want STORE_ERROR JobError", err, err)
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() error = %v, want errors.Is(err, store.ErrNotFound)", err)
	}
}

func TestServiceCreateCanceledContextDoesNotPersist(t *testing.T) {
	jobs := newMemoryJobStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewService(jobs, NewQueue()).Create(ctx, validSourceRequest())
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeContextCanceled {
		t.Fatalf("Create() error = %T %v, want CONTEXT_CANCELED JobError", err, err)
	}
	if jobs.creates != 0 {
		t.Fatalf("Create() persisted %d jobs with canceled context", jobs.creates)
	}
}
