package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/store"
)

func TestHealthReturnsServiceStatusJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter(Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(rec.Body.String(), `"service":"local-print-agent"`) {
		t.Fatal(rec.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["api_version"] != "v1" || response["status"] != "ok" {
		t.Fatalf("response = %v, want api_version=v1 and status=ok", response)
	}
}

func TestAPICreateJobReturnsAcceptedEnvelope(t *testing.T) {
	svc := &stubJobService{createJob: testJob("created", jobs.StatusQueued)}
	response := serveAPI(NewRouter(Dependencies{Jobs: svc}), http.MethodPost, "/api/v1/print-jobs", validCreateBody(), "application/json")

	assertEnvelope(t, response, http.StatusAccepted, "")
	if !strings.Contains(response.Body.String(), `"id":"created"`) {
		t.Fatalf("response body = %s, want created job", response.Body.String())
	}
}

func TestAPICreateJobRejectsMalformedBodies(t *testing.T) {
	tooLarge := "{\"type\":\"balloon_ticket\",\"printer_name\":\"front-desk\",\"payload\":{\"team_name\":\"" + strings.Repeat("x", 1024*1024) + "\"}}"
	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{name: "malformed JSON", body: `{`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "INVALID_REQUEST"},
		{name: "unknown field", body: `{"type":"balloon_ticket","printer_name":"front-desk","payload":{},"extra":true}`, contentType: "application/json; charset=utf-8", wantStatus: http.StatusBadRequest, wantCode: "INVALID_REQUEST"},
		{name: "trailing JSON", body: validCreateBody() + `{}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "INVALID_REQUEST"},
		{name: "null body", body: `null`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "INVALID_REQUEST"},
		{name: "wrong content type", body: validCreateBody(), contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "body too large", body: tooLarge, contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "REQUEST_BODY_TOO_LARGE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveAPI(NewRouter(Dependencies{Jobs: &stubJobService{}}), http.MethodPost, "/api/v1/print-jobs", test.body, test.contentType)
			assertEnvelope(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func TestAPIJobListAndDetail(t *testing.T) {
	svc := &stubJobService{listJobs: []*jobs.Job{testJob("first", jobs.StatusQueued)}, getJob: testJob("one", jobs.StatusSucceeded)}
	router := NewRouter(Dependencies{Jobs: svc})

	list := serveAPI(router, http.MethodGet, "/api/v1/print-jobs", "", "")
	assertEnvelope(t, list, http.StatusOK, "")
	if !strings.Contains(list.Body.String(), `"id":"first"`) {
		t.Fatalf("list = %s, want first job", list.Body.String())
	}

	detail := serveAPI(router, http.MethodGet, "/api/v1/print-jobs/one", "", "")
	assertEnvelope(t, detail, http.StatusOK, "")
	if !strings.Contains(detail.Body.String(), `"id":"one"`) {
		t.Fatalf("detail = %s, want one job", detail.Body.String())
	}
}

func TestAPIJobNotFoundAndSafeDynamicPaths(t *testing.T) {
	svc := &stubJobService{getErr: store.ErrNotFound}
	router := NewRouter(Dependencies{Jobs: svc})

	for _, path := range []string{"/api/v1/print-jobs/missing", "/api/v1/print-jobs/", "/api/v1/print-jobs/one/extra", "/api/v1/print-jobs/%2F"} {
		t.Run(path, func(t *testing.T) {
			response := serveAPI(router, http.MethodGet, path, "", "")
			assertEnvelope(t, response, http.StatusNotFound, "NOT_FOUND")
		})
	}
}

func TestJobRouteRejectsMalformedEscapedID(t *testing.T) {
	_, _, ok := jobRoute(&url.URL{Path: jobsPath + "/id", RawPath: jobsPath + "/%zz"})
	if ok {
		t.Fatal("jobRoute accepted a malformed escaped ID")
	}
}

func TestAPIRetryMapsConflictAndNotFound(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "non failed", err: &jobs.JobError{Code: jobs.ErrorCodeRetryNotAllowed, Message: "only failed jobs can be retried"}, wantStatus: http.StatusConflict, wantCode: string(jobs.ErrorCodeRetryNotAllowed)},
		{name: "missing", err: fmt.Errorf("load: %w", store.ErrNotFound), wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveAPI(NewRouter(Dependencies{Jobs: &stubJobService{retryErr: test.err}}), http.MethodPost, "/api/v1/print-jobs/a/retry", "", "")
			assertEnvelope(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func TestAPIListsPrintersAndMapsAdapterFailures(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		response := serveAPI(NewRouter(Dependencies{Printers: stubPrinter{items: []printer.Info{{Name: "front-desk", IsDefault: true}}}}), http.MethodGet, "/api/v1/printers", "", "")
		assertEnvelope(t, response, http.StatusOK, "")
		if !strings.Contains(response.Body.String(), `"name":"front-desk"`) {
			t.Fatalf("response = %s, want printer", response.Body.String())
		}
	})
	t.Run("adapter failure", func(t *testing.T) {
		response := serveAPI(NewRouter(Dependencies{Printers: stubPrinter{err: errors.New("OS path: unavailable")}}), http.MethodGet, "/api/v1/printers", "", "")
		assertEnvelope(t, response, http.StatusInternalServerError, "PRINTER_LIST_FAILED")
		if strings.Contains(response.Body.String(), "OS path") {
			t.Fatalf("response leaks internal details: %s", response.Body.String())
		}
	})
}

func TestAPIPreviewIsRegisteredButNotImplemented(t *testing.T) {
	response := serveAPI(NewRouter(Dependencies{}), http.MethodGet, "/api/v1/print-jobs/a/preview", "", "")
	assertEnvelope(t, response, http.StatusNotImplemented, "PREVIEW_NOT_IMPLEMENTED")
}

func TestAPIMethodNotAllowedHasEnvelopeAndAllowHeader(t *testing.T) {
	tests := []struct {
		method string
		path   string
		allow  string
	}{
		{method: http.MethodDelete, path: "/api/v1/printers", allow: http.MethodGet},
		{method: http.MethodPut, path: "/api/v1/print-jobs", allow: "GET, POST"},
		{method: http.MethodGet, path: "/api/v1/print-jobs/a/retry", allow: http.MethodPost},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := serveAPI(NewRouter(Dependencies{}), test.method, test.path, "", "")
			assertEnvelope(t, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			if got := response.Header().Get("Allow"); got != test.allow {
				t.Fatalf("Allow = %q, want %q", got, test.allow)
			}
		})
	}
}

func TestAPINilDependenciesAreDiagnosable(t *testing.T) {
	for _, path := range []string{"/api/v1/print-jobs", "/api/v1/printers"} {
		t.Run(path, func(t *testing.T) {
			response := serveAPI(NewRouter(Dependencies{}), http.MethodGet, path, "", "")
			assertEnvelope(t, response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
		})
	}
}

func TestAPIInternalErrorsDoNotLeakDetails(t *testing.T) {
	response := serveAPI(NewRouter(Dependencies{Jobs: &stubJobService{listErr: errors.New(`open C:\\secret\\jobs.json: denied`)}}), http.MethodGet, "/api/v1/print-jobs", "", "")
	assertEnvelope(t, response, http.StatusInternalServerError, "INTERNAL_ERROR")
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response leaks internal details: %s", response.Body.String())
	}
}

type stubJobService struct {
	createJob *jobs.Job
	createErr error
	getJob    *jobs.Job
	getErr    error
	listJobs  []*jobs.Job
	listErr   error
	retryJob  *jobs.Job
	retryErr  error
}

func (s *stubJobService) Create(context.Context, jobs.CreateJobRequest) (*jobs.Job, error) {
	return s.createJob, s.createErr
}

func (s *stubJobService) Get(context.Context, string) (*jobs.Job, error) {
	return s.getJob, s.getErr
}

func (s *stubJobService) List(context.Context) ([]*jobs.Job, error) {
	return s.listJobs, s.listErr
}

func (s *stubJobService) Retry(context.Context, string) (*jobs.Job, error) {
	return s.retryJob, s.retryErr
}

type stubPrinter struct {
	items []printer.Info
	err   error
}

func (p stubPrinter) List(context.Context) ([]printer.Info, error) { return p.items, p.err }
func (p stubPrinter) Print(context.Context, string, string) error  { return nil }

func testJob(id string, status jobs.Status) *jobs.Job {
	return &jobs.Job{ID: id, Type: jobs.JobTypeBalloon, PrinterName: "front-desk", Payload: json.RawMessage(`{"team_name":"Team Atlas","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}`), Status: status, CreatedAt: time.Date(2026, 8, 19, 9, 30, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 19, 9, 30, 0, 0, time.UTC)}
}

func validCreateBody() string {
	return `{"type":"balloon_ticket","printer_name":"front-desk","payload":{"team_name":"Team Atlas","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"}}`
}

func serveAPI(handler http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, rec.Body.String())
	}
	if wantCode == "" {
		if envelope.Error != nil || string(envelope.Data) == "null" || len(envelope.Data) == 0 {
			t.Fatalf("success envelope = %s, want data and error:null", rec.Body.String())
		}
		return
	}
	if string(envelope.Data) != "null" || envelope.Error == nil || envelope.Error.Code != wantCode || envelope.Error.Message == "" {
		t.Fatalf("error envelope = %s, want data:null and code=%s", rec.Body.String(), wantCode)
	}
}
