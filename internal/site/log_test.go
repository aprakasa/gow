package site

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestLog_ErrorMode_UsesErrorLog(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Log(context.Background(), "blog.test", "error", 100, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Log() = %v", err)
	}

	found := false
	for _, cmd := range rr.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "blog.test.error.log") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tail of blog.test.error.log, got calls: %v", rr.commands)
	}
}

func TestLog_AccessMode_UsesAccessLog(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Log(context.Background(), "blog.test", "access", 100, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Log() = %v", err)
	}

	found := false
	for _, cmd := range rr.commands {
		if strings.Contains(strings.Join(cmd, " "), "blog.test.access.log") {
			found = true
		}
	}
	if !found {
		t.Error("expected tail of blog.test.access.log")
	}
}

func TestLog_NoFollow_OmitsFFlag(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Log(context.Background(), "blog.test", "error", 100, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Log() = %v", err)
	}

	for _, cmd := range rr.commands {
		for _, a := range cmd {
			if a == "-f" {
				t.Errorf("should not pass -f when follow=false, got: %v", cmd)
			}
		}
	}
}

func TestLog_Follow_PassesFFlag(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Log(context.Background(), "blog.test", "error", 100, true, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Log() = %v", err)
	}

	foundF := false
	for _, cmd := range rr.commands {
		for _, a := range cmd {
			if a == "-f" {
				foundF = true
			}
		}
	}
	if !foundF {
		t.Error("expected -f flag when follow=true")
	}
}

func TestLog_LinesFlag_PassesN(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Log(context.Background(), "blog.test", "error", 42, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Log() = %v", err)
	}

	found := false
	for _, cmd := range rr.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "-n") && strings.Contains(joined, "42") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected -n 42, got calls: %v", rr.commands)
	}
}

func TestLog_SiteNotFound_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	err := m.Log(context.Background(), "nope.test", "error", 100, false, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestLog_InvalidMode_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	err := m.Log(context.Background(), "blog.test", "bogus", 100, false, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}
