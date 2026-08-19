package jobs

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCanTransitionAllowsOnlyDocumentedPaths(t *testing.T) {
	allowed := []struct {
		from Status
		to   Status
	}{
		{StatusQueued, StatusRendering},
		{StatusRendering, StatusPrinting},
		{StatusRendering, StatusFailed},
		{StatusPrinting, StatusSucceeded},
		{StatusPrinting, StatusFailed},
		{StatusFailed, StatusQueued},
	}

	for _, tc := range allowed {
		if !CanTransition(tc.from, tc.to) {
			t.Errorf("CanTransition(%q, %q) = false, want true", tc.from, tc.to)
		}
	}
}

func testTimePointer(value time.Time) *time.Time {
	timestamp := value
	return &timestamp
}

func TestJobJSONOmitsClearedLifecycleTimes(t *testing.T) {
	now := time.Date(2026, time.August, 19, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		job  Job
	}{
		{
			name: "new queued job",
			job:  Job{Status: StatusQueued},
		},
		{
			name: "failed job retried to queued",
			job: Job{
				Status:     StatusFailed,
				StartedAt:  testTimePointer(now.Add(-2 * time.Minute)),
				FinishedAt: testTimePointer(now.Add(-time.Minute)),
				Error:      &JobError{Code: ErrorCodePrintFailed, Message: "paper out"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := tc.job
			if job.Status == StatusFailed {
				if err := Transition(&job, StatusQueued, now); err != nil {
					t.Fatalf("Transition() error = %v", err)
				}
			}

			encoded, err := json.Marshal(job)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			for _, key := range []string{"started_at", "finished_at"} {
				if _, present := fields[key]; present {
					t.Errorf("queued job JSON includes %q: %s", key, encoded)
				}
			}
		})
	}
}

func TestTransitionEnteringRenderingStartsNewAttempt(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	job := Job{
		Status:     StatusQueued,
		Attempts:   2,
		FinishedAt: testTimePointer(now.Add(-time.Minute)),
		Error:      &JobError{Code: ErrorCodeRenderFailed, Message: "old failure"},
	}

	if err := Transition(&job, StatusRendering, now); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if job.Status != StatusRendering || job.StartedAt == nil || !job.StartedAt.Equal(now) || !job.UpdatedAt.Equal(now) {
		t.Fatalf("job lifecycle fields = %#v, want rendering started and updated at now", job)
	}
	if job.StartedAt == &now {
		t.Fatal("StartedAt aliases the caller's time value")
	}
	if job.Attempts != 3 {
		t.Fatalf("Attempts = %d, want 3", job.Attempts)
	}
	if job.FinishedAt != nil || job.Error != nil {
		t.Fatalf("stale completion fields were not cleared: %#v", job)
	}
}

func TestTransitionMarksRenderingFailureWithoutOverwritingError(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 1, 0, 0, time.UTC)
	errInfo := &JobError{Code: ErrorCodeRenderFailed, Message: "renderer timed out"}
	job := Job{Status: StatusRendering, Error: errInfo}

	if err := Transition(&job, StatusFailed, now); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if job.Status != StatusFailed || job.FinishedAt == nil || !job.FinishedAt.Equal(now) || !job.UpdatedAt.Equal(now) {
		t.Fatalf("job lifecycle fields = %#v, want failed and finished at now", job)
	}
	if job.Error != errInfo {
		t.Fatalf("Error = %#v, want pre-set failure %#v preserved", job.Error, errInfo)
	}
}

func TestTransitionMarksPrintingFailureWithoutOverwritingError(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 2, 0, 0, time.UTC)
	errInfo := &JobError{Code: ErrorCodePrintFailed, Message: "printer offline"}
	job := Job{Status: StatusPrinting, Error: errInfo}

	if err := Transition(&job, StatusFailed, now); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if job.Status != StatusFailed || job.FinishedAt == nil || !job.FinishedAt.Equal(now) || job.Error != errInfo {
		t.Fatalf("job = %#v, want failed with finished time and preserved error", job)
	}
}

func TestTransitionPrintingSucceededFinishesAndClearsError(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 3, 0, 0, time.UTC)
	job := Job{Status: StatusPrinting, Error: &JobError{Code: ErrorCodePrintFailed, Message: "transient"}}

	if err := Transition(&job, StatusSucceeded, now); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if job.Status != StatusSucceeded || job.FinishedAt == nil || !job.FinishedAt.Equal(now) || !job.UpdatedAt.Equal(now) || job.Error != nil {
		t.Fatalf("job = %#v, want succeeded with completion time and no error", job)
	}
}

func TestTransitionRetryQueuesFailedJobWithoutStartingAttempt(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 4, 0, 0, time.UTC)
	job := Job{
		Status:     StatusFailed,
		Attempts:   4,
		StartedAt:  testTimePointer(now.Add(-2 * time.Minute)),
		FinishedAt: testTimePointer(now.Add(-time.Minute)),
		Error:      &JobError{Code: ErrorCodePrintFailed, Message: "paper out"},
	}

	if err := Transition(&job, StatusQueued, now); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if job.Status != StatusQueued || !job.UpdatedAt.Equal(now) || job.Attempts != 4 {
		t.Fatalf("job = %#v, want queued at now without a new attempt", job)
	}
	if job.StartedAt != nil || job.FinishedAt != nil || job.Error != nil {
		t.Fatalf("retry state was not reset: %#v", job)
	}
}

func TestTransitionRejectsIllegalStateChangesWithoutModifyingJob(t *testing.T) {
	now := time.Date(2026, time.August, 19, 10, 5, 0, 0, time.UTC)
	cases := []struct {
		name string
		from Status
		to   Status
	}{
		{"succeeded to queued", StatusSucceeded, StatusQueued},
		{"queued to succeeded", StatusQueued, StatusSucceeded},
		{"failed to printing", StatusFailed, StatusPrinting},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := Job{
				Status:     tc.from,
				Attempts:   7,
				StartedAt:  testTimePointer(now.Add(-2 * time.Minute)),
				FinishedAt: testTimePointer(now.Add(-time.Minute)),
				UpdatedAt:  now.Add(-3 * time.Minute),
				Error:      &JobError{Code: ErrorCodePrintFailed, Message: "keep me"},
			}
			before := job

			err := Transition(&job, tc.to, now)
			var jobErr *JobError
			if !errors.As(err, &jobErr) {
				t.Fatalf("Transition() error = %T %v, want an errors.As-compatible *JobError", err, err)
			}
			if jobErr.Code != ErrorCodeInvalidTransition {
				t.Fatalf("JobError.Code = %q, want %q", jobErr.Code, ErrorCodeInvalidTransition)
			}
			if !reflect.DeepEqual(job, before) {
				t.Fatalf("illegal transition modified job: got %#v, want %#v", job, before)
			}
			if job.StartedAt != before.StartedAt || job.FinishedAt != before.FinishedAt {
				t.Fatalf("illegal transition replaced lifecycle timestamp pointers: got %#v, want %#v", job, before)
			}
		})
	}
}
