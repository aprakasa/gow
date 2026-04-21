package site

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
)

// WP runs wp-cli against the site's document root with stdio passthrough so
// interactive commands (wp shell, wp db cli) and paginated output work. It
// auto-prepends `--allow-root --path=<docroot>` to the user's args. For
// multisite, the caller is responsible for passing `--url=<blog>` since only
// the user knows which blog context they want.
func (m *Manager) WP(ctx context.Context, name string, stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	s, ok := m.store.Find(name)
	if !ok {
		return fmt.Errorf("site: wp %s: not found", name)
	}
	if siteType(s) == "html" {
		return fmt.Errorf("site: wp %s: not applicable for html sites", name)
	}

	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	full := append([]string{"--allow-root", "--path=" + docRoot}, args...)

	if err := m.runner.Stream(ctx, stdin, stdout, stderr, "wp", full...); err != nil {
		return fmt.Errorf("site: wp %s: %w", name, err)
	}
	return nil
}
