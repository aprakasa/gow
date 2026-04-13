package site

import (
	"fmt"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

// Tune changes the preset for an existing site and re-reconciles all OLS
// configurations. It validates the new preset before mutating state so a bad
// request is rejected early and the original preset is preserved. When preset
// is "custom", custom must be non-nil with both fields > 0; when switching
// from custom to a named preset, CustomPreset is cleared.
func (m *Manager) Tune(name, preset string, custom *state.CustomPreset) error {
	if preset == "custom" {
		if custom == nil || custom.PHPMemoryMB == 0 || custom.WorkerBudgetMB == 0 {
			return fmt.Errorf("site: tune %s: custom preset requires php_memory_mb and worker_budget_mb > 0", name)
		}
	} else {
		if _, err := allocator.LookupPreset(preset); err != nil {
			return fmt.Errorf("site: tune %s: %w", name, err)
		}
	}

	if err := m.store.Update(name, func(s *state.Site) {
		s.Preset = preset
		s.CustomPreset = custom
	}); err != nil {
		return fmt.Errorf("site: tune %s: %w", name, err)
	}

	if err := m.Reconcile(); err != nil {
		return fmt.Errorf("site: tune %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
