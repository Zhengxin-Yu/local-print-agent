package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateCreateRequest(t *testing.T) {
	validSource := `{"language":"go","source_code":"func main() {}"}`
	cases := []struct {
		name    string
		request CreateJobRequest
		wantErr bool
	}{
		{
			name: "accepts balloon ticket",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":"Team Atlas","problem_id":"A","solved_at":"2026-08-19T09:30:00+08:00"}`),
			},
		},
		{
			name: "accepts source code",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(validSource),
			},
		},
		{
			name: "accepts source code at six UTF-8 byte lower boundary",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"你好"}`),
			},
		},
		{
			name: "accepts source code containing HTML characters",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"<x>abc"}`),
			},
		},
		{
			name: "accepts source code at 65536 UTF-8 byte upper boundary",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"` + strings.Repeat("a", 65536) + `"}`),
			},
		},
		{
			name: "rejects missing printer name",
			request: CreateJobRequest{
				Type:    JobTypeSource,
				Payload: json.RawMessage(validSource),
			},
			wantErr: true,
		},
		{
			name: "rejects source payload with unknown field",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"func main() {}","copies":2}`),
			},
			wantErr: true,
		},
		{
			name: "rejects unknown job type",
			request: CreateJobRequest{
				Type:        JobType("photo"),
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "rejects blank balloon team name",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":" ","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects blank balloon problem id",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":"Team Atlas","problem_id":" \t","solved_at":"2026-08-19T09:30:00Z"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects source code shorter than six UTF-8 bytes",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"12345"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects source code longer than 65536 UTF-8 bytes",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"` + strings.Repeat("a", 65537) + `"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects unsupported source language",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"rust","source_code":"fn main() {}"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects balloon timestamp outside RFC3339",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":"Team Atlas","problem_id":"A","solved_at":"19-08-2026 09:30"}`),
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateRequest(tc.request)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateCreateRequest() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateCreateRequestNormalizesPayloadStrings(t *testing.T) {
	req := CreateJobRequest{
		Type:        JobTypeBalloon,
		PrinterName: " front-desk ",
		Payload:     json.RawMessage(`{"team_name":"  Team  Atlas  ","problem_id":" A ","solved_at":"2026-08-19T09:30:00Z"}`),
	}

	if err := ValidateCreateRequest(req); err != nil {
		t.Fatalf("ValidateCreateRequest() error = %v", err)
	}

	var payload BalloonPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TeamName != "Team  Atlas" || payload.ProblemID != "A" {
		t.Fatalf("payload = %#v, want trimmed display strings", payload)
	}
}
