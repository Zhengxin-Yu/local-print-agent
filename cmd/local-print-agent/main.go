package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"local-print-agent/internal/config"
	"local-print-agent/internal/httpapi"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/render"
	"local-print-agent/internal/server"
	"local-print-agent/internal/store"
	"local-print-agent/internal/worker"
	"local-print-agent/web"
)

const fakePrinterName = "Mock Printer（不执行实体打印）"

type application struct {
	Handler http.Handler
	worker  *worker.Worker
}

type runningServer struct {
	URL  string
	Done <-chan error
}

func buildApplication(cfg config.Config) (*application, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	renderer, err := render.NewPDFRenderer(cfg.DataDir, "")
	if err != nil {
		return nil, err
	}
	return buildApplicationWithRenderer(cfg, renderer)
}

func buildApplicationWithRenderer(cfg config.Config, renderer render.Renderer) (*application, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if renderer == nil {
		return nil, errors.New("renderer is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	jobStore, err := store.NewJSONStore(filepath.Join(cfg.DataDir, "jobs.json"))
	if err != nil {
		return nil, err
	}
	if err := jobStore.RecoverInterrupted(context.Background()); err != nil {
		return nil, fmt.Errorf("recover interrupted jobs: %w", err)
	}
	printers := printer.NewFake([]printer.Info{{Name: fakePrinterName, IsDefault: true}})
	service, jobWorker := worker.NewPipeline(jobStore, renderer, printers)
	if _, err := service.ResumeQueued(context.Background()); err != nil {
		return nil, fmt.Errorf("restore queued jobs: %w", err)
	}
	return &application{Handler: httpapi.NewRouter(httpapi.Dependencies{Jobs: service, Printers: printers, Web: web.Assets, PreviewRoot: filepath.Join(cfg.DataDir, "jobs")}), worker: jobWorker}, nil
}

func start(ctx context.Context, cfg config.Config) (*runningServer, error) {
	return startWithBuilder(ctx, cfg, buildApplication)
}

func startWithBuilder(ctx context.Context, cfg config.Config, builder func(config.Config) (*application, error)) (*runningServer, error) {
	if ctx == nil {
		return nil, errors.New("service context is required")
	}
	if builder == nil {
		return nil, errors.New("application builder is required")
	}
	application, err := builder(cfg)
	if err != nil {
		return nil, err
	}
	listener, port, err := server.ListenFirstAvailable(cfg.Host, cfg.CandidatePorts())
	if err != nil {
		return nil, err
	}
	httpServer := &http.Server{Handler: application.Handler, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go application.worker.Run(ctx)
	go consumeWorkerErrors(ctx, application.worker.Errors())
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			log.Printf("HTTP shutdown failed")
		}
	}()
	return &runningServer{URL: fmt.Sprintf("http://%s:%d", cfg.Host, port), Done: done}, nil
}

func consumeWorkerErrors(ctx context.Context, errors <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-errors:
			if !ok {
				return
			}
			// Deliberately omit error text: it could contain user source code.
			log.Printf("worker observed an internal error")
		}
	}
}

func main() {
	cfg := config.Default()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	running, err := start(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("local-print-agent listening on %s", running.URL)
	if err := <-running.Done; err != nil {
		log.Fatal(err)
	}
}
