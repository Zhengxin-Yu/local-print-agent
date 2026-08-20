package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const QueueCapacity = 100
const maxIDGenerationAttempts = 8

// NewQueue creates the fixed-capacity queue used by the opaque Pipeline
// assembly. Prefer worker.NewPipeline; this function is a low-level entry.
func NewQueue() chan string { return make(chan string, QueueCapacity) }

type JobStore interface {
	Create(context.Context, *Job) error
	Update(context.Context, *Job) error
	Get(context.Context, string) (*Job, error)
	List(context.Context) ([]*Job, error)
}

type IDGenerator func() (string, error)

var queueRegistry = struct {
	sync.Mutex
	owners map[chan<- string]*Service
}{owners: make(map[chan<- string]*Service)}

// Service is the sole queue sender. NewService is a low-level compatibility
// constructor: queue must be non-nil with cap QueueCapacity, exactly one
// Service may own it, and callers must not send, close, or reuse it. Prefer
// worker.NewPipeline, which keeps the channel opaque.
type Service struct {
	store            JobStore
	queue            chan<- string
	idGenerator      IDGenerator
	mu               sync.Mutex
	deliveries       int
	contractViolated bool
	closed           bool
}

func NewService(store JobStore, queue chan<- string) *Service {
	return NewServiceWithIDGenerator(store, queue, cryptoID)
}

func NewServiceWithIDGenerator(store JobStore, queue chan<- string, generator IDGenerator) *Service {
	if queue == nil || cap(queue) != QueueCapacity {
		panic(fmt.Sprintf("jobs.NewService requires a non-nil queue with capacity %d", QueueCapacity))
	}
	if generator == nil {
		generator = cryptoID
	}
	service := &Service{store: store, queue: queue, idGenerator: generator}
	queueRegistry.Lock()
	_, service.contractViolated = queueRegistry.owners[queue]
	if !service.contractViolated {
		queueRegistry.owners[queue] = service
	}
	queueRegistry.Unlock()
	return service
}

// Close releases the low-level queue ownership registration. It does not
// close the caller-owned channel. NewPipeline closes this lifecycle
// automatically when its Worker exits.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	queueRegistry.Lock()
	if queueRegistry.owners[s.queue] == s {
		delete(queueRegistry.owners, s.queue)
	}
	queueRegistry.Unlock()
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
	if s.closed {
		return nil, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "print queue is unavailable"}
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	violation := s.queueInterfered()
	if queueFull(s.queue) && !violation {
		return nil, &JobError{Code: ErrorCodeQueueFull, Message: "print queue is full"}
	}
	for attempt := 0; attempt < maxIDGenerationAttempts; attempt++ {
		id, err := s.idGenerator()
		if err != nil {
			return nil, wrapStoreError("generate job ID", err)
		}
		if id == "" {
			return nil, &JobError{Code: ErrorCodeStore, Message: "generate job ID: empty ID"}
		}
		now := time.Now().UTC()
		job := &Job{ID: id, Type: normalized.Type, PrinterName: normalized.PrinterName, Payload: append([]byte(nil), normalized.Payload...), Status: StatusQueued, CreatedAt: now, UpdatedAt: now}
		if err := s.store.Create(ctx, job); err != nil {
			if errors.Is(err, ErrAlreadyExists) {
				continue
			}
			return nil, wrapStoreError("create job", err)
		}
		return s.deliver(job, violation)
	}
	return nil, &JobError{Code: ErrorCodeStore, Message: "generate job ID: too many collisions", cause: ErrAlreadyExists}
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
	if s.closed {
		return nil, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "print queue is unavailable"}
	}
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
	violation := s.queueInterfered()
	if queueFull(s.queue) && !violation {
		return nil, &JobError{Code: ErrorCodeQueueFull, Message: "print queue is full"}
	}
	if err := Transition(job, StatusQueued, time.Now().UTC()); err != nil {
		return nil, &JobError{Code: ErrorCodeRetryNotAllowed, Message: err.Error(), cause: err}
	}
	if err := s.store.Update(ctx, job); err != nil {
		return nil, wrapStoreError("retry job", err)
	}
	return s.deliver(job, violation)
}

// ResumeQueued restores durable queued jobs to this process's private worker
// queue after a restart. It verifies capacity before sending anything, so a
// queue over 100 returns a visible error instead of silently losing work.
func (s *Service) ResumeQueued(ctx context.Context) (int, error) {
	if err := contextErr(ctx); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "print queue is unavailable"}
	}
	if s.store == nil {
		return 0, &JobError{Code: ErrorCodeStore, Message: "job store is required"}
	}
	if s.queueInterfered() {
		return 0, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "worker queue ownership was interrupted"}
	}
	list, err := s.store.List(ctx)
	if err != nil {
		return 0, wrapStoreError("list queued jobs for recovery", err)
	}
	queued := make([]*Job, 0, len(list))
	for _, job := range list {
		if job != nil && job.Status == StatusQueued {
			queued = append(queued, job)
		}
	}
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].CreatedAt.Equal(queued[j].CreatedAt) {
			return queued[i].ID < queued[j].ID
		}
		return queued[i].CreatedAt.Before(queued[j].CreatedAt)
	})
	if len(queued) > cap(s.queue)-len(s.queue) {
		return 0, &JobError{Code: ErrorCodeQueueFull, Message: "too many queued jobs to restore to the worker queue"}
	}
	for _, job := range queued {
		if !trySend(s.queue, job.ID) {
			return 0, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "queued job could not be restored to worker queue"}
		}
		s.deliveries++
	}
	return len(queued), nil
}

func (s *Service) queueInterfered() bool { return s.contractViolated || len(s.queue) > s.deliveries }

func (s *Service) deliver(job *Job, violation bool) (*Job, error) {
	copy := cloneServiceJob(job)
	if violation || !trySend(s.queue, job.ID) {
		return copy, &JobError{Code: ErrorCodeQueueDeliveryFailed, Message: "queued job could not be delivered to worker queue"}
	}
	s.deliveries++
	return copy, nil
}

func trySend(queue chan<- string, id string) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case queue <- id:
		return true
	default:
		return false
	}
}

func queueFull(queue chan<- string) bool { return len(queue) >= cap(queue) }

func cryptoID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
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
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &JobError{Code: ErrorCodeContextCanceled, Message: fmt.Sprintf("%s: %v", operation, err), cause: err}
	}
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
