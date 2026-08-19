package jobs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	minSourceCodeBytes = 6
	maxSourceCodeBytes = 65536
)

// ValidateCreateRequest validates a request without modifying it.
func ValidateCreateRequest(req CreateJobRequest) error {
	_, err := NormalizeCreateRequest(req)
	return err
}

// NormalizeCreateRequest returns a deep-copied, fully normalized request after
// validating its type, printer, and payload.
func NormalizeCreateRequest(req CreateJobRequest) (CreateJobRequest, error) {
	normalized := CreateJobRequest{
		Type:        JobType(strings.TrimSpace(string(req.Type))),
		PrinterName: strings.TrimSpace(req.PrinterName),
	}
	if normalized.PrinterName == "" {
		return CreateJobRequest{}, errors.New("printer_name is required")
	}

	var err error
	switch normalized.Type {
	case JobTypeBalloon:
		normalized.Payload, err = normalizeBalloonPayload(req.Payload)
	case JobTypeSource:
		normalized.Payload, err = normalizeSourceCodePayload(req.Payload)
	default:
		return CreateJobRequest{}, fmt.Errorf("unsupported job type %q", normalized.Type)
	}
	if err != nil {
		return CreateJobRequest{}, err
	}
	return normalized, nil
}

func normalizeBalloonPayload(raw json.RawMessage) (json.RawMessage, error) {
	var payload BalloonPayload
	if err := decodePayload(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid balloon payload: %w", err)
	}

	payload.TeamName = strings.TrimSpace(payload.TeamName)
	payload.ProblemID = strings.TrimSpace(payload.ProblemID)
	payload.SolvedAt = strings.TrimSpace(payload.SolvedAt)
	if payload.TeamName == "" {
		return nil, errors.New("balloon team_name is required")
	}
	if payload.ProblemID == "" {
		return nil, errors.New("balloon problem_id is required")
	}
	if strings.Contains(payload.SolvedAt, ",") {
		return nil, errors.New("balloon solved_at must use a period for fractional seconds")
	}
	if _, err := time.Parse(time.RFC3339, payload.SolvedAt); err != nil {
		return nil, fmt.Errorf("balloon solved_at must be RFC3339: %w", err)
	}

	return marshalPayload(payload)
}

func normalizeSourceCodePayload(raw json.RawMessage) (json.RawMessage, error) {
	var payload SourceCodePayload
	if err := decodePayload(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid source code payload: %w", err)
	}

	payload.Language = strings.TrimSpace(payload.Language)
	payload.SourceCode = strings.TrimSpace(payload.SourceCode)
	if !isSupportedLanguage(payload.Language) {
		return nil, fmt.Errorf("unsupported source language %q", payload.Language)
	}
	if byteCount := len([]byte(payload.SourceCode)); byteCount < minSourceCodeBytes || byteCount > maxSourceCodeBytes {
		return nil, fmt.Errorf("source_code must be between %d and %d UTF-8 bytes", minSourceCodeBytes, maxSourceCodeBytes)
	}

	return marshalPayload(payload)
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("payload is required")
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
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

func marshalPayload(payload any) (json.RawMessage, error) {
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(normalized), nil
}

func isSupportedLanguage(language string) bool {
	switch language {
	case "cpp", "go", "python", "java":
		return true
	default:
		return false
	}
}
