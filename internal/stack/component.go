package stack

import "fmt"

// Component describes a single installable stack element with lifecycle hooks.
type Component struct {
	Name string

	// Package lifecycle
	InstallFn func(Runner) error
	UpgradeFn func(Runner) error
	RemoveFn  func(Runner) error // apt-get remove; keeps configs
	PurgeFn   func(Runner) error // apt-get purge + deep clean
	VerifyFn  func(Runner) error
	StatusFn  func(Runner) (string, error)

	// Service lifecycle (nil for binary-only components)
	StartFn   func(Runner) error
	StopFn    func(Runner) error
	RestartFn func(Runner) error
	ReloadFn  func(Runner) error

	// Component-specific (nil for most)
	MigrateFn func(Runner, string) error // MariaDB only; arg = target version

	// ActiveFn checks if the service is currently running. nil for non-service components.
	ActiveFn func(Runner) error
}

func (c Component) Install(r Runner) error {
	if c.InstallFn == nil {
		return nil
	}
	if err := c.InstallFn(r); err != nil {
		return fmt.Errorf("stack: install %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Upgrade(r Runner) error {
	if c.UpgradeFn == nil {
		return nil
	}
	if err := c.UpgradeFn(r); err != nil {
		return fmt.Errorf("stack: upgrade %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Remove(r Runner) error {
	if c.RemoveFn == nil {
		return nil
	}
	if err := c.RemoveFn(r); err != nil {
		return fmt.Errorf("stack: remove %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Purge(r Runner) error {
	if c.PurgeFn == nil {
		return nil
	}
	if err := c.PurgeFn(r); err != nil {
		return fmt.Errorf("stack: purge %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Verify(r Runner) error {
	if err := c.VerifyFn(r); err != nil {
		return fmt.Errorf("stack: verify %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Status(r Runner) (string, error) {
	if c.StatusFn == nil {
		return "", nil
	}
	return c.StatusFn(r)
}

func (c Component) Start(r Runner) error {
	if c.StartFn == nil {
		return nil
	}
	if err := c.StartFn(r); err != nil {
		return fmt.Errorf("stack: start %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Stop(r Runner) error {
	if c.StopFn == nil {
		return nil
	}
	if err := c.StopFn(r); err != nil {
		return fmt.Errorf("stack: stop %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Restart(r Runner) error {
	if c.RestartFn == nil {
		return nil
	}
	if err := c.RestartFn(r); err != nil {
		return fmt.Errorf("stack: restart %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Reload(r Runner) error {
	if c.ReloadFn == nil {
		return nil
	}
	if err := c.ReloadFn(r); err != nil {
		return fmt.Errorf("stack: reload %s: %w", c.Name, err)
	}
	return nil
}

func (c Component) Migrate(r Runner, target string) error {
	if c.MigrateFn == nil {
		return fmt.Errorf("stack: migrate not supported for %s", c.Name)
	}
	if err := c.MigrateFn(r, target); err != nil {
		return fmt.Errorf("stack: migrate %s: %w", c.Name, err)
	}
	return nil
}

// HasService returns true if the component has any service lifecycle functions.
func (c Component) HasService() bool {
	return c.StartFn != nil
}

// Active checks if the component's service is currently running.
// Returns false (nil error) if ActiveFn is not set.
func (c Component) Active(r Runner) (bool, error) {
	if c.ActiveFn == nil {
		return false, nil
	}
	err := c.ActiveFn(r)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Registry returns all known components in install order.
func Registry(phpVersions []string) []Component {
	var components []Component
	components = append(components, OLS())
	for _, v := range phpVersions {
		components = append(components, LSPHP(v))
	}
	components = append(components, MariaDB(), Redis(), WPCLI(), Composer())
	return components
}

// Lookup returns components matching the given names. If names is empty,
// returns all components.
func Lookup(names []string, phpVersions []string) []Component {
	if len(names) == 0 {
		return Registry(phpVersions)
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []Component
	for _, c := range Registry(phpVersions) {
		if want[c.Name] {
			out = append(out, c)
		}
	}
	return out
}
