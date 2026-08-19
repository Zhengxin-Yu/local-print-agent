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

func TestFileOriginCORSIsLimitedToNullOrigin(t *testing.T) {
	router := NewRouter(Dependencies{})
	for _, test := range []struct {
		name       string
		method     string
		origin     string
		wantStatus int
		wantCORS   string
	}{
		{"null origin GET", http.MethodGet, "null", http.StatusOK, "null"},
		{"null origin preflight", http.MethodOptions, "null", http.StatusNoContent, "null"},
		{"web origin GET", http.MethodGet, "https://example.test", http.StatusOK, ""},
		{"null origin static page is not an API CORS response", http.MethodGet, "null", http.StatusNotFound, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := "/health"
			if strings.Contains(test.name, "static page") {
				path = "/"
			}
			req := httptest.NewRequest(test.method, path, nil)
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
