package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/store"
)

const maxCreateBodyBytes int64 = 1 << 20

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiEnvelope struct {
	Data  any       `json:"data"`
	Error *apiError `json:"error"`
}

func (r router) createJob(w http.ResponseWriter, request *http.Request) {
	if r.dependencies.Jobs == nil {
		dependencyUnavailable(w, "job service is unavailable")
		return
	}
	if !isJSONContentType(request.Header.Get("Content-Type")) {
		writeAPIError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, maxCreateBodyBytes)
	defer request.Body.Close()
	var input *jobs.CreateJobRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		if bodyTooLarge(err) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "request body exceeds 1 MiB")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must be valid JSON")
		return
	}
	if input == nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must be a JSON object")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if bodyTooLarge(err) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "request body exceeds 1 MiB")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one JSON value")
		return
	}
	job, err := r.dependencies.Jobs.Create(request.Context(), *input)
	if err != nil {
		writeJobError(w, err)
		return
	}
	if job == nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeAPISuccess(w, http.StatusAccepted, job)
}

func (r router) listJobs(w http.ResponseWriter, request *http.Request) {
	if r.dependencies.Jobs == nil {
		dependencyUnavailable(w, "job service is unavailable")
		return
	}
	list, err := r.dependencies.Jobs.List(request.Context())
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeAPISuccess(w, http.StatusOK, list)
}

func (r router) getJob(w http.ResponseWriter, request *http.Request, id string) {
	if r.dependencies.Jobs == nil {
		dependencyUnavailable(w, "job service is unavailable")
		return
	}
	job, err := r.dependencies.Jobs.Get(request.Context(), id)
	if err != nil {
		writeJobError(w, err)
		return
	}
	if job == nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeAPISuccess(w, http.StatusOK, job)
}

func (r router) retryJob(w http.ResponseWriter, request *http.Request, id string) {
	if r.dependencies.Jobs == nil {
		dependencyUnavailable(w, "job service is unavailable")
		return
	}
	job, err := r.dependencies.Jobs.Retry(request.Context(), id)
	if err != nil {
		writeJobError(w, err)
		return
	}
	if job == nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeAPISuccess(w, http.StatusOK, job)
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func bodyTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}

func writeJobError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		notFound(w)
		return
	}
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	switch jobError.Code {
	case jobs.ErrorCodeInvalidRequest:
		writeAPIError(w, http.StatusBadRequest, string(jobError.Code), jobError.Message)
	case jobs.ErrorCodeRetryNotAllowed:
		writeAPIError(w, http.StatusConflict, string(jobError.Code), "only failed jobs can be retried")
	case jobs.ErrorCodeQueueFull:
		writeAPIError(w, http.StatusServiceUnavailable, string(jobError.Code), "print queue is full")
	case jobs.ErrorCodeQueueDeliveryFailed:
		writeAPIError(w, http.StatusServiceUnavailable, string(jobError.Code), "print queue is unavailable")
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func writeAPISuccess(w http.ResponseWriter, status int, data any) {
	writeAPI(w, status, apiEnvelope{Data: data, Error: nil})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeAPI(w, status, apiEnvelope{Data: nil, Error: &apiError{Code: code, Message: message}})
}

func writeAPI(w http.ResponseWriter, status int, response apiEnvelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func notFound(w http.ResponseWriter) {
	writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
}

func dependencyUnavailable(w http.ResponseWriter, message string) {
	writeAPIError(w, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", message)
}

func previewNotImplemented(w http.ResponseWriter) {
	writeAPIError(w, http.StatusNotImplemented, "PREVIEW_NOT_IMPLEMENTED", "preview is not implemented")
}
