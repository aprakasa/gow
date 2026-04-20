package site

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/aprakasa/gow/internal/state"
)

// Restore recreates a site from a backup archive. It reads the archived state
// entry, creates site infrastructure via Create, then restores the document
// root, database, and vhost config. The site does not need to exist beforehand.
// SSL is NOT restored — run `gow site ssl` separately after restore.
func (m *Manager) Restore(ctx context.Context, name, archivePath string) error {
	if _, err := os.Stat(archivePath); err != nil {
		return fmt.Errorf("site: restore %s: archive %s: %w", name, archivePath, err)
	}

	tmp, err := os.MkdirTemp("", "gow-restore-"+name)
	if err != nil {
		return fmt.Errorf("site: restore %s: tmpdir: %w", name, err)
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // cleanup

	// Extract archive.
	if err := extractArchive(archivePath, tmp); err != nil {
		return fmt.Errorf("site: restore %s: extract: %w", name, err)
	}

	// Read archived site state.
	siteJSON, err := os.ReadFile(filepath.Join(tmp, "site.json")) //nolint:gosec // backup metadata
	if err != nil {
		return fmt.Errorf("site: restore %s: read site.json: %w", name, err)
	}
	var origSite state.Site
	if err := json.Unmarshal(siteJSON, &origSite); err != nil {
		return fmt.Errorf("site: restore %s: parse site.json: %w", name, err)
	}

	st := siteType(origSite)

	// Create infrastructure (doc root, unix user, vhost, state entry).
	if err := m.Create(ctx, name, st, origSite.PHPVersion, origSite.Preset, origSite.CacheMode, origSite.Multisite, origSite.CustomPreset); err != nil {
		return fmt.Errorf("site: restore %s: create: %w", name, err)
	}

	// Rollback on subsequent failure.
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = m.Delete(context.Background(), name)
	}()

	docRoot := filepath.Join(m.webRoot, name, "htdocs")

	// Restore document root: clear auto-created files, copy archived ones.
	entries, _ := os.ReadDir(docRoot)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(docRoot, e.Name()))
	}
	archivedHtdocs := filepath.Join(tmp, "htdocs")
	if _, err := os.Stat(archivedHtdocs); err == nil {
		if err := m.runner.Run(ctx, "cp", "-a", archivedHtdocs+"/.", docRoot); err != nil {
			return fmt.Errorf("site: restore %s: copy htdocs: %w", name, err)
		}
	}

	// Restore database for WP sites.
	if st == "wp" {
		dbDump := filepath.Join(tmp, "db.sql")
		if _, err := os.Stat(dbDump); err != nil {
			return fmt.Errorf("site: restore %s: missing db.sql in archive: %w", name, err)
		}

		dbName := dbNameFromDomain(name)
		dbPass := generatePassword(20)
		qDB := quoteIdent(dbName)
		qUser := quoteIdent(dbName)
		qPass := sqlEscape(dbPass)

		sql := fmt.Sprintf(
			"CREATE DATABASE IF NOT EXISTS %s; CREATE USER IF NOT EXISTS %s@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON %s.* TO %s@'localhost'; FLUSH PRIVILEGES;",
			qDB, qUser, qPass, qDB, qUser,
		)
		if err := m.runner.Run(ctx, "mariadb", "-e", sql); err != nil {
			return fmt.Errorf("site: restore %s: create db: %w", name, err)
		}
		if err := m.runner.Run(ctx, "bash", "-c", fmt.Sprintf("mariadb -D %s < %s", dbName, dbDump)); err != nil {
			return fmt.Errorf("site: restore %s: import db: %w", name, err)
		}

		// Rewrite wp-config.php DB credentials via WP-CLI.
		if err := m.runner.Run(ctx, "wp", "config", "set", "DB_NAME", dbName, "--path="+docRoot, "--allow-root"); err != nil {
			return fmt.Errorf("site: restore %s: set DB_NAME: %w", name, err)
		}
		if err := m.runner.Run(ctx, "wp", "config", "set", "DB_USER", dbName, "--path="+docRoot, "--allow-root"); err != nil {
			return fmt.Errorf("site: restore %s: set DB_USER: %w", name, err)
		}
		if err := m.runner.Run(ctx, "wp", "config", "set", "DB_PASSWORD", dbPass, "--path="+docRoot, "--allow-root"); err != nil {
			return fmt.Errorf("site: restore %s: set DB_PASSWORD: %w", name, err)
		}

		// Search-replace old domain if restoring to a different name.
		origDomain := origSite.Name
		if origDomain != "" && origDomain != name {
			if err := m.runner.Run(ctx, "wp", "search-replace", origDomain, name, "--all-tables", "--path="+docRoot, "--allow-root"); err != nil {
				return fmt.Errorf("site: restore %s: search-replace: %w", name, err)
			}
		}
	}

	// Fix ownership.
	siteEntry, _ := m.store.Find(name)
	if siteEntry.UnixUser != "" {
		siteRoot := filepath.Join(m.webRoot, name)
		_ = m.runner.Run(ctx, "chown", "-R", siteEntry.UnixUser+":"+siteEntry.UnixUser, siteRoot)
	}

	committed = true
	return nil
}

// quoteIdent is a minimal backtick wrapper for DB identifiers used by
// restore/clone. It only allows safe characters.
func quoteIdent(name string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)
	return "`" + safe + "`"
}

// sqlEscape escapes a string for MariaDB single-quoted literals.
func sqlEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		"\x00", `\0`,
		"\n", `\n`,
	)
	return r.Replace(s)
}

// generatePassword returns a cryptographically random alphanumeric password.
func generatePassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, length)
	for i := range buf {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		buf[i] = chars[n.Int64()]
	}
	return string(buf)
}

// extractArchive extracts a .tar.gz archive into dst using pure Go (no shell
// commands), so it works in tests with mocked runners.
//
//nolint:gosec // G301/G304/G305: archive extraction from trusted backup files
func extractArchive(archivePath, dst string) error {
	f, err := os.Open(archivePath) //nolint:gosec // path validated by caller
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only file

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close() //nolint:errcheck // reader

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode)) //nolint:gosec // extracted file
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close() //nolint:errcheck // written and closing
				return err
			}
			out.Close() //nolint:errcheck // written and closing
		}
	}
	return nil
}
