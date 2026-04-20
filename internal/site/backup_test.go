package site

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestBackup_SiteNotFound(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	_, err := m.Backup(context.Background(), "nope.test")
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestBackup_WP_Success(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	// Seed site with htdocs and a fake file.
	if err := os.MkdirAll(filepath.Join(dir, "www", "wp.test", "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "www", "wp.test", "htdocs", "index.php"), []byte("<?php"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write: %v", err)
	}
	if err := m.store.Add(state.Site{Name: "wp.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	// Redirect backup dir to temp.
	orig := BackupDir
	BackupDir = filepath.Join(dir, "backups")
	t.Cleanup(func() { BackupDir = orig })

	path, err := m.Backup(ctx, "wp.test")
	if err != nil {
		t.Fatalf("Backup() = %v", err)
	}
	if !strings.HasSuffix(path, ".tar.gz") {
		t.Errorf("archive path = %q, want .tar.gz suffix", path)
	}

	// Verify commands issued.
	var sawDump, sawTar, sawCp bool
	for _, cmd := range rr.commands {
		all := strings.Join(cmd, " ")
		if strings.Contains(all, "mariadb-dump") {
			sawDump = true
		}
		if cmd[0] == "tar" {
			sawTar = true
		}
		if cmd[0] == "cp" && strings.Contains(all, "htdocs") {
			sawCp = true
		}
	}
	if !sawDump {
		t.Error("expected mariadb-dump for WP site")
	}
	if !sawTar {
		t.Error("expected tar command")
	}
	if !sawCp {
		t.Error("expected cp for htdocs")
	}
}

func TestBackup_HTML_NoDBDump(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	if err := os.MkdirAll(filepath.Join(dir, "www", "static.test", "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir: %v", err)
	}
	if err := m.store.Add(state.Site{Name: "static.test", Type: "html", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	orig := BackupDir
	BackupDir = filepath.Join(dir, "backups")
	t.Cleanup(func() { BackupDir = orig })

	if _, err := m.Backup(ctx, "static.test"); err != nil {
		t.Fatalf("Backup() = %v", err)
	}

	for _, cmd := range rr.commands {
		if cmd[0] == "mariadb-dump" {
			t.Error("HTML site should not trigger mariadb-dump")
		}
	}
}
