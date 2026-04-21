package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

func (stubController) Validate(context.Context) error       { return nil }
func (stubController) GracefulReload(context.Context) error { return nil }

type testEnv struct {
	deps Deps
	cfg  CLIConfig
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
		deps: Deps{
			Ctx:    context.Background(),
			Stdin:  strings.NewReader(""),
			Stdout: io.Discard,
			Stderr: io.Discard,
			DetectSpecs: func() (system.Specs, error) {
				return system.Specs{TotalRAMMB: 8192, CPUCores: 4}, nil
			},
			LoadPolicy: func(string) (allocator.Policy, error) {
				return allocator.DefaultPolicy(), nil
			},
			OpenStore: func(string) (*state.Store, error) {
				return store, nil
			},
			NewOLS: func() ols.Controller {
				return &stubController{}
			},
			NewRunner: func() stack.Runner {
				return &testmock.NoopRunner{}
			},
			InstalledPHP: func() []string {
				return []string{"81", "83"}
			},
			PHPAvailable: func(ver string) bool {
				return ver == "81" || ver == "83"
			},
			WPInstall: func(string, string, string, string) error {
				return nil
			},
			DBCleanup: func(string) error {
				return nil
			},
		},
		cfg: CLIConfig{
			ConfDir:    confDir,
			StateFile:  statePath,
			PolicyFile: "",
			WebRoot:    webRoot,
			LogDir:     filepath.Join(dir, "logs"),
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

// --- Integration tests ---

func TestRunCreate_Success(t *testing.T) {
	e := newTestEnv(t)
	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunCreate() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	got, ok := store.Find("blog.test")
	if !ok {
		t.Fatal("site not found after create")
	}
	if got.Preset != "standard" {
		t.Errorf("preset = %q, want %q", got.Preset, "standard")
	}
}

func TestRunCreate_InvalidPreset(t *testing.T) {
	e := newTestEnv(t)
	err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "nonexistent", PHP: "83"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
}

func TestRunCreate_PHPNotInstalled(t *testing.T) {
	e := newTestEnv(t)
	err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "84"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error for uninstalled PHP version")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error = %q, want 'not installed'", err.Error())
	}
}

func TestRunCreate_NoPHPInstalled(t *testing.T) {
	e := newTestEnv(t)
	e.deps.InstalledPHP = func() []string { return nil }
	e.deps.PHPAvailable = func(string) bool { return false }
	err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog"}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error when no PHP installed")
	}
	if !strings.Contains(err.Error(), "no LSPHP versions found") {
		t.Errorf("error = %q, want 'no LSPHP versions found'", err.Error())
	}
}

func TestRunCreate_AutoDetectPHP(t *testing.T) {
	e := newTestEnv(t)
	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog"}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunCreate() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	got, _ := store.Find("blog.test")
	if got.PHPVersion != "83" {
		t.Errorf("PHP = %q, want %q", got.PHPVersion, "83")
	}
}

func TestRunCreate_HTMLSkipsPHP(t *testing.T) {
	e := newTestEnv(t)
	e.deps.InstalledPHP = func() []string { return nil }
	e.deps.PHPAvailable = func(string) bool { return false }
	if err := RunCreate(e.cfg, SiteFlags{SiteType: "html"}, "static.test", e.deps); err != nil {
		t.Fatalf("RunCreate() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	got, _ := store.Find("static.test")
	if got.PHPVersion != "" {
		t.Errorf("PHP = %q, want empty for HTML site", got.PHPVersion)
	}
}

func TestRunDelete_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunDelete(e.cfg, SiteFlags{NoPrompt: true}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunDelete() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	if _, ok := store.Find("blog.test"); ok {
		t.Error("site should be gone after delete")
	}
}

func TestRunDelete_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := RunDelete(e.cfg, SiteFlags{NoPrompt: true}, "nope.test", e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestRunList_Empty(t *testing.T) {
	e := newTestEnv(t)
	var buf bytes.Buffer
	if err := RunList(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("RunList() = %v", err)
	}
	if !strings.Contains(buf.String(), "No sites") {
		t.Errorf("expected 'No sites', got %q", buf.String())
	}
}

func TestRunList_ShowsSite(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := RunList(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("RunList() = %v", err)
	}
	if !strings.Contains(buf.String(), "blog.test") {
		t.Errorf("expected 'blog.test', got %q", buf.String())
	}
}

func TestRunUpdate_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunUpdate(e.cfg, SiteFlags{Preset: "woocommerce"}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunUpdate() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	got, _ := store.Find("blog.test")
	if got.Preset != "woocommerce" {
		t.Errorf("preset = %q, want %q", got.Preset, "woocommerce")
	}
}

func TestRunUpdate_EmptyPreset(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunUpdate(e.cfg, SiteFlags{Preset: ""}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunUpdate() = %v", err)
	}
}

func TestRunReconcile_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunReconcile(e.cfg, e.deps); err != nil {
		t.Fatalf("RunReconcile() = %v", err)
	}
}

func TestRunStatus_Empty(t *testing.T) {
	e := newTestEnv(t)
	var buf bytes.Buffer
	if err := RunStatus(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("RunStatus() = %v", err)
	}
	if !strings.Contains(buf.String(), "No sites") {
		t.Errorf("expected 'No sites', got %q", buf.String())
	}
}

func TestRunStatus_ShowAllocation(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := RunStatus(e.cfg, &buf, e.deps); err != nil {
		t.Fatalf("RunStatus() = %v", err)
	}
	if !strings.Contains(buf.String(), "blog.test") {
		t.Errorf("expected 'blog.test', got %q", buf.String())
	}
}

func TestRunSSL_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "ssl.test", e.deps)
	if err := RunSSL(e.cfg, SiteFlags{SSLEmail: "admin@ssl.test"}, "ssl.test", e.deps); err != nil {
		t.Fatalf("RunSSL() = %v", err)
	}
	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	got, _ := store.Find("ssl.test")
	if !got.SSLEnabled {
		t.Error("SSLEnabled should be true after ssl command")
	}
}

func TestRunSSL_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := RunSSL(e.cfg, SiteFlags{SSLEmail: "admin@test.com"}, "nope.test", e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestRunFlush_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunFlush(e.cfg, "blog.test", e.deps); err != nil {
		t.Fatalf("RunFlush() = %v", err)
	}
}

func TestRunFlush_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := RunFlush(e.cfg, "nope.test", e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestRunLog_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunLog(e.cfg, LogFlags{Lines: 50}, "blog.test", e.deps); err != nil {
		t.Fatalf("RunLog() = %v", err)
	}
}

func TestRunLog_AccessAndErrorConflict(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	err := RunLog(e.cfg, LogFlags{Access: true, Error: true}, "blog.test", e.deps)
	if err == nil {
		t.Fatal("expected error when both --access and --error are set")
	}
}

func TestRunWP_Success(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	if err := RunWP(e.cfg, "blog.test", []string{"plugin", "list"}, e.deps); err != nil {
		t.Fatalf("RunWP() = %v", err)
	}
}

func TestRunWP_NotFound(t *testing.T) {
	e := newTestEnv(t)
	err := RunWP(e.cfg, "nope.test", []string{"plugin", "list"}, e.deps)
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestNewManager_DetectFails(t *testing.T) {
	e := newTestEnv(t)
	e.deps.DetectSpecs = func() (system.Specs, error) {
		return system.Specs{}, errTestDetect
	}
	_, err := NewManager(e.cfg, e.deps)
	if err == nil {
		t.Fatal("expected error when detect fails")
	}
	if !strings.Contains(err.Error(), "detect hardware") {
		t.Errorf("error = %q, want 'detect hardware'", err.Error())
	}
}

func TestNewManager_OpenStoreFails(t *testing.T) {
	e := newTestEnv(t)
	e.deps.OpenStore = func(string) (*state.Store, error) {
		return nil, fmt.Errorf("permission denied")
	}
	_, err := NewManager(e.cfg, e.deps)
	if err == nil {
		t.Fatal("expected error when openStore fails")
	}
	if !strings.Contains(err.Error(), "open state") {
		t.Errorf("error = %q, want 'open state'", err.Error())
	}
}

func TestConfigureObjectCache_CallsWPEvalAndCP(t *testing.T) {
	r := &testmock.LoggingRunner{}
	docRoot := "/var/www/example.com/htdocs"

	if err := configureObjectCache(context.Background(), r, docRoot, "", ""); err != nil {
		t.Fatalf("configureObjectCache() = %v", err)
	}

	foundEval, foundCP := false, false
	for _, c := range r.Calls {
		if c.Name == stack.WPCLIBinPath {
			foundEval = true
			hasEval := false
			hasPath := false
			for _, a := range c.Args {
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
		if c.Name == "cp" {
			foundCP = true
			hasNoClobber := false
			hasDropIn := false
			for _, a := range c.Args {
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
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)

	err := RunStackPurge(e.cfg, StackFlags{Redis: true}, e.deps)
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
	err := RunStackPurge(e.cfg, StackFlags{Redis: true}, e.deps)
	if err != nil && strings.Contains(err.Error(), "cannot purge") {
		t.Fatalf("purge should not be blocked when no sites exist: %v", err)
	}
}
