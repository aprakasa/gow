package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aprakasa/gow/internal/dbsql"
	"github.com/aprakasa/gow/internal/stack"
)

// Clone creates a new site that is a copy of src at dst. The source must be
// type "wp". It creates the destination infrastructure via Create, copies the
// document root, dumps and re-imports the database, rewrites wp-config.php
// with new DB credentials, and runs wp search-replace to update domain refs.
func (m *Manager) Clone(ctx context.Context, src, dst string) error {
	srcSite, ok := m.store.Find(src)
	if !ok {
		return fmt.Errorf("site: clone %s → %s: source not found", src, dst)
	}
	if siteType(srcSite) != "wp" {
		return fmt.Errorf("site: clone %s → %s: source is type %q, only wp supported", src, dst, siteType(srcSite))
	}
	if _, exists := m.store.Find(dst); exists {
		return fmt.Errorf("site: clone %s → %s: destination already exists", src, dst)
	}

	// Create destination infrastructure.
	if err := m.Create(ctx, dst, srcSite.Type, srcSite.PHPVersion, srcSite.Preset, srcSite.CacheMode, srcSite.Multisite, srcSite.CustomPreset); err != nil {
		return fmt.Errorf("site: clone %s → %s: create dest: %w", src, dst, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		_ = m.Delete(context.Background(), dst)
	}()

	srcDocRoot := filepath.Join(m.webRoot, src, "htdocs")
	dstDocRoot := filepath.Join(m.webRoot, dst, "htdocs")

	// Clear auto-created files and copy source docroot.
	entries, _ := os.ReadDir(dstDocRoot)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dstDocRoot, e.Name()))
	}
	if err := m.runner.Run(ctx, "cp", "-a", srcDocRoot+"/.", dstDocRoot); err != nil {
		return fmt.Errorf("site: clone %s → %s: copy htdocs: %w", src, dst, err)
	}

	// Dump source database.
	srcDB := dbsql.DBName(src)
	dumpOut, err := m.runner.Output(ctx, "mariadb-dump", srcDB)
	if err != nil {
		return fmt.Errorf("site: clone %s → %s: dump db: %w", src, dst, err)
	}
	tmpDump, err := os.CreateTemp("", "gow-clone-"+dst+"-*.sql")
	if err != nil {
		return fmt.Errorf("site: clone %s → %s: tmpfile: %w", src, dst, err)
	}
	defer os.Remove(tmpDump.Name()) //nolint:errcheck // cleanup
	if _, err := tmpDump.WriteString(dumpOut); err != nil {
		tmpDump.Close() //nolint:errcheck,gosec // closing after write
		return fmt.Errorf("site: clone %s → %s: write dump: %w", src, dst, err)
	}
	tmpDump.Close() //nolint:errcheck,gosec // closing after write

	// Create destination database.
	dstDB := dbsql.DBName(dst)
	dbPass := dbsql.Password(20)
	qDB, err := dbsql.QuoteIdent(dstDB)
	if err != nil {
		return fmt.Errorf("site: clone %s → %s: %w", src, dst, err)
	}
	qUser, err := dbsql.QuoteIdent(dstDB)
	if err != nil {
		return fmt.Errorf("site: clone %s → %s: %w", src, dst, err)
	}
	qPass := dbsql.Escape(dbPass)

	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS %s; CREATE USER IF NOT EXISTS %s@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON %s.* TO %s@'localhost'; FLUSH PRIVILEGES;",
		qDB, qUser, qPass, qDB, qUser,
	)
	if err := m.runner.Run(ctx, "mariadb", "-e", sql); err != nil {
		return fmt.Errorf("site: clone %s → %s: create db: %w", src, dst, err)
	}

	// Import dump into destination database.
	if err := m.runner.Run(ctx, "bash", "-c", fmt.Sprintf("mariadb -D %s < %s", dstDB, tmpDump.Name())); err != nil {
		return fmt.Errorf("site: clone %s → %s: import db: %w", src, dst, err)
	}

	// Rewrite wp-config.php DB credentials. DB_NAME/DB_USER go through wp-cli
	// (they are not secrets). DB_PASSWORD is written directly to avoid argv
	// exposure via /proc/<pid>/cmdline while wp-cli runs.
	if err := m.runner.Run(ctx, stack.WPCLIBinPath, "config", "set", "DB_NAME", dstDB, "--path="+dstDocRoot, "--allow-root"); err != nil {
		return fmt.Errorf("site: clone %s → %s: set DB_NAME: %w", src, dst, err)
	}
	if err := m.runner.Run(ctx, stack.WPCLIBinPath, "config", "set", "DB_USER", dstDB, "--path="+dstDocRoot, "--allow-root"); err != nil {
		return fmt.Errorf("site: clone %s → %s: set DB_USER: %w", src, dst, err)
	}
	if err := writeWPConfigPassword(dstDocRoot, dbPass); err != nil {
		return fmt.Errorf("site: clone %s → %s: set DB_PASSWORD: %w", src, dst, err)
	}

	// Search-replace domain references.
	if err := m.runner.Run(ctx, stack.WPCLIBinPath, "search-replace", src, dst, "--all-tables", "--path="+dstDocRoot, "--allow-root"); err != nil {
		return fmt.Errorf("site: clone %s → %s: search-replace: %w", src, dst, err)
	}

	// Fix ownership.
	dstSite, _ := m.store.Find(dst)
	if dstSite.UnixUser != "" {
		_ = m.runner.Run(ctx, "chown", "-R", dstSite.UnixUser+":"+dstSite.UnixUser, filepath.Join(m.webRoot, dst))
	}

	committed = true
	return nil
}
