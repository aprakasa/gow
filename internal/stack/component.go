package stack

import (
	"context"
	"fmt"
)

// Component describes a single installable stack element with lifecycle hooks.
type Component struct {
	Name string

	// Package lifecycle
	InstallFn func(context.Context, Runner) error
	UpgradeFn func(context.Context, Runner) error
	RemoveFn  func(context.Context, Runner) error // apt-get remove; keeps configs
	PurgeFn   func(context.Context, Runner) error // apt-get purge + deep clean
	VerifyFn  func(context.Context, Runner) error
	StatusFn  func(context.Context, Runner) (string, error)

	// Service lifecycle (nil for binary-only components)
	StartFn   func(context.Context, Runner) error
	StopFn    func(context.Context, Runner) error
	RestartFn func(context.Context, Runner) error
	ReloadFn  func(context.Context, Runner) error

	// Component-specific (nil for most)
	MigrateFn func(context.Context, Runner, string) error // MariaDB only; arg = target version

	// ActiveFn checks if the service is currently running. nil for non-service components.
	ActiveFn func(context.Context, Runner) error
}

// Install runs the component's InstallFn if set.
func (c Component) Install(ctx context.Context, r Runner) error {
	if c.InstallFn == nil {
		return nil
	}
	if err := c.InstallFn(ctx, r); err != nil {
		return fmt.Errorf("stack: install %s: %w", c.Name, err)
	}
	return nil
}

// Upgrade runs the component's UpgradeFn if set.
func (c Component) Upgrade(ctx context.Context, r Runner) error {
	if c.UpgradeFn == nil {
		return nil
	}
	if err := c.UpgradeFn(ctx, r); err != nil {
		return fmt.Errorf("stack: upgrade %s: %w", c.Name, err)
	}
	return nil
}

// Remove runs the component's RemoveFn if set (apt-get remove; keeps configs).
func (c Component) Remove(ctx context.Context, r Runner) error {
	if c.RemoveFn == nil {
		return nil
	}
	if err := c.RemoveFn(ctx, r); err != nil {
		return fmt.Errorf("stack: remove %s: %w", c.Name, err)
	}
	return nil
}

// Purge runs the component's PurgeFn if set (apt-get purge + deep clean).
func (c Component) Purge(ctx context.Context, r Runner) error {
	if c.PurgeFn == nil {
		return nil
	}
	if err := c.PurgeFn(ctx, r); err != nil {
		return fmt.Errorf("stack: purge %s: %w", c.Name, err)
	}
	return nil
}

// Verify runs the component's VerifyFn.
func (c Component) Verify(ctx context.Context, r Runner) error {
	if err := c.VerifyFn(ctx, r); err != nil {
		return fmt.Errorf("stack: verify %s: %w", c.Name, err)
	}
	return nil
}

// Status returns the component's StatusFn output, or empty string if unset.
func (c Component) Status(ctx context.Context, r Runner) (string, error) {
	if c.StatusFn == nil {
		return "", nil
	}
	return c.StatusFn(ctx, r)
}

// Start runs the component's StartFn if set.
func (c Component) Start(ctx context.Context, r Runner) error {
	if c.StartFn == nil {
		return nil
	}
	if err := c.StartFn(ctx, r); err != nil {
		return fmt.Errorf("stack: start %s: %w", c.Name, err)
	}
	return nil
}

// Stop runs the component's StopFn if set.
func (c Component) Stop(ctx context.Context, r Runner) error {
	if c.StopFn == nil {
		return nil
	}
	if err := c.StopFn(ctx, r); err != nil {
		return fmt.Errorf("stack: stop %s: %w", c.Name, err)
	}
	return nil
}

// Restart runs the component's RestartFn if set.
func (c Component) Restart(ctx context.Context, r Runner) error {
	if c.RestartFn == nil {
		return nil
	}
	if err := c.RestartFn(ctx, r); err != nil {
		return fmt.Errorf("stack: restart %s: %w", c.Name, err)
	}
	return nil
}

// Reload runs the component's ReloadFn if set.
func (c Component) Reload(ctx context.Context, r Runner) error {
	if c.ReloadFn == nil {
		return nil
	}
	if err := c.ReloadFn(ctx, r); err != nil {
		return fmt.Errorf("stack: reload %s: %w", c.Name, err)
	}
	return nil
}

// Migrate runs the component's MigrateFn to move to the target version.
func (c Component) Migrate(ctx context.Context, r Runner, target string) error {
	if c.MigrateFn == nil {
		return fmt.Errorf("stack: migrate not supported for %s", c.Name)
	}
	if err := c.MigrateFn(ctx, r, target); err != nil {
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
func (c Component) Active(ctx context.Context, r Runner) (bool, error) {
	if c.ActiveFn == nil {
		return false, nil
	}
	if err := c.ActiveFn(ctx, r); err != nil {
		return false, nil //nolint:nilerr // a failed check means "not active", not a bug
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
	components = append(components, MariaDB(), Redis(), WPCLI(), Composer(), Certbot())
	return components
}

// Lookup returns components matching the given names plus any LSPHP versions.
// If names is empty and phpVersions is non-empty, returns only LSPHP components.
// If both are empty, returns the full registry (no PHP versions).
func Lookup(names []string, phpVersions []string) []Component {
	if len(names) == 0 && len(phpVersions) > 0 {
		var out []Component
		for _, v := range phpVersions {
			out = append(out, LSPHP(v))
		}
		return out
	}
	if len(names) == 0 {
		return Registry(nil)
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	lsphpWant := make(map[string]bool, len(phpVersions))
	for _, v := range phpVersions {
		lsphpWant["lsphp"+v] = true
	}
	var out []Component
	for _, c := range Registry(phpVersions) {
		if want[c.Name] || lsphpWant[c.Name] {
			out = append(out, c)
		}
	}
	return out
}
