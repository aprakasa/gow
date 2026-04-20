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
	r.commands = append(r.commands, append([]string{name}, args...))
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

	if err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "admin@ssl.test"}); err != nil {
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

	if err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "admin@ssl.test", Staging: true}); err != nil {
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

	err := m.EnableSSL(ctx, "nope.test", SSLOptions{Email: "admin@test.com"})
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

// seedSSLSite is a small helper used by the wildcard/DNS tests. The site
// name is fixed because every test in this group targets the same fake site.
const sslTestSite = "ssl.test"

func seedSSLSite(t *testing.T, m *Manager, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "www", sslTestSite, "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir htdocs: %v", err)
	}
	if err := m.store.Add(state.Site{Name: sslTestSite, Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
}

// redirectDNSCredsDir points dnsCredsDir at a temp dir for the duration of
// the test, so we can create and remove fake INI files without touching
// /etc/gow/dns.
func redirectDNSCredsDir(t *testing.T) string {
	t.Helper()
	orig := dnsCredsDir
	tmp := t.TempDir()
	dnsCredsDir = tmp
	t.Cleanup(func() { dnsCredsDir = orig })
	return tmp
}

func findCertbotCmd(t *testing.T, cmds [][]string) []string {
	t.Helper()
	for _, cmd := range cmds {
		if len(cmd) > 0 && cmd[0] == "certbot" {
			return cmd
		}
	}
	t.Fatal("certbot was not invoked")
	return nil
}

func TestEnableSSL_Wildcard_RequiresDNS(t *testing.T) {
	ctx := context.Background()
	m, dir := setupManagerWithRunner(t, &recordingRunner{})
	seedSSLSite(t, m, dir)

	err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "a@b.c", Wildcard: true})
	if err == nil {
		t.Fatal("expected error when --wildcard used without --dns")
	}
	if !strings.Contains(err.Error(), "requires --dns") {
		t.Errorf("error = %q, want 'requires --dns'", err.Error())
	}
}

func TestEnableSSL_DNS_UnsupportedProvider(t *testing.T) {
	ctx := context.Background()
	m, dir := setupManagerWithRunner(t, &recordingRunner{})
	seedSSLSite(t, m, dir)

	err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "a@b.c", DNS: "route53"})
	if err == nil {
		t.Fatal("expected error for unsupported --dns provider")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, want 'unsupported'", err.Error())
	}
}

func TestEnableSSL_DNS_MissingCredsFile(t *testing.T) {
	ctx := context.Background()
	m, dir := setupManagerWithRunner(t, &recordingRunner{})
	seedSSLSite(t, m, dir)
	redirectDNSCredsDir(t) // empty dir — creds file absent

	err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "a@b.c", DNS: "cloudflare"})
	if err == nil {
		t.Fatal("expected error when creds file missing")
	}
	if !strings.Contains(err.Error(), "DNS credentials") {
		t.Errorf("error = %q, want 'DNS credentials'", err.Error())
	}
}

func TestEnableSSL_Wildcard_Cloudflare_InvokesCertbot(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)
	seedSSLSite(t, m, dir)

	credsDir := redirectDNSCredsDir(t)
	credsPath := filepath.Join(credsDir, "cloudflare.ini")
	if err := os.WriteFile(credsPath, []byte("dns_cloudflare_api_token = fake\n"), 0o600); err != nil { //nolint:gosec // test file
		t.Fatalf("write creds: %v", err)
	}

	if err := m.EnableSSL(ctx, "ssl.test", SSLOptions{
		Email:    "a@b.c",
		Wildcard: true,
		DNS:      "cloudflare",
	}); err != nil {
		t.Fatalf("EnableSSL() = %v", err)
	}

	cmd := findCertbotCmd(t, rr.commands)
	all := strings.Join(cmd, " ")
	if !strings.Contains(all, "--dns-cloudflare") {
		t.Error("certbot should use --dns-cloudflare")
	}
	if !strings.Contains(all, "--dns-cloudflare-credentials "+credsPath) {
		t.Errorf("certbot should point at creds file, got: %s", all)
	}
	if !strings.Contains(all, "-d ssl.test") || !strings.Contains(all, "-d *.ssl.test") {
		t.Errorf("certbot should request apex + wildcard, got: %s", all)
	}
	if strings.Contains(all, "--webroot") {
		t.Error("DNS-01 path should not fall back to --webroot")
	}
}

func TestEnableSSL_HSTS_PersistsOnSite(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)
	seedSSLSite(t, m, dir)

	if err := m.EnableSSL(ctx, "ssl.test", SSLOptions{Email: "a@b.c", HSTS: true}); err != nil {
		t.Fatalf("EnableSSL() = %v", err)
	}
	got, _ := m.store.Find("ssl.test")
	if !got.HSTS {
		t.Error("Site.HSTS should be true after EnableSSL with HSTS: true")
	}
}
