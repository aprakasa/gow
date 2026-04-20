package app

import (
	"fmt"
)

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
	fmt.Fprintf(d.Stdout, "Site %s cloned to %s successfully.\n", src, dst)
	return nil
}
