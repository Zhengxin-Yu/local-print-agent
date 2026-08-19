package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWebAssetsServeRootAndNeverOverrideAPI(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":  {Data: []byte("<!doctype html><title>控制台</title>")},
		"app.js":      {Data: []byte("console.log('app')")},
		"styles.css":  {Data: []byte("body { color: black; }")},
		"secret.txt":  {Data: []byte("not public")},
		"folder/file": {Data: []byte("not public")},
	}
	router := NewRouter(Dependencies{Web: assets})

	for _, want := range []struct {
		path        string
		status      int
		contentType string
	}{
		{"/", http.StatusOK, "text/html"},
		{"/app.js", http.StatusOK, "text/javascript"},
		{"/styles.css", http.StatusOK, "text/css"},
		{"/secret.txt", http.StatusNotFound, "application/json"},
		{"/folder/", http.StatusNotFound, "application/json"},
		{"/../index.html", http.StatusNotFound, "application/json"},
		{"/api/v1/print-jobs", http.StatusServiceUnavailable, "application/json"},
	} {
		t.Run(want.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, want.path, nil))
			if rec.Code != want.status {
				t.Fatalf("status=%d, want %d: %s", rec.Code, want.status, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, want.contentType) {
				t.Fatalf("Content-Type=%q, want prefix %q", got, want.contentType)
			}
		})
	}
}

func TestFileOriginCORSRequiresLaunchCapability(t *testing.T) {
	const token = "unguessable-launch-capability"
	router := NewRouter(Dependencies{FileOriginToken: token})
	for _, test := range []struct {
		name       string
		method     string
		origin     string
		path       string
		wantStatus int
		wantCORS   string
	}{
		{"correct capability GET", http.MethodGet, "null", "/health?local_print_agent_token=" + token, http.StatusOK, "null"},
		{"missing capability GET", http.MethodGet, "null", "/health", http.StatusOK, ""},
		{"wrong capability GET", http.MethodGet, "null", "/health?local_print_agent_token=wrong", http.StatusOK, ""},
		{"correct capability preflight", http.MethodOptions, "null", "/api/v1/print-jobs?local_print_agent_token=" + token, http.StatusNoContent, "null"},
		{"missing capability preflight", http.MethodOptions, "null", "/api/v1/print-jobs", http.StatusMethodNotAllowed, ""},
		{"wrong capability preflight", http.MethodOptions, "null", "/api/v1/print-jobs?local_print_agent_token=wrong", http.StatusMethodNotAllowed, ""},
		{"web origin GET", http.MethodGet, "https://example.test", "/health?local_print_agent_token=" + token, http.StatusOK, ""},
		{"null origin static page is not an API CORS response", http.MethodGet, "null", "/?local_print_agent_token=" + token, http.StatusNotFound, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			req.Header.Set("Origin", test.origin)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("status=%d, want %d", rec.Code, test.wantStatus)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != test.wantCORS {
				t.Fatalf("Access-Control-Allow-Origin=%q, want %q", got, test.wantCORS)
			}
		})
	}
}

func TestEmptyFileOriginCapabilityNeverEnablesNullOriginCORS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health?local_print_agent_token=anything", nil)
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q with no configured capability", got)
	}
}
