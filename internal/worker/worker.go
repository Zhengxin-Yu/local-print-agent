// Package worker processes queued job IDs in FIFO order on one goroutine.
package worker

import (
	"context"
	"fmt"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/render"
)

// Store is the narrow persistence boundary required by a worker.
type Store interface {
	Get(context.Context, string) (*jobs.Job, error)
	Update(context.Context, *jobs.Job) error
}

// Worker consumes a shared FIFO queue. It processes one ID fully before
// receiving the next, so no two jobs print concurrently through this worker.
type Worker struct {
	store    Store
	renderer render.Renderer
	printer  printer.Adapter
	queue    <-chan string
}

func New(store Store, renderer render.Renderer, printerAdapter printer.Adapter, queue <-chan string) *Worker {
	return &Worker{store: store, renderer: renderer, printer: printerAdapter, queue: queue}
}

// Run returns when context is canceled or queue is closed. A canceled context
// is checked before consuming an ID, leaving any queued work durable for a
// later worker or restart recovery.
func (w *Worker) Run(ctx context.Context) {
	if ctx == nil || w == nil || w.queue == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case id, open := <-w.queue:
			if !open {
				return
			}
			if ctx.Err() != nil {
				return
			}
			w.process(ctx, id)
		}
	}
}

func (w *Worker) process(ctx context.Context, id string) {
	if w.store == nil {
		return
	}
	job, err := w.store.Get(ctx, id)
	if err != nil || job == nil || job.Status != jobs.StatusQueued {
		return
	}
	if err := jobs.Transition(job, jobs.StatusRendering, time.Now().UTC()); err != nil {
		return
	}
	if err := w.store.Update(ctx, job); err != nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if w.renderer == nil {
		w.fail(ctx, job, jobs.ErrorCodeRenderFailed, fmt.Errorf("renderer is required"))
		return
	}
	pdfPath, err := w.renderer.Render(ctx, job)
	if err != nil {
		w.fail(ctx, job, jobs.ErrorCodeRenderFailed, err)
		return
	}
	job.PDFPath = pdfPath
	if err := jobs.Transition(job, jobs.StatusPrinting, time.Now().UTC()); err != nil {
		return
	}
	if err := w.store.Update(ctx, job); err != nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if w.printer == nil {
		w.fail(ctx, job, jobs.ErrorCodePrintFailed, fmt.Errorf("printer adapter is required"))
		return
	}
	if err := w.printer.Print(ctx, job.PrinterName, job.PDFPath); err != nil {
		w.fail(ctx, job, jobs.ErrorCodePrintFailed, err)
		return
	}
	if err := jobs.Transition(job, jobs.StatusSucceeded, time.Now().UTC()); err != nil {
		return
	}
	_ = w.store.Update(ctx, job)
}

func (w *Worker) fail(ctx context.Context, job *jobs.Job, code jobs.ErrorCode, cause error) {
	if job == nil || cause == nil {
		return
	}
	job.Error = &jobs.JobError{Code: code, Message: cause.Error()}
	if err := jobs.Transition(job, jobs.StatusFailed, time.Now().UTC()); err != nil {
		return
	}
	_ = w.store.Update(ctx, job)
}
