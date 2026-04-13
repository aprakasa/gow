package site

import (
	"fmt"
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

// Create adds a new site to the state store and runs Reconcile to render its
// OLS config and reload the server. It validates the preset before touching
// state so a bad request is rejected early. When preset is "custom", custom
// must be non-nil with both PHPMemoryMB and WorkerBudgetMB > 0.
func (m *Manager) Create(name, phpVersion, preset string, custom *state.CustomPreset) error {
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
		PHPVersion:   phpVersion,
		Preset:       preset,
		CustomPreset: custom,
		CreatedAt:    time.Now().UTC(),
	}
	if err := m.store.Add(site); err != nil {
		return fmt.Errorf("site: create %s: %w", name, err)
	}

	if err := m.Reconcile(); err != nil {
		// Best-effort rollback: remove the site we just added.
		_ = m.store.Remove(name)
		return fmt.Errorf("site: create %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
