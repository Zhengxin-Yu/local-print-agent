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
		{
			name: "rejects balloon timestamp with comma fractional seconds",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":"Team Atlas","problem_id":"A","solved_at":"2026-08-19T09:30:00,5Z"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects payload with trailing JSON",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":"func main() {}"} {}`),
			},
			wantErr: true,
		},
		{
			name: "rejects null payload",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`null`),
			},
			wantErr: true,
		},
		{
			name: "rejects null required payload field",
			request: CreateJobRequest{
				Type:        JobTypeBalloon,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"team_name":null,"problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`),
			},
			wantErr: true,
		},
		{
			name: "rejects payload field with wrong JSON type",
			request: CreateJobRequest{
				Type:        JobTypeSource,
				PrinterName: "front-desk",
				Payload:     json.RawMessage(`{"language":"go","source_code":42}`),
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

func TestNormalizeCreateRequestReturnsDeepCopiedNormalizedRequest(t *testing.T) {
	req := CreateJobRequest{
		Type:        JobType(" source_code "),
		PrinterName: "  hallway  printer  ",
		Payload:     json.RawMessage("{\"language\":\" go \",\"source_code\":\"  alpha  \u2028beta\u2029gamma  \"}"),
	}
	alias := req.Payload
	before := string(req.Payload)

	normalized, err := NormalizeCreateRequest(req)
	if err != nil {
		t.Fatalf("NormalizeCreateRequest() error = %v", err)
	}

	if normalized.Type != JobTypeSource {
		t.Fatalf("Type = %q, want %q", normalized.Type, JobTypeSource)
	}
	if normalized.PrinterName != "hallway  printer" {
		t.Fatalf("PrinterName = %q, want internal spaces preserved", normalized.PrinterName)
	}
	if got, want := string(normalized.Payload), `{"language":"go","source_code":"alpha  \u2028beta\u2029gamma"}`; got != want {
		t.Fatalf("Payload = %q, want %q", got, want)
	}
	if got := string(req.Payload); got != before {
		t.Fatalf("input Payload changed to %q, want %q", got, before)
	}
	if got := string(alias); got != before {
		t.Fatalf("RawMessage alias changed to %q, want %q", got, before)
	}
	if &normalized.Payload[0] == &req.Payload[0] {
		t.Fatal("normalized Payload aliases input Payload")
	}
}

func TestNormalizeCreateRequestNormalizesBalloonPayloadStrings(t *testing.T) {
	normalized, err := NormalizeCreateRequest(CreateJobRequest{
		Type:        JobType(" balloon_ticket "),
		PrinterName: "  front  desk  ",
		Payload:     json.RawMessage(`{"team_name":"  Team  Atlas  ","problem_id":" A ","solved_at":" 2026-08-19T09:30:00Z "}`),
	})
	if err != nil {
		t.Fatalf("NormalizeCreateRequest() error = %v", err)
	}

	if normalized.Type != JobTypeBalloon || normalized.PrinterName != "front  desk" {
		t.Fatalf("request = %#v, want normalized top-level strings", normalized)
	}
	if got, want := string(normalized.Payload), `{"team_name":"Team  Atlas","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`; got != want {
		t.Fatalf("Payload = %q, want %q", got, want)
	}
}

func TestValidateCreateRequestDoesNotModifyRequest(t *testing.T) {
	req := CreateJobRequest{
		Type:        JobType(" balloon_ticket "),
		PrinterName: " front-desk ",
		Payload:     json.RawMessage(`{"team_name":"  Team  Atlas  ","problem_id":" A ","solved_at":"2026-08-19T09:30:00Z"}`),
	}
	before := req
	beforePayload := string(req.Payload)

	if err := ValidateCreateRequest(req); err != nil {
		t.Fatalf("ValidateCreateRequest() error = %v", err)
	}
	if req.Type != before.Type || req.PrinterName != before.PrinterName || string(req.Payload) != beforePayload {
		t.Fatalf("ValidateCreateRequest() modified request: got %#v, want %#v", req, before)
	}
}
