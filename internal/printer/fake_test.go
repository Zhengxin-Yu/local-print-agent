package printer

import (
	"context"
	"errors"
	"testing"
)

// This catches a fake that claims success without preserving the command that
// would have been sent to the operating-system printer adapter.
func TestFakeListsConfiguredPrintersAndRecordsPrintCalls(t *testing.T) {
	fake := NewFake([]Info{{Name: "front-desk", IsDefault: true}})
	listed, err := fake.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Name != "front-desk" || !listed[0].IsDefault {
		t.Fatalf("List() = %#v, %v; want configured printer", listed, err)
	}
	if err := fake.Print(context.Background(), "front-desk", "C:/tmp/job.pdf"); err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	calls := fake.Calls()
	if len(calls) != 1 || calls[0].PrinterName != "front-desk" || calls[0].PDFPath != "C:/tmp/job.pdf" {
		t.Fatalf("Calls() = %#v, want recorded OS-adapter command", calls)
	}

	fake.SetPrintError(errors.New("offline"))
	if err := fake.Print(context.Background(), "front-desk", "C:/tmp/again.pdf"); err == nil {
		t.Fatal("Print() succeeded after injected error")
	}
}
