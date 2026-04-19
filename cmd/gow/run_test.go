package main

import (
	"bytes"
	"errors"
	"fmt"
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

var errTestDetect = errors.New("detect failed")

type stubController struct{}

func (stubController) Validate() error       { return nil }
func (stubController) GracefulReload() error { return nil }

type testEnv struct {
	deps deps
	cfg  cliConfig
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	confDir := filepath.Join(dir, "conf")
	webRoot := filepath.Join(dir, "www")
	for _, d := range []string{confDir, webRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // test dir
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}

	store, err := state.Open(statePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	return &testEnv{
		deps: deps{
			detectSpecs: func() (system.Specs, error) {
				return system.Specs{TotalRAMMB: 8192, CPUCores: 4}, nil
			},
			loadPolicy: func(string) (allocator.Policy, error) {
				return allocator.DefaultPolicy(), nil
			},
			openStore: func(string) (*state.Store, error) {
				return store, nil
			},
			newOLS: func() ols.Controller {
				return &stubController{}
			},
			newRunner: func() stack.Runner {
				return &testmock.NoopRunner{}
			},
			installedPHP: func() []string {
				return []string{"81", "83"}
			},
			phpAvailable: func(ver string) bool {
				return ver == "81" || ver == "83"
			},
			wpInstall: func(string, string) error {
				return nil
			},
			dbCleanup: func(string) error {
				return nil
			},
		},
		cfg: cliConfig{
			confDir:    confDir,
			stateFile:  statePath,
			policyFile: "",
			webRoot:    webRoot,
		},
	}
}

const baseOLSConf = `serverName localhost
virtualHost Example {
    configFile               conf/vhosts/Example/vhconf.conf
}
listener Default {
    address                  *:80
    map                      Example *
}
`

func TestRunCreateWithDeps_Success(t *testing.T) {
	e := newTestEnv(t)
	if err := runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps); err != nil {
		t.Fatalf("runCreateWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	got, ok := store.Find("blog.test")
	if !ok {
		t.Fatal("site not found after create")
	}
	if got.Preset != "standard" {
		t.Errorf("preset = %q, want %q", got.Preset, "standard")
	}
}

func TestRunCreateWithDeps_InvalidPreset(t *testing.T) {
	e := newTestEnv(t)
	err := runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "nonexistent", php: "83"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
}

func TestRunCreateWithDeps_PHPNotInstalled(t *testing.T) {
	e := newTestEnv(t)
	err := runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "84"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error for uninstalled PHP version")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error = %q, want 'not installed'", err.Error())
	}
}

func TestRunCreateWithDeps_NoPHPInstalled(t *testing.T) {
	e := newTestEnv(t)
	e.deps.installedPHP = func() []string { return nil }
	e.deps.phpAvailable = func(string) bool { return false }
	err := runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error when no PHP installed")
	}
	if !strings.Contains(err.Error(), "no LSPHP versions found") {
		t.Errorf("error = %q, want 'no LSPHP versions found'", err.Error())
	}
}

func TestRunCreateWithDeps_AutoDetectPHP(t *testing.T) {
	e := newTestEnv(t)
	// No --php flag: should auto-detect latest (83).
	if err := runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog"}, "blog.test", e.deps); err != nil {
		t.Fatalf("runCreateWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	got, _ := store.Find("blog.test")
	if got.PHPVersion != "83" {
		t.Errorf("PHP = %q, want %q", got.PHPVersion, "83")
	}
}

func TestRunCreateWithDeps_HTMLSkipsPHP(t *testing.T) {
	e := newTestEnv(t)
	e.deps.installedPHP = func() []string { return nil }
	e.deps.phpAvailable = func(string) bool { return false }
	if err := runCreateWithDeps(e.cfg, siteFlags{siteType: "html"}, "static.test", e.deps); err != nil {
		t.Fatalf("runCreateWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	got, _ := store.Find("static.test")
	if got.PHPVersion != "" {
		t.Errorf("PHP = %q, want empty for HTML site", got.PHPVersion)
	}
}

func TestRunDeleteWithDeps_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	if err := runDeleteWithDeps(e.cfg, siteFlags{noPrompt: true}, "blog.test", e.deps); err != nil {
		t.Fatalf("runDeleteWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	if _, ok := store.Find("blog.test"); ok {
		t.Error("site should be gone after delete")
	}
}

func TestRunDeleteWithDeps_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := runDeleteWithDeps(e.cfg, siteFlags{noPrompt: true}, "nope.test", e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestRunListWithDeps_Empty(t *testing.T) {
	e := newTestEnv(t)
	var buf bytes.Buffer
	if err := runListWithDeps(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("runListWithDeps() = %v", err)
	}
	if !strings.Contains(buf.String(), "No sites") {
		t.Errorf("expected 'No sites', got %q", buf.String())
	}
}

func TestRunListWithDeps_ShowsSite(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := runListWithDeps(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("runListWithDeps() = %v", err)
	}
	if !strings.Contains(buf.String(), "blog.test") {
		t.Errorf("expected 'blog.test', got %q", buf.String())
	}
}

func TestRunUpdateWithDeps_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	if err := runUpdateWithDeps(e.cfg, siteFlags{preset: "woocommerce"}, "blog.test", e.deps); err != nil {
		t.Fatalf("runUpdateWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	got, _ := store.Find("blog.test")
	if got.Preset != "woocommerce" {
		t.Errorf("preset = %q, want %q", got.Preset, "woocommerce")
	}
}

func TestRunUpdateWithDeps_EmptyPreset(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	// Empty preset should succeed — it means "no change"
	if err := runUpdateWithDeps(e.cfg, siteFlags{preset: ""}, "blog.test", e.deps); err != nil {
		t.Fatalf("runUpdateWithDeps() = %v", err)
	}
}

func TestRunReconcileWithDeps_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	if err := runReconcileWithDeps(e.cfg, e.deps); err != nil {
		t.Fatalf("runReconcileWithDeps() = %v", err)
	}
}

func TestRunStatusWithDeps_Empty(t *testing.T) {
	e := newTestEnv(t)
	var buf bytes.Buffer
	if err := runStatusWithDeps(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("runStatusWithDeps() = %v", err)
	}
	if !strings.Contains(buf.String(), "No sites") {
		t.Errorf("expected 'No sites', got %q", buf.String())
	}
}

func TestRunStatusWithDeps_ShowAllocation(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := runStatusWithDeps(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("runStatusWithDeps() = %v", err)
	}
	if !strings.Contains(buf.String(), "blog.test") {
		t.Errorf("expected 'blog.test', got %q", buf.String())
	}
}

func TestRunSSLWithDeps_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "ssl.test", e.deps)
	if err := runSSLWithDeps(e.cfg, siteFlags{sslEmail: "admin@ssl.test"}, "ssl.test", e.deps); err != nil {
		t.Fatalf("runSSLWithDeps() = %v", err)
	}
	store, _ := e.deps.openStore(e.cfg.stateFile)
	got, _ := store.Find("ssl.test")
	if !got.SSLEnabled {
		t.Error("SSLEnabled should be true after ssl command")
	}
}

func TestRunSSLWithDeps_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := runSSLWithDeps(e.cfg, siteFlags{sslEmail: "admin@test.com"}, "nope.test", e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestNewManagerWithDeps_DetectFails(t *testing.T) {
	e := newTestEnv(t)
	e.deps.detectSpecs = func() (system.Specs, error) {
		return system.Specs{}, errTestDetect
	}
	_, err := newManagerWithDeps(e.cfg, e.deps)
	if err == nil {
		t.Fatal("expected error when detect fails")
	}
	if !strings.Contains(err.Error(), "detect hardware") {
		t.Errorf("error = %q, want 'detect hardware'", err.Error())
	}
}

func TestNewManagerWithDeps_OpenStoreFails(t *testing.T) {
	e := newTestEnv(t)
	e.deps.openStore = func(string) (*state.Store, error) {
		return nil, fmt.Errorf("permission denied")
	}
	_, err := newManagerWithDeps(e.cfg, e.deps)
	if err == nil {
		t.Fatal("expected error when openStore fails")
	}
	if !strings.Contains(err.Error(), "open state") {
		t.Errorf("error = %q, want 'open state'", err.Error())
	}
}

// --- Logging runner for unit tests ---

type cmdCall struct {
	name string
	args []string
}

type cmdLoggingRunner struct {
	calls []cmdCall
}

func (r *cmdLoggingRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, cmdCall{name, args})
	return nil
}

func (r *cmdLoggingRunner) Output(name string, args ...string) (string, error) {
	r.calls = append(r.calls, cmdCall{name, args})
	return "", nil
}

func TestConfigureObjectCache_CallsWPEvalAndCP(t *testing.T) {
	r := &cmdLoggingRunner{}
	docRoot := "/var/www/example.com/htdocs"

	if err := configureObjectCache(r, docRoot); err != nil {
		t.Fatalf("configureObjectCache() = %v", err)
	}

	foundEval, foundCP := false, false
	for _, c := range r.calls {
		if c.name == stack.WPCLIBinPath {
			foundEval = true
			hasEval := false
			hasPath := false
			for _, a := range c.args {
				if a == "eval" {
					hasEval = true
				}
				if strings.Contains(a, "--path="+docRoot) {
					hasPath = true
				}
			}
			if !hasEval {
				t.Error("wp eval call missing 'eval' arg")
			}
			if !hasPath {
				t.Error("wp eval call missing --path flag")
			}
		}
		if c.name == "cp" {
			foundCP = true
			hasNoClobber := false
			hasDropIn := false
			for _, a := range c.args {
				if a == "-n" {
					hasNoClobber = true
				}
				if strings.Contains(a, "object-cache.php") {
					hasDropIn = true
				}
			}
			if !hasNoClobber {
				t.Error("cp call missing -n flag")
			}
			if !hasDropIn {
				t.Error("cp call missing object-cache.php path")
			}
		}
	}
	if !foundEval {
		t.Error("expected wp eval call")
	}
	if !foundCP {
		t.Error("expected cp object-cache.php drop-in call")
	}
}

func TestRunStackPurge_BlockedBySites(t *testing.T) {
	e := newTestEnv(t)
	_ = runCreateWithDeps(e.cfg, siteFlags{siteType: "wp", preset: "blog", php: "83"}, "blog.test", e.deps)

	err := runStackPurge(stackFlags{redis: true}, e.cfg)
	if err == nil {
		t.Fatal("expected purge to be blocked by dependent sites")
	}
	if !strings.Contains(err.Error(), "cannot purge redis") {
		t.Errorf("error = %q, want 'cannot purge redis'", err.Error())
	}
	if !strings.Contains(err.Error(), "blog.test") {
		t.Errorf("error = %q, want site name in error", err.Error())
	}
}

func TestRunStackPurge_AllowedWhenNoSites(t *testing.T) {
	e := newTestEnv(t)
	// No sites created — purge should proceed (component not installed in test,
	// but the dependency check should pass).
	err := runStackPurge(stackFlags{redis: true}, e.cfg)
	// Will fail because redis isn't installed in test env, but should NOT
	// fail with "cannot purge" dependency error.
	if err != nil && strings.Contains(err.Error(), "cannot purge") {
		t.Fatalf("purge should not be blocked when no sites exist: %v", err)
	}
}
