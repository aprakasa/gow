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
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	sitePkg "github.com/aprakasa/gow/internal/site"
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

// newTestEnvRealStore mirrors newTestEnv but wires OpenStore to state.Open so
// every NewManager call acquires a fresh lock — matching production behavior
// and required for any test that asserts lock release.
func newTestEnvRealStore(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	// The base env pre-opens a store and reuses it. Close that one so the
	// test file lock is released and subsequent state.Open calls succeed.
	if s, err := e.deps.OpenStore(e.cfg.StateFile); err == nil {
		_ = s.Close()
	}
	e.deps.OpenStore = func(path string) (*state.Store, error) {
		return state.OpenWithTimeout(path, 2*time.Second)
	}
	return e
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

func TestRunMetrics_Table(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := RunMetrics(e.cfg, &buf, e.deps, false); err != nil {
		t.Fatalf("RunMetrics() = %v", err)
	}
	if !strings.Contains(buf.String(), "blog.test") {
		t.Errorf("expected 'blog.test' in output, got %q", buf.String())
	}
}

func TestRunMetrics_JSON(t *testing.T) {
	e := newTestEnv(t)
	_ = RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "blog.test", e.deps)
	var buf bytes.Buffer
	if err := RunMetrics(e.cfg, &buf, e.deps, true); err != nil {
		t.Fatalf("RunMetrics(--json) = %v", err)
	}
	if !strings.Contains(buf.String(), `"sites"`) {
		t.Errorf("expected JSON with 'sites' key, got %q", buf.String())
	}
}

func TestRunMetrics_Empty(t *testing.T) {
	e := newTestEnv(t)
	var buf bytes.Buffer
	if err := RunMetrics(e.cfg, &buf, e.deps, false); err != nil {
		t.Fatalf("RunMetrics() with no sites = %v", err)
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

	foundOptionUpdate, foundEval, foundCP := false, false, false
	for _, c := range r.Calls {
		if c.Name == stack.WPCLIBinPath {
			hasPath := false
			for _, a := range c.Args {
				if strings.Contains(a, "--path="+docRoot) {
					hasPath = true
				}
			}
			if !hasPath {
				t.Error("wp call missing --path flag")
			}
			for _, a := range c.Args {
				if a == "option" {
					foundOptionUpdate = true
				}
				if a == "eval" {
					foundEval = true
				}
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
	if !foundOptionUpdate {
		t.Error("expected wp option update calls for litespeed.conf.*")
	}
	if !foundEval {
		t.Error("expected wp eval call for .litespeed_conf.dat")
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

func redirectCronDir(t *testing.T) string {
	t.Helper()
	orig := cronDir
	tmp := t.TempDir()
	cronDir = tmp
	t.Cleanup(func() { cronDir = orig })
	return tmp
}

func TestWriteCronFile_Content(t *testing.T) {
	dir := redirectCronDir(t)

	if err := writeCronFile("example.com", "/var/www/example.com/htdocs"); err != nil {
		t.Fatalf("writeCronFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "gow-example.com")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read cron file: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		"*/5 * * * * root",
		"cd /var/www/example.com/htdocs",
		"cron event run --due-now",
		"--allow-root",
		">/dev/null 2>&1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cron file should contain %q, got:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("cron file should end with newline")
	}
}

func TestRemoveCronFile_Idempotent(t *testing.T) {
	dir := redirectCronDir(t)

	// Removing a non-existent file should succeed.
	if err := removeCronFile("nope.test"); err != nil {
		t.Fatalf("removeCronFile on missing file: %v", err)
	}

	// Write then remove.
	if err := writeCronFile("exists.test", "/var/www/exists.test/htdocs"); err != nil {
		t.Fatalf("writeCronFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gow-exists.test")); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if err := removeCronFile("exists.test"); err != nil {
		t.Fatalf("removeCronFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gow-exists.test")); !os.IsNotExist(err) {
		t.Error("file should be gone after remove")
	}
}

func TestRunBackupSchedule_Success(t *testing.T) {
	e := newTestEnv(t)
	cronDir := redirectCronDir(t)
	gowBinPath = filepath.Join(t.TempDir(), "gow")
	t.Cleanup(func() { gowBinPath = "/usr/local/bin/gow" })

	// Create a site first.
	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "sched.test", e.deps); err != nil {
		t.Fatalf("RunCreate: %v", err)
	}

	if err := RunBackupSchedule(e.cfg, "sched.test", "daily", 7, e.deps); err != nil {
		t.Fatalf("RunBackupSchedule: %v", err)
	}

	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	s, ok := store.Find("sched.test")
	if !ok {
		t.Fatal("site not found")
	}
	if s.BackupSchedule != "daily" {
		t.Errorf("BackupSchedule = %q, want daily", s.BackupSchedule)
	}
	if s.BackupRetain != 7 {
		t.Errorf("BackupRetain = %d, want 7", s.BackupRetain)
	}

	data, err := os.ReadFile(filepath.Join(cronDir, "gow-backups")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read cron file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "backup-cron") {
		t.Errorf("cron file should contain 'backup-cron', got: %s", got)
	}
}

func TestRunBackupSchedule_InvalidSchedule(t *testing.T) {
	e := newTestEnv(t)
	if err := RunBackupSchedule(e.cfg, "sched.test", "hourly", 7, e.deps); err == nil {
		t.Fatal("expected error for invalid schedule")
	}
}

func TestRunBackupSchedule_SiteNotFound(t *testing.T) {
	e := newTestEnv(t)
	if err := RunBackupSchedule(e.cfg, "nope.test", "daily", 7, e.deps); err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}

func TestRunBackupUnschedule_Success(t *testing.T) {
	e := newTestEnv(t)
	redirectCronDir(t)

	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "sched.test", e.deps); err != nil {
		t.Fatalf("RunCreate: %v", err)
	}
	if err := RunBackupSchedule(e.cfg, "sched.test", "daily", 7, e.deps); err != nil {
		t.Fatalf("RunBackupSchedule: %v", err)
	}

	var buf bytes.Buffer
	e.deps.Stdout = &buf
	if err := RunBackupUnschedule(e.cfg, "sched.test", e.deps); err != nil {
		t.Fatalf("RunBackupUnschedule: %v", err)
	}

	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	s, _ := store.Find("sched.test")
	if s.BackupSchedule != "" {
		t.Errorf("BackupSchedule = %q, want empty", s.BackupSchedule)
	}
}

func TestRunBackupUnschedule_LastSiteRemovesCronFile(t *testing.T) {
	e := newTestEnv(t)
	cronDir := redirectCronDir(t)

	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "a.test", e.deps); err != nil {
		t.Fatalf("RunCreate a: %v", err)
	}
	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "b.test", e.deps); err != nil {
		t.Fatalf("RunCreate b: %v", err)
	}
	if err := RunBackupSchedule(e.cfg, "a.test", "daily", 7, e.deps); err != nil {
		t.Fatalf("Schedule a: %v", err)
	}
	if err := RunBackupSchedule(e.cfg, "b.test", "daily", 7, e.deps); err != nil {
		t.Fatalf("Schedule b: %v", err)
	}

	// Unschedule one — cron file should still exist.
	if err := RunBackupUnschedule(e.cfg, "a.test", e.deps); err != nil {
		t.Fatalf("Unschedule a: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cronDir, "gow-backups")); os.IsNotExist(err) {
		t.Error("cron file should still exist with one scheduled site")
	}

	// Unschedule the other — cron file should be removed.
	if err := RunBackupUnschedule(e.cfg, "b.test", e.deps); err != nil {
		t.Fatalf("Unschedule b: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cronDir, "gow-backups")); !os.IsNotExist(err) {
		t.Error("cron file should be removed when no sites have schedules")
	}
}

func TestRunBackupCron_DailySite(t *testing.T) {
	e := newTestEnv(t)
	redirectCronDir(t)

	backupDir := filepath.Join(t.TempDir(), "backups")
	origBackupDir := sitePkg.BackupDir
	sitePkg.BackupDir = backupDir
	t.Cleanup(func() { sitePkg.BackupDir = origBackupDir })

	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "cron.test", e.deps); err != nil {
		t.Fatalf("RunCreate: %v", err)
	}

	store, _ := e.deps.OpenStore(e.cfg.StateFile)
	_ = store.Update("cron.test", func(s *state.Site) {
		s.BackupSchedule = "daily"
		s.BackupRetain = 3
	})
	_ = store.Save()

	// RunBackupCron succeeds — the NoopRunner doesn't create real archives,
	// so we verify only that the cron loop completes without error.
	if err := RunBackupCron(e.cfg, e.deps); err != nil {
		t.Fatalf("RunBackupCron: %v", err)
	}
}

func TestRunDelete_ClearsSchedule(t *testing.T) {
	e := newTestEnv(t)
	cronDir := redirectCronDir(t)

	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "del.test", e.deps); err != nil {
		t.Fatalf("RunCreate: %v", err)
	}
	if err := RunBackupSchedule(e.cfg, "del.test", "daily", 7, e.deps); err != nil {
		t.Fatalf("RunBackupSchedule: %v", err)
	}

	// Verify cron file exists.
	if _, err := os.Stat(filepath.Join(cronDir, "gow-backups")); os.IsNotExist(err) {
		t.Fatal("cron file should exist after scheduling")
	}

	if err := RunDelete(e.cfg, SiteFlags{NoPrompt: true}, "del.test", e.deps); err != nil {
		t.Fatalf("RunDelete: %v", err)
	}

	// Cron file should be removed since it was the only scheduled site.
	if _, err := os.Stat(filepath.Join(cronDir, "gow-backups")); !os.IsNotExist(err) {
		t.Error("cron file should be removed after deleting the only scheduled site")
	}
}

func TestEnsureGlobalBackupCron_Content(t *testing.T) {
	dir := redirectCronDir(t)

	if err := ensureGlobalBackupCron(); err != nil {
		t.Fatalf("ensureGlobalBackupCron: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "gow-backups")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)
	for _, want := range []string{"0 2 * * *", "root", "backup-cron"} {
		if !strings.Contains(got, want) {
			t.Errorf("cron file should contain %q, got: %s", want, got)
		}
	}
}

func TestRemoveGlobalBackupCron_Idempotent(t *testing.T) {
	redirectCronDir(t)

	if err := removeGlobalBackupCron(); err != nil {
		t.Fatalf("removeGlobalBackupCron on nonexistent file: %v", err)
	}
}

// TestRunCreate_ReleasesLockOnSuccess verifies that after a successful
// RunCreate, the state file lock is released — a fresh OpenWithTimeout on
// the same path must succeed within 100ms. If the lock leaked, this would
// block for defaultLockTimeout (5 minutes) and then fail.
func TestRunCreate_ReleasesLockOnSuccess(t *testing.T) {
	e := newTestEnvRealStore(t)

	if err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "a.test", e.deps); err != nil {
		t.Fatalf("RunCreate() = %v", err)
	}

	s, err := state.OpenWithTimeout(e.cfg.StateFile, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("state lock still held after RunCreate: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
}

// TestRunCreate_ReleasesLockOnFailure verifies that when RunCreate fails
// after the manager has opened the store, the lock is still released by
// the deferred Close. Forces failure via DetectSpecs returning an error
// (triggered inside NewManager before the manager is built, but the
// symmetric path — a later failure after NewManager — also needs the
// defer to run; we exercise that via an invalid preset passed through to
// Manager.Create).
func TestRunCreate_ReleasesLockOnFailure(t *testing.T) {
	e := newTestEnvRealStore(t)

	// Invalid preset — resolveTuneFlags accepts arbitrary strings as preset
	// names, so the failure surfaces inside Manager.Create (after NewManager
	// has opened the store). This is the path where defer m.Close() matters.
	err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "nonexistent", PHP: "83"}, "b.test", e.deps)
	if err == nil {
		t.Fatal("expected RunCreate to fail with invalid preset")
	}

	s, openErr := state.OpenWithTimeout(e.cfg.StateFile, 100*time.Millisecond)
	if openErr != nil {
		t.Fatalf("state lock still held after failed RunCreate: %v", openErr)
	}
	t.Cleanup(func() { _ = s.Close() })
}

func TestRunCreate_WPInstallFailure_SurfacesCleanupWarnings(t *testing.T) {
	e := newTestEnvRealStore(t)

	wpErr := errors.New("wp-install failed")
	dbErr := errors.New("db cleanup failed")

	var stderr bytes.Buffer
	e.deps.Stderr = &stderr
	e.deps.WPInstall = func(string, string, string, string) error {
		return wpErr
	}
	e.deps.DBCleanup = func(string) error {
		return dbErr
	}

	err := RunCreate(e.cfg, SiteFlags{SiteType: "wp", Preset: "blog", PHP: "83"}, "cleanup.test", e.deps)
	if !errors.Is(err, wpErr) {
		t.Fatalf("RunCreate() error = %v, want %v", err, wpErr)
	}

	got := stderr.String()
	if !strings.Contains(got, "cleanup warning") {
		t.Errorf("stderr = %q, want cleanup warning", got)
	}
	if !strings.Contains(got, "db cleanup failed") {
		t.Errorf("stderr = %q, want db error message", got)
	}
}
