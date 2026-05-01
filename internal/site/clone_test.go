package site

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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
	for _, s := range rr.stdins {
		if strings.Contains(s, "CREATE DATABASE") {
			sawCreateDB = true
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

// dbPassRE pulls the DB_PASSWORD literal back out of wp-config.php so the
// no-argv-leak test can search for it.
var dbPassRE = regexp.MustCompile(`define\(\s*'DB_PASSWORD'\s*,\s*'([^']+)'\s*\)`)

// TestClone_DBPasswordNeverInArgv is the regression test for commit 7cfe510
// ("pipe SQL with credentials via stdin"). The generated DB password must:
//   - never appear in any recorded argv (would leak via /proc/<pid>/cmdline,
//     audit logs, ps output)
//   - appear inside a Stream stdin payload alongside `IDENTIFIED BY`
//
// If this fails, someone reverted the hardening — likely by switching the
// CREATE USER SQL back to `mariadb -e <sql>` or to `wp config set DB_PASSWORD`.
func TestClone_DBPasswordNeverInArgv(t *testing.T) {
	ctx := context.Background()
	rr := &recordingRunner{}
	m, dir := setupManagerWithRunner(t, rr)

	srcDocRoot := filepath.Join(dir, "www", "src.test", "htdocs")
	if err := os.MkdirAll(srcDocRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDocRoot, "wp-config.php"), []byte("<?php\ndefine('DB_PASSWORD', 'oldpass');\n"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatalf("write wp-config: %v", err)
	}
	if err := m.store.Add(state.Site{Name: "src.test", Type: "wp", PHPVersion: "83", Preset: "standard", CacheMode: "lscache"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Clone(ctx, "src.test", "dst.test"); err != nil {
		t.Fatalf("Clone() = %v", err)
	}

	// Read the password the clone actually generated and persisted.
	dstWPConfig := filepath.Join(dir, "www", "dst.test", "htdocs", "wp-config.php")
	cfg, err := os.ReadFile(dstWPConfig) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read dst wp-config: %v", err)
	}
	match := dbPassRE.FindSubmatch(cfg)
	if match == nil {
		t.Fatalf("DB_PASSWORD not found in dst wp-config.php; content:\n%s", cfg)
	}
	dbPass := string(match[1])
	if dbPass == "" || dbPass == "oldpass" || dbPass == "placeholder" {
		t.Fatalf("expected freshly generated DB password in wp-config.php, got %q", dbPass)
	}

	// Negative: must not leak into any subprocess argv.
	for _, cmd := range rr.commands {
		for i, a := range cmd {
			if strings.Contains(a, dbPass) {
				t.Fatalf("DB password leaked in argv at index %d:\n  cmd: %v\n  password must be delivered via stdin only", i, cmd)
			}
		}
	}

	// Positive: must appear inside a Stream stdin payload, in a CREATE USER
	// statement. Confirms the credential really did travel via stdin and the
	// negative assertion above isn't passing because the fix is silently broken.
	var found bool
	for _, s := range rr.stdins {
		if strings.Contains(s, dbPass) && strings.Contains(s, "IDENTIFIED BY") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("DB password not found in any Stream stdin; expected CREATE USER ... IDENTIFIED BY '<pass>' delivered via stdin")
	}
}
