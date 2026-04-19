// Package testmock provides test helper utilities shared across internal
// packages. It is only imported by _test.go files.
package testmock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// NoopRunner implements stack.Runner by doing nothing. Use in tests that
// verify state changes without executing real shell commands.
type NoopRunner struct{}

// Run discards the command and returns nil.
func (NoopRunner) Run(_ context.Context, _ string, _ ...string) error { return nil }

// Output discards the command and returns an empty string.
func (NoopRunner) Output(_ context.Context, _ string, _ ...string) (string, error) {
	return "", nil
}

// WriteMock creates a temporary executable shell script that runs body and
// returns its path.
func WriteMock(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mock")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write mock: %v", err)
	}
	return p
}

// WriteArgMock creates a mock script in dir that captures its arguments to a
// file called "args" in the same directory, then returns the script path.
func WriteArgMock(t *testing.T, dir string) string {
	t.Helper()
	argFile := filepath.Join(dir, "args")
	p := filepath.Join(dir, "mock")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$@\" > '%s'\nexit 0", argFile)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write arg mock: %v", err)
	}
	return p
}
