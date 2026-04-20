package site

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
	"github.com/aprakasa/gow/internal/testmock"
)

// recordingRunner records all Run calls and always succeeds.
type recordingRunner struct {
	commands [][]string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	return nil
}

func (r *recordingRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	return "", nil
}

// Verify recordingRunner satisfies stack.Runner at compile time.
var _ stack.Runner = (*recordingRunner)(nil)

// setupManagerWithRunner creates a Manager with a custom runner for tests that
// need to inspect command invocations.
func setupManagerWithRunner(t *testing.T, runner stack.Runner) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}
	webRoot := filepath.Join(dir, "www")
	if err := os.MkdirAll(webRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir www: %v", err)
	}
	ctrl := ols.NewController(testmock.WriteMock(t, "exit 0"))
	return NewManager(store, ctrl, system.Specs{TotalRAMMB: 8192, CPUCores: 4}, allocator.DefaultPolicy(), confDir, webRoot, runner), dir
}

func TestEnableSSL_Success(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)
	if err := os.MkdirAll(filepath.Join(dir, "www", "ssl.test", "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir htdocs: %v", err)
	}

	if err := m.store.Add(state.Site{
		Name:       "ssl.test",
		Type:       "wp",
		PHPVersion: "83",
		Preset:     "standard",
	}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.EnableSSL(ctx, "ssl.test", "admin@ssl.test", false); err != nil {
		t.Fatalf("EnableSSL() = %v", err)
	}

	s, _ := m.store.Find("ssl.test")
	if !s.SSLEnabled {
		t.Error("SSLEnabled should be true")
	}
	if s.CertPath != "/etc/letsencrypt/live/ssl.test/fullchain.pem" {
		t.Errorf("CertPath = %q", s.CertPath)
	}
	if s.KeyPath != "/etc/letsencrypt/live/ssl.test/privkey.pem" {
		t.Errorf("KeyPath = %q", s.KeyPath)
	}

	found := false
	for _, cmd := range rr.commands {
		if len(cmd) > 0 && cmd[0] == "certbot" {
			found = true
			all := strings.Join(cmd, " ")
			if !strings.Contains(all, "--webroot") {
				t.Error("certbot should use --webroot")
			}
			if !strings.Contains(all, "admin@ssl.test") {
				t.Error("certbot should use provided email")
			}
			if strings.Contains(all, "--test-cert") {
				t.Error("should not use --test-cert when staging=false")
			}
			break
		}
	}
	if !found {
		t.Error("certbot was not called")
	}
}

func TestEnableSSL_Staging(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)
	if err := os.MkdirAll(filepath.Join(dir, "www", "ssl.test", "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir htdocs: %v", err)
	}

	if err := m.store.Add(state.Site{Name: "ssl.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.EnableSSL(ctx, "ssl.test", "admin@ssl.test", true); err != nil {
		t.Fatalf("EnableSSL() = %v", err)
	}

	found := false
	for _, cmd := range rr.commands {
		if len(cmd) > 0 && cmd[0] == "certbot" {
			all := strings.Join(cmd, " ")
			if strings.Contains(all, "--test-cert") {
				found = true
			}
			break
		}
	}
	if !found {
		t.Error("certbot should include --test-cert for staging")
	}
}

func TestEnableSSL_SiteNotFound(t *testing.T) {
	ctx := context.Background()
	m, _ := setupManagerWithRunner(t, &recordingRunner{})

	err := m.EnableSSL(ctx, "nope.test", "admin@test.com", false)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}
