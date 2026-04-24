// Package site orchestrates the WordPress-on-OLS lifecycle: create, delete,
// and reconcile. The Manager holds injected dependencies so every operation
// is testable without real hardware or a running OLS instance.
package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	store             *state.Store
	ols               ols.Controller
	specs             system.Specs
	policy            allocator.Policy
	confDir           string
	webRoot           string
	logDir            string
	runner            stack.Runner
	defaultPHP        string // fallback PHP version for sites without one (e.g. HTML in maintenance)
	logrotateConfPath string // override for tests; defaults to /etc/logrotate.d/gow
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
		logDir:  defaultLogDir,
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
		// Clean up stale SSL listener when no sites remain.
		httpdConfPath := filepath.Join(m.confDir, "httpd_config.conf")
		hc, err := ols.OpenHttpd(httpdConfPath)
		if err != nil {
			return fmt.Errorf("site: open httpd config: %w", err)
		}
		hc.RemoveSSLListener()
		if err := hc.Save(); err != nil {
			return fmt.Errorf("site: save httpd config: %w", err)
		}
		if err := m.writeLogrotateConfig(); err != nil {
			return fmt.Errorf("site: logrotate: %w", err)
		}
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
	} else {
		hc.RemoveSSLListener()
	}

	for _, s := range sites {
		if err := m.renderAndRegisterSite(ctx, s, allocByName[s.Name], hc); err != nil {
			return err
		}
	}

	if err := hc.Save(); err != nil {
		return fmt.Errorf("site: save httpd config: %w", err)
	}

	if err := m.writeLogrotateConfig(); err != nil {
		return fmt.Errorf("site: logrotate: %w", err)
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
	if err := os.MkdirAll(m.logDir, 0o755); err != nil { //nolint:gosec // OLS needs readable log dirs
		return fmt.Errorf("site: create log dir %s: %w", m.logDir, err)
	}
	data := template.VHostData{
		Site:             s.Name,
		Domain:           s.Name,
		WebRoot:          siteRoot,
		LogDir:           m.logDir,
		PHPVer:           s.PHPVersion,
		Children:         alloc.Children,
		PHPMemoryLimitMB: alloc.PHPMemoryLimitMB,
		MemSoftMB:        alloc.MemSoftMB,
		MemHardMB:        alloc.MemHardMB,
		SSLEnabled:       s.SSLEnabled,
		CertPath:         s.CertPath,
		KeyPath:          s.KeyPath,
		HSTS:             s.HSTS,
		CacheMode:        s.CacheMode,
		Multisite:        s.Multisite,
	}

	variant := siteType(s)
	var content string
	var err error
	if s.Maintenance {
		// Maintenance vhost needs a PHP handler to send 503 status code.
		if data.PHPVer == "" {
			data.PHPVer = m.defaultPHP
		}
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

	// Write .user.ini so PHP picks up the allocated memory_limit. Without
	// this, PHP falls back to the system default (usually 128M) regardless
	// of the PHP_MEMORY_LIMIT env var in the OLS extprocessor config.
	if alloc.PHPMemoryLimitMB > 0 && s.Type != "html" {
		docRoot := filepath.Join(siteRoot, "htdocs")
		if err := os.MkdirAll(docRoot, 0o755); err != nil { //nolint:gosec // web root
			return fmt.Errorf("site: create %s: %w", docRoot, err)
		}
		iniPath := filepath.Join(docRoot, ".user.ini")
		iniContent := fmt.Sprintf(
				"memory_limit = %dM\nmax_execution_time = 3600\nmax_input_vars = 5000\n",
				alloc.PHPMemoryLimitMB,
			)
		if err := os.WriteFile(iniPath, []byte(iniContent), 0o644); err != nil { //nolint:gosec // php ini, not secret
			return fmt.Errorf("site: write %s: %w", iniPath, err)
		}
	}

	// Set WP_MEMORY_LIMIT in wp-config.php so WordPress doesn't cap itself
	// at its default 40M, overriding the PHP memory_limit at runtime.
	if alloc.PHPMemoryLimitMB > 0 && s.Type == "wp" {
		docRoot := filepath.Join(siteRoot, "htdocs")
		if err := writeWPConfigMemoryLimit(docRoot, alloc.PHPMemoryLimitMB); err != nil {
			// Non-fatal: wp-config.php may not exist for brand-new sites
			// that haven't run WP install yet.
			fmt.Fprintf(os.Stderr, "warning: wp-config memory limit for %s: %v\n", s.Name, err)
		}
	}

	return nil
}

// renderMaintenanceVHost renders a vhost that serves a 503 maintenance page
// via a small PHP script. It writes maintenance.php to the docRoot and renders
// the vhost-maintenance template which rewrites all requests to it. phpVer
// provides the PHP binary path — it comes from the site's own version or a
// fallback default.
func renderMaintenanceVHost(data template.VHostData) (string, error) {
	maintPHP, err := template.RenderMaintenancePHP(data.Domain)
	if err != nil {
		return "", err
	}
	htdocsDir := filepath.Join(data.WebRoot, "htdocs")
	if err := os.MkdirAll(htdocsDir, 0o755); err != nil { //nolint:gosec // lsws must traverse
		return "", fmt.Errorf("create htdocs dir: %w", err)
	}
	phpPath := filepath.Join(htdocsDir, "maintenance.php")
	if err := os.WriteFile(phpPath, []byte(maintPHP), 0o644); err != nil { //nolint:gosec // lsws must read
		return "", fmt.Errorf("write maintenance page: %w", err)
	}
	return template.RenderVHost("maintenance", data)
}

// siteType returns the template variant name for a site. Sites created before
// the Type field was added default to "wp" for backward compatibility.
func siteType(s state.Site) string {
	if s.Type == "" {
		return "wp"
	}
	return s.Type
}

// SetLogDir overrides the per-site log directory (used by tests and the app
// layer to point at a temp directory instead of /var/log/lsws).
func (m *Manager) SetLogDir(dir string) {
	m.logDir = dir
}

// SetLogrotateConfPath overrides the logrotate config file path (used by tests
// to avoid writing to /etc/logrotate.d).
func (m *Manager) SetLogrotateConfPath(path string) {
	m.logrotateConfPath = path
}

// SetDefaultPHP sets the fallback PHP version used when rendering maintenance
// mode for sites that have no PHP version (e.g. HTML sites).
func (m *Manager) SetDefaultPHP(ver string) {
	m.defaultPHP = ver
}

// UserName returns the system user name for a site domain.
// Dots are replaced with hyphens so the name works with chown's user:group
// syntax (chown treats '.' as a user/group separator).
func UserName(domain string) string {
	return "site-" + strings.ReplaceAll(domain, ".", "-")
}

// needsIsolation returns true for site types that run PHP and need a
// dedicated system user.
func needsIsolation(siteType string) bool {
	return siteType != "html"
}

// wpMemoryLimitRE matches a WP_MEMORY_LIMIT define line in wp-config.php.
var wpMemoryLimitRE = regexp.MustCompile(`(?m)^\s*define\s*\(\s*['"]WP_MEMORY_LIMIT['"]\s*,\s*['"][^'"]*['"]\s*\)\s*;`)

// writeWPConfigMemoryLimit sets or updates WP_MEMORY_LIMIT in wp-config.php.
// If the constant already exists it is replaced; otherwise it is inserted
// just before the "/* That's all, stop editing! */" marker.
func writeWPConfigMemoryLimit(docRoot string, limitMB uint64) error {
	path := filepath.Join(docRoot, "wp-config.php")
	data, err := os.ReadFile(path) //nolint:gosec // derived from validated site name
	if err != nil {
		return fmt.Errorf("read wp-config.php: %w", err)
	}
	val := fmt.Sprintf("%dM", limitMB)
	replacement := "define('WP_MEMORY_LIMIT', '" + val + "');"
	content := string(data)

	if wpMemoryLimitRE.MatchString(content) {
		content = wpMemoryLimitRE.ReplaceAllString(content, replacement)
	} else {
		marker := "/* That's all, stop editing!"
		content = strings.Replace(content, marker, replacement+"\n\n"+marker, 1)
	}
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // wp-config, perms set by installer
}

// userExists checks whether a system user exists by running `id <name>`.
func (m *Manager) userExists(ctx context.Context, name string) bool {
	return m.runner.Run(ctx, "id", name) == nil
}
