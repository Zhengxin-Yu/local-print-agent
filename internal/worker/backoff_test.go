package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
)

type backoffStore struct{ failures int }

func (s *backoffStore) Get(context.Context, string) (*jobs.Job, error) {
	if s.failures > 0 {
		s.failures--
		return nil, errors.New("temporary read failure")
	}
	return &jobs.Job{ID: "done", Status: jobs.StatusSucceeded}, nil
}

func (*backoffStore) Update(context.Context, *jobs.Job) error { return nil }

// This exercises the retry loop itself. A fixed delay, reset attempt counter,
// or unused policy cannot satisfy the observed delay sequence.
func TestWorkerUsesBoundedExponentialBackoffForStoreRetries(t *testing.T) {
	queue := make(chan string, 1)
	queue <- "done"
	close(queue)
	w := New(&backoffStore{failures: 8}, nil, nil, queue)
	var observed []time.Duration
	w.waitRetry = func(_ context.Context, delay time.Duration) bool {
		observed = append(observed, delay)
		return true
	}
	w.Run(context.Background())
	want := []time.Duration{5, 10, 20, 40, 80, 160, 250, 250}
	for index := range want {
		want[index] *= time.Millisecond
	}
	if !reflect.DeepEqual(observed, want) {
		t.Fatalf("retry delays = %v, want bounded exponential %v", observed, want)
	}
}
