package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aprakasa/gow/internal/ols"
)

// Delete removes a site from the state store, unregisters its virtualHost from
// httpd_config.conf, and runs Reconcile to update the OLS configuration. It
// returns an error if the site does not exist.
func (m *Manager) Delete(ctx context.Context, name string) error {
	// Save UnixUser before removing from store.
	site, _ := m.store.Find(name)

	if err := m.store.Remove(name); err != nil {
		return fmt.Errorf("site: delete %s: %w", name, err)
	}

	// Unregister virtualHost block and listener map entry.
	httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
	if err := ols.UnregisterVHost(httpdConfPath, name); err != nil {
		return fmt.Errorf("site: delete %s: unregister vhost: %w", name, err)
	}

	// Remove SSL map entry if site had SSL.
	if site.SSLEnabled {
		_ = ols.RemoveSSLMapEntry(httpdConfPath, name)
	}

	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: delete %s: reconcile: %w", name, err)
	}

	// Remove the dedicated system user.
	if site.UnixUser != "" {
		_ = m.runner.Run(ctx, "userdel", site.UnixUser)
	}

	siteRoot := filepath.Join(m.webRoot, name)
	if err := os.RemoveAll(siteRoot); err != nil {
		return fmt.Errorf("site: delete %s: remove %s: %w", name, siteRoot, err)
	}

	return m.store.Save()
}
