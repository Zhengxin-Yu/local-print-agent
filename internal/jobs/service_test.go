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
		return fmt.Errorf("duplicate job %q: %w", job.ID, ErrAlreadyExists)
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
	if job.PrinterName != "front-desk" || string(job.Payload) != `{"language":"go","source_code":" func main() {} "}` {
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
	service := NewService(jobs, queue)
	for index := 0; index < QueueCapacity; index++ {
		if _, err := service.Create(context.Background(), validSourceRequest()); err != nil {
			t.Fatalf("Create() filling queue at %d: %v", index, err)
		}
	}

	_, err := service.Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueFull {
		t.Fatalf("Create() error = %T %v, want QUEUE_FULL JobError", err, err)
	}
	if jobs.creates != QueueCapacity {
		t.Fatalf("Create() persisted %d jobs while queue was full, want %d", jobs.creates, QueueCapacity)
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
	service := NewService(jobs, queue)
	for index := 0; index < QueueCapacity; index++ {
		if _, err := service.Create(context.Background(), validSourceRequest()); err != nil {
			t.Fatalf("Create() filling queue at %d: %v", index, err)
		}
	}

	_, err := service.Retry(context.Background(), failed.ID)
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueFull {
		t.Fatalf("Retry() error = %T %v, want QUEUE_FULL JobError", err, err)
	}
	persisted, getErr := jobs.Get(context.Background(), failed.ID)
	if getErr != nil || persisted.Status != StatusFailed || persisted.Error == nil {
		t.Fatalf("Retry() changed failed job on full queue: job=%#v, err=%v", persisted, getErr)
	}
}

func TestNewServiceRejectsInvalidQueue(t *testing.T) {
	cases := []struct {
		name  string
		queue chan string
	}{
		{"nil", nil},
		{"unbuffered", make(chan string)},
		{"too small", make(chan string, QueueCapacity-1)},
		{"too large", make(chan string, QueueCapacity+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("NewService() did not reject invalid queue")
				}
			}()
			_ = NewService(newMemoryJobStore(), tc.queue)
		})
	}
}

func TestServiceExternalFullQueuePersistsThenReportsDeliveryFailure(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	for index := 0; index < QueueCapacity; index++ {
		queue <- fmt.Sprintf("external-%d", index)
	}

	job, err := NewService(jobs, queue).Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if job == nil || !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueDeliveryFailed {
		t.Fatalf("Create() = %#v, %v; want persisted job and QUEUE_DELIVERY_FAILED", job, err)
	}
	persisted, getErr := jobs.Get(context.Background(), job.ID)
	if getErr != nil || persisted.Status != StatusQueued {
		t.Fatalf("persisted job = %#v, %v; want queued durable job", persisted, getErr)
	}
}

func TestServiceClosedQueueReturnsPersistedDeliveryFailure(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	close(queue)

	job, err := NewService(jobs, queue).Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if job == nil || !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueDeliveryFailed {
		t.Fatalf("Create() = %#v, %v; want persisted job and QUEUE_DELIVERY_FAILED", job, err)
	}
	if persisted, getErr := jobs.Get(context.Background(), job.ID); getErr != nil || persisted.Status != StatusQueued {
		t.Fatalf("closed-queue job = %#v, %v; want queued durable job", persisted, getErr)
	}
}

func TestSecondServiceReportsDeliveryFailureWithoutSending(t *testing.T) {
	jobs := newMemoryJobStore()
	queue := NewQueue()
	_ = NewService(jobs, queue)
	second := NewService(jobs, queue)
	job, err := second.Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if job == nil || !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueDeliveryFailed {
		t.Fatalf("second Create() = %#v, %v; want durable QUEUE_DELIVERY_FAILED", job, err)
	}
	select {
	case delivered := <-queue:
		t.Fatalf("second Service delivered %q despite ownership violation", delivered)
	default:
	}
}

func TestServiceCloseReleasesLowLevelQueueOwnership(t *testing.T) {
	queue := NewQueue()
	first := NewService(newMemoryJobStore(), queue)
	first.Close()
	second := NewService(newMemoryJobStore(), queue)
	defer second.Close()

	job, err := second.Create(context.Background(), validSourceRequest())
	if err != nil {
		t.Fatalf("second Create() after first Close() error = %v", err)
	}
	select {
	case id := <-queue:
		if id != job.ID {
			t.Fatalf("queued ID = %q, want %q", id, job.ID)
		}
	default:
		t.Fatal("released queue was not reusable by a new Service")
	}
}

func TestServiceRegeneratesDuplicateIDAndMapsStoreContextError(t *testing.T) {
	jobs := newMemoryJobStore()
	if err := jobs.Create(context.Background(), &Job{ID: "collision", Status: StatusQueued}); err != nil {
		t.Fatal(err)
	}
	ids := []string{"collision", "fresh"}
	service := NewServiceWithIDGenerator(jobs, NewQueue(), func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	})
	job, err := service.Create(context.Background(), validSourceRequest())
	if err != nil || job.ID != "fresh" {
		t.Fatalf("Create() = %#v, %v; want regenerated fresh ID", job, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewService(jobs, NewQueue()).Get(ctx, "anything")
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeContextCanceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("Get() canceled error = %T %v, want CONTEXT_CANCELED preserving cause", err, err)
	}
}

func TestServiceIDGeneratorFailureDoesNotPersist(t *testing.T) {
	jobs := newMemoryJobStore()
	service := NewServiceWithIDGenerator(jobs, NewQueue(), func() (string, error) { return "", errors.New("random unavailable") })
	_, err := service.Create(context.Background(), validSourceRequest())
	var jobErr *JobError
	if !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeStore || jobs.creates != 0 {
		t.Fatalf("Create() = %T %v, creates=%d; want no write and STORE_ERROR", err, err, jobs.creates)
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

// This catches restart code that silently drops durable queued work when its
// fixed-size worker queue cannot accept every recovered item.
func TestServiceResumeQueuedEnqueuesInCreationOrderOrFailsBeforeDelivery(t *testing.T) {
	t.Run("ordered delivery", func(t *testing.T) {
		memory := newMemoryJobStore()
		early := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
		late := early.Add(time.Minute)
		for _, job := range []*Job{{ID: "later", Status: StatusQueued, CreatedAt: late}, {ID: "earlier", Status: StatusQueued, CreatedAt: early}, {ID: "failed", Status: StatusFailed, CreatedAt: early}} {
			if err := memory.Create(context.Background(), job); err != nil {
				t.Fatal(err)
			}
		}
		queue := NewQueue()
		count, err := NewService(memory, queue).ResumeQueued(context.Background())
		if err != nil || count != 2 {
			t.Fatalf("ResumeQueued() = %d, %v; want 2, nil", count, err)
		}
		if first, second := <-queue, <-queue; first != "earlier" || second != "later" {
			t.Fatalf("recovery queue = %q, %q; want earlier, later", first, second)
		}
	})
	t.Run("over capacity", func(t *testing.T) {
		memory := newMemoryJobStore()
		for index := 0; index <= QueueCapacity; index++ {
			if err := memory.Create(context.Background(), &Job{ID: fmt.Sprintf("queued-%03d", index), Status: StatusQueued, CreatedAt: time.Unix(int64(index), 0)}); err != nil {
				t.Fatal(err)
			}
		}
		queue := NewQueue()
		count, err := NewService(memory, queue).ResumeQueued(context.Background())
		var jobErr *JobError
		if count != 0 || !errors.As(err, &jobErr) || jobErr.Code != ErrorCodeQueueFull {
			t.Fatalf("ResumeQueued() = %d, %v; want 0, QUEUE_FULL", count, err)
		}
		if len(queue) != 0 {
			t.Fatalf("recovery partially delivered %d jobs", len(queue))
		}
	})
}
