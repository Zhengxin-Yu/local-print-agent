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
		"setInterval(() => refreshJobs(), 2000)",
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
