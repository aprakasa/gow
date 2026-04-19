package ols

import (
	"os"
	"strings"
	"testing"
)

// findListenerEnd returns the position just after the closing brace of a block
// that starts at startPos.
func findListenerEnd(content string, startPos int) int {
	depth := 0
	for i := startPos; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(content)
}

// extractListenerBlock extracts the text of a named listener block from content.
func extractListenerBlock(content, listenerName string) string {
	header := "listener " + listenerName + " {"
	idx := strings.Index(content, header)
	if idx == -1 {
		return ""
	}
	end := findListenerEnd(content, idx)
	return content[idx:end]
}

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
