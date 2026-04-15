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
}

// NewManager creates a Manager with the given dependencies. confDir is the
// base directory for rendered OLS configs (e.g., /usr/local/lsws/conf).
func NewManager(store *state.Store, ctrl ols.Controller, specs system.Specs, policy allocator.Policy, confDir string) *Manager {
	return &Manager{
		store:   store,
		ols:     ctrl,
		specs:   specs,
		policy:  policy,
		confDir: confDir,
	}
}

// Reconcile recomputes allocations for all sites, renders their OLS configs,
// validates the configuration, and triggers a graceful OLS reload. With zero
// sites it returns immediately without touching OLS.
func (m *Manager) Reconcile() error {
	sites := m.store.Sites()
	if len(sites) == 0 {
		return nil
	}

	inputs := make([]allocator.SiteInput, len(sites))
	for i, s := range sites {
		in := allocator.SiteInput{
			Name:   s.Name,
			Preset: s.Preset,
		}
		if s.CustomPreset != nil {
			in.CustomPHPMemoryMB = s.CustomPreset.PHPMemoryMB
			in.CustomWorkerBudgetMB = s.CustomPreset.WorkerBudgetMB
		}
		inputs[i] = in
	}

	allocs, err := allocator.Compute(m.specs.TotalRAMMB, m.specs.CPUCores, inputs, m.policy)
	if err != nil {
		return fmt.Errorf("site: reconcile: %w", err)
	}

	for i, a := range allocs {
		s := sites[i]
		vhostDir := filepath.Join(m.confDir, "vhosts", a.Site)
		if err := os.MkdirAll(vhostDir, 0o750); err != nil { //nolint:gosec // OLS needs readable vhost dirs
			return fmt.Errorf("site: create vhost dir %s: %w", vhostDir, err)
		}

		data := template.VHostData{
			Site:             a.Site,
			Domain:           a.Site,
			WebRoot:          filepath.Join("/var/www", a.Site),
			LogDir:           "/var/log/lsws",
			PHPVer:           s.PHPVersion,
			Children:         a.Children,
			PHPMemoryLimitMB: a.PHPMemoryLimitMB,
			MemSoftMB:        a.MemSoftMB,
			MemHardMB:        a.MemHardMB,
		}

		content, err := template.RenderVHost(data)
		if err != nil {
			return fmt.Errorf("site: render vhost %s: %w", a.Site, err)
		}

		vhostPath := filepath.Join(vhostDir, "vhconf.conf")
		if err := os.WriteFile(vhostPath, []byte(content), 0o644); err != nil { //nolint:gosec // config file, not secret
			return fmt.Errorf("site: write %s: %w", vhostPath, err)
		}
	}

	if err := m.ols.Validate(); err != nil {
		return fmt.Errorf("site: reconcile validate: %w", err)
	}
	if err := m.ols.GracefulReload(); err != nil {
		return fmt.Errorf("site: reconcile reload: %w", err)
	}
	return nil
}
