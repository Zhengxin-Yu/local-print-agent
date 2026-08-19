package main

import (
	"log"
	"net/http"

	"local-print-agent/internal/config"
	"local-print-agent/internal/httpapi"
	"local-print-agent/internal/server"
)

func main() {
	cfg := config.Default()
	listener, port, err := server.ListenFirstAvailable("127.0.0.1", cfg.CandidatePorts())
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("local-print-agent listening on http://127.0.0.1:%d", port)
	log.Fatal(http.Serve(listener, httpapi.NewRouter(httpapi.Dependencies{})))
}
