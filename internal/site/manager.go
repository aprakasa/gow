// Package site orchestrates the WordPress-on-OLS lifecycle: create, delete,
// and reconcile. The Manager holds injected dependencies so every operation
// is testable without real hardware or a running OLS instance.
package site

import (
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
func (m *Manager) Reconcile() error {
	sites := m.store.Sites()
	if len(sites) == 0 {
		return nil
	}

	// Build allocator inputs only for PHP-enabled sites.
	inputs := make([]allocator.SiteInput, 0, len(sites))
	siteIndex := make([]int, 0, len(sites)) // maps allocs index -> sites index
	for i, s := range sites {
		if siteType(s) == "html" {
			continue
		}
		in := allocator.SiteInput{
			Name:   s.Name,
			Preset: s.Preset,
		}
		if s.CustomPreset != nil {
			in.CustomPHPMemoryMB = s.CustomPreset.PHPMemoryMB
			in.CustomWorkerBudgetMB = s.CustomPreset.WorkerBudgetMB
		}
		inputs = append(inputs, in)
		siteIndex = append(siteIndex, i)
	}

	// Render html sites (no PHP, no allocator).
	for _, s := range sites {
		if siteType(s) != "html" {
			continue
		}
		vhostDir := filepath.Join(m.confDir, "vhosts", s.Name)
		if err := os.MkdirAll(vhostDir, 0o750); err != nil { //nolint:gosec // OLS needs readable vhost dirs
			return fmt.Errorf("site: create vhost dir %s: %w", vhostDir, err)
		}
		siteRoot := filepath.Join(m.webRoot, s.Name)
		data := template.VHostData{
			Site:    s.Name,
			Domain:  s.Name,
			WebRoot: siteRoot,
			LogDir:  "/var/log/lsws",
		}
		var content string
		var err error
		if s.Maintenance {
			content, err = renderMaintenanceVHost(data)
		} else {
			content, err = template.RenderVHost("html", data)
		}
		if err != nil {
			return fmt.Errorf("site: render vhost %s: %w", s.Name, err)
		}
		vhostPath := filepath.Join(vhostDir, "vhconf.conf")
		if err := os.WriteFile(vhostPath, []byte(content), 0o644); err != nil { //nolint:gosec // config file, not secret
			return fmt.Errorf("site: write %s: %w", vhostPath, err)
		}
		confFile := "conf/vhosts/" + s.Name + "/vhconf.conf"
		httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
		if err := ols.RegisterVHost(httpdConfPath, s.Name, siteRoot, confFile); err != nil {
			return fmt.Errorf("site: register vhost %s: %w", s.Name, err)
		}
	}

	// Compute allocations for PHP-enabled sites.
	if len(inputs) > 0 {
		allocs, err := allocator.Compute(m.specs.TotalRAMMB, m.specs.CPUCores, inputs, m.policy)
		if err != nil {
			return fmt.Errorf("site: reconcile: %w", err)
		}

		for j, a := range allocs {
			s := sites[siteIndex[j]]
			vhostDir := filepath.Join(m.confDir, "vhosts", a.Site)
			if err := os.MkdirAll(vhostDir, 0o750); err != nil { //nolint:gosec // OLS needs readable vhost dirs
				return fmt.Errorf("site: create vhost dir %s: %w", vhostDir, err)
			}
			siteRoot := filepath.Join(m.webRoot, a.Site)
			data := template.VHostData{
				Site:             a.Site,
				Domain:           a.Site,
				WebRoot:          siteRoot,
				LogDir:           "/var/log/lsws",
				PHPVer:           s.PHPVersion,
				Children:         int(a.Children),
				PHPMemoryLimitMB: a.PHPMemoryLimitMB,
				MemSoftMB:        a.MemSoftMB,
				MemHardMB:        a.MemHardMB,
			}

			var content string
			if s.Maintenance {
				content, err = renderMaintenanceVHost(data)
			} else {
				content, err = template.RenderVHost(siteType(s), data)
			}
			if err != nil {
				return fmt.Errorf("site: render vhost %s: %w", a.Site, err)
			}

			vhostPath := filepath.Join(vhostDir, "vhconf.conf")
			if err := os.WriteFile(vhostPath, []byte(content), 0o644); err != nil { //nolint:gosec // config file, not secret
				return fmt.Errorf("site: write %s: %w", vhostPath, err)
			}

			confFile := "conf/vhosts/" + a.Site + "/vhconf.conf"
			httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
			if err := ols.RegisterVHost(httpdConfPath, a.Site, siteRoot, confFile); err != nil {
				return fmt.Errorf("site: register vhost %s: %w", a.Site, err)
			}
		}
	}

	// Validate and reload OLS.
	if err := m.ols.Validate(); err != nil {
		return fmt.Errorf("site: validate: %w", err)
	}
	return m.ols.GracefulReload()
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
	if err := os.MkdirAll(htdocsDir, 0o755); err != nil {
		return "", fmt.Errorf("create htdocs dir: %w", err)
	}
	indexPath := filepath.Join(htdocsDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(maintHTML), 0o644); err != nil {
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

// SiteUserName returns the system user name for a site domain.
func SiteUserName(domain string) string {
	return "site-" + domain
}

// needsIsolation returns true for site types that run PHP and need a
// dedicated system user.
func needsIsolation(siteType string) bool {
	return siteType != "html"
}

// userExists checks whether a system user exists by running `id <name>`.
func (m *Manager) userExists(name string) bool {
	return m.runner.Run("id", name) == nil
}
