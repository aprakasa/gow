package ols

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureSSLListener_AddsListener(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	block := extractListenerBlock(content, "SSL")
	if block == "" {
		t.Fatal("SSL listener block not found")
	}
	if !strings.Contains(block, "address                  *:443") {
		t.Error("SSL listener missing address *:443")
	}
	if !strings.Contains(block, "secure                   1") {
		t.Error("SSL listener missing secure 1")
	}
	if !strings.Contains(block, "map                      Example *") {
		t.Error("SSL listener missing Example map entry")
	}
}

func TestEnsureSSLListener_Idempotent(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("first EnsureSSLListener() = %v", err)
	}
	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("second EnsureSSLListener() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	count := strings.Count(content, "listener SSL {")
	if count != 1 {
		t.Errorf("SSL listener block appears %d times, want 1", count)
	}
}

func TestEnsureSSLListener_PreservesExisting(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	if !strings.Contains(content, "listener Default {") {
		t.Error("Default listener should still be present")
	}
	if !strings.Contains(content, "address                  *:8088") {
		t.Error("Default listener address should be unchanged")
	}
}

func TestAddSSLMapEntry(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("AddSSLMapEntry() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	// Verify the map entry exists inside the SSL listener block.
	sslBlock := extractListenerBlock(content, "SSL")
	expectedMap := "map                      blog.test blog.test"
	if !strings.Contains(sslBlock, expectedMap) {
		t.Errorf("SSL listener block does not contain map entry for blog.test.\nBlock:\n%s", sslBlock)
	}

	// Verify it is NOT in the Default listener block.
	defaultBlock := extractListenerBlock(content, "Default")
	if strings.Contains(defaultBlock, expectedMap) {
		t.Error("blog.test map entry should not be in Default listener")
	}
}

func TestAddSSLMapEntry_Idempotent(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("first AddSSLMapEntry() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("second AddSSLMapEntry() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	count := strings.Count(content, "map                      blog.test blog.test")
	if count != 1 {
		t.Errorf("blog.test map entry appears %d times, want 1", count)
	}
}

func TestRemoveSSLMapEntry(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("AddSSLMapEntry() = %v", err)
	}
	if err := RemoveSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("RemoveSSLMapEntry() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	if strings.Contains(content, "map                      blog.test blog.test") {
		t.Error("blog.test map entry should be removed")
	}

	// SSL listener should still exist.
	if !strings.Contains(content, "listener SSL {") {
		t.Error("SSL listener block should still be present")
	}
}

func TestRemoveSSLMapEntry_NotFound(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := RemoveSSLMapEntry(p, "nope.test"); err != nil {
		t.Fatalf("RemoveSSLMapEntry() = %v, want nil for missing entry", err)
	}
}

func TestRemoveSSLListener(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("AddSSLMapEntry() = %v", err)
	}

	// Open batch editor and remove.
	hc, err := OpenHttpd(p)
	if err != nil {
		t.Fatalf("OpenHttpd() = %v", err)
	}
	hc.RemoveSSLListener()
	if err := hc.Save(); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	if strings.Contains(content, "listener SSL {") {
		t.Error("SSL listener block should be removed")
	}
	if strings.Contains(content, "secure                   1") {
		t.Error("SSL listener content should be gone")
	}
	// Default listener must be untouched.
	if !strings.Contains(content, "listener Default {") {
		t.Error("Default listener should still be present")
	}
}

func TestRemoveSSLListener_NoopWhenMissing(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	hc, err := OpenHttpd(p)
	if err != nil {
		t.Fatalf("OpenHttpd() = %v", err)
	}
	hc.RemoveSSLListener()
	if err := hc.Save(); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	if !strings.Contains(string(got), "listener Default {") {
		t.Error("Default listener should be unchanged")
	}
}

// TestAddSSLMapEntry_WithExistingDefaultMap verifies that AddSSLMapEntry correctly
// adds the entry to the SSL listener even when the same map text already exists
// in the Default listener (the bug fixed after code review).
func TestAddSSLMapEntry_WithExistingDefaultMap(t *testing.T) {
	// Start with blog.test already mapped in Default listener (simulates
	// RegisterVHost having run before AddSSLMapEntry).
	conf := baseHttpdConf()
	conf = strings.Replace(conf,
		"map                      Example *",
		"map                      Example *\n    map                      blog.test blog.test",
		1,
	)
	p := writeHttpdConf(t, conf)

	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener: %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("AddSSLMapEntry() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	// Must be in SSL listener specifically.
	sslBlock := extractListenerBlock(content, "SSL")
	if !strings.Contains(sslBlock, "map                      blog.test blog.test") {
		t.Errorf("SSL listener missing map entry for blog.test.\nSSL block:\n%s", sslBlock)
	}

	// Should be exactly 2 occurrences total (Default + SSL).
	count := strings.Count(content, "map                      blog.test blog.test")
	if count != 2 {
		t.Errorf("blog.test map appears %d times, want 2 (Default + SSL)", count)
	}
}

// TestRemoveSSLMapEntry_OnlyRemovesFromSSL verifies that RemoveSSLMapEntry
// removes from the SSL listener, not the Default listener.
func TestRemoveSSLMapEntry_OnlyRemovesFromSSL(t *testing.T) {
	conf := baseHttpdConf()
	// Add blog.test to both Default and SSL listener.
	conf = strings.Replace(conf,
		"map                      Example *",
		"map                      Example *\n    map                      blog.test blog.test",
		1,
	)
	p := writeHttpdConf(t, conf)
	if err := EnsureSSLListener(p); err != nil {
		t.Fatalf("EnsureSSLListener() = %v", err)
	}
	if err := AddSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("AddSSLMapEntry() = %v", err)
	}

	if err := RemoveSSLMapEntry(p, "blog.test"); err != nil {
		t.Fatalf("RemoveSSLMapEntry() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	content := string(got)

	// Default listener should still have the map entry.
	defaultBlock := extractListenerBlock(content, "Default")
	if !strings.Contains(defaultBlock, "map                      blog.test blog.test") {
		t.Error("Default listener should still have blog.test map entry")
	}

	// SSL listener should NOT have it.
	sslBlock := extractListenerBlock(content, "SSL")
	if strings.Contains(sslBlock, "map                      blog.test blog.test") {
		t.Error("SSL listener should NOT have blog.test map entry after removal")
	}
}
