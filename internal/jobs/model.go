package jobs

import (
	"encoding/json"
	"time"
)

// JobType identifies the document workflow a print job belongs to.
type JobType string

const (
	JobTypeBalloon JobType = "balloon_ticket"
	JobTypeSource  JobType = "source_code"
)

// Status is the lifecycle state persisted for a print job.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRendering Status = "rendering"
	StatusPrinting  Status = "printing"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// ErrorCode is a stable machine-readable reason for a failed job.
type ErrorCode string

const (
	ErrorCodeInvalidTransition ErrorCode = "INVALID_TRANSITION"
	ErrorCodeInvalidRequest    ErrorCode = "INVALID_REQUEST"
	ErrorCodeQueueFull         ErrorCode = "QUEUE_FULL"
	ErrorCodeRetryNotAllowed   ErrorCode = "RETRY_NOT_ALLOWED"
	ErrorCodeStore             ErrorCode = "STORE_ERROR"
	ErrorCodeContextCanceled   ErrorCode = "CONTEXT_CANCELED"
	ErrorCodeRenderFailed      ErrorCode = "RENDER_FAILED"
	ErrorCodePrintFailed       ErrorCode = "PRINT_COMMAND_FAILED"
)

// JobError provides a stable machine-readable code alongside a readable
// explanation suitable for logs and API responses.
type JobError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	cause   error
}

func (e *JobError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Unwrap retains the underlying storage or context error for diagnostics.
func (e *JobError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Job contains the fields required by the future persistent job state machine.
type Job struct {
	ID          string          `json:"id"`
	Type        JobType         `json:"type"`
	PrinterName string          `json:"printer_name"`
	Payload     json.RawMessage `json:"payload"`
	Status      Status          `json:"status"`
	Error       *JobError       `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	Attempts    int             `json:"attempts"`
	PDFPath     string          `json:"pdf_path,omitempty"`
}

// CreateJobRequest is the request body accepted when a job is created.
type CreateJobRequest struct {
	Type        JobType         `json:"type"`
	PrinterName string          `json:"printer_name"`
	Payload     json.RawMessage `json:"payload"`
}

// BalloonPayload is the content printed on a balloon ticket.
type BalloonPayload struct {
	TeamName  string `json:"team_name"`
	ProblemID string `json:"problem_id"`
	SolvedAt  string `json:"solved_at"`
}

// SourceCodePayload is the content printed for a source-code job.
type SourceCodePayload struct {
	Language   string `json:"language"`
	SourceCode string `json:"source_code"`
}
