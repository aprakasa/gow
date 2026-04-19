package ols

import (
	"context"
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
	Validate(ctx context.Context) error
	GracefulReload(ctx context.Context) error
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
func (c *LSControl) Validate(ctx context.Context) error {
	return nil
}

// GracefulReload restarts OLS to pick up config changes.
func (c *LSControl) GracefulReload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binPath, "restart") //nolint:gosec // binPath set by CLI, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrReloadFailed, out)
	}
	return nil
}
