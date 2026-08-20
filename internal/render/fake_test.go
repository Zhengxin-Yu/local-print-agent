package render

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	assertMinimalPDFStructure(t, contents, "job-123")
}

func TestFakeRendererPassesConfiguredPDFInfo(t *testing.T) {
	command := strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_PDFINFO"))
	if command == "" {
		t.Skip("set LOCAL_PRINT_AGENT_PDFINFO to opt in to the external pdfinfo check")
	}
	renderer, err := NewFake(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path, err := renderer.Render(context.Background(), &jobs.Job{ID: "external-pdfinfo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("configured pdfinfo is unavailable: %v", err)
	}
	if output, err := exec.Command(command, path).CombinedOutput(); err != nil {
		lower := strings.ToLower(string(output))
		if strings.Contains(lower, "fresh tex installation") || strings.Contains(lower, "create directory") || strings.Contains(lower, "access is denied") || strings.Contains(string(output), "拒绝访问") {
			t.Skipf("configured pdfinfo cannot initialize in this environment: %v", err)
		}
		t.Fatalf("pdfinfo failed: %v\n%s", err, output)
	}
}

func TestEscapePDFStringEscapesLiteralStringDelimiters(t *testing.T) {
	if got := escapePDFString("job-(\\)\r\nnext"); got != "job-\\(\\\\\\) next" {
		t.Fatalf("escapePDFString() = %q", got)
	}
}

// assertMinimalPDFStructure checks the cross-references itself instead of
// trusting only an external viewer. A removed xref, Root, Page, or stream
// object must make this test fail.
func assertMinimalPDFStructure(t *testing.T, contents []byte, jobID string) {
	t.Helper()
	startMarker := []byte("startxref\n")
	start := bytes.LastIndex(contents, startMarker)
	if start < 0 {
		t.Fatal("PDF has no startxref")
	}
	end := bytes.IndexByte(contents[start+len(startMarker):], '\n')
	if end < 0 {
		t.Fatal("PDF startxref has no offset")
	}
	offsetText := string(contents[start+len(startMarker) : start+len(startMarker)+end])
	xrefOffset, err := strconv.Atoi(offsetText)
	if err != nil || xrefOffset < 0 || xrefOffset >= len(contents) || !bytes.HasPrefix(contents[xrefOffset:], []byte("xref\n")) {
		t.Fatalf("startxref=%q does not point to xref", offsetText)
	}
	if !bytes.HasSuffix(contents, []byte("%%EOF\n")) {
		t.Fatal("PDF has no final EOF marker")
	}
	for object, want := range map[int]string{
		1: "/Type /Catalog /Pages 2 0 R",
		2: "/Type /Pages /Kids [3 0 R] /Count 1",
		3: "/Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R",
		4: "/Type /Font /Subtype /Type1 /BaseFont /Helvetica",
		5: "stream\n",
	} {
		objectHeader := []byte(fmt.Sprintf("%d 0 obj\n", object))
		position := bytes.Index(contents, objectHeader)
		if position < 0 || !bytes.Contains(contents[position:], []byte(want)) {
			t.Fatalf("PDF object %d is missing required structure %q", object, want)
		}
		entry := []byte(fmt.Sprintf("%010d 00000 n \n", position))
		if !bytes.Contains(contents[xrefOffset:], entry) {
			t.Fatalf("xref does not point to object %d at %d", object, position)
		}
	}
	if !bytes.Contains(contents, []byte("BT\n/F1 16 Tf\n")) || !bytes.Contains(contents, []byte("FAKE RENDERER")) || !bytes.Contains(contents, []byte(jobID)) {
		t.Fatal("PDF content stream lacks fake-renderer text and job ID")
	}
	if !bytes.Contains(contents[xrefOffset:], []byte("trailer\n<< /Size 6 /Root 1 0 R >>\n")) {
		t.Fatal("PDF trailer lacks catalog Root")
	}
}
