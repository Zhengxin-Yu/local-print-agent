package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"strings"

	"local-print-agent/internal/jobs"
	"local-print-agent/templates"
)

type balloonTemplateData struct {
	JobID        string
	ContestName  string
	TeamID       string
	TeamName     string
	Room         string
	ProblemID    string
	BalloonColor string
	SolvedAt     string
}

// RenderBalloonHTML renders one validated balloon job as a self-contained
// narrow-paper document. Metadata remains plain strings for template escaping.
func RenderBalloonHTML(job *jobs.Job) ([]byte, error) {
	if job == nil {
		return nil, errors.New("balloon render job is required")
	}
	if job.Type != jobs.JobTypeBalloon {
		return nil, fmt.Errorf("balloon render requires job type %q", jobs.JobTypeBalloon)
	}
	var payload jobs.BalloonPayload
	if err := decodeRenderPayload(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode balloon render payload: %w", err)
	}
	return executeTemplate("balloon_ticket.html.tmpl", balloonTemplateData{
		JobID: job.ID, ContestName: displayOrPlaceholder(payload.ContestName), TeamID: displayOrPlaceholder(payload.TeamID),
		TeamName: displayOrPlaceholder(payload.TeamName), Room: displayOrPlaceholder(payload.Room), ProblemID: displayOrPlaceholder(payload.ProblemID),
		BalloonColor: displayOrPlaceholder(payload.BalloonColor), SolvedAt: displayOrPlaceholder(payload.SolvedAt),
	})
}

func executeTemplate(name string, data any) ([]byte, error) {
	contents, err := templates.Assets.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", name, err)
	}
	return executeTemplateContents(name, contents, data)
}

func executeTemplateContents(name string, contents []byte, data any) ([]byte, error) {
	parsed, err := template.New(name).Parse(string(contents))
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", name, err)
	}
	return output.Bytes(), nil
}

func decodeRenderPayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("payload is required")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("payload must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("payload must contain one JSON value")
		}
		return err
	}
	return nil
}

func displayOrPlaceholder(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "未提供"
}
