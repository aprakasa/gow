package stack

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/testmock"
)

func TestOLS_Install_CallsRepoSetupAndAptInstall(t *testing.T) {
	dir := t.TempDir()
	callLog := filepath.Join(dir, "calls")

	// Create a script that logs the command name + args and exits 0.
	mock := testmock.WriteArgMock(t, dir)

	// Rename the mock to simulate multiple binaries.
	// The runner calls Run(mockPath, args...), but OLS calls sh/apt-get/lswsctrl.
	// We need a runner that logs all calls.
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	_ = callLog
	_ = mock

	// Should call: sh -c "wget ... repo setup", apt-get install, lswsctrl start, lswsctrl status
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls, got %d", len(calls))
	}

	// First call: repo setup via sh
	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q", calls[0].name, "sh")
	}

	// Second call: apt-get install
	foundAptInstall := false
	for _, c := range calls {
		if c.name == "apt-get" && len(c.args) > 0 && c.args[0] == "install" {
			foundAptInstall = true
			if !containsAny(c.args, "openlitespeed") {
				t.Errorf("apt-get install args = %v, want openlitespeed", c.args)
			}
		}
	}
	if !foundAptInstall {
		t.Error("expected apt-get install call")
	}
}

func TestOLS_Install_StartsService(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "start") {
			found = true
		}
	}
	if !found {
		t.Error("expected lswsctrl start call")
	}
}

func TestOLS_Install_VerifiesStatus(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "status") {
			found = true
		}
	}
	if !found {
		t.Error("expected lswsctrl status verification call")
	}
}

func TestOLS_Install_RepoSetupFails(t *testing.T) {
	mr := &mockRunner{runErr: errGeneric}

	c := OLS()
	err := c.Install(mr)
	if err == nil {
		t.Fatal("expected error when repo setup fails")
	}
}

func TestOLS_Uninstall_PurgesPackage(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Uninstall(mr); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "purge") && containsAny(c.args, "openlitespeed") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get purge openlitespeed call")
	}
}

// --- helpers ---

// loggingRunner records all calls for inspection.
type loggingRunner struct {
	calls *[]call
}

func (l *loggingRunner) Run(name string, args ...string) error {
	*l.calls = append(*l.calls, call{name, args})
	return nil
}

func (l *loggingRunner) Output(name string, args ...string) (string, error) {
	*l.calls = append(*l.calls, call{name, args})
	// Return sensible output for known verification commands.
	if strings.Contains(name, "redis-cli") {
		return "PONG\n", nil
	}
	if strings.Contains(name, "lsphp") {
		return "PHP 8.4.0", nil
	}
	return "", nil
}

func containsAny(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
