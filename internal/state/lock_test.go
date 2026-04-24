package state

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileLock_BlocksSecondAcquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")

	l1, err := acquireFileLock(path, 0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer l1.release()

	start := time.Now()
	if _, err := acquireFileLock(path, 100*time.Millisecond); err == nil {
		t.Fatal("expected second acquire to fail while first holds lock")
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("expected to wait ~100ms, waited %v", elapsed)
	}
}

func TestFileLock_ReleaseAllowsSecondAcquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")

	l1, err := acquireFileLock(path, 0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	l1.release()

	l2, err := acquireFileLock(path, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	l2.release()
}

func TestFileLock_ConcurrentSerialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")

	var wg sync.WaitGroup
	var mu sync.Mutex
	active := 0
	maxActive := 0

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := acquireFileLock(path, 5*time.Second)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
			l.release()
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("expected mutual exclusion (max 1 active), saw %d", maxActive)
	}
}
