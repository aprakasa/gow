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
)

const (
	presetStandard = "standard"
	presetCustom   = "custom"
)

// writeMock creates a temporary executable shell script that runs body.
func writeMock(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mock")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write mock: %v", err)
	}
	return p
}

// writeArgMock creates a mock script that captures arguments to a file.
func writeArgMock(t *testing.T, dir string) string {
	t.Helper()
	argFile := filepath.Join(dir, "args")
	p := filepath.Join(dir, "mock")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$@\" > '%s'\nexit 0", argFile)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // test mock needs execute bit
		t.Fatalf("write arg mock: %v", err)
	}
	return p
}

func fixtureSite(name, preset string) state.Site {
	return state.Site{
		Name:       name,
		PHPVersion: "83",
		Preset:     preset,
		CreatedAt:  time.Date(2026, 4, 13, 22, 0, 0, 0, time.UTC),
	}
}

// --- Reconcile ---

func TestReconcile_NoSites(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	// Mock that fails if called — verifies Reconcile skips OLS with 0 sites.
	ctrl := ols.NewController(writeMock(t, "echo 'unexpected OLS call' >&2; exit 1"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

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
	defer store.Close()

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	// Verify vhost config was written.
	vhostPath := filepath.Join(dir, "vhosts", "blog.test", "vhconf.conf")
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
	defer store.Close()

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add blog.test: %v", err)
	}
	if err := store.Add(fixtureSite("shop.test", "woocommerce")); err != nil {
		t.Fatalf("Add shop.test: %v", err)
	}

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	for _, name := range []string{"blog.test", "shop.test"} {
		p := filepath.Join(dir, "vhosts", name, "vhconf.conf")
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
	defer store.Close()

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	mockDir := t.TempDir()
	ctrl := ols.NewController(writeArgMock(t, mockDir))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Reconcile(); err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(mockDir, "args")) //nolint:gosec // test reads from temp dir
	args := string(got)
	// The mock captures the last call's args. After Reconcile, the last OLS
	// call should be "restart" (reload). "test" (validate) was called before it.
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
	defer store.Close()

	if err := store.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add site: %v", err)
	}

	ctrl := ols.NewController(writeMock(t, "echo 'syntax error' >&2; exit 1"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

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
	defer store.Close()

	// Add many heavy sites on a tiny server.
	for i := range 10 {
		name := fmt.Sprintf("site%d.test", i)
		if err := store.Add(fixtureSite(name, "heavy")); err != nil {
			t.Fatalf("Add %s: %v", name, err)
		}
	}

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 512, CPUCores: 1}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	err = m.Reconcile()
	if err == nil {
		t.Fatal("expected error for insufficient RAM")
	}
}

// --- Create ---

func TestCreate_AddsSiteAndReconciles(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Create("blog.test", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	// Site should be in the store.
	got := store.Find("blog.test")
	if got == nil {
		t.Fatal("site not found in store after Create")
	}
	if got.Preset != presetStandard {
		t.Errorf("preset = %q, want %q", got.Preset, presetStandard)
	}

	// Config file should be written.
	vhostPath := filepath.Join(dir, "vhosts", "blog.test", "vhconf.conf")
	if _, err := os.Stat(vhostPath); os.IsNotExist(err) {
		t.Error("vhost config not created by Create")
	}
}

func TestCreate_DuplicateReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Create("blog.test", "83", "standard", nil); err != nil {
		t.Fatalf("first Create() = %v", err)
	}
	err = m.Create("blog.test", "83", "standard", nil)
	if err == nil {
		t.Fatal("duplicate Create should return error")
	}
}

func TestCreate_InvalidPresetReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	err = m.Create("blog.test", "83", "nonexistent", nil)
	if err == nil {
		t.Fatal("invalid preset should return error")
	}
}

// --- Delete ---

func TestDelete_RemovesSiteAndReconciles(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	// Create first, then delete.
	if err := m.Create("blog.test", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Delete("blog.test"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}

	// Site should be gone from the store.
	if got := store.Find("blog.test"); got != nil {
		t.Error("site should be removed from store after Delete")
	}
}

func TestDelete_NotFoundReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	err = m.Delete("nope.test")
	if err == nil {
		t.Fatal("deleting nonexistent site should return error")
	}
}

// --- Tune ---

func TestTune_ChangesPresetAndReconciles(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	// Create with standard, then tune to woocommerce.
	if err := m.Create("shop.test", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := m.Tune("shop.test", "woocommerce", nil); err != nil {
		t.Fatalf("Tune() = %v", err)
	}

	// Preset should be updated in store.
	got := store.Find("shop.test")
	if got == nil {
		t.Fatal("site not found after tune")
	}
	if got.Preset != "woocommerce" {
		t.Errorf("preset = %q, want %q", got.Preset, "woocommerce")
	}

	// Config should be rewritten with new allocation.
	vhostPath := filepath.Join(dir, "vhosts", "shop.test", "vhconf.conf")
	data, err := os.ReadFile(vhostPath) //nolint:gosec // test reads from temp dir
	if err != nil {
		t.Fatalf("read vhost: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "shop.test") {
		t.Error("vhost config should contain site name after tune")
	}
}

func TestTune_NotFoundReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	err = m.Tune("nope.test", "heavy", nil)
	if err == nil {
		t.Fatal("tuning nonexistent site should return error")
	}
}

func TestTune_InvalidPresetReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Create("blog.test", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	err = m.Tune("blog.test", "nonexistent", nil)
	if err == nil {
		t.Fatal("invalid preset should return error")
	}

	// Original preset should be unchanged.
	got := store.Find("blog.test")
	if got.Preset != presetStandard {
		t.Errorf("preset should remain %q after failed tune, got %q", presetStandard, got.Preset)
	}
}

// --- Custom preset ---

func TestCreate_CustomPreset(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	custom := &state.CustomPreset{PHPMemoryMB: 320, WorkerBudgetMB: 160}
	if err := m.Create("custom.test", "83", "custom", custom); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	got := store.Find("custom.test")
	if got == nil {
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

	// Config file should be written with custom values.
	vhostPath := filepath.Join(dir, "vhosts", "custom.test", "vhconf.conf")
	data, err := os.ReadFile(vhostPath) //nolint:gosec // test reads from temp dir
	if err != nil {
		t.Fatalf("read vhost: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "custom.test") {
		t.Error("vhost config should contain site name")
	}
}

func TestTune_ToCustomPreset(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	if err := m.Create("shop.test", "83", "standard", nil); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	custom := &state.CustomPreset{PHPMemoryMB: 512, WorkerBudgetMB: 256}
	if err := m.Tune("shop.test", "custom", custom); err != nil {
		t.Fatalf("Tune() = %v", err)
	}

	got := store.Find("shop.test")
	if got.Preset != presetCustom {
		t.Errorf("preset = %q, want %q", got.Preset, presetCustom)
	}
	if got.CustomPreset == nil {
		t.Fatal("CustomPreset should not be nil after tuning to custom")
	}
	if got.CustomPreset.PHPMemoryMB != 512 {
		t.Errorf("CustomPreset.PHPMemoryMB = %d, want 512", got.CustomPreset.PHPMemoryMB)
	}
}

func TestTune_FromCustomToNamed(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	ctrl := ols.NewController(writeMock(t, "exit 0"))
	specs := system.Specs{TotalRAMMB: 8192, CPUCores: 4}

	m := NewManager(store, ctrl, specs, allocator.DefaultPolicy(), dir)

	custom := &state.CustomPreset{PHPMemoryMB: 320, WorkerBudgetMB: 160}
	if err := m.Create("blog.test", "83", "custom", custom); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	if err := m.Tune("blog.test", "standard", nil); err != nil {
		t.Fatalf("Tune() = %v", err)
	}

	got := store.Find("blog.test")
	if got.Preset != presetStandard {
		t.Errorf("preset = %q, want %q", got.Preset, presetStandard)
	}
	if got.CustomPreset != nil {
		t.Error("CustomPreset should be nil after tuning from custom to named preset")
	}
}
