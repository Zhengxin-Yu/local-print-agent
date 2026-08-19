package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
)

const jobsPath = "/api/v1/print-jobs"

// JobService is the HTTP boundary required to manage print jobs.
type JobService interface {
	Create(context.Context, jobs.CreateJobRequest) (*jobs.Job, error)
	Get(context.Context, string) (*jobs.Job, error)
	List(context.Context) ([]*jobs.Job, error)
	Retry(context.Context, string) (*jobs.Job, error)
}

// Dependencies holds collaborators used by HTTP handlers.
type Dependencies struct {
	Jobs     JobService
	Printers printer.Adapter
	Web      fs.FS
}

type router struct{ dependencies Dependencies }

// NewRouter creates the local API router. Health deliberately remains a bare
// compatibility response; all versioned API responses use an envelope.
func NewRouter(dependencies Dependencies) http.Handler {
	return router{dependencies: dependencies}
}

func (r router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/health" {
		if request.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		healthHandler(w, request)
		return
	}

	if request.URL.Path == "/api/v1/printers" {
		if request.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		r.listPrinters(w, request)
		return
	}

	if request.URL.Path == jobsPath {
		switch request.Method {
		case http.MethodGet:
			r.listJobs(w, request)
		case http.MethodPost:
			r.createJob(w, request)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}

	jobID, action, ok := jobRoute(request.URL)
	if !ok {
		notFound(w)
		return
	}
	switch action {
	case "":
		if request.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		r.getJob(w, request, jobID)
	case "preview":
		if request.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		previewNotImplemented(w)
	case "retry":
		if request.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		r.retryJob(w, request, jobID)
	default:
		notFound(w)
	}
}

// jobRoute accepts exactly one non-empty decoded job ID and an optional known
// action. Keeping parsing here avoids treating IDs as filesystem paths.
func jobRoute(u *url.URL) (jobID, action string, ok bool) {
	if u == nil || !strings.HasPrefix(u.Path, jobsPath+"/") {
		return "", "", false
	}
	decoded := strings.Split(strings.TrimPrefix(u.Path, jobsPath+"/"), "/")
	rawPath := u.EscapedPath()
	if u.RawPath != "" {
		rawPath = u.RawPath
	}
	raw := strings.Split(strings.TrimPrefix(rawPath, jobsPath+"/"), "/")
	if len(decoded) < 1 || len(decoded) > 2 || len(raw) != len(decoded) || decoded[0] == "" {
		return "", "", false
	}
	decodedID, err := url.PathUnescape(raw[0])
	if err != nil || decodedID == "" || decodedID != decoded[0] || strings.Contains(decodedID, "/") {
		return "", "", false
	}
	if len(decoded) == 2 && decoded[1] == "" {
		return "", "", false
	}
	return decodedID, strings.Join(decoded[1:], ""), true
}
