package site

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestRestore_ArchiveNotFound(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	err := m.Restore(context.Background(), "test.com", "/no/such/file.tar.gz")
	if err == nil {
		t.Fatal("expected error for missing archive")
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Errorf("error = %q, want 'archive'", err.Error())
	}
}

func TestRestore_WP_Success(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	// Create a fake backup archive.
	archivePath := createTestArchive(t, dir, state.Site{
		Name:       "old.test",
		Type:       "wp",
		PHPVersion: "83",
		Preset:     "standard",
		CacheMode:  "lscache",
	})

	if err := m.Restore(ctx, "new.test", archivePath); err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	// Verify site was created in store.
	s, ok := m.store.Find("new.test")
	if !ok {
		t.Fatal("site not found in store")
	}
	if s.Type != "wp" {
		t.Errorf("Type = %q, want wp", s.Type)
	}
	if s.PHPVersion != "83" {
		t.Errorf("PHPVersion = %q, want 83", s.PHPVersion)
	}

	// Verify command sequence: mariadb create, mariadb import,
	// wp config set x3, wp search-replace.
	var sawCreateDB, sawImportDB, sawSearchReplace bool
	for _, cmd := range rr.commands {
		all := strings.Join(cmd, " ")
		if strings.Contains(all, "CREATE DATABASE") {
			sawCreateDB = true
		}
		if cmd[0] == "bash" && strings.Contains(all, "mariadb") {
			sawImportDB = true
		}
		if isWPCmd(cmd) && strings.Contains(all, "search-replace") {
			sawSearchReplace = true
		}
	}
	if !sawCreateDB {
		t.Error("expected CREATE DATABASE")
	}
	if !sawImportDB {
		t.Error("expected mariadb import")
	}
	if !sawSearchReplace {
		t.Error("expected wp search-replace for domain rename")
	}
}

func TestRestore_SameDomain_NoSearchReplace(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	archivePath := createTestArchive(t, dir, state.Site{
		Name:       "same.test",
		Type:       "wp",
		PHPVersion: "83",
		Preset:     "standard",
	})

	if err := m.Restore(ctx, "same.test", archivePath); err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	for _, cmd := range rr.commands {
		if isWPCmd(cmd) && strings.Contains(strings.Join(cmd, " "), "search-replace") {
			t.Error("search-replace should not run when domain unchanged")
		}
	}
}

func TestRestore_HTML_NoDBOps(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	archivePath := createTestArchive(t, dir, state.Site{
		Name:   "static.test",
		Type:   "html",
		Preset: "standard",
	})

	if err := m.Restore(ctx, "static.test", archivePath); err != nil {
		t.Fatalf("Restore() = %v", err)
	}

	for _, cmd := range rr.commands {
		if cmd[0] == "mariadb" || strings.Contains(strings.Join(cmd, " "), "mariadb") {
			t.Error("HTML site should not trigger mariadb commands")
		}
	}
}

// createTestArchive builds a minimal .tar.gz with site.json, db.sql, and
// htdocs/index.php at the expected paths.
func createTestArchive(t *testing.T, dir string, site state.Site) string {
	t.Helper()
	staging := filepath.Join(dir, "archive-staging")
	if err := os.MkdirAll(filepath.Join(staging, "htdocs"), 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir staging: %v", err)
	}

	siteJSON, _ := json.MarshalIndent(site, "", "  ")
	if err := os.WriteFile(filepath.Join(staging, "site.json"), siteJSON, 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write site.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staging, "db.sql"), []byte("-- fake dump\n"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write db.sql: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staging, "htdocs", "index.php"), []byte("<?php"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write index.php: %v", err)
	}
	wpConfig := []byte("<?php\ndefine('DB_PASSWORD', 'oldpass');\n")
	if err := os.WriteFile(filepath.Join(staging, "htdocs", "wp-config.php"), wpConfig, 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write wp-config.php: %v", err)
	}

	archivePath := filepath.Join(dir, "test-backup.tar.gz")
	f, err := os.Create(archivePath) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close() //nolint:errcheck // test file

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	writeTarFile(t, tw, "site.json", siteJSON)
	writeTarFile(t, tw, "db.sql", []byte("-- fake dump\n"))
	writeTarFile(t, tw, "htdocs/index.php", []byte("<?php"))
	writeTarFile(t, tw, "htdocs/wp-config.php", wpConfig)

	tw.Close() //nolint:errcheck,gosec // test cleanup
	gw.Close() //nolint:errcheck,gosec // test cleanup
	return archivePath
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	hdr := &tar.Header{Name: name, Size: int64(len(data)), Mode: 0o644}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header %s: %v", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write %s: %v", name, err)
	}
}
