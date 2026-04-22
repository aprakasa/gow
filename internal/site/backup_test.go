package site

import (
	"context"
	"fmt"
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

func TestListBackups_Sorted(t *testing.T) {
	tmp := t.TempDir()
	orig := BackupDir
	BackupDir = tmp
	t.Cleanup(func() { BackupDir = orig })

	for _, name := range []string{
		"example.com-20260101-120000.tar.gz",
		"example.com-20260115-080000.tar.gz",
		"example.com-20260301-000000.tar.gz",
		"other.com-20260101-120000.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte{}, 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("write: %v", err)
		}
	}

	got, err := ListBackups("example.com")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	want := []string{
		"example.com-20260101-120000.tar.gz",
		"example.com-20260115-080000.tar.gz",
		"example.com-20260301-000000.tar.gz",
	}
	if len(got) != len(want) {
		t.Fatalf("ListBackups = %d entries, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestListBackups_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	orig := BackupDir
	BackupDir = tmp
	t.Cleanup(func() { BackupDir = orig })

	got, err := ListBackups("example.com")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListBackups = %v, want empty", got)
	}
}

func TestPruneBackups_KeepsNewest(t *testing.T) {
	tmp := t.TempDir()
	orig := BackupDir
	BackupDir = tmp
	t.Cleanup(func() { BackupDir = orig })

	m, _ := setupManagerWithRunner(t, &recordingRunner{})

	// Create 10 fake archives.
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("test.com-202601%02d-120000.tar.gz", i)
		if err := os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("write: %v", err)
		}
	}

	if err := m.PruneBackups("test.com", 5); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	remaining, _ := ListBackups("test.com")
	if len(remaining) != 5 {
		t.Fatalf("PruneBackups left %d files, want 5", len(remaining))
	}
	// Should keep the 5 newest (Jan 06-10).
	for _, f := range remaining {
		if strings.Contains(f, "20260101") || strings.Contains(f, "20260102") ||
			strings.Contains(f, "20260103") || strings.Contains(f, "20260104") ||
			strings.Contains(f, "20260105") {
			t.Errorf("old backup should have been pruned: %s", f)
		}
	}
}

func TestPruneBackups_NoPruneNeeded(t *testing.T) {
	tmp := t.TempDir()
	orig := BackupDir
	BackupDir = tmp
	t.Cleanup(func() { BackupDir = orig })

	m, _ := setupManagerWithRunner(t, &recordingRunner{})

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("test.com-202601%02d-120000.tar.gz", i)
		if err := os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("write: %v", err)
		}
	}

	if err := m.PruneBackups("test.com", 5); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	remaining, _ := ListBackups("test.com")
	if len(remaining) != 3 {
		t.Errorf("PruneBackups left %d files, want 3", len(remaining))
	}
}

func TestPruneBackups_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	orig := BackupDir
	BackupDir = filepath.Join(tmp, "nonexistent")
	t.Cleanup(func() { BackupDir = orig })

	m, _ := setupManagerWithRunner(t, &recordingRunner{})

	if err := m.PruneBackups("test.com", 5); err != nil {
		t.Fatalf("PruneBackups on missing dir: %v", err)
	}
}
