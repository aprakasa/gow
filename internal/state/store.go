// Package state persists the site registry to a JSON file so that allocations
// survive across gow invocations. The store is the single source of truth for
// which sites exist, their chosen presets, and PHP version.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Site represents a managed WordPress site in the persistent registry.
type Site struct {
	Name         string        `json:"name"`
	PHPVersion   string        `json:"php_version"`
	Preset       string        `json:"preset"`
	CustomPreset *CustomPreset `json:"preset_custom,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

// CustomPreset stores the user-specified memory parameters when Preset is
// "custom". The allocator uses these instead of a named preset's values.
type CustomPreset struct {
	PHPMemoryMB    uint64 `json:"php_memory_mb"`
	WorkerBudgetMB uint64 `json:"worker_budget_mb"`
}

// --- Store ---

// Store manages the site registry backed by a JSON file on disk.
type Store struct {
	mu    sync.Mutex
	path  string
	sites []Site
}

// Open loads the store from path. If the file does not exist it is created
// with an empty site list.
func Open(path string) (*Store, error) {
	s := &Store{path: path}

	data, err := os.ReadFile(path) //nolint:gosec // path is set by the CLI, not user input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Fresh store — write an empty registry so callers can rely on the
			// file existing after Open returns.
			s.sites = []Site{}
			if saveErr := s.Save(); saveErr != nil {
				return nil, fmt.Errorf("state: create %s: %w", path, saveErr)
			}
			return s, nil
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}

	if len(data) == 0 {
		s.sites = []Site{}
		return s, nil
	}

	var wrapper struct {
		Sites []Site `json:"sites"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w", path, err)
	}
	s.sites = wrapper.Sites
	return s, nil
}

// Sites returns a copy of the current site list.
func (s *Store) Sites() []Site {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Site, len(s.sites))
	copy(out, s.sites)
	return out
}

// Find returns the site with the given name. The second return value is false
// if no matching site exists.
func (s *Store) Find(name string) (Site, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sites {
		if s.sites[i].Name == name {
			return s.sites[i], true
		}
	}
	return Site{}, false
}

// Add appends a new site. Returns an error if a site with the same name
// already exists.
func (s *Store) Add(site Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.sites {
		if existing.Name == site.Name {
			return fmt.Errorf("state: site %q already exists", site.Name)
		}
	}
	s.sites = append(s.sites, site)
	return nil
}

// Remove deletes the site with the given name. Returns an error if the site
// does not exist.
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, site := range s.sites {
		if site.Name == name {
			s.sites = append(s.sites[:i], s.sites[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("state: site %q not found", name)
}

// Update applies fn to the site with the given name. Returns an error if the
// site does not exist.
func (s *Store) Update(name string, fn func(*Site)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sites {
		if s.sites[i].Name == name {
			fn(&s.sites[i])
			return nil
		}
	}
	return fmt.Errorf("state: site %q not found", name)
}

// Save persists the current state to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wrapper := struct {
		Sites []Site `json:"sites"`
	}{Sites: s.sites}

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("state: write %s: %w", s.path, err)
	}
	return nil
}
