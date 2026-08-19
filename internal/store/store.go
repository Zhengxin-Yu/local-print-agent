// Package store provides durable storage for print jobs.
package store

import (
	"context"
	"errors"

	"local-print-agent/internal/jobs"
)

// ErrNotFound reports that no stored job has the requested ID.
var ErrNotFound = errors.New("job not found")

// ErrAlreadyExists aliases the domain duplicate sentinel so callers can use
// errors.Is without introducing a jobs-to-store import cycle.
var ErrAlreadyExists = jobs.ErrAlreadyExists

// Store persists jobs and returns copies that callers may safely modify.
type Store interface {
	Create(context.Context, *jobs.Job) error
	Update(context.Context, *jobs.Job) error
	Get(context.Context, string) (*jobs.Job, error)
	List(context.Context) ([]*jobs.Job, error)
	RecoverInterrupted(context.Context) error
}
