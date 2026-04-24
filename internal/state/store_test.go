package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const presetHeavy = "heavy"

// helper creates a temp file path and cleans up after the test.
func tempStorePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "state.json")
}

func fixtureSite(name, preset string) Site {
	return Site{
		Name:       name,
		PHPVersion: "83",
		Preset:     preset,
		CreatedAt:  time.Date(2026, 4, 13, 22, 0, 0, 0, time.UTC),
	}
}

// --- Load / Open ---

func TestOpenCreatesFile(t *testing.T) {
	path := tempStorePath(t)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_ = s

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Open should create the state file if absent")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := tempStorePath(t)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if len(s.Sites()) != 0 {
		t.Errorf("expected 0 sites, got %d", len(s.Sites()))
	}
}

func TestOpenExistingEmptyFile_Normalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("create empty file: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if len(s.Sites()) != 0 {
		t.Errorf("expected 0 sites, got %d", len(s.Sites()))
	}
	data, err := os.ReadFile(path) //nolint:gosec // test reads from temp dir
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) == 0 {
		t.Error("empty file should be normalized to valid JSON by Open")
	}
}

func TestOpen_LocksStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer func() { _ = s1.Close() }()

	// Second open with a short deadline must fail.
	if _, err := OpenWithTimeout(path, 100*time.Millisecond); err == nil {
		t.Fatal("expected second Open to fail while first holds lock")
	}
}

func TestOpen_CloseReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open after close: %v", err)
	}
	_ = s2.Close()
}

// --- Add ---

func TestAddSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	if err := s.Add(fixtureSite("blog.test", "standard")); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	sites := s.Sites()
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].Name != "blog.test" {
		t.Errorf("site name = %q, want %q", sites[0].Name, "blog.test")
	}
	if sites[0].Preset != "standard" {
		t.Errorf("preset = %q, want %q", sites[0].Preset, "standard")
	}
}

func TestAddDuplicateReturnsError(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	_ = s.Add(fixtureSite("blog.test", "standard"))
	err := s.Add(fixtureSite("blog.test", presetHeavy))
	if err == nil {
		t.Error("adding duplicate site should return an error")
	}
}

// --- Remove ---

func TestRemoveSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	_ = s.Add(fixtureSite("blog.test", "standard"))
	if err := s.Remove("blog.test"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(s.Sites()) != 0 {
		t.Errorf("expected 0 sites after remove, got %d", len(s.Sites()))
	}
}

func TestRemoveNotFoundReturnsError(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	err := s.Remove("nope.test")
	if err == nil {
		t.Error("removing nonexistent site should return an error")
	}
}

// --- Find ---

func TestFindSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	_ = s.Add(fixtureSite("blog.test", "standard"))
	_ = s.Add(fixtureSite("shop.test", "woocommerce"))

	got, ok := s.Find("shop.test")
	if !ok {
		t.Fatal("Find() returned false for existing site")
	}
	if got.Name != "shop.test" {
		t.Errorf("Find() name = %q, want %q", got.Name, "shop.test")
	}
	if got.Preset != "woocommerce" {
		t.Errorf("Find() preset = %q, want %q", got.Preset, "woocommerce")
	}
}

func TestFindNotFound(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	if _, ok := s.Find("nope.test"); ok {
		t.Error("Find() should return false for missing site")
	}
}

func TestFind_ReturnsDefensiveCopy(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	_ = s.Add(Site{
		Name:         "custom.test",
		PHPVersion:   "83",
		Preset:       "custom",
		CustomPreset: &CustomPreset{PHPMemoryMB: 320, WorkerBudgetMB: 160},
		CreatedAt:    time.Date(2026, 4, 13, 22, 0, 0, 0, time.UTC),
	})

	got, ok := s.Find("custom.test")
	if !ok {
		t.Fatal("Find() returned false for existing site")
	}
	got.CustomPreset.PHPMemoryMB = 9999

	original, _ := s.Find("custom.test")
	if original.CustomPreset.PHPMemoryMB == 9999 {
		t.Error("Find() should return a defensive copy; mutating the result must not affect the store")
	}
}

func TestAddWithoutSave_NotPersisted(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	_ = s1.Add(fixtureSite("blog.test", "standard"))
	_ = s1.Close()

	s2, _ := Open(path)
	defer func() { _ = s2.Close() }()
	if _, ok := s2.Find("blog.test"); ok {
		t.Error("site added without Save should not be visible in a new store instance")
	}
}

// --- Update ---

func TestUpdateSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	_ = s.Add(fixtureSite("blog.test", "standard"))
	err := s.Update("blog.test", func(site *Site) {
		site.Preset = presetHeavy
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, _ := s.Find("blog.test")
	if got.Preset != presetHeavy {
		t.Errorf("after Update, preset = %q, want %q", got.Preset, presetHeavy)
	}
}

func TestUpdateNotFoundReturnsError(t *testing.T) {
	s, _ := Open(tempStorePath(t))

	err := s.Update("nope.test", func(site *Site) {
		site.Preset = presetHeavy
	})
	if err == nil {
		t.Error("updating nonexistent site should return an error")
	}
}

// --- Save / round-trip ---

func TestRoundTrip(t *testing.T) {
	path := tempStorePath(t)

	// First session: add two sites and save.
	s1, _ := Open(path)
	_ = s1.Add(fixtureSite("blog.test", "standard"))
	_ = s1.Add(fixtureSite("shop.test", "woocommerce"))
	if err := s1.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	_ = s1.Close()

	// Second session: load and verify.
	s2, _ := Open(path)
	defer func() { _ = s2.Close() }()

	sites := s2.Sites()
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites after round-trip, got %d", len(sites))
	}

	blog, ok := s2.Find("blog.test")
	if !ok {
		t.Fatal("blog.test not found after round-trip")
	}
	if blog.Preset != "standard" {
		t.Errorf("blog.test preset = %q, want %q", blog.Preset, "standard")
	}
	if blog.PHPVersion != "83" {
		t.Errorf("blog.test php_version = %q, want %q", blog.PHPVersion, "83")
	}
}

func TestRoundTripPreservesTimestamps(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	ts := time.Date(2026, 4, 13, 22, 30, 0, 0, time.UTC)
	_ = s1.Add(Site{
		Name:       "blog.test",
		PHPVersion: "83",
		Preset:     "standard",
		CreatedAt:  ts,
	})
	_ = s1.Save()
	_ = s1.Close()

	s2, _ := Open(path)
	defer func() { _ = s2.Close() }()

	got, _ := s2.Find("blog.test")
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

// --- CustomPreset ---

func TestCustomPresetRoundTrip(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	custom := &CustomPreset{
		PHPMemoryMB:    320,
		WorkerBudgetMB: 160,
	}
	_ = s1.Add(Site{
		Name:         "custom.test",
		PHPVersion:   "83",
		Preset:       "custom",
		CustomPreset: custom,
		CreatedAt:    time.Date(2026, 4, 13, 23, 0, 0, 0, time.UTC),
	})
	_ = s1.Save()
	_ = s1.Close()

	s2, _ := Open(path)
	defer func() { _ = s2.Close() }()

	got, _ := s2.Find("custom.test")
	if got.CustomPreset == nil {
		t.Fatal("CustomPreset lost in round-trip")
	}
	if got.CustomPreset.PHPMemoryMB != 320 {
		t.Errorf("PHPMemoryMB = %d, want 320", got.CustomPreset.PHPMemoryMB)
	}
	if got.CustomPreset.WorkerBudgetMB != 160 {
		t.Errorf("WorkerBudgetMB = %d, want 160", got.CustomPreset.WorkerBudgetMB)
	}
}

func TestCustomPresetOmittedWhenNil(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	_ = s1.Add(fixtureSite("blog.test", "standard"))
	_ = s1.Save()

	// Read raw JSON and confirm preset_custom is absent.
	raw, _ := os.ReadFile(path) //nolint:gosec // test reads from temp dir
	var doc map[string]json.RawMessage
	_ = json.Unmarshal(raw, &doc)

	var sites []json.RawMessage
	_ = json.Unmarshal(doc["sites"], &sites)
	if len(sites) == 0 {
		t.Fatal("expected at least one site in JSON")
	}

	var siteMap map[string]any
	_ = json.Unmarshal(sites[0], &siteMap)
	if _, ok := siteMap["preset_custom"]; ok {
		t.Error("preset_custom should be omitted when nil, but it was present")
	}
}

// --- SSL fields ---

func TestSiteJSON_SSLEnabled(t *testing.T) {
	original := Site{
		Name:       "ssl.test",
		Type:       "wp",
		SSLEnabled: true,
		CertPath:   "/etc/letsencrypt/live/ssl.test/fullchain.pem",
		KeyPath:    "/etc/letsencrypt/live/ssl.test/privkey.pem",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Site
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.SSLEnabled {
		t.Error("SSLEnabled = false, want true")
	}
	if got.CertPath != original.CertPath {
		t.Errorf("CertPath = %q, want %q", got.CertPath, original.CertPath)
	}
	if got.KeyPath != original.KeyPath {
		t.Errorf("KeyPath = %q, want %q", got.KeyPath, original.KeyPath)
	}
}

// --- CacheMode migration ---

func TestOpen_MigratesEmptyCacheModeForWP(t *testing.T) {
	path := tempStorePath(t)
	// Write a pre-CacheMode state file: WP site without cache_mode.
	raw := `{
  "sites": [
    {"name": "legacy-wp.test", "type": "wp", "php_version": "83", "preset": "standard", "created_at": "2026-04-13T22:00:00Z"},
    {"name": "legacy-default.test", "type": "", "php_version": "83", "preset": "standard", "created_at": "2026-04-13T22:00:00Z"},
    {"name": "legacy-html.test", "type": "html", "created_at": "2026-04-13T22:00:00Z"},
    {"name": "legacy-php.test", "type": "php", "php_version": "83", "preset": "standard", "created_at": "2026-04-13T22:00:00Z"}
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	wp, _ := s.Find("legacy-wp.test")
	if wp.CacheMode != "lscache" {
		t.Errorf("legacy wp CacheMode = %q, want lscache", wp.CacheMode)
	}
	def, _ := s.Find("legacy-default.test")
	if def.CacheMode != "lscache" {
		t.Errorf("legacy default-typed site CacheMode = %q, want lscache", def.CacheMode)
	}
	html, _ := s.Find("legacy-html.test")
	if html.CacheMode != "" {
		t.Errorf("legacy html CacheMode = %q, want empty", html.CacheMode)
	}
	php, _ := s.Find("legacy-php.test")
	if php.CacheMode != "" {
		t.Errorf("legacy php CacheMode = %q, want empty", php.CacheMode)
	}
}

func TestOpen_PreservesExplicitCacheModeNone(t *testing.T) {
	path := tempStorePath(t)
	raw := `{"sites":[{"name":"x.test","type":"wp","php_version":"83","preset":"standard","cache_mode":"none","created_at":"2026-04-13T22:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write: %v", err)
	}
	s, _ := Open(path)
	got, _ := s.Find("x.test")
	if got.CacheMode != "none" {
		t.Errorf("explicit CacheMode preserved as %q, want none", got.CacheMode)
	}
}

// --- Delete propagation ---

func TestSaveAfterRemove(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	_ = s1.Add(fixtureSite("blog.test", "standard"))
	_ = s1.Add(fixtureSite("shop.test", "woocommerce"))
	_ = s1.Save()
	_ = s1.Close()

	s2, _ := Open(path)
	_ = s2.Remove("blog.test")
	_ = s2.Save()
	_ = s2.Close()

	s3, _ := Open(path)
	defer func() { _ = s3.Close() }()

	if _, ok := s3.Find("blog.test"); ok {
		t.Error("blog.test should be gone after remove + save")
	}
	if _, ok := s3.Find("shop.test"); !ok {
		t.Error("shop.test should still exist")
	}
}
