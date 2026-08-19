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

// ValidateCreateRequest validates a request and normalizes its payload strings.
func ValidateCreateRequest(req CreateJobRequest) error {
	req.Type = JobType(strings.TrimSpace(string(req.Type)))
	req.PrinterName = strings.TrimSpace(req.PrinterName)
	if req.PrinterName == "" {
		return errors.New("printer_name is required")
	}

	switch req.Type {
	case JobTypeBalloon:
		return validateBalloonPayload(req.Payload)
	case JobTypeSource:
		return validateSourceCodePayload(req.Payload)
	default:
		return fmt.Errorf("unsupported job type %q", req.Type)
	}
}

func validateBalloonPayload(raw json.RawMessage) error {
	var payload BalloonPayload
	if err := decodePayload(raw, &payload); err != nil {
		return fmt.Errorf("invalid balloon payload: %w", err)
	}

	payload.TeamName = strings.TrimSpace(payload.TeamName)
	payload.ProblemID = strings.TrimSpace(payload.ProblemID)
	payload.SolvedAt = strings.TrimSpace(payload.SolvedAt)
	if payload.TeamName == "" {
		return errors.New("balloon team_name is required")
	}
	if payload.ProblemID == "" {
		return errors.New("balloon problem_id is required")
	}
	if _, err := time.Parse(time.RFC3339, payload.SolvedAt); err != nil {
		return fmt.Errorf("balloon solved_at must be RFC3339: %w", err)
	}

	return replaceRawMessage(raw, payload)
}

func validateSourceCodePayload(raw json.RawMessage) error {
	var payload SourceCodePayload
	if err := decodePayload(raw, &payload); err != nil {
		return fmt.Errorf("invalid source code payload: %w", err)
	}

	payload.Language = strings.TrimSpace(payload.Language)
	payload.SourceCode = strings.TrimSpace(payload.SourceCode)
	if !isSupportedLanguage(payload.Language) {
		return fmt.Errorf("unsupported source language %q", payload.Language)
	}
	if byteCount := len([]byte(payload.SourceCode)); byteCount < minSourceCodeBytes || byteCount > maxSourceCodeBytes {
		return fmt.Errorf("source_code must be between %d and %d UTF-8 bytes", minSourceCodeBytes, maxSourceCodeBytes)
	}

	return replaceRawMessage(raw, payload)
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("payload is required")
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

func replaceRawMessage(raw json.RawMessage, payload any) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return err
	}
	normalized := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
	if len(normalized) > len(raw) {
		// ValidateCreateRequest receives the request by value, so the caller's
		// RawMessage slice cannot be resized. Keep an already-valid payload if
		// JSON escaping would make its canonical representation longer.
		return nil
	}

	copy(raw, normalized)
	for i := len(normalized); i < len(raw); i++ {
		raw[i] = ' '
	}
	return nil
}

func isSupportedLanguage(language string) bool {
	switch language {
	case "cpp", "go", "python", "java":
		return true
	default:
		return false
	}
}
