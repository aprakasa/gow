package site

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aprakasa/gow/internal/ols"
)

// Delete removes a site from the state store, unregisters its virtualHost from
// httpd_config.conf, and runs Reconcile to update the OLS configuration. It
// returns an error if the site does not exist.
func (m *Manager) Delete(name string) error {
	if err := m.store.Remove(name); err != nil {
		return fmt.Errorf("site: delete %s: %w", name, err)
	}

	// Unregister virtualHost block and listener map entry.
	httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
	if err := ols.UnregisterVHost(httpdConfPath, name); err != nil {
		return fmt.Errorf("site: delete %s: unregister vhost: %w", name, err)
	}

	if err := m.Reconcile(); err != nil {
		return fmt.Errorf("site: delete %s: reconcile: %w", name, err)
	}

	siteRoot := filepath.Join(m.webRoot, name)
	if err := os.RemoveAll(siteRoot); err != nil {
		return fmt.Errorf("site: delete %s: remove %s: %w", name, siteRoot, err)
	}

	return m.store.Save()
}
