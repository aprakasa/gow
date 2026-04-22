package site

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultLogrotateConfPath is the production path for the gow logrotate config.
const defaultLogrotateConfPath = "/etc/logrotate.d/gow"

// olsLogsDir is the OLS internal logs directory containing PHP slow logs.
const olsLogsDir = "/usr/local/lsws/logs"

// writeLogrotateConfig writes a logrotate config for gow-managed log files.
// It covers two wildcard paths: per-site access/error logs and OLS PHP slow
// logs. The config is idempotent — safe to call on every reconcile.
func (m *Manager) writeLogrotateConfig() error {
	siteLogPattern := filepath.Join(m.logDir, "*.log")
	slowLogPattern := filepath.Join(olsLogsDir, "php_slowlog_*.log")

	content := fmt.Sprintf(`%s {
    weekly
    rotate 4
    compress
    delaycompress
    missingok
    notifempty
    create 0644 root root
    sharedscripts
    postrotate
        systemctl reload lshttpd >/dev/null 2>&1 || true
    endscript
}

%s {
    weekly
    rotate 2
    compress
    delaycompress
    missingok
    notifempty
    nocreate
    size 10M
}
`, siteLogPattern, slowLogPattern)

	confPath := defaultLogrotateConfPath
	if m.logrotateConfPath != "" {
		confPath = m.logrotateConfPath
	}

	dir := filepath.Dir(confPath)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // logrotate.d must be world-readable
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(confPath, []byte(content), 0o644); err != nil { //nolint:gosec // logrotate config, not secret
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	return nil
}
