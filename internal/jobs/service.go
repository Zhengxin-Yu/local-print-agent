package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// QueueCapacity is the maximum number of jobs waiting for the single worker.
const QueueCapacity = 100

// NewQueue creates the fixed-capacity FIFO queue used by a Service and Worker.
func NewQueue() chan string { return make(chan string, QueueCapacity) }

// JobStore is the persistence boundary used by Service.
type JobStore interface {
	Create(context.Context, *Job) error
	Update(context.Context, *Job) error
	Get(context.Context, string) (*Job, error)
	List(context.Context) ([]*Job, error)
}

// Service owns queue submission. Its mutex makes checking capacity, durable
// state changes, and sending IDs an indivisible operation relative to Retry.
type Service struct {
	store JobStore
	queue chan<- string
	mu    sync.Mutex
	next  uint64
}

func NewService(store JobStore, queue chan<- string) *Service {
	return &Service{store: store, queue: queue}
}

func (s *Service) Create(ctx context.Context, request CreateJobRequest) (*Job, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	normalized, err := NormalizeCreateRequest(request)
	if err != nil {
		return nil, &JobError{Code: ErrorCodeInvalidRequest, Message: err.Error(), cause: err}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	if queueFull(s.queue) {
		return nil, &JobError{Code: ErrorCodeQueueFull, Message: "print queue is full"}
	}
	now := time.Now().UTC()
	s.next++
	job := &Job{
		ID:          fmt.Sprintf("job-%d-%d", now.UnixNano(), s.next),
		Type:        normalized.Type,
		PrinterName: normalized.PrinterName,
		Payload:     append([]byte(nil), normalized.Payload...),
		Status:      StatusQueued,
		CreatedAt:   now,
		UpdatedAt:   now,
		Attempts:    0,
	}
	if err := s.store.Create(ctx, job); err != nil {
		return nil, wrapStoreError("create job", err)
	}
	// Service is the only sender, so the capacity check above remains valid
	// while its mutex is held. Never let a full queue silently drop an ID.
	s.queue <- job.ID
	return cloneServiceJob(job), nil
}

func (s *Service) Get(ctx context.Context, id string) (*Job, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	job, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, wrapStoreError("get job", err)
	}
	return cloneServiceJob(job), nil
}

func (s *Service) List(ctx context.Context) ([]*Job, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	list, err := s.store.List(ctx)
	if err != nil {
		return nil, wrapStoreError("list jobs", err)
	}
	for index, job := range list {
		list[index] = cloneServiceJob(job)
	}
	return list, nil
}

func (s *Service) Retry(ctx context.Context, id string) (*Job, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	job, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, wrapStoreError("get job for retry", err)
	}
	if job.Status != StatusFailed {
		return nil, &JobError{Code: ErrorCodeRetryNotAllowed, Message: "only failed jobs can be retried"}
	}
	if queueFull(s.queue) {
		return nil, &JobError{Code: ErrorCodeQueueFull, Message: "print queue is full"}
	}
	if err := Transition(job, StatusQueued, time.Now().UTC()); err != nil {
		return nil, &JobError{Code: ErrorCodeRetryNotAllowed, Message: err.Error(), cause: err}
	}
	if err := s.store.Update(ctx, job); err != nil {
		return nil, wrapStoreError("retry job", err)
	}
	s.queue <- job.ID
	return cloneServiceJob(job), nil
}

func queueFull(queue chan<- string) bool {
	return queue == nil || cap(queue) == 0 || len(queue) >= cap(queue)
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return &JobError{Code: ErrorCodeContextCanceled, Message: "context is required"}
	}
	if err := ctx.Err(); err != nil {
		return &JobError{Code: ErrorCodeContextCanceled, Message: err.Error(), cause: err}
	}
	return nil
}

func wrapStoreError(operation string, err error) *JobError {
	return &JobError{Code: ErrorCodeStore, Message: fmt.Sprintf("%s: %v", operation, err), cause: err}
}

func cloneServiceJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	copy := *job
	copy.Payload = append([]byte(nil), job.Payload...)
	if job.Error != nil {
		errCopy := *job.Error
		copy.Error = &errCopy
	}
	if job.StartedAt != nil {
		value := *job.StartedAt
		copy.StartedAt = &value
	}
	if job.FinishedAt != nil {
		value := *job.FinishedAt
		copy.FinishedAt = &value
	}
	return &copy
}
