// Package worker processes queued job IDs in FIFO order on one goroutine.
package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"local-print-agent/internal/jobs"
	"local-print-agent/internal/printer"
	"local-print-agent/internal/render"
	"local-print-agent/internal/store"
)

const (
	initialRetryDelay = 5 * time.Millisecond
	maximumRetryDelay = 250 * time.Millisecond
)

type Store interface {
	Get(context.Context, string) (*jobs.Job, error)
	Update(context.Context, *jobs.Job) error
}

// Worker processes one ID through all durable updates before receiving another.
type Worker struct {
	store     Store
	renderer  render.Renderer
	printer   printer.Adapter
	queue     <-chan string
	errors    chan error
	done      chan struct{}
	runOnce   sync.Once
	waitRetry func(context.Context, time.Duration) bool
	onDone    func()
}

func New(store Store, renderer render.Renderer, printerAdapter printer.Adapter, queue <-chan string) *Worker {
	return &Worker{store: store, renderer: renderer, printer: printerAdapter, queue: queue, errors: make(chan error, 32), done: make(chan struct{}), waitRetry: waitForRetry}
}

// NewPipeline is the production assembly entry point. Its queue is private:
// callers receive no endpoint that can send to or close it.
func NewPipeline(store jobs.JobStore, renderer render.Renderer, printerAdapter printer.Adapter) (*jobs.Service, *Worker) {
	queue := jobs.NewQueue()
	service := jobs.NewService(store, queue)
	jobWorker := New(store, renderer, printerAdapter, queue)
	jobWorker.onDone = service.Close
	return service, jobWorker
}

// Errors exposes a bounded, non-blocking observation stream for storage and
// lookup failures. Consumers must not infer physical print completion from it.
func (w *Worker) Errors() <-chan error {
	if w == nil {
		return nil
	}
	return w.errors
}

// Done closes after Run has returned, including cleanup performed by an active renderer.
func (w *Worker) Done() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.done
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	w.runOnce.Do(func() {
		defer func() {
			if w.onDone != nil {
				w.onDone()
			}
			close(w.errors)
			close(w.done)
		}()
		w.run(ctx)
	})
}

func (w *Worker) run(ctx context.Context) {
	if ctx == nil || w.queue == nil {
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
		err := errors.New("renderer is required")
		w.publish(err)
		w.fail(ctx, job, jobs.ErrorCodeRenderFailed, "PDF rendering failed")
		return
	}
	pdfPath, err := w.renderer.Render(ctx, job)
	if err != nil {
		code := jobs.ErrorCodeRenderFailed
		message := "PDF rendering failed"
		var jobError *jobs.JobError
		if errors.As(err, &jobError) {
			code = jobError.Code
			message = jobError.Message
		}
		w.publish(err)
		w.fail(ctx, job, code, message)
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
		err := errors.New("printer adapter is required")
		w.publish(err)
		w.fail(ctx, job, jobs.ErrorCodePrintFailed, "Printing failed")
		return
	}
	if err := w.printer.Print(ctx, job.PrinterName, job.PDFPath); err != nil {
		code := jobs.ErrorCodePrintFailed
		message := "Printing failed"
		var jobError *jobs.JobError
		if errors.As(err, &jobError) {
			code = jobError.Code
			message = jobError.Message
		}
		w.publish(err)
		w.fail(ctx, job, code, message)
		return
	}
	if err := jobs.Transition(job, jobs.StatusSucceeded, time.Now().UTC()); err != nil {
		w.publish(err)
		return
	}
	w.persist(ctx, job)
}

func (w *Worker) get(ctx context.Context, id string) *jobs.Job {
	attempt := 0
	for {
		job, err := w.store.Get(ctx, id)
		if err == nil {
			return job
		}
		w.publish(err)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if !w.waitRetry(ctx, retryDelayForAttempt(attempt)) {
			return nil
		}
		attempt++
	}
}

func (w *Worker) persist(ctx context.Context, job *jobs.Job) bool {
	attempt := 0
	for {
		if err := w.store.Update(ctx, job); err == nil {
			return true
		} else {
			w.publish(err)
		}
		if !w.waitRetry(ctx, retryDelayForAttempt(attempt)) {
			return false
		}
		attempt++
	}
}

func (w *Worker) fail(ctx context.Context, job *jobs.Job, code jobs.ErrorCode, message string) {
	if job == nil || message == "" {
		return
	}
	job.Error = &jobs.JobError{Code: code, Message: message}
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

func retryDelayForAttempt(attempt int) time.Duration {
	delay := initialRetryDelay
	for index := 0; index < attempt && delay < maximumRetryDelay; index++ {
		delay *= 2
		if delay > maximumRetryDelay {
			delay = maximumRetryDelay
		}
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
