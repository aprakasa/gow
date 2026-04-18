package site

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/template"
)

// Create adds a new site to the state store, creates its document root
// directory, and runs Reconcile to render its OLS config and reload the
// server. It validates the preset before touching state so a bad request is
// rejected early. When preset is "custom", custom must be non-nil with both
// PHPMemoryMB and WorkerBudgetMB > 0.
func (m *Manager) Create(name, siteType, phpVersion, preset string, custom *state.CustomPreset) error {
	switch siteType {
	case "html", "php", "wp":
	default:
		return fmt.Errorf("site: create %s: invalid type %q (html, php, wp)", name, siteType)
	}

	if preset == "custom" {
		if custom == nil || custom.PHPMemoryMB == 0 || custom.WorkerBudgetMB == 0 {
			return fmt.Errorf("site: create %s: custom preset requires php_memory_mb and worker_budget_mb > 0", name)
		}
	} else {
		if _, err := allocator.LookupPreset(preset); err != nil {
			return fmt.Errorf("site: create %s: %w", name, err)
		}
	}

	site := state.Site{
		Name:         name,
		Type:         siteType,
		PHPVersion:   phpVersion,
		Preset:       preset,
		CustomPreset: custom,
		UnixUser:     SiteUserName(name),
		CreatedAt:    time.Now().UTC(),
	}
	if err := m.store.Add(site); err != nil {
		return fmt.Errorf("site: create %s: %w", name, err)
	}

	// Create document root directory.
	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	if err := os.MkdirAll(docRoot, 0o755); err != nil { //nolint:gosec // web root, world-readable is fine
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: mkdir %s: %w", name, docRoot, err)
	}

	// Create system user for per-site isolation.
	if err := m.runner.Run("useradd", "--system", "--no-create-home",
		"--shell", "/usr/sbin/nologin", site.UnixUser); err != nil {
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: create user: %w", name, err)
	}
	siteRoot := filepath.Join(m.webRoot, name)
	if err := m.runner.Run("chown", "-R", site.UnixUser+":"+site.UnixUser, siteRoot); err != nil {
		_ = m.runner.Run("userdel", site.UnixUser)
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: chown: %w", name, err)
	}
	if err := m.runner.Run("usermod", "-aG", "redis", site.UnixUser); err != nil {
		_ = m.runner.Run("userdel", site.UnixUser)
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: add to redis group: %w", name, err)
	}

	// Write a placeholder index page for HTML sites.
	if siteType == "html" {
		idx, err := template.RenderIndexHTML(name)
		if err != nil {
			return fmt.Errorf("site: create %s: render index: %w", name, err)
		}
		indexPath := filepath.Join(docRoot, "index.html")
		if err := os.WriteFile(indexPath, []byte(idx), 0o644); err != nil { //nolint:gosec // static HTML, not secret
			_ = m.store.Remove(name)
			return fmt.Errorf("site: create %s: write index: %w", name, err)
		}
	}

	// Write a PHP info page for PHP sites.
	if siteType == "php" {
		idx, err := template.RenderIndexPHP(name)
		if err != nil {
			return fmt.Errorf("site: create %s: render index: %w", name, err)
		}
		indexPath := filepath.Join(docRoot, "index.php")
		if err := os.WriteFile(indexPath, []byte(idx), 0o644); err != nil { //nolint:gosec // generated PHP, not secret
			_ = m.store.Remove(name)
			return fmt.Errorf("site: create %s: write index: %w", name, err)
		}
	}

	if err := m.Reconcile(); err != nil {
		// Best-effort rollback: remove the site and user we just created.
		_ = m.runner.Run("userdel", site.UnixUser)
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
