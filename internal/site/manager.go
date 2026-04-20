// Package site orchestrates the WordPress-on-OLS lifecycle: create, delete,
// and reconcile. The Manager holds injected dependencies so every operation
// is testable without real hardware or a running OLS instance.
package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
	"github.com/aprakasa/gow/internal/template"
)

// Manager ties together the state store, OLS controller, hardware specs, and
// allocator policy. Each method is a lifecycle operation that mutates state
// and reconciles the OLS configuration.
type Manager struct {
	store   *state.Store
	ols     ols.Controller
	specs   system.Specs
	policy  allocator.Policy
	confDir string
	webRoot string
	runner  stack.Runner
}

// NewManager creates a Manager with the given dependencies. confDir is the
// base directory for rendered OLS configs (e.g., /usr/local/lsws/conf).
// webRoot is the base directory for site document roots (e.g., /var/www).
func NewManager(store *state.Store, ctrl ols.Controller, specs system.Specs, policy allocator.Policy, confDir, webRoot string, runner stack.Runner) *Manager {
	return &Manager{
		store:   store,
		ols:     ctrl,
		specs:   specs,
		policy:  policy,
		confDir: confDir,
		webRoot: webRoot,
		runner:  runner,
	}
}

// Reconcile recomputes allocations for all sites, renders their OLS configs,
// registers virtualHosts in httpd_config.conf, and triggers a graceful OLS
// reload. With zero sites it returns immediately without touching OLS.
//
// HTML sites are rendered separately (no PHP, no allocator). PHP-enabled sites
// go through the allocator for resource computation. Sites in maintenance mode
// get a static 503 page instead of their normal vhost config.
func (m *Manager) Reconcile(ctx context.Context) error {
	sites := m.store.Sites()
	if len(sites) == 0 {
		return nil
	}

	// Compute allocations once for PHP-enabled sites. HTML sites skip the
	// allocator entirely — their VHostData fields for PHP/memory stay zero.
	allocByName, err := m.computeAllocations(sites)
	if err != nil {
		return err
	}

	// Open httpd_config.conf once and batch every register/SSL edit.
	httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
	hc, err := ols.OpenHttpd(httpdConfPath)
	if err != nil {
		return fmt.Errorf("site: open httpd config: %w", err)
	}

	anySSL := false
	for _, s := range sites {
		if s.SSLEnabled {
			anySSL = true
			break
		}
	}
	if anySSL {
		hc.EnsureSSLListener()
		for _, s := range sites {
			if s.SSLEnabled {
				hc.SetSSLListenerCerts(s.CertPath, s.KeyPath)
				break
			}
		}
	}

	for _, s := range sites {
		if err := m.renderAndRegisterSite(ctx, s, allocByName[s.Name], hc); err != nil {
			return err
		}
	}

	if err := hc.Save(); err != nil {
		return fmt.Errorf("site: save httpd config: %w", err)
	}

	if err := m.ols.Validate(ctx); err != nil {
		return fmt.Errorf("site: validate: %w", err)
	}
	return m.ols.GracefulReload(ctx)
}

// computeAllocations returns a name→Allocation map for PHP-enabled sites.
// HTML sites are absent from the map; callers get the zero Allocation, which
// the template handles correctly (no PHP fields emitted).
func (m *Manager) computeAllocations(sites []state.Site) (map[string]allocator.Allocation, error) {
	inputs := make([]allocator.SiteInput, 0, len(sites))
	for _, s := range sites {
		if siteType(s) == "html" {
			continue
		}
		in := allocator.SiteInput{Name: s.Name, Preset: s.Preset}
		if s.CustomPreset != nil {
			in.CustomPHPMemoryMB = s.CustomPreset.PHPMemoryMB
			in.CustomWorkerBudgetMB = s.CustomPreset.WorkerBudgetMB
		}
		inputs = append(inputs, in)
	}
	out := map[string]allocator.Allocation{}
	if len(inputs) == 0 {
		return out, nil
	}
	allocs, err := allocator.Compute(m.specs.TotalRAMMB, m.specs.CPUCores, inputs, m.policy)
	if err != nil {
		return nil, fmt.Errorf("site: reconcile: %w", err)
	}
	for _, a := range allocs {
		out[a.Site] = a
	}
	return out, nil
}

// renderAndRegisterSite writes the site's vhconf.conf and stages all
// httpd_config.conf edits onto hc. The caller commits hc.Save() once all
// sites are processed. alloc is zero-valued for HTML sites.
func (m *Manager) renderAndRegisterSite(_ context.Context, s state.Site, alloc allocator.Allocation, hc *ols.HttpdConf) error {
	vhostDir := filepath.Join(m.confDir, "vhosts", s.Name)
	if err := os.MkdirAll(vhostDir, 0o750); err != nil { //nolint:gosec // OLS needs readable vhost dirs
		return fmt.Errorf("site: create vhost dir %s: %w", vhostDir, err)
	}
	siteRoot := filepath.Join(m.webRoot, s.Name)
	data := template.VHostData{
		Site:             s.Name,
		Domain:           s.Name,
		WebRoot:          siteRoot,
		LogDir:           "/var/log/lsws",
		PHPVer:           s.PHPVersion,
		Children:         alloc.Children,
		PHPMemoryLimitMB: alloc.PHPMemoryLimitMB,
		MemSoftMB:        alloc.MemSoftMB,
		MemHardMB:        alloc.MemHardMB,
		SSLEnabled:       s.SSLEnabled,
		CertPath:         s.CertPath,
		KeyPath:          s.KeyPath,
	}

	variant := siteType(s)
	var content string
	var err error
	if s.Maintenance {
		content, err = renderMaintenanceVHost(data)
	} else {
		content, err = template.RenderVHost(variant, data)
	}
	if err != nil {
		return fmt.Errorf("site: render vhost %s: %w", s.Name, err)
	}

	vhostPath := filepath.Join(vhostDir, "vhconf.conf")
	if err := os.WriteFile(vhostPath, []byte(content), 0o644); err != nil { //nolint:gosec // config file, not secret
		return fmt.Errorf("site: write %s: %w", vhostPath, err)
	}

	confFile := "conf/vhosts/" + s.Name + "/vhconf.conf"
	hc.RegisterVHost(s.Name, siteRoot, confFile)
	if s.SSLEnabled {
		hc.AddSSLMapEntry(s.Name)
		hc.SetVHostSSL(s.Name, s.CertPath, s.KeyPath)
	}
	return nil
}

// renderMaintenanceVHost renders a vhost that serves a static 503 maintenance
// page. It uses the html template (no PHP handler) and writes the maintenance
// HTML to the docRoot as index.html.
func renderMaintenanceVHost(data template.VHostData) (string, error) {
	maintHTML, err := template.RenderMaintenance(data.Domain)
	if err != nil {
		return "", err
	}
	// Write the maintenance page to docRoot/index.html.
	htdocsDir := filepath.Join(data.WebRoot, "htdocs")
	if err := os.MkdirAll(htdocsDir, 0o755); err != nil { //nolint:gosec // lsws must traverse
		return "", fmt.Errorf("create htdocs dir: %w", err)
	}
	indexPath := filepath.Join(htdocsDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(maintHTML), 0o644); err != nil { //nolint:gosec // lsws must read
		return "", fmt.Errorf("write maintenance page: %w", err)
	}
	return template.RenderVHost("html", data)
}

// siteType returns the template variant name for a site. Sites created before
// the Type field was added default to "wp" for backward compatibility.
func siteType(s state.Site) string {
	if s.Type == "" {
		return "wp"
	}
	return s.Type
}

// UserName returns the system user name for a site domain.
func UserName(domain string) string {
	return "site-" + domain
}

// needsIsolation returns true for site types that run PHP and need a
// dedicated system user.
func needsIsolation(siteType string) bool {
	return siteType != "html"
}

// userExists checks whether a system user exists by running `id <name>`.
func (m *Manager) userExists(ctx context.Context, name string) bool {
	return m.runner.Run(ctx, "id", name) == nil
}
