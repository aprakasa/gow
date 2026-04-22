package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aprakasa/gow/internal/state"
)

const defaultBackupRetain = 7

// gowBinPath is the path to the gow binary used in the cron entry.
var gowBinPath = "/usr/local/bin/gow"

// RunBackup validates the domain, creates a Manager, and calls Backup.
func RunBackup(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	path, err := m.Backup(d.Ctx, domain)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Backup created: %s\n", path)
	return nil
}

// RunRestore validates inputs, creates a Manager, and calls Restore.
func RunRestore(cfg CLIConfig, domain, archivePath string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Restoring %s from %s...\n", domain, archivePath)
	if err := m.Restore(d.Ctx, domain, archivePath); err != nil {
		return err
	}
	if err := writeCronFile(domain, filepath.Join(cfg.WebRoot, domain, "htdocs")); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Site %s restored successfully.\n", domain)
	return nil
}

// RunClone validates inputs, creates a Manager, and calls Clone.
func RunClone(cfg CLIConfig, src, dst string, d Deps) error {
	if err := ValidateDomain(src); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := ValidateDomain(dst); err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Cloning %s → %s...\n", src, dst)
	if err := m.Clone(d.Ctx, src, dst); err != nil {
		return err
	}
	if err := writeCronFile(dst, filepath.Join(cfg.WebRoot, dst, "htdocs")); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Site %s cloned to %s successfully.\n", src, dst)
	return nil
}

// RunBackupSchedule sets or updates the automatic backup schedule for a site.
func RunBackupSchedule(cfg CLIConfig, domain, schedule string, retain int, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	if schedule != "daily" && schedule != "weekly" {
		return fmt.Errorf("--schedule must be daily or weekly")
	}
	if retain < 1 {
		return fmt.Errorf("--retain must be >= 1")
	}
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return err
	}
	if _, ok := store.Find(domain); !ok {
		return fmt.Errorf("site %q not found", domain)
	}
	if err := store.Update(domain, func(s *state.Site) {
		s.BackupSchedule = schedule
		s.BackupRetain = retain
	}); err != nil {
		return err
	}
	if err := store.Save(); err != nil {
		return err
	}
	if err := ensureGlobalBackupCron(); err != nil {
		return fmt.Errorf("write backup cron: %w", err)
	}
	fmt.Fprintf(d.Stdout, "Backup scheduled for %s: %s, retain %d\n", domain, schedule, retain)
	return nil
}

// RunBackupUnschedule removes the automatic backup schedule for a site.
func RunBackupUnschedule(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return err
	}
	s, ok := store.Find(domain)
	if !ok {
		return fmt.Errorf("site %q not found", domain)
	}
	if s.BackupSchedule == "" {
		fmt.Fprintf(d.Stdout, "No backup schedule set for %s.\n", domain)
		return nil
	}
	if err := store.Update(domain, func(s *state.Site) {
		s.BackupSchedule = ""
		s.BackupRetain = 0
	}); err != nil {
		return err
	}
	if err := store.Save(); err != nil {
		return err
	}
	// Remove global cron file if no sites have schedules.
	hasScheduled := false
	for _, s := range store.Sites() {
		if s.BackupSchedule != "" {
			hasScheduled = true
			break
		}
	}
	if !hasScheduled {
		_ = removeGlobalBackupCron()
	}
	fmt.Fprintf(d.Stdout, "Backup schedule removed for %s.\n", domain)
	return nil
}

// RunBackupCron is the entry point for the hidden backup-cron command.
// It iterates all sites with a backup schedule, runs backups, and prunes.
func RunBackupCron(cfg CLIConfig, d Deps) error {
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	now := time.Now().UTC()
	for _, s := range store.Sites() {
		if s.BackupSchedule == "" {
			continue
		}
		if s.BackupSchedule == "weekly" && now.Weekday() != time.Sunday {
			continue
		}
		if _, err := m.Backup(d.Ctx, s.Name); err != nil {
			log.Printf("gow-backup-cron: backup %s: %v", s.Name, err)
			continue
		}
		retain := s.BackupRetain
		if retain <= 0 {
			retain = defaultBackupRetain
		}
		if err := m.PruneBackups(s.Name, retain); err != nil {
			log.Printf("gow-backup-cron: prune %s: %v", s.Name, err)
		}
	}
	return nil
}

// ensureGlobalBackupCron writes the global backup cron file. Idempotent.
func ensureGlobalBackupCron() error {
	if err := os.MkdirAll(cronDir, 0o755); err != nil { //nolint:gosec // cron.d must be world-readable
		return fmt.Errorf("create %s: %w", cronDir, err)
	}
	content := fmt.Sprintf("0 2 * * * root %s backup-cron >/dev/null 2>&1\n", gowBinPath)
	path := filepath.Join(cronDir, "gow-backups")
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // cron entry, not secret
}

// removeGlobalBackupCron removes the global backup cron file. Tolerates already-gone.
func removeGlobalBackupCron() error {
	path := filepath.Join(cronDir, "gow-backups")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove backup cron: %w", err)
	}
	return nil
}
