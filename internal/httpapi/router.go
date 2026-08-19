package httpapi

import (
	"context"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
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
	if corsForFileOrigin(w, request) {
		return
	}
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
	if ok {
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
		return
	}
	r.serveWeb(w, request)
}

func corsForFileOrigin(w http.ResponseWriter, request *http.Request) bool {
	if request.Header.Get("Origin") != "null" || (request.URL.Path != "/health" && !strings.HasPrefix(request.URL.Path, "/api/v1/")) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", "null")
	w.Header().Set("Vary", "Origin")
	if request.Method != http.MethodOptions {
		return false
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (r router) serveWeb(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	if r.dependencies.Web == nil {
		notFound(w)
		return
	}
	name := ""
	switch request.URL.Path {
	case "/":
		name = "index.html"
	case "/app.js":
		name = "app.js"
	case "/styles.css":
		name = "styles.css"
	default:
		notFound(w)
		return
	}
	if path.Base(name) != name {
		notFound(w)
		return
	}
	file, err := r.dependencies.Web.Open(name)
	if err != nil {
		notFound(w)
		return
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		notFound(w)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if request.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(contents)
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
