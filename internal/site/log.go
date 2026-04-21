package site

import (
	"context"
	"fmt"
	"io"
	"strconv"
)

// defaultLogDir is the OLS per-site log directory used in production.
const defaultLogDir = "/var/log/lsws"

// Log tails the site's access or error log to stdout/stderr. mode must be
// "error" or "access". lines is passed as `tail -n`. When follow is true,
// `tail -f` is appended so the call blocks until ctx is canceled.
func (m *Manager) Log(ctx context.Context, name, mode string, lines int, follow bool, stdout, stderr io.Writer) error {
	if _, ok := m.store.Find(name); !ok {
		return fmt.Errorf("site: log %s: not found", name)
	}
	if mode != "error" && mode != "access" {
		return fmt.Errorf("site: log %s: invalid mode %q (use error or access)", name, mode)
	}

	path := fmt.Sprintf("%s/%s.%s.log", m.logDir, name, mode)
	args := []string{"-n", strconv.Itoa(lines)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, path)

	if err := m.runner.Stream(ctx, nil, stdout, stderr, "tail", args...); err != nil {
		return fmt.Errorf("site: log %s: %w", name, err)
	}
	return nil
}
