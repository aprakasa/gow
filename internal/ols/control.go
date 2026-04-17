// Package ols wraps OpenLiteSpeed control operations (config validation,
// graceful reload) behind testable interfaces.
package ols

import (
	"errors"
	"fmt"
	"os/exec"
)

// DefaultBinPath is the standard location of the OLS control binary.
const DefaultBinPath = "/usr/local/lsws/bin/lswsctrl"

var (
	// ErrReloadFailed is returned when a graceful reload cannot be completed.
	ErrReloadFailed = errors.New("ols: graceful reload failed")
)

// Controller abstracts OpenLiteSpeed control operations for testability.
type Controller interface {
	Validate() error
	GracefulReload() error
}

var _ Controller = (*LSControl)(nil)

// LSControl implements Controller by shelling out to the OLS binary.
type LSControl struct {
	binPath string
}

// NewController creates an LSControl that delegates to the given binary path.
func NewController(binPath string) *LSControl {
	return &LSControl{binPath: binPath}
}

// Validate is a no-op because OpenLiteSpeed has no config-test subcommand
// (unlike nginx -t). Bad configs are caught by GracefulReload instead.
func (c *LSControl) Validate() error {
	return nil
}

// GracefulReload restarts OLS to pick up config changes.
func (c *LSControl) GracefulReload() error {
	cmd := exec.Command(c.binPath, "restart") //nolint:gosec // binPath set by CLI, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrReloadFailed, out)
	}
	return nil
}
