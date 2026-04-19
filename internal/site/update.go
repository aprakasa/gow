package site

import (
	"fmt"
	"path/filepath"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/state"
)

// Update modifies PHP version and/or preset for an existing site, then
// re-reconciles all OLS configurations. Empty phpVersion or preset means
// "no change". It validates the new preset before mutating state.
// When isolate is true, it creates the system user, chowns the site directory,
// adds the user to the redis group, and updates restrained in the httpd config.
func (m *Manager) Update(name, phpVersion, preset string, custom *state.CustomPreset, isolate bool) error {
	if preset != "" && preset != "custom" {
		if _, err := allocator.LookupPreset(preset); err != nil {
			return fmt.Errorf("site: update %s: %w", name, err)
		}
	}
	if preset == "custom" {
		if custom == nil || custom.PHPMemoryMB == 0 || custom.WorkerBudgetMB == 0 {
			return fmt.Errorf("site: update %s: custom preset requires php_memory_mb and worker_budget_mb > 0", name)
		}
	}

	if err := m.store.Update(name, func(s *state.Site) {
		if phpVersion != "" {
			s.PHPVersion = phpVersion
		}
		if preset != "" {
			s.Preset = preset
			s.CustomPreset = custom
		}
		if isolate {
			s.UnixUser = SiteUserName(name)
		}
	}); err != nil {
		return fmt.Errorf("site: update %s: %w", name, err)
	}

	// Create system user and chown if isolating.
	if isolate {
		s, _ := m.store.Find(name)
		if !m.userExists(s.UnixUser) {
			if err := m.runner.Run("useradd", "--system", "--no-create-home",
				"--shell", "/usr/sbin/nologin", s.UnixUser); err != nil {
				// Roll back the UnixUser we just set in the store.
				m.store.Update(name, func(si *state.Site) { si.UnixUser = "" })
				return fmt.Errorf("site: update %s: create user: %w", name, err)
			}
		}
		siteRoot := filepath.Join(m.webRoot, name)
		if err := m.runner.Run("chown", "-R", s.UnixUser+":"+s.UnixUser, siteRoot); err != nil {
			_ = m.runner.Run("userdel", s.UnixUser)
			m.store.Update(name, func(si *state.Site) { si.UnixUser = "" })
			return fmt.Errorf("site: update %s: chown: %w", name, err)
		}
		if err := m.runner.Run("usermod", "-aG", "redis", s.UnixUser); err != nil {
			_ = m.runner.Run("userdel", s.UnixUser)
			m.store.Update(name, func(si *state.Site) { si.UnixUser = "" })
			return fmt.Errorf("site: update %s: add to redis group: %w", name, err)
		}

		// Update restrained in httpd_config.conf.
		httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
		if err := ols.UpdateVHostRestrained(httpdConfPath, name, 1); err != nil {
			return fmt.Errorf("site: update %s: set restrained: %w", name, err)
		}
	}

	if err := m.Reconcile(); err != nil {
		return fmt.Errorf("site: update %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
