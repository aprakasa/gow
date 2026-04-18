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
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
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
