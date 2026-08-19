package httpapi

import "net/http"

// Dependencies holds collaborators used by HTTP handlers.
type Dependencies struct{}

func NewRouter(_ Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	return mux
}
