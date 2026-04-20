package site

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBackupDir = "/var/backups/gow"

// BackupDir is the output directory for backup archives. Tests override this.
var BackupDir = defaultBackupDir

// dbNameFromDomain returns the MariaDB database name for a domain.
func dbNameFromDomain(domain string) string {
	return "wp_" + strings.ReplaceAll(domain, ".", "_")
}

// Backup creates a tar.gz archive of the site's state, document root,
// database dump, and vhost configuration. It returns the archive path.
// For non-WP sites the DB dump is skipped.
func (m *Manager) Backup(ctx context.Context, name string) (string, error) {
	site, ok := m.store.Find(name)
	if !ok {
		return "", fmt.Errorf("site: backup %s: not found", name)
	}

	tmp, err := os.MkdirTemp("", "gow-backup-"+name)
	if err != nil {
		return "", fmt.Errorf("site: backup %s: tmpdir: %w", name, err)
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // cleanup

	// Serialize site state.
	siteJSON, err := json.MarshalIndent(site, "", "  ")
	if err != nil {
		return "", fmt.Errorf("site: backup %s: marshal state: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "site.json"), siteJSON, 0o644); err != nil { //nolint:gosec // backup metadata
		return "", fmt.Errorf("site: backup %s: write site.json: %w", name, err)
	}

	// Dump MariaDB database for WP sites.
	st := siteType(site)
	if st == "wp" {
		dbName := dbNameFromDomain(name)
		dumpPath := filepath.Join(tmp, "db.sql")
		if out, err := m.runner.Output(ctx, "mariadb-dump", dbName); err != nil {
			return "", fmt.Errorf("site: backup %s: db dump: %w", name, err)
		} else if err := os.WriteFile(dumpPath, []byte(out), 0o644); err != nil { //nolint:gosec // db dump
			return "", fmt.Errorf("site: backup %s: write db.sql: %w", name, err)
		}
	}

	// Copy document root.
	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	dstHtdocs := filepath.Join(tmp, "htdocs")
	if err := m.runner.Run(ctx, "cp", "-a", docRoot, dstHtdocs); err != nil {
		return "", fmt.Errorf("site: backup %s: copy htdocs: %w", name, err)
	}

	// Copy vhost config if it exists.
	vhostConf := filepath.Join(m.confDir, "vhosts", name, "vhconf.conf")
	if _, err := os.Stat(vhostConf); err == nil {
		vhostDir := filepath.Join(tmp, "vhost")
		if err := os.MkdirAll(vhostDir, 0o755); err != nil { //nolint:gosec // backup staging dir
			return "", fmt.Errorf("site: backup %s: mkdir vhost: %w", name, err)
		}
		if err := m.runner.Run(ctx, "cp", vhostConf, filepath.Join(vhostDir, "vhconf.conf")); err != nil {
			return "", fmt.Errorf("site: backup %s: copy vhost: %w", name, err)
		}
	}

	// Create archive.
	if err := os.MkdirAll(BackupDir, 0o755); err != nil { //nolint:gosec // backup dir
		return "", fmt.Errorf("site: backup %s: mkdir %s: %w", name, BackupDir, err)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	archivePath := filepath.Join(BackupDir, name+"-"+ts+".tar.gz")
	if err := m.runner.Run(ctx, "tar", "czf", archivePath, "-C", tmp, "."); err != nil {
		return "", fmt.Errorf("site: backup %s: tar: %w", name, err)
	}

	return archivePath, nil
}
