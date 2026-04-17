package ols

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/testmock"
)

// --- Validate ---

func TestValidate_AlwaysReturnsNil(t *testing.T) {
	c := NewController(testmock.WriteMock(t, "exit 1")) // even a failing binary should return nil
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil (no-op)", err)
	}
}

func TestValidate_NoBinaryRequired(t *testing.T) {
	c := NewController("/no/such/binary")
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil (no-op)", err)
	}
}

// --- GracefulReload ---

func TestGracefulReload_Success(t *testing.T) {
	c := NewController(testmock.WriteMock(t, "exit 0"))
	if err := c.GracefulReload(); err != nil {
		t.Fatalf("GracefulReload() = %v", err)
	}
}

func TestGracefulReload_Failure(t *testing.T) {
	c := NewController(testmock.WriteMock(t, "exit 1"))
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
	c := NewController(testmock.WriteArgMock(t, dir))
	if err := c.GracefulReload(); err != nil {
		t.Fatalf("GracefulReload() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "args")) //nolint:gosec // test reads from temp dir
	if !strings.Contains(string(got), "restart") {
		t.Errorf("args = %q, want subcommand %q", string(got), "restart")
	}
}
