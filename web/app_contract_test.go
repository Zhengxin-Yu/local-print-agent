package web

import (
	"io/fs"
	"strings"
	"testing"
)

// This static contract guards security- and deployment-critical browser
// behavior which would otherwise be hard to cover in the Go HTTP tests.
func TestAppJavaScriptHasSafeLocalServiceContract(t *testing.T) {
	contents, err := fs.ReadFile(Assets, "app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(contents)
	for _, required := range []string{
		"window.location.protocol !== \"file:\"",
		"port <= 17660",
		"AbortController",
		"window.setInterval",
		"}, 2000)",
		"window.clearInterval",
		"textContent",
		"replaceChildren",
		"encodeURIComponent",
		"job.status === \"failed\"",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("app.js lacks required local-console behavior %q", required)
		}
	}
	if strings.Contains(source, "innerHTML") {
		t.Fatal("app.js must not inject untrusted API values with innerHTML")
	}
}

// This prevents background polling from erasing a visible error that belongs
// to a preview, create, retry, or connection operation.
func TestAppJavaScriptKeepsErrorsScopedToTheirOperation(t *testing.T) {
	contents, err := fs.ReadFile(Assets, "app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(contents)
	for _, required := range []string{
		"const errorState = { source: \"\", message: \"\" }",
		"function showError(source, message)",
		"function clearError(source)",
		"showError(\"preview\"",
		"showError(\"refresh\"",
		"clearError(\"refresh\")",
		"throw error;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("app.js lacks scoped error behavior %q", required)
		}
	}
	start := strings.Index(source, "async function refreshJobs")
	end := strings.Index(source[start:], "async function loadDetail")
	if start < 0 || end < 0 {
		t.Fatal("cannot locate refreshJobs function")
	}
	refresh := source[start : start+end]
	if strings.Contains(refresh, "clearError()") {
		t.Fatal("refreshJobs clears every operation error instead of only refresh errors")
	}
}
