package site

import (
	"context"
	"fmt"
	"path/filepath"
)

// Flush purges the site's caches. Always runs `wp cache flush` for WordPress
// object cache. For LSCache-enabled sites it additionally runs
// `wp litespeed-purge all`; failures of that second call are treated as
// warnings (plugin may be disabled) and do not fail the overall command.
func (m *Manager) Flush(ctx context.Context, name string) error {
	s, ok := m.store.Find(name)
	if !ok {
		return fmt.Errorf("site: flush %s: not found", name)
	}
	if siteType(s) == "html" {
		return fmt.Errorf("site: flush %s: not applicable for html sites", name)
	}

	docRoot := filepath.Join(m.webRoot, name, "htdocs")
	base := []string{"--allow-root", "--path=" + docRoot}
	if s.Multisite != "" {
		base = append(base, "--url="+name)
	}

	flushArgs := append([]string{"cache", "flush"}, base...)
	if err := m.runner.Run(ctx, "wp", flushArgs...); err != nil {
		return fmt.Errorf("site: flush %s: cache flush: %w", name, err)
	}

	if s.CacheMode == "lscache" {
		purgeArgs := append([]string{"litespeed-purge", "all"}, base...)
		// Plugin may be disabled; don't fail the command on this.
		_ = m.runner.Run(ctx, "wp", purgeArgs...)
	}
	return nil
}
