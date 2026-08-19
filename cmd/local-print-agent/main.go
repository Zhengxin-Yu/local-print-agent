package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"local-print-agent/internal/instance"
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
	URL             string
	FileOriginToken string
	Done            <-chan error
}

type platformPrinterFactory func(printer.PlatformConfig) (printer.Adapter, error)

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
	return buildApplicationWithRendererAndPrinterFactory(cfg, renderer, printer.NewPlatformAdapter)
}

func buildApplicationWithRendererAndPrinterFactory(cfg config.Config, renderer render.Renderer, platformFactory platformPrinterFactory) (*application, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if renderer == nil {
		return nil, errors.New("renderer is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	printers, err := configuredPrinter(cfg, platformFactory)
	if err != nil {
		return nil, err
	}
	jobStore, err := store.NewJSONStore(filepath.Join(cfg.DataDir, "jobs.json"))
	if err != nil {
		return nil, err
	}
	if err := jobStore.RecoverInterrupted(context.Background()); err != nil {
		return nil, fmt.Errorf("recover interrupted jobs: %w", err)
	}
	service, jobWorker := worker.NewPipeline(jobStore, renderer, printers)
	if _, err := service.ResumeQueued(context.Background()); err != nil {
		return nil, fmt.Errorf("restore queued jobs: %w", err)
	}
	return &application{Handler: httpapi.NewRouter(httpapi.Dependencies{Jobs: service, Printers: printers, Web: web.Assets, PreviewRoot: filepath.Join(cfg.DataDir, "jobs"), FileOriginToken: cfg.FileOriginToken}), worker: jobWorker}, nil
}

func configuredPrinter(cfg config.Config, platformFactory platformPrinterFactory) (printer.Adapter, error) {
	switch cfg.PrinterMode {
	case "", config.PrinterModeDemo:
		return printer.NewFake([]printer.Info{{Name: fakePrinterName, IsDefault: true}}), nil
	case config.PrinterModePlatform:
		if platformFactory == nil {
			return nil, errors.New("platform printer adapter factory is required")
		}
		adapter, err := platformFactory(printer.PlatformConfig{
			DataDir:        cfg.DataDir,
			SumatraPDFPath: cfg.SumatraPDFPath,
		})
		if err != nil {
			return nil, err
		}
		if adapter == nil {
			return nil, errors.New("platform printer adapter is required")
		}
		return adapter, nil
	default:
		return nil, fmt.Errorf("unsupported printer mode %q: use demo or platform", cfg.PrinterMode)
	}
}

func start(ctx context.Context, cfg config.Config) (*runningServer, error) {
	return startWithBuilder(ctx, cfg, buildApplication)
}

func startWithBuilder(ctx context.Context, cfg config.Config, builder func(config.Config) (*application, error)) (running *runningServer, err error) {
	if ctx == nil {
		return nil, errors.New("service context is required")
	}
	if builder == nil {
		return nil, errors.New("application builder is required")
	}
	instanceLock, err := instance.Acquire(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	startupComplete := false
	defer func() {
		if startupComplete {
			return
		}
		if closeErr := instanceLock.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("release instance lock: %w", closeErr))
		}
	}()
	fileOriginToken, err := newFileOriginToken()
	if err != nil {
		return nil, err
	}
	cfg.FileOriginToken = fileOriginToken
	application, err := builder(cfg)
	if err != nil {
		return nil, err
	}
	listener, port, err := server.ListenFirstAvailable(cfg.Host, cfg.CandidatePorts())
	if err != nil {
		return nil, err
	}
	httpServer := &http.Server{Handler: application.Handler, ReadHeaderTimeout: 5 * time.Second}
	serviceContext, cancelService := context.WithCancel(ctx)
	httpServeDone := make(chan error, 1)
	httpShutdownDone := make(chan error, 1)
	done := make(chan error, 1)
	go application.worker.Run(serviceContext)
	go consumeWorkerErrors(serviceContext, application.worker.Errors())
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		cancelService()
		httpServeDone <- err
	}()
	go func() {
		<-serviceContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := httpServer.Shutdown(shutdownContext)
		if err != nil {
			log.Printf("HTTP shutdown failed")
		}
		httpShutdownDone <- err
	}()
	go func() {
		err := <-httpServeDone
		cancelService()
		if shutdownErr := <-httpShutdownDone; shutdownErr != nil {
			err = errors.Join(err, fmt.Errorf("HTTP shutdown failed: %w", shutdownErr))
		}
		<-application.worker.Done()
		if closeErr := instanceLock.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("release instance lock: %w", closeErr))
		}
		done <- err
	}()
	startupComplete = true
	return &runningServer{URL: fmt.Sprintf("http://%s:%d", cfg.Host, port), FileOriginToken: fileOriginToken, Done: done}, nil
}

func newFileOriginToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate file-origin capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
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
	log.Printf("optional file console: web/index.html?local_print_agent_token=%s", running.FileOriginToken)
	if err := <-running.Done; err != nil {
		log.Fatal(err)
	}
}
