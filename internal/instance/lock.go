// Package instance prevents multiple service processes from sharing one data directory.
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrAlreadyRunning = errors.New("another local-print-agent instance is using this data directory")

// Lock holds exclusive ownership of one data directory until Close returns.
type Lock struct {
	file     *os.File
	closeErr error
	once     sync.Once
}

// Acquire takes a non-blocking advisory lock for dataDir.
func Acquire(dataDir string) (*Lock, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("prepare data directory for instance lock: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dataDir, ".local-print-agent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrAlreadyRunning) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}
	return &Lock{file: file}, nil
}

// Close releases the advisory lock. It is safe to call more than once.
func (lock *Lock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		if lock.file == nil {
			return
		}
		unlockErr := unlockFile(lock.file)
		closeErr := lock.file.Close()
		lock.closeErr = errors.Join(unlockErr, closeErr)
	})
	return lock.closeErr
}
