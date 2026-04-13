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
	// ErrConfigInvalid is returned when the OLS configuration fails validation.
	ErrConfigInvalid = errors.New("ols: configuration validation failed")
	// ErrReloadFailed is returned when a graceful reload cannot be completed.
	ErrReloadFailed = errors.New("ols: graceful reload failed")
)

// Controller wraps the OLS control binary (typically lswsctrl).
type Controller struct {
	binPath string
}

// NewController creates a Controller that delegates to the given binary.
func NewController(binPath string) *Controller {
	return &Controller{binPath: binPath}
}

// Validate runs the config-test subcommand and returns ErrConfigInvalid on
// failure. Stderr from the binary is included in the error message.
func (c *Controller) Validate() error {
	cmd := exec.Command(c.binPath, "test") //nolint:gosec // binPath set by CLI, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrConfigInvalid, out)
	}
	return nil
}

// GracefulReload triggers a graceful OLS restart and returns ErrReloadFailed
// on failure.
func (c *Controller) GracefulReload() error {
	cmd := exec.Command(c.binPath, "restart") //nolint:gosec // binPath set by CLI, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrReloadFailed, out)
	}
	return nil
}
