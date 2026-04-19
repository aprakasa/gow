package site

import (
	"context"
	"fmt"

	"github.com/aprakasa/gow/internal/state"
)

// Offline puts a site into maintenance mode.
func (m *Manager) Offline(ctx context.Context, name string) error {
	if err := m.store.Update(name, func(s *state.Site) {
		s.Maintenance = true
	}); err != nil {
		return fmt.Errorf("site: offline %s: %w", name, err)
	}
	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: offline %s: reconcile: %w", name, err)
	}
	return m.store.Save()
}

// Online takes a site out of maintenance mode.
func (m *Manager) Online(ctx context.Context, name string) error {
	if err := m.store.Update(name, func(s *state.Site) {
		s.Maintenance = false
	}); err != nil {
		return fmt.Errorf("site: online %s: %w", name, err)
	}
	if err := m.Reconcile(ctx); err != nil {
		return fmt.Errorf("site: online %s: reconcile: %w", name, err)
	}
	return m.store.Save()
}
