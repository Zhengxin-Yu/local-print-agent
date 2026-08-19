package jobs

import (
	"fmt"
	"time"
)

// CanTransition reports whether a job can move directly from one lifecycle
// status to another.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusQueued:
		return to == StatusRendering
	case StatusRendering:
		return to == StatusPrinting || to == StatusFailed
	case StatusPrinting:
		return to == StatusSucceeded || to == StatusFailed
	case StatusFailed:
		return to == StatusQueued
	default:
		return false
	}
}

// Transition advances job to a permitted status and updates lifecycle fields
// using now. On failure, the job is left unchanged.
func Transition(job *Job, to Status, now time.Time) error {
	if job == nil {
		return &JobError{Code: ErrorCodeInvalidTransition, Message: "cannot transition a nil job"}
	}
	if !CanTransition(job.Status, to) {
		return &JobError{
			Code:    ErrorCodeInvalidTransition,
			Message: fmt.Sprintf("cannot transition job from %q to %q", job.Status, to),
		}
	}

	job.Status = to
	job.UpdatedAt = now

	switch to {
	case StatusRendering:
		job.StartedAt = timestampPointer(now)
		job.FinishedAt = nil
		job.Attempts++
		job.Error = nil
	case StatusFailed:
		job.FinishedAt = timestampPointer(now)
	case StatusSucceeded:
		job.FinishedAt = timestampPointer(now)
		job.Error = nil
	case StatusQueued:
		job.StartedAt = nil
		job.FinishedAt = nil
		job.Error = nil
	}
	return nil
}

func timestampPointer(value time.Time) *time.Time {
	timestamp := value
	return &timestamp
}
