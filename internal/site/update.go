package site

import (
	"fmt"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

// Update modifies PHP version and/or preset for an existing site, then
// re-reconciles all OLS configurations. Empty phpVersion or preset means
// "no change". It validates the new preset before mutating state.
func (m *Manager) Update(name, phpVersion, preset string, custom *state.CustomPreset) error {
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
	}); err != nil {
		return fmt.Errorf("site: update %s: %w", name, err)
	}

	if err := m.Reconcile(); err != nil {
		return fmt.Errorf("site: update %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
