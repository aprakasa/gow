package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
	"github.com/aprakasa/gow/internal/testmock"
)

const (
	presetStandard = "standard"
	presetCustom   = "custom"
)

// baseOLSConf returns a minimal httpd_config.conf content.
const baseOLSConf = `serverName localhost
virtualHost Example {
    configFile               conf/vhosts/Example/vhconf.conf
}
listener Default {
    address                  *:80
    map                      Example *
}
`

// setupManager creates a Manager with temp dirs and a base httpd_config.conf.
func setupManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	// Write base httpd_config.conf.
	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}

	ctrl := ols.NewController(testmock.WriteMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	webRoot := filepath.Join(dir, "www")
	if err := os.MkdirAll(webRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir www: %v", err)
	}

	return NewManager(store, ctrl, specs, allocator.DefaultPolicy(), confDir, webRoot), dir
}

func fixtureSite(name, preset string) state.Site {
	return state.Site{
		Name:       name,
		Type:       "wp",
		PHPVersion: "83",
		Preset:     preset,
		CreatedAt:  time.Date(2026, 4, 13, 22, 0, 0, 0, time.UTC),
	}
}

func httpdContent(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "conf", "httpd_config.conf")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read httpd_config: %v", err)
	}
	return string(data)
}

// setupReconcileTest creates a Manager for tests that call Reconcile directly
// with pre-populated sites. It sets up confDir, webRoot, and httpd_config.conf.
func setupReconcileTest(t *testing.T, store *state.Store) *Manager {
	t.Helper()
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	webRoot := filepath.Join(dir, "www")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.MkdirAll(webRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir www: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}
	ctrl := ols.NewController(testmock.WriteMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}
	return NewManager(store, ctrl, specs, allocator.DefaultPolicy(), confDir, webRoot)
}

// --- Reconcile ---

func TestReconcile_NoSites(t *testing.T) {
	m, dir := setupManager(t)

	// Mock that fails if called — verifies Reconcile skips OLS with 0 sites.
	_ = dir
	ctrl := ols.NewController(testmock.WriteMock(t, "echo 'unexpected OLS call' >&2; exit 1"))
	m.ols = ctrl

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}
}

func TestReconcile_SingleSite(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	m := setupReconcileTest(t, store)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	// Verify vhost config was written.
	vhostPath := filepath.Join(m.confDir, "vhosts", "blog.test", "vhconf.conf")
	data, err := os.ReadFile(vhostPath) //nolint:gosec // test reads from temp dir
	if err != nil {
		t.Fatalf("vhost config not found at %s", vhostPath)
	}
	content := string(data)
	if !strings.Contains(content, "blog.test") {
		t.Error("vhost config should contain site name")
	}
	// Verify allocator-derived values are present (not hardcoded).
	// 8192 MB, 4 cores, 1 standard site → Children=16, MemSoftMB=3276, MemHardMB=4096
	if !strings.Contains(content, "maxConns                16") {
		t.Error("vhost config should use allocator-computed maxConns")
	}
}

func TestReconcile_MultipleSites(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add blog.test: %v", err)
	}
	if err := store.Add(fixtureSite("shop.test", "woocommerce")); err != nil {
		t.Fatalf("Add shop.test: %v", err)
	}

	m := setupReconcileTest(t, store)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	for _, name := range []string{"blog.test", "shop.test"} {
		p := filepath.Join(m.confDir, "vhosts", name, "vhconf.conf")
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("vhost config missing for %s", name)
		}
	}
}

func TestReconcile_CallsValidateAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	confDir := filepath.Join(dir, "conf")
	webRoot := filepath.Join(dir, "www")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.MkdirAll(webRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir www: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}

	mockDir := t.TempDir()
	ctrl := ols.NewController(testmock.WriteArgMock(t, mockDir))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), confDir, webRoot)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(mockDir, "args")) //nolint:gosec // test reads from temp dir
	args := string(got)
	if !strings.Contains(args, "restart") {
		t.Errorf("expected 'restart' subcommand in OLS args, got %q", args)
	}
}

func TestReconcile_OLSValidateFails(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	m := setupReconcileTest(t, store)
	m.ols = ols.NewController(testmock.WriteMock(t, "echo 'syntax error' >&2; exit 1"))

	err = m.Reconcile()
	if err == nil {
		t.Fatal("expected error when OLS validate fails")
	}
}

func TestReconcile_InsufficientRAM(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	// Add many heavy sites on a tiny server.
	for i := range 10 {
		name := fmt.Sprintf("site%d.test", i)
		if err := store.Add(fixtureSite(name, "heavy")); err != nil {
			t.Fatalf("Add %s: %v", name, err)
		}
	}

	m := setupReconcileTest(t, store)
	m.specs = system.Specs{TotalRAMMB: 512, CPUCores: 1}

	err = m.Reconcile()
	if err == nil {
		t.Fatal("expected error for insufficient RAM")
	}
}

// --- Create ---

func TestCreate_AddsSiteAndReconciles(t *testing.T) {
	m, dir := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	// Site should be in the store.
	got, ok := m.store.Find("blog.test")
	if !ok {
		t.Fatal("site not found in store after Create")
	}
	if got.Preset != presetStandard {
		t.Errorf("preset = %q, want %q", got.Preset, presetStandard)
	}

	// Config file should be written.
	vhostPath := filepath.Join(dir, "conf", "vhosts", "blog.test", "vhconf.conf")
	if _, err := os.Stat(vhostPath); os.IsNotExist(err) {
		t.Error("vhost config not created by Create")
	}
}

func TestCreate_RegistersVirtualHostInHttpdConfig(t *testing.T) {
	m, dir := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	content := httpdContent(t, dir)
	if !strings.Contains(content, "virtualHost blog.test {") {
		t.Error("httpd_config.conf should contain virtualHost block for blog.test")
	}
	if !strings.Contains(content, "map                      blog.test blog.test") {
		t.Error("httpd_config.conf should contain listener map entry for blog.test")
	}
}

func TestCreate_CreatesDocRoot(t *testing.T) {
	m, dir := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	docRoot := filepath.Join(dir, "www", "blog.test", "htdocs")
	info, err := os.Stat(docRoot)
	if err != nil {
		t.Fatalf("docRoot not created at %s: %v", docRoot, err)
	}
	if !info.IsDir() {
		t.Error("docRoot should be a directory")
	}
}

func TestCreate_DuplicateReturnsError(t *testing.T) {
	m, _ := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("first Create() = %v", err)
	}
	err := m.Create("blog.test", "wp", "83", "standard", nil)
	if err == nil {
		t.Fatal("duplicate Create should return error")
	}
}

func TestCreate_InvalidPresetReturnsError(t *testing.T) {
	m, _ := setupManager(t)

	err := m.Create("blog.test", "wp", "83", "nonexistent", nil)
	if err == nil {
		t.Fatal("invalid preset should return error")
	}
}

// --- Delete ---

func TestDelete_RemovesSiteAndReconciles(t *testing.T) {
	m, _ := setupManager(t)

	// Create first, then delete.
	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Delete("blog.test"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}

	// Site should be gone from the store.
	if _, ok := m.store.Find("blog.test"); ok {
		t.Error("site should be removed from store after Delete")
	}
}

func TestDelete_UnregistersVirtualHostFromHttpdConfig(t *testing.T) {
	m, dir := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Delete("blog.test"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}

	content := httpdContent(t, dir)
	if strings.Contains(content, "virtualHost blog.test") {
		t.Error("httpd_config.conf should not contain virtualHost block after delete")
	}
	if strings.Contains(content, "blog.test blog.test") {
		t.Error("httpd_config.conf should not contain listener map entry after delete")
	}
}

func TestDelete_NotFoundReturnsError(t *testing.T) {
	m, _ := setupManager(t)

	err := m.Delete("nope.test")
	if err == nil {
		t.Fatal("deleting nonexistent site should return error")
	}
}

func TestDelete_RemovesSiteRoot(t *testing.T) {
	m, dir := setupManager(t)

	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	siteRoot := filepath.Join(dir, "www", "blog.test")
	if _, err := os.Stat(siteRoot); os.IsNotExist(err) {
		t.Fatalf("site root should exist before delete")
	}

	if err := m.Delete("blog.test"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}

	if _, err := os.Stat(siteRoot); !os.IsNotExist(err) {
		t.Error("site root directory should be removed after Delete")
	}
}

// --- Update ---

func TestUpdate_ChangesPHPVersion(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Update("blog.test", "82", "", nil); err != nil {
		t.Fatalf("Update() = %v", err)
	}
	got, _ := m.store.Find("blog.test")
	if got.PHPVersion != "82" {
		t.Errorf("PHPVersion = %q, want %q", got.PHPVersion, "82")
	}
}

func TestUpdate_ChangesPreset(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("shop.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Update("shop.test", "", "woocommerce", nil); err != nil {
		t.Fatalf("Update() = %v", err)
	}
	got, _ := m.store.Find("shop.test")
	if got.Preset != "woocommerce" {
		t.Errorf("Preset = %q, want %q", got.Preset, "woocommerce")
	}
}

func TestUpdate_NotFoundReturnsError(t *testing.T) {
	m, _ := setupManager(t)
	err := m.Update("nope.test", "83", "standard", nil)
	if err == nil {
		t.Fatal("updating nonexistent site should return error")
	}
}

func TestUpdate_InvalidPresetReturnsError(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	err := m.Update("blog.test", "", "nonexistent", nil)
	if err == nil {
		t.Fatal("invalid preset should return error")
	}
	got, _ := m.store.Find("blog.test")
	if got.Preset != "standard" {
		t.Error("preset should be unchanged after failed update")
	}
}

func TestUpdate_ToCustomPreset(t *testing.T) {
	m, _ := setupManager(t)

	if err := m.Create("shop.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	custom := &state.CustomPreset{PHPMemoryMB: 512, WorkerBudgetMB: 256}
	if err := m.Update("shop.test", "", "custom", custom); err != nil {
		t.Fatalf("Update() = %v", err)
	}

	got, _ := m.store.Find("shop.test")
	if got.Preset != presetCustom {
		t.Errorf("preset = %q, want %q", got.Preset, presetCustom)
	}
	if got.CustomPreset == nil {
		t.Fatal("CustomPreset should not be nil after updating to custom")
	}
	if got.CustomPreset.PHPMemoryMB != 512 {
		t.Errorf("CustomPreset.PHPMemoryMB = %d, want 512", got.CustomPreset.PHPMemoryMB)
	}
}

func TestUpdate_FromCustomToNamed(t *testing.T) {
	m, _ := setupManager(t)

	custom := &state.CustomPreset{PHPMemoryMB: 320, WorkerBudgetMB: 160}
	if err := m.Create("blog.test", "wp", "83", "custom", custom); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	if err := m.Update("blog.test", "", "standard", nil); err != nil {
		t.Fatalf("Update() = %v", err)
	}

	got, _ := m.store.Find("blog.test")
	if got.Preset != presetStandard {
		t.Errorf("preset = %q, want %q", got.Preset, presetStandard)
	}
	if got.CustomPreset != nil {
		t.Error("CustomPreset should be nil after updating from custom to named preset")
	}
}

// --- Custom preset ---

func TestCreate_CustomPreset(t *testing.T) {
	m, _ := setupManager(t)

	custom := &state.CustomPreset{PHPMemoryMB: 320, WorkerBudgetMB: 160}
	if err := m.Create("custom.test", "wp", "83", "custom", custom); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	got, ok := m.store.Find("custom.test")
	if !ok {
		t.Fatal("site not found in store after Create with custom preset")
	}
	if got.Preset != presetCustom {
		t.Errorf("preset = %q, want %q", got.Preset, presetCustom)
	}
	if got.CustomPreset == nil {
		t.Fatal("CustomPreset should not be nil")
	}
	if got.CustomPreset.PHPMemoryMB != 320 {
		t.Errorf("CustomPreset.PHPMemoryMB = %d, want 320", got.CustomPreset.PHPMemoryMB)
	}
	if got.CustomPreset.WorkerBudgetMB != 160 {
		t.Errorf("CustomPreset.WorkerBudgetMB = %d, want 160", got.CustomPreset.WorkerBudgetMB)
	}
}

// --- Online / Offline ---

func TestOffline_SetsMaintenanceAndReconciles(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Offline("blog.test"); err != nil {
		t.Fatalf("Offline() = %v", err)
	}
	got, _ := m.store.Find("blog.test")
	if !got.Maintenance {
		t.Error("Maintenance should be true after Offline")
	}
}

func TestOnline_ClearsMaintenanceAndReconciles(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("blog.test", "wp", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Offline("blog.test"); err != nil {
		t.Fatalf("Offline() = %v", err)
	}
	if err := m.Online("blog.test"); err != nil {
		t.Fatalf("Online() = %v", err)
	}
	got, _ := m.store.Find("blog.test")
	if got.Maintenance {
		t.Error("Maintenance should be false after Online")
	}
}

func TestOffline_NotFoundReturnsError(t *testing.T) {
	m, _ := setupManager(t)
	err := m.Offline("nope.test")
	if err == nil {
		t.Fatal("offlining nonexistent site should return error")
	}
}

func TestOnline_NotFoundReturnsError(t *testing.T) {
	m, _ := setupManager(t)
	err := m.Online("nope.test")
	if err == nil {
		t.Fatal("onlining nonexistent site should return error")
	}
}

// --- HTML site reconciliation ---

func TestReconcile_HTMLSiteNoPHP(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	if err := store.Add(state.Site{
		Name:      "static.test",
		Type:      "html",
		Preset:    "standard",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	m := setupReconcileTest(t, store)
	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	vhostPath := filepath.Join(m.confDir, "vhosts", "static.test", "vhconf.conf")
	data, err := os.ReadFile(vhostPath) //nolint:gosec // test
	if err != nil {
		t.Fatalf("vhost config not found: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "extprocessor") {
		t.Error("html vhost should not contain extprocessor block")
	}
	if strings.Contains(content, "scripthandler") {
		t.Error("html vhost should not contain scripthandler")
	}
}

// --- Maintenance mode ---

func TestReconcile_MaintenanceMode(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	if err := store.Add(state.Site{
		Name:        "blog.test",
		Type:        "wp",
		PHPVersion:  "83",
		Preset:      "standard",
		Maintenance: true,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	m := setupReconcileTest(t, store)
	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	// Check vhost uses html template (no PHP).
	vhostPath := filepath.Join(m.confDir, "vhosts", "blog.test", "vhconf.conf")
	data, err := os.ReadFile(vhostPath) //nolint:gosec // test
	if err != nil {
		t.Fatalf("vhost config not found: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "scripthandler") {
		t.Error("maintenance vhost should not contain scripthandler")
	}

	// Check maintenance page written to docRoot.
	maintPath := filepath.Join(m.webRoot, "blog.test", "htdocs", "index.html")
	maintData, err := os.ReadFile(maintPath) //nolint:gosec // test
	if err != nil {
		t.Fatalf("maintenance page not found: %v", err)
	}
	if !strings.Contains(string(maintData), "Under Maintenance") {
		t.Error("maintenance page should contain 'Under Maintenance'")
	}
}

// --- Create by type ---

func TestCreate_HTMLSite(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("static.test", "html", "", "standard", nil); err != nil {
		t.Fatalf("Create(html) = %v", err)
	}
	got, _ := m.store.Find("static.test")
	if got.Type != "html" {
		t.Errorf("Type = %q, want %q", got.Type, "html")
	}
}

func TestCreate_PHPSite(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Create("app.test", "php", "83", "standard", nil); err != nil {
		t.Fatalf("Create(php) = %v", err)
	}
	got, _ := m.store.Find("app.test")
	if got.Type != "php" {
		t.Errorf("Type = %q, want %q", got.Type, "php")
	}
}
