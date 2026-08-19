package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"local-print-agent/internal/config"
)

func TestStartServesHealthAndShutsDownWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	running, err := start(ctx, config.Config{Host: "127.0.0.1", FirstPort: 0, LastPort: 0, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: time.Second}).Get(running.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) == "" {
		t.Fatalf("health = %d %s", response.StatusCode, body)
	}
	cancel()
	select {
	case err := <-running.Done:
		if err != nil {
			t.Fatalf("server shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

func TestBuildApplicationProvidesAVisibleFakePrinter(t *testing.T) {
	application, err := buildApplication(config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "/api/v1/printers", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Mock Printer（不执行实体打印）") {
		t.Fatalf("printers = %d %s", response.Code, response.Body.String())
	}
}
