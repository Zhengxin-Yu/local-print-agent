package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"local-print-agent/internal/jobs"
)

const serviceRestartedErrorCode jobs.ErrorCode = "SERVICE_RESTARTED"

// JSONStore stores all jobs in one JSON file. Its mutex protects both the
// in-memory map and writes to the backing file.
type JSONStore struct {
	mu     sync.RWMutex
	path   string
	jobs   map[string]*jobs.Job
	rename func(string, string) error
}

var _ Store = (*JSONStore)(nil)

type persistedJobs struct {
	Jobs []*jobs.Job `json:"jobs"`
}

// NewJSONStore loads an existing store from path. A missing file starts as an
// empty store; invalid existing contents are left untouched and reported.
func NewJSONStore(path string) (*JSONStore, error) {
	if path == "" {
		return nil, errors.New("JSON store path is required")
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &JSONStore{path: path, jobs: make(map[string]*jobs.Job), rename: os.Rename}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read jobs store %q: %w", path, err)
	}

	var persisted persistedJobs
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode jobs store %q: %w", path, err)
	}

	loaded := make(map[string]*jobs.Job, len(persisted.Jobs))
	for index, job := range persisted.Jobs {
		if job == nil {
			return nil, fmt.Errorf("decode jobs store %q: job at index %d is null", path, index)
		}
		if job.ID == "" {
			return nil, fmt.Errorf("decode jobs store %q: job at index %d has an empty ID", path, index)
		}
		if _, exists := loaded[job.ID]; exists {
			return nil, fmt.Errorf("decode jobs store %q: duplicate job ID %q", path, job.ID)
		}
		loaded[job.ID] = cloneJob(job)
	}
	return &JSONStore{path: path, jobs: loaded, rename: os.Rename}, nil
}

// Create persists job unless another job already uses its ID.
func (s *JSONStore) Create(ctx context.Context, job *jobs.Job) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if job == nil {
		return errors.New("cannot create a nil job")
	}
	if job.ID == "" {
		return errors.New("cannot create a job with an empty ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("create job %q: %w", job.ID, ErrAlreadyExists)
	}

	next := cloneJobs(s.jobs)
	next[job.ID] = cloneJob(job)
	if err := s.writeLocked(ctx, next); err != nil {
		return err
	}
	s.jobs = next
	return nil
}

// Update replaces an existing job with the supplied value.
func (s *JSONStore) Update(ctx context.Context, job *jobs.Job) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if job == nil {
		return errors.New("cannot update a nil job")
	}
	if job.ID == "" {
		return errors.New("cannot update a job with an empty ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("update job %q: %w", job.ID, ErrNotFound)
	}

	next := cloneJobs(s.jobs)
	next[job.ID] = cloneJob(job)
	if err := s.writeLocked(ctx, next); err != nil {
		return err
	}
	s.jobs = next
	return nil
}

// Get returns an independent copy of the requested job.
func (s *JSONStore) Get(ctx context.Context, id string) (*jobs.Job, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("get job %q: %w", id, ErrNotFound)
	}
	return cloneJob(job), nil
}

// List returns independent copies ordered by creation time and then ID.
func (s *JSONStore) List(ctx context.Context) ([]*jobs.Job, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	result := make([]*jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, cloneJob(job))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// RecoverInterrupted marks jobs left rendering or printing by a previous
// process as failed, and writes the resulting collection once.
func (s *JSONStore) RecoverInterrupted(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}

	next := cloneJobs(s.jobs)
	changed := false
	now := time.Now().UTC()
	for _, job := range next {
		if job.Status != jobs.StatusRendering && job.Status != jobs.StatusPrinting {
			continue
		}
		job.Status = jobs.StatusFailed
		job.Error = &jobs.JobError{
			Code:    serviceRestartedErrorCode,
			Message: "job interrupted because the print service restarted",
		}
		job.UpdatedAt = now
		job.FinishedAt = timePointer(now)
		changed = true
	}
	if !changed {
		return nil
	}
	if err := s.writeLocked(ctx, next); err != nil {
		return err
	}
	s.jobs = next
	return nil
}

func (s *JSONStore) writeLocked(ctx context.Context, values map[string]*jobs.Job) (err error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	entries := make([]*jobs.Job, 0, len(values))
	for _, job := range values {
		entries = append(entries, cloneJob(job))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	data, err := json.Marshal(persistedJobs{Jobs: entries})
	if err != nil {
		return fmt.Errorf("encode jobs store %q: %w", s.path, err)
	}

	dir := filepath.Dir(s.path)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary jobs store beside %q: %w", s.path, err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); err == nil && closeErr != nil {
				err = fmt.Errorf("close temporary jobs store %q: %w", temporaryPath, closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil {
			err = fmt.Errorf("remove temporary jobs store %q: %w", temporaryPath, removeErr)
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary jobs store %q: %w", temporaryPath, err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush temporary jobs store %q: %w", temporaryPath, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary jobs store %q: %w", temporaryPath, err)
	}
	closed = true
	if err := contextError(ctx); err != nil {
		return err
	}
	// Use one replacement operation, never a remove-then-rename sequence. The
	// in-memory state is committed only when this call reports success.
	if err := s.rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace jobs store %q with completed temporary file: %w", s.path, err)
	}
	return nil
}

func cloneJobs(source map[string]*jobs.Job) map[string]*jobs.Job {
	result := make(map[string]*jobs.Job, len(source))
	for id, job := range source {
		result[id] = cloneJob(job)
	}
	return result
}

func cloneJob(job *jobs.Job) *jobs.Job {
	if job == nil {
		return nil
	}
	copy := *job
	if job.Payload != nil {
		copy.Payload = append([]byte(nil), job.Payload...)
	}
	if job.Error != nil {
		errorCopy := *job.Error
		copy.Error = &errorCopy
	}
	copy.StartedAt = timePointerValue(job.StartedAt)
	copy.FinishedAt = timePointerValue(job.FinishedAt)
	return &copy
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}

func timePointerValue(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return timePointer(*value)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	return ctx.Err()
}
