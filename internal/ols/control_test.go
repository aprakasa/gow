package ols

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMock creates a temporary executable shell script that runs body.
func writeMock(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mock")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write mock: %v", err)
	}
	return p
}

// writeArgMock creates a mock script in dir that captures its arguments to
// a file called "args" in the same directory.
func writeArgMock(t *testing.T, dir string) string {
	t.Helper()
	argFile := filepath.Join(dir, "args")
	p := filepath.Join(dir, "mock")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$@\" > '%s'\nexit 0", argFile)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write arg mock: %v", err)
	}
	return p
}

// --- Validate ---

func TestValidate_Success(t *testing.T) {
	c := NewController(writeMock(t, "exit 0"))
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestValidate_Failure(t *testing.T) {
	c := NewController(writeMock(t, "echo 'syntax error' >&2; exit 1"))
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestValidate_IncludesStderr(t *testing.T) {
	c := NewController(writeMock(t, "echo 'syntax error on line 42' >&2; exit 1"))
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "syntax error on line 42") {
		t.Errorf("error message = %q, want stderr included", err.Error())
	}
}

func TestValidate_PassesTestSubcommand(t *testing.T) {
	dir := t.TempDir()
	c := NewController(writeArgMock(t, dir))
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "args")) //nolint:gosec // test reads from temp dir
	if !strings.Contains(string(got), "test") {
		t.Errorf("args = %q, want subcommand %q", string(got), "test")
	}
}

func TestValidate_BinaryNotFound(t *testing.T) {
	c := NewController("/no/such/binary")
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

// --- GracefulReload ---

func TestGracefulReload_Success(t *testing.T) {
	c := NewController(writeMock(t, "exit 0"))
	if err := c.GracefulReload(); err != nil {
		t.Fatalf("GracefulReload() = %v", err)
	}
}

func TestGracefulReload_Failure(t *testing.T) {
	c := NewController(writeMock(t, "exit 1"))
	err := c.GracefulReload()
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if !errors.Is(err, ErrReloadFailed) {
		t.Errorf("error = %v, want ErrReloadFailed", err)
	}
}

func TestGracefulReload_PassesRestartSubcommand(t *testing.T) {
	dir := t.TempDir()
	c := NewController(writeArgMock(t, dir))
	if err := c.GracefulReload(); err != nil {
		t.Fatalf("GracefulReload() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "args")) //nolint:gosec // test reads from temp dir
	if !strings.Contains(string(got), "restart") {
		t.Errorf("args = %q, want subcommand %q", string(got), "restart")
	}
}
