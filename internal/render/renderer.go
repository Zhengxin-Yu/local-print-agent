// Package render defines document rendering without committing to a PDF engine.
package render

import (
	"context"

	"local-print-agent/internal/jobs"
)

// Renderer writes the job document to a temporary PDF and returns its path.
// Production renderers own the file lifecycle; this interface performs no
// printing itself.
type Renderer interface {
	Render(context.Context, *jobs.Job) (string, error)
}
