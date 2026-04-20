package site

import (
	"context"
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
//
// cacheMode controls LSCache wiring for WP sites: "lscache" (default) or
// "none". For non-wp types it must be empty. The caller resolves the default.
//
// On failure after the state entry is added, every side effect (system user,
// docroot, OLS vhost dir) is undone via a deferred rollback.
func (m *Manager) Create(ctx context.Context, name, siteType, phpVersion, preset, cacheMode string, custom *state.CustomPreset) error {
	switch siteType {
	case "html", "php", "wp":
	default:
		return fmt.Errorf("site: create %s: invalid type %q (html, php, wp)", name, siteType)
	}

	switch cacheMode {
	case "":
		// ok for any type
	case "lscache", "none":
		if siteType != "wp" {
			return fmt.Errorf("site: create %s: cache mode %q only valid for --type wp", name, cacheMode)
		}
	default:
		return fmt.Errorf("site: create %s: invalid cache mode %q (lscache, none)", name, cacheMode)
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
		CacheMode:    cacheMode,
		CreatedAt:    time.Now().UTC(),
	}
	if needsIsolation(siteType) {
		site.UnixUser = UserName(name)
	}
	if err := m.store.Add(site); err != nil {
		return fmt.Errorf("site: create %s: %w", name, err)
	}

	// Rollback stack — pushed in order, executed LIFO on failure. Cleared
	// on success by flipping committed = true.
	committed := false
	var rollbacks []func()
	rollbacks = append(rollbacks, func() { _ = m.store.Remove(name) })
	defer func() {
		if committed {
			return
		}
		for i := len(rollbacks) - 1; i >= 0; i-- {
			rollbacks[i]()
		}
	}()

	siteRoot := filepath.Join(m.webRoot, name)
	docRoot := filepath.Join(siteRoot, "htdocs")
	if err := os.MkdirAll(docRoot, 0o755); err != nil { //nolint:gosec // web root, world-readable is fine
		return fmt.Errorf("site: create %s: mkdir %s: %w", name, docRoot, err)
	}
	rollbacks = append(rollbacks, func() { _ = os.RemoveAll(siteRoot) })

	if site.UnixUser != "" {
		createdUser := false
		if !m.userExists(ctx, site.UnixUser) {
			if err := m.runner.Run(ctx, "useradd", "--system", "--no-create-home",
				"--shell", "/usr/sbin/nologin", site.UnixUser); err != nil {
				return fmt.Errorf("site: create %s: create user: %w", name, err)
			}
			createdUser = true
			rollbacks = append(rollbacks, func() { _ = m.runner.Run(context.Background(), "userdel", site.UnixUser) })
		}
		if err := m.runner.Run(ctx, "chown", "-R", site.UnixUser+":"+site.UnixUser, siteRoot); err != nil {
			return fmt.Errorf("site: create %s: chown: %w", name, err)
		}
		if err := m.runner.Run(ctx, "usermod", "-aG", "redis", site.UnixUser); err != nil {
			return fmt.Errorf("site: create %s: add to redis group: %w", name, err)
		}
		_ = createdUser // suppress ineffective-assign lint if the flag is unused
	}

	if siteType == "html" {
		idx, err := template.RenderIndexHTML(name)
		if err != nil {
			return fmt.Errorf("site: create %s: render index: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(docRoot, "index.html"), []byte(idx), 0o644); err != nil { //nolint:gosec // static HTML, not secret
			return fmt.Errorf("site: create %s: write index: %w", name, err)
		}
	}

	if siteType == "php" {
		idx, err := template.RenderIndexPHP(name)
		if err != nil {
			return fmt.Errorf("site: create %s: render index: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(docRoot, "index.php"), []byte(idx), 0o644); err != nil { //nolint:gosec // generated PHP, not secret
			return fmt.Errorf("site: create %s: write index: %w", name, err)
		}
	}

	vhostDir := filepath.Join(m.confDir, "vhosts", name)
	rollbacks = append(rollbacks, func() { _ = os.RemoveAll(vhostDir) })

	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: create %s: reconcile: %w", name, err)
	}

	if err := m.store.Save(); err != nil {
		return err
	}
	committed = true
	return nil
}
