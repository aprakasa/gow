package site

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestClone_SourceNotFound(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	err := m.Clone(context.Background(), "nope.test", "dest.test")
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("error = %q, want 'source not found'", err.Error())
	}
}

func TestClone_SourceNotWP(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	if err := m.store.Add(state.Site{Name: "static.test", Type: "html", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	err := m.Clone(context.Background(), "static.test", "dest.test")
	if err == nil {
		t.Fatal("expected error for non-wp source")
	}
	if !strings.Contains(err.Error(), "only wp supported") {
		t.Errorf("error = %q, want 'only wp supported'", err.Error())
	}
}

func TestClone_DestAlreadyExists(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	if err := m.store.Add(state.Site{Name: "src.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	if err := m.store.Add(state.Site{Name: "dst.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	err := m.Clone(context.Background(), "src.test", "dst.test")
	if err == nil {
		t.Fatal("expected error for existing destination")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestClone_Success(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	// Seed source site with htdocs.
	if err := os.MkdirAll(filepath.Join(dir, "www", "src.test", "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "www", "src.test", "htdocs", "wp-config.php"), []byte("<?php\ndefine('DB_PASSWORD', 'oldpass');\n"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write: %v", err)
	}
	if err := m.store.Add(state.Site{Name: "src.test", Type: "wp", PHPVersion: "83", Preset: "standard", CacheMode: "lscache"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Clone(ctx, "src.test", "dst.test"); err != nil {
		t.Fatalf("Clone() = %v", err)
	}

	// Verify destination in store.
	s, ok := m.store.Find("dst.test")
	if !ok {
		t.Fatal("destination not in store")
	}
	if s.Type != "wp" {
		t.Errorf("Type = %q, want wp", s.Type)
	}
	if s.PHPVersion != "83" {
		t.Errorf("PHPVersion = %q, want 83", s.PHPVersion)
	}
	if s.CacheMode != "lscache" {
		t.Errorf("CacheMode = %q, want lscache", s.CacheMode)
	}

	// Verify command sequence.
	var sawDump, sawCreateDB, sawImportDB, sawSearchReplace, sawConfigSet bool
	for _, cmd := range rr.commands {
		all := strings.Join(cmd, " ")
		if strings.Contains(all, "mariadb-dump") {
			sawDump = true
		}
		if strings.Contains(all, "CREATE DATABASE") {
			sawCreateDB = true
		}
		if cmd[0] == "bash" && strings.Contains(all, "mariadb") {
			sawImportDB = true
		}
		if isWPCmd(cmd) && strings.Contains(all, "search-replace") {
			sawSearchReplace = true
		}
		if isWPCmd(cmd) && strings.Contains(all, "config set DB_NAME") {
			sawConfigSet = true
		}
	}
	if !sawDump {
		t.Error("expected mariadb-dump")
	}
	if !sawCreateDB {
		t.Error("expected CREATE DATABASE")
	}
	if !sawImportDB {
		t.Error("expected mariadb import")
	}
	if !sawSearchReplace {
		t.Error("expected wp search-replace")
	}
	if !sawConfigSet {
		t.Error("expected wp config set DB_NAME")
	}
}
