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
	s.Close()

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
	defer s.Close()

	if len(s.Sites()) != 0 {
		t.Errorf("expected 0 sites, got %d", len(s.Sites()))
	}
}

// --- Add ---

func TestAddSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))
	defer s.Close()

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
	defer s.Close()

	_ = s.Add(fixtureSite("blog.test", "standard"))
	err := s.Add(fixtureSite("blog.test", presetHeavy))
	if err == nil {
		t.Error("adding duplicate site should return an error")
	}
}

// --- Remove ---

func TestRemoveSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))
	defer s.Close()

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
	defer s.Close()

	err := s.Remove("nope.test")
	if err == nil {
		t.Error("removing nonexistent site should return an error")
	}
}

// --- Find ---

func TestFindSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))
	defer s.Close()

	_ = s.Add(fixtureSite("blog.test", "standard"))
	_ = s.Add(fixtureSite("shop.test", "woocommerce"))

	got := s.Find("shop.test")
	if got == nil {
		t.Fatal("Find() returned nil for existing site")
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
	defer s.Close()

	if got := s.Find("nope.test"); got != nil {
		t.Error("Find() should return nil for missing site")
	}
}

// --- Update ---

func TestUpdateSite(t *testing.T) {
	s, _ := Open(tempStorePath(t))
	defer s.Close()

	_ = s.Add(fixtureSite("blog.test", "standard"))
	err := s.Update("blog.test", func(site *Site) {
		site.Preset = presetHeavy
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got := s.Find("blog.test")
	if got.Preset != presetHeavy {
		t.Errorf("after Update, preset = %q, want %q", got.Preset, presetHeavy)
	}
}

func TestUpdateNotFoundReturnsError(t *testing.T) {
	s, _ := Open(tempStorePath(t))
	defer s.Close()

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
	s1.Close()

	// Second session: load and verify.
	s2, _ := Open(path)
	defer s2.Close()

	sites := s2.Sites()
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites after round-trip, got %d", len(sites))
	}

	blog := s2.Find("blog.test")
	if blog == nil {
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
	s1.Close()

	s2, _ := Open(path)
	defer s2.Close()

	got := s2.Find("blog.test")
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
	s1.Close()

	s2, _ := Open(path)
	defer s2.Close()

	got := s2.Find("custom.test")
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
	s1.Close()

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

// --- Delete propagation ---

func TestSaveAfterRemove(t *testing.T) {
	path := tempStorePath(t)

	s1, _ := Open(path)
	_ = s1.Add(fixtureSite("blog.test", "standard"))
	_ = s1.Add(fixtureSite("shop.test", "woocommerce"))
	_ = s1.Save()
	s1.Close()

	s2, _ := Open(path)
	_ = s2.Remove("blog.test")
	_ = s2.Save()
	s2.Close()

	s3, _ := Open(path)
	defer s3.Close()

	if s3.Find("blog.test") != nil {
		t.Error("blog.test should be gone after remove + save")
	}
	if s3.Find("shop.test") == nil {
		t.Error("shop.test should still exist")
	}
}
