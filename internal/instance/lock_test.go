package instance

import (
	"errors"
	"testing"
)

func TestAcquirePreventsConcurrentUseAndReleaseAllowsReacquisition(t *testing.T) {
	dataDir := t.TempDir()
	first, err := Acquire(dataDir)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Acquire(dataDir)
	if second != nil {
		_ = second.Close()
		t.Fatal("second acquire returned a lock while the first lock was held")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want ErrAlreadyRunning", err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reacquired, err := Acquire(dataDir)
	if err != nil {
		t.Fatalf("reacquire after close: %v", err)
	}
	if err := reacquired.Close(); err != nil {
		t.Fatal(err)
	}
}
