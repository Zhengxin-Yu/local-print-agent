// Package worker processes queued job IDs in FIFO order on one goroutine.
package worker

import (
	"context"
	"errors"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/render"
	"local-print-agent/internal/store"
)

const retryDelay = time.Millisecond

type Store interface {
	Get(context.Context, string) (*jobs.Job, error)
	Update(context.Context, *jobs.Job) error
}

// Worker processes one ID through all durable updates before receiving another.
type Worker struct {
	store    Store
	renderer render.Renderer
	printer  printer.Adapter
	queue    <-chan string
	errors   chan error
}

func New(store Store, renderer render.Renderer, printerAdapter printer.Adapter, queue <-chan string) *Worker {
	return &Worker{store: store, renderer: renderer, printer: printerAdapter, queue: queue, errors: make(chan error, 32)}
}

// NewPipeline is the production assembly entry point. Its queue is private:
// callers receive no endpoint that can send to or close it.
func NewPipeline(store jobs.JobStore, renderer render.Renderer, printerAdapter printer.Adapter) (*jobs.Service, *Worker) {
	queue := jobs.NewQueue()
	return jobs.NewService(store, queue), New(store, renderer, printerAdapter, queue)
}

// Errors exposes a bounded, non-blocking observation stream for storage and
// lookup failures. Consumers must not infer physical print completion from it.
func (w *Worker) Errors() <-chan error {
	if w == nil {
		return nil
	}
	return w.errors
}

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
			if !open || ctx.Err() != nil {
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
	job := w.get(ctx, id)
	if job == nil || job.Status != jobs.StatusQueued {
		return
	}
	if err := jobs.Transition(job, jobs.StatusRendering, time.Now().UTC()); err != nil {
		w.publish(err)
		return
	}
	if !w.persist(ctx, job) || ctx.Err() != nil {
		return
	}
	if w.renderer == nil {
		w.fail(ctx, job, jobs.ErrorCodeRenderFailed, errors.New("renderer is required"))
		return
	}
	pdfPath, err := w.renderer.Render(ctx, job)
	if err != nil {
		w.fail(ctx, job, jobs.ErrorCodeRenderFailed, err)
		return
	}
	job.PDFPath = pdfPath
	if err := jobs.Transition(job, jobs.StatusPrinting, time.Now().UTC()); err != nil {
		w.publish(err)
		return
	}
	if !w.persist(ctx, job) || ctx.Err() != nil {
		return
	}
	if w.printer == nil {
		w.fail(ctx, job, jobs.ErrorCodePrintFailed, errors.New("printer adapter is required"))
		return
	}
	if err := w.printer.Print(ctx, job.PrinterName, job.PDFPath); err != nil {
		w.fail(ctx, job, jobs.ErrorCodePrintFailed, err)
		return
	}
	if err := jobs.Transition(job, jobs.StatusSucceeded, time.Now().UTC()); err != nil {
		w.publish(err)
		return
	}
	w.persist(ctx, job)
}

func (w *Worker) get(ctx context.Context, id string) *jobs.Job {
	for {
		job, err := w.store.Get(ctx, id)
		if err == nil {
			return job
		}
		w.publish(err)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if !retryWait(ctx) {
			return nil
		}
	}
}

func (w *Worker) persist(ctx context.Context, job *jobs.Job) bool {
	for {
		if err := w.store.Update(ctx, job); err == nil {
			return true
		} else {
			w.publish(err)
		}
		if !retryWait(ctx) {
			return false
		}
	}
}

func (w *Worker) fail(ctx context.Context, job *jobs.Job, code jobs.ErrorCode, cause error) {
	if job == nil || cause == nil {
		return
	}
	job.Error = &jobs.JobError{Code: code, Message: cause.Error()}
	if err := jobs.Transition(job, jobs.StatusFailed, time.Now().UTC()); err != nil {
		w.publish(err)
		return
	}
	w.persist(ctx, job)
}

func (w *Worker) publish(err error) {
	if err == nil || w == nil {
		return
	}
	select {
	case w.errors <- err:
	default:
	}
}

func retryWait(ctx context.Context) bool {
	timer := time.NewTimer(retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
