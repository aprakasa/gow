package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
)

// SSLOptions groups the per-request knobs for EnableSSL. Adding new options
// here avoids churning every caller when we extend the feature set.
type SSLOptions struct {
	Email    string
	Staging  bool
	Wildcard bool   // request *.domain alongside domain (requires DNS != "")
	DNS      string // DNS provider for DNS-01 (currently: "cloudflare")
	HSTS     bool   // persist Strict-Transport-Security for the vhost
}

// dnsCredsDir is the directory holding certbot DNS plugin credentials.
// Tests override this to point at a temp dir.
var dnsCredsDir = "/etc/gow/dns" //nolint:gosec // filesystem path, not a secret

// dnsCredsPath returns the expected credentials INI path for a DNS provider.
// Operators drop the file there with 0600 permissions before running
// `gow site ssl ... --dns <provider>`.
func dnsCredsPath(provider string) string {
	return filepath.Join(dnsCredsDir, provider+".ini")
}

// EnableSSL requests a Let's Encrypt certificate for the named site.
//
// Default path is HTTP-01 via certbot's webroot plugin. When opts.DNS is set,
// DNS-01 via the matching certbot-dns-* plugin is used instead, which is the
// only way to obtain a wildcard certificate. When opts.Wildcard is true, the
// cert covers both domain and *.domain.
//
// Cloudflare is the only DNS provider wired up for v1. The credentials file
// at /etc/gow/dns/<provider>.ini must exist with 0600 permissions before
// calling; we fail fast with an actionable error otherwise.
func (m *Manager) EnableSSL(ctx context.Context, name string, opts SSLOptions) error {
	if _, ok := m.store.Find(name); !ok {
		return fmt.Errorf("site: ssl %s: not found", name)
	}
	if opts.Wildcard && opts.DNS == "" {
		return fmt.Errorf("site: ssl %s: --wildcard requires --dns <provider>", name)
	}
	if opts.DNS != "" && opts.DNS != "cloudflare" {
		return fmt.Errorf("site: ssl %s: unsupported --dns %q (supported: cloudflare)", name, opts.DNS)
	}

	args := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--email", opts.Email,
	}

	if opts.DNS != "" {
		credsPath := dnsCredsPath(opts.DNS)
		if _, err := os.Stat(credsPath); err != nil {
			return fmt.Errorf("site: ssl %s: missing DNS credentials at %s (create it with 0600 perms; see docs)", name, credsPath)
		}
		if opts.DNS == "cloudflare" {
			args = append(args, "--dns-cloudflare", "--dns-cloudflare-credentials", credsPath)
		}
	} else {
		docRoot := filepath.Join(m.webRoot, name, "htdocs")
		args = append(args, "--webroot", "-w", docRoot)
	}

	args = append(args, "-d", name)
	if opts.Wildcard {
		args = append(args, "-d", "*."+name)
	}

	if opts.Staging {
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
		s.HSTS = opts.HSTS
	}); err != nil {
		return fmt.Errorf("site: ssl %s: update state: %w", name, err)
	}

	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	if err := m.runner.Run(ctx, stack.WPCLIBinPath, "config", "set", "FORCE_SSL_ADMIN", "true",
		"--type=constant", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("site: ssl %s: set FORCE_SSL_ADMIN: %w", name, err)
	}

	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: ssl %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
