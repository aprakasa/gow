package site

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/aprakasa/gow/internal/state"
)

// EnableSSL requests a Let's Encrypt certificate for the named site using
// certbot's webroot plugin. If staging is true the Let's Encrypt staging
// server is used (--test-cert). On success the site state is updated with
// the certificate paths and OLS is reconciled.
func (m *Manager) EnableSSL(ctx context.Context, name, email string, staging bool) error {
	if _, ok := m.store.Find(name); !ok {
		return fmt.Errorf("site: ssl %s: not found", name)
	}

	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	args := []string{
		"certonly", "--webroot",
		"-w", docRoot,
		"-d", name,
		"--non-interactive",
		"--agree-tos",
		"--email", email,
	}
	if staging {
		args = append(args, "--test-cert")
	}

	if err := m.runner.Run(ctx, "certbot", args...); err != nil {
		return fmt.Errorf("site: ssl %s: certbot: %w", name, err)
	}

	certPath := "/etc/letsencrypt/live/" + name + "/fullchain.pem"
	keyPath := "/etc/letsencrypt/live/" + name + "/privkey.pem"

	for _, p := range []string{certPath, keyPath} {
		if err := m.runner.Run(ctx, "test", "-f", p); err != nil {
			return fmt.Errorf("site: ssl %s: cert file not found: %s: %w", name, p, err)
		}
	}

	if err := m.store.Update(name, func(s *state.Site) {
		s.SSLEnabled = true
		s.CertPath = certPath
		s.KeyPath = keyPath
	}); err != nil {
		return fmt.Errorf("site: ssl %s: update state: %w", name, err)
	}

	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: ssl %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
