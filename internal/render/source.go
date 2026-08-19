package render

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"local-print-agent/internal/jobs"
)

type sourceTemplateData struct {
	JobID              string
	MinimumChromeMajor int
	ContestName        string
	TeamID             string
	TeamName           string
	Room               string
	ProblemID          string
	HighlightedCode    template.HTML
}

// MinimumChromeMajor is the first Chromium major version that supports the
// @page margin boxes used for reliable source-listing page numbers.
// Future browser renderers must reject older versions with
// RENDERER_VERSION_UNSUPPORTED rather than silently omit page numbers.
const MinimumChromeMajor = 131

// RenderSourceHTML renders supported source code with Chroma-controlled HTML.
// The highlighted fragment is the only value intentionally marked as safe.
func RenderSourceHTML(job *jobs.Job) ([]byte, error) {
	if job == nil {
		return nil, errors.New("source render job is required")
	}
	if job.Type != jobs.JobTypeSource {
		return nil, fmt.Errorf("source render requires job type %q", jobs.JobTypeSource)
	}
	var payload jobs.SourceCodePayload
	if err := decodeRenderPayload(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode source render payload: %w", err)
	}
	highlighted, err := highlightSource(payload.Language, payload.SourceCode)
	if err != nil {
		return nil, err
	}
	return executeTemplate("source_code.html.tmpl", sourceTemplateData{
		JobID: job.ID, MinimumChromeMajor: MinimumChromeMajor, ContestName: displayOrPlaceholder(payload.ContestName), TeamID: displayOrPlaceholder(payload.TeamID),
		TeamName: displayOrPlaceholder(payload.TeamName), Room: displayOrPlaceholder(payload.Room), ProblemID: displayOrPlaceholder(payload.ProblemID),
		HighlightedCode: template.HTML(highlighted), // Chroma's formatter escapes source text before this trusted insertion.
	})
}

func highlightSource(language, source string) ([]byte, error) {
	lexer := map[string]chroma.Lexer{
		"cpp":    lexers.Get("cpp"),
		"go":     lexers.Go,
		"python": lexers.Get("python"),
		"java":   lexers.Get("java"),
	}[language]
	if lexer == nil {
		return nil, fmt.Errorf("unsupported source language %q", language)
	}
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil, fmt.Errorf("tokenise %s source: %w", language, err)
	}
	var output bytes.Buffer
	formatter := html.New(html.WithClasses(true), html.WithLineNumbers(true))
	if err := formatter.Format(&output, styles.Get("github"), iterator); err != nil {
		return nil, fmt.Errorf("format highlighted %s source: %w", language, err)
	}
	return output.Bytes(), nil
}
