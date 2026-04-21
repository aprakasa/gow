package site

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestWP_PrependsAllowRootAndPath(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.WP(context.Background(), "blog.test", nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"plugin", "list"}); err != nil {
		t.Fatalf("WP() = %v", err)
	}

	if len(rr.commands) != 1 {
		t.Fatalf("expected 1 wp call, got %d: %v", len(rr.commands), rr.commands)
	}
	cmd := rr.commands[0]
	if cmd[0] != "wp" {
		t.Errorf("expected wp binary, got %q", cmd[0])
	}
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "--allow-root") {
		t.Errorf("expected --allow-root, got: %s", joined)
	}
	if !strings.Contains(joined, "--path=") {
		t.Errorf("expected --path=, got: %s", joined)
	}
	if !strings.Contains(joined, "/blog.test/htdocs") {
		t.Errorf("expected htdocs path, got: %s", joined)
	}
}

func TestWP_PreservesUserArgsOrder(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.WP(context.Background(), "blog.test", nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"option", "get", "siteurl"}); err != nil {
		t.Fatalf("WP() = %v", err)
	}

	cmd := rr.commands[0]
	// User args should appear in order. Implementation detail: we allow them
	// either before or after the auto-injected flags, as long as internal
	// order is preserved.
	indexOf := func(s string) int {
		for i, a := range cmd {
			if a == s {
				return i
			}
		}
		return -1
	}
	iOpt, iGet, iURL := indexOf("option"), indexOf("get"), indexOf("siteurl")
	if iOpt < 0 || iGet < 0 || iURL < 0 {
		t.Fatalf("user args missing from call: %v", cmd)
	}
	if iOpt >= iGet || iGet >= iURL {
		t.Errorf("user args out of order: option@%d, get@%d, siteurl@%d", iOpt, iGet, iURL)
	}
}

func TestWP_Multisite_DoesNotAutoInjectURL(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "net.test", Type: "wp", PHPVersion: "83", Preset: "standard", Multisite: "subdirectory"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.WP(context.Background(), "net.test", nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"plugin", "list"}); err != nil {
		t.Fatalf("WP() = %v", err)
	}

	for _, a := range rr.commands[0] {
		if strings.HasPrefix(a, "--url=") {
			t.Errorf("WP should not auto-inject --url for multisite (user picks blog context); got %q", a)
		}
	}
}

func TestWP_SiteNotFound_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	err := m.WP(context.Background(), "nope.test", nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"plugin", "list"})
	if err == nil {
		t.Fatal("expected error for missing site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestWP_HTMLSite_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	if err := m.store.Add(state.Site{Name: "static.test", Type: "html"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	err := m.WP(context.Background(), "static.test", nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"plugin", "list"})
	if err == nil {
		t.Fatal("expected error for html site")
	}
}
