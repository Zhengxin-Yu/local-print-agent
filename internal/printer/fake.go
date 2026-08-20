package printer

import (
	"context"
	"sync"
)

// PrintCall is a command accepted by Fake for inspection in tests or local UI
// development. It represents no physical print operation.
type PrintCall struct {
	PrinterName string
	PDFPath     string
}

// Fake is a thread-safe, non-printing Adapter. It lists configured printers
// and records commands while allowing a test to inject a command error.
type Fake struct {
	mu       sync.Mutex
	printers []Info
	calls    []PrintCall
	printErr error
}

var _ Adapter = (*Fake)(nil)

func NewFake(printers []Info) *Fake {
	return &Fake{printers: append([]Info(nil), printers...)}
}

func (f *Fake) List(ctx context.Context) ([]Info, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Info(nil), f.printers...), nil
}

func (f *Fake) Print(ctx context.Context, printerName, pdfPath string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, PrintCall{PrinterName: printerName, PDFPath: pdfPath})
	return f.printErr
}

func (f *Fake) SetPrintError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.printErr = err
}

func (f *Fake) Calls() []PrintCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PrintCall(nil), f.calls...)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}
