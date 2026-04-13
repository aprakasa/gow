package site

import (
	"fmt"
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

// Create adds a new site to the state store and runs Reconcile to render its
// OLS config and reload the server. It validates the preset before touching
// state so a bad request is rejected early.
func (m *Manager) Create(name, phpVersion, preset string) error {
	if _, err := allocator.LookupPreset(preset); err != nil {
		return fmt.Errorf("site: create %s: %w", name, err)
	}

	site := state.Site{
		Name:       name,
		PHPVersion: phpVersion,
		Preset:     preset,
		CreatedAt:  time.Now().UTC(),
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
