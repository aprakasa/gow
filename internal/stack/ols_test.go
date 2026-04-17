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

	mock := testmock.WriteArgMock(t, dir)

	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	_ = callLog
	_ = mock

	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls, got %d", len(calls))
	}

	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q", calls[0].name, "sh")
	}

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

func TestOLS_Purge_DeepCleans(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Purge(mr); err != nil {
		t.Fatalf("Purge() = %v", err)
	}

	checks := map[string]bool{
		"stop":       false,
		"purge":      false,
		"rm_lsws":    false,
		"rm_repo":    false,
		"autoremove": false,
		"update":     false,
	}
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "stop") {
			checks["stop"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "purge") && containsAny(c.args, "openlitespeed") {
			checks["purge"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/usr/local/lsws") {
			checks["rm_lsws"] = true
		}
		if c.name == "rm" && (containsAny(c.args, "lst_deb_repo.list") || containsAny(c.args, "lst_deb_repo.all")) {
			checks["rm_repo"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "autoremove") {
			checks["autoremove"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "update") {
			checks["update"] = true
		}
	}
	for k, v := range checks {
		if !v {
			t.Errorf("expected %s call", k)
		}
	}
}

func TestOLS_Start(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Start(mr); err != nil {
		t.Fatalf("Start() = %v", err)
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

func TestOLS_Stop(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Stop(mr); err != nil {
		t.Fatalf("Stop() = %v", err)
	}

	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "stop") {
			found = true
		}
	}
	if !found {
		t.Error("expected lswsctrl stop call")
	}
}

func TestOLS_Restart(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Restart(mr); err != nil {
		t.Fatalf("Restart() = %v", err)
	}

	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "restart") {
			found = true
		}
	}
	if !found {
		t.Error("expected lswsctrl restart call")
	}
}

func TestOLS_Reload(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Reload(mr); err != nil {
		t.Fatalf("Reload() = %v", err)
	}

	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lswsctrl") && containsAny(c.args, "reload") {
			found = true
		}
	}
	if !found {
		t.Error("expected lswsctrl reload call")
	}
}

func TestOLS_Upgrade(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Upgrade(mr); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}

	foundUpdate, foundUpgrade := false, false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "update") {
			foundUpdate = true
		}
		if c.name == "apt-get" && containsAny(c.args, "upgrade") && containsAny(c.args, "openlitespeed") {
			foundUpgrade = true
		}
	}
	if !foundUpdate {
		t.Error("expected apt-get update call")
	}
	if !foundUpgrade {
		t.Error("expected apt-get upgrade openlitespeed call")
	}
}

func TestOLS_Remove(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := OLS()
	if err := c.Remove(mr); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "remove") && containsAny(c.args, "openlitespeed") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get remove openlitespeed call")
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
	if strings.Contains(name, "redis-cli") {
		return "PONG\n", nil
	}
	if strings.Contains(name, "lsphp") {
		return "PHP 8.4.0", nil
	}
	if strings.Contains(name, "wp") {
		return "WP-CLI 2.11.0", nil
	}
	if strings.Contains(name, "composer") {
		return "Composer version 2.8.0", nil
	}
	if strings.Contains(name, "mariadb") || strings.Contains(name, "mysql") {
		return "mariadb from 11.4", nil
	}
	if strings.Contains(name, "dpkg-query") {
		return "1.7.0", nil
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
