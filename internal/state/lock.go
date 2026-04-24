package state

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// fileLock is an exclusive OS-level lock on a sidecar file. Held for the
// lifetime of a gow command to serialize state mutations across processes.
type fileLock struct {
	f *os.File
}

// acquireFileLock opens path (creating it if missing) and takes an exclusive
// flock. If timeout > 0, it retries every 50ms until acquired or timeout
// elapses. timeout == 0 means non-blocking (single attempt).
func acquireFileLock(path string, timeout time.Duration) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock sidecar path is set by CLI, not user input
	if err != nil {
		return nil, fmt.Errorf("state: open lock %s: %w", path, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // fd fits in int on all supported platforms
		if err == nil {
			return &fileLock{f: f}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("state: flock %s: %w", path, err)
		}
		if timeout == 0 || time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("state: lock %s busy (held by another gow process)", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *fileLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN) //nolint:gosec // fd fits in int on all supported platforms
	_ = l.f.Close()
	l.f = nil
}
