package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	if err := writeFakePDF(path, minimalPDF(job.ID)); err != nil {
		return "", fmt.Errorf("write fake PDF: %w", err)
	}
	return path, nil
}

func writeFakePDF(path string, contents []byte) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mock-pdf-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err = temporary.Write(contents); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func minimalPDF(jobID string) []byte {
	stream := []byte("BT\n/F1 16 Tf\n72 720 Td\n(FAKE RENDERER - job " + escapePDFString(jobID) + ") Tj\nET\n")
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>"),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		[]byte("<< /Length " + strconv.Itoa(len(stream)) + " >>\nstream\n" + string(stream) + "endstream"),
	}
	var document bytes.Buffer
	document.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = document.Len()
		fmt.Fprintf(&document, "%d 0 obj\n", index+1)
		document.Write(object)
		document.WriteString("\nendobj\n")
	}
	xrefOffset := document.Len()
	fmt.Fprintf(&document, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&document, "%010d 00000 n \n", offset)
	}
	document.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	document.WriteString(strconv.Itoa(xrefOffset))
	document.WriteString("\n%%EOF\n")
	return document.Bytes()
}

func escapePDFString(value string) string {
	var escaped strings.Builder
	spaceWritten := false
	for _, character := range []byte(value) {
		switch character {
		case '\\', '(', ')':
			escaped.WriteByte('\\')
			escaped.WriteByte(character)
			spaceWritten = false
		case '\r', '\n':
			if !spaceWritten {
				escaped.WriteByte(' ')
				spaceWritten = true
			}
		default:
			if character < 0x20 || character == 0x7f {
				if !spaceWritten {
					escaped.WriteByte(' ')
					spaceWritten = true
				}
				continue
			}
			escaped.WriteByte(character)
			spaceWritten = false
		}
	}
	return escaped.String()
}
