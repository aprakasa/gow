package stack

import "fmt"

// Component describes a single installable stack element with lifecycle hooks.
type Component struct {
	Name        string
	InstallFn   func(Runner) error
	UninstallFn func(Runner) error
	VerifyFn    func(Runner) error
	StatusFn    func(Runner) (string, error) // optional: returns version/detail string
}

// Install runs the component's install function. On error, it returns
// a wrapped error with the component name.
func (c Component) Install(r Runner) error {
	if err := c.InstallFn(r); err != nil {
		return fmt.Errorf("stack: install %s: %w", c.Name, err)
	}
	return nil
}

// Uninstall runs the component's uninstall function.
func (c Component) Uninstall(r Runner) error {
	if err := c.UninstallFn(r); err != nil {
		return fmt.Errorf("stack: uninstall %s: %w", c.Name, err)
	}
	return nil
}

// Verify checks that the component is healthy.
func (c Component) Verify(r Runner) error {
	if err := c.VerifyFn(r); err != nil {
		return fmt.Errorf("stack: verify %s: %w", c.Name, err)
	}
	return nil
}

// Status returns a human-readable detail string for the component (e.g. version).
// Returns ("", nil) if StatusFn is not set.
func (c Component) Status(r Runner) (string, error) {
	if c.StatusFn == nil {
		return "", nil
	}
	return c.StatusFn(r)
}

// Registry returns all known components in install order.
func Registry(phpVer string) []Component {
	return []Component{OLS(), LSPHP(phpVer), MariaDB(), Redis()}
}

// Lookup returns components matching the given names. If names is empty,
// returns all components.
func Lookup(names []string, phpVer string) []Component {
	if len(names) == 0 {
		return Registry(phpVer)
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []Component
	for _, c := range Registry(phpVer) {
		if want[c.Name] {
			out = append(out, c)
		}
	}
	return out
}
