// Package store provides durable storage for print jobs.
package store

import (
	"context"
	"errors"

	"local-print-agent/internal/jobs"
)

// ErrNotFound reports that no stored job has the requested ID.
var ErrNotFound = errors.New("job not found")

// Store persists jobs and returns copies that callers may safely modify.
type Store interface {
	Create(context.Context, *jobs.Job) error
	Update(context.Context, *jobs.Job) error
	Get(context.Context, string) (*jobs.Job, error)
	List(context.Context) ([]*jobs.Job, error)
	RecoverInterrupted(context.Context) error
}
