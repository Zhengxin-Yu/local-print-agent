package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local-print-agent/internal/jobs"
)

func TestFakeRendererWritesRecognizableControlledPDF(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "controlled-output")
	renderer, err := NewFake(directory)
	if err != nil {
		t.Fatal(err)
	}
	path, err := renderer.Render(context.Background(), &jobs.Job{ID: "job-123"})
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		t.Fatalf("PDF path %q is outside controlled directory %q", path, directory)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "%PDF-") || !strings.Contains(string(contents), "FAKE RENDERER") {
		t.Fatalf("fake PDF does not identify itself: %q", contents)
	}
}
