package site

import "fmt"

// Delete removes a site from the state store and runs Reconcile to update the
// OLS configuration. It returns an error if the site does not exist.
func (m *Manager) Delete(name string) error {
	if err := m.store.Remove(name); err != nil {
		return fmt.Errorf("site: delete %s: %w", name, err)
	}

	if err := m.Reconcile(); err != nil {
		return fmt.Errorf("site: delete %s: reconcile: %w", name, err)
	}

	return m.store.Save()
}
