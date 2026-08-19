package render

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"local-print-agent/internal/jobs"
)

// Fake is a local development renderer. It writes a deliberately recognizable
// placeholder PDF and never attempts HTML, browser, or Chromedp rendering.
type Fake struct{ outputDir string }

var _ Renderer = (*Fake)(nil)

// NewFake creates the program-controlled directory used for placeholder PDFs.
func NewFake(outputDir string) (*Fake, error) {
	if outputDir == "" {
		return nil, errors.New("fake renderer output directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create fake renderer output directory: %w", err)
	}
	return &Fake{outputDir: outputDir}, nil
}

// Render writes an identifiable test artifact. The digest means job input can
// never select a path outside the output directory.
func (f *Fake) Render(ctx context.Context, job *jobs.Job) (string, error) {
	if ctx == nil {
		return "", context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f == nil || f.outputDir == "" {
		return "", errors.New("fake renderer is required")
	}
	if job == nil || job.ID == "" {
		return "", errors.New("fake renderer requires a job ID")
	}
	digest := sha256.Sum256([]byte(job.ID))
	path := filepath.Join(f.outputDir, fmt.Sprintf("mock-%x.pdf", digest[:8]))
	contents := []byte("%PDF-1.4\n% LOCAL-PRINT-AGENT FAKE RENDERER — NOT A REAL PDF\n%%EOF\n")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return "", fmt.Errorf("write fake PDF: %w", err)
	}
	return path, nil
}
