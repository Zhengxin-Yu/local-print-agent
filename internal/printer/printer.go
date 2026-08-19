// Package printer defines the operating-system printer boundary.
package printer

import "context"

// Info describes one printer visible to the local operating system.
type Info struct {
	Name      string
	IsDefault bool
}

// Adapter submits an already-rendered PDF to an operating-system print queue.
// A nil error means the OS accepted the command; it never claims a physical
// page has left the printer.
type Adapter interface {
	List(context.Context) ([]Info, error)
	Print(context.Context, string, string) error
}
