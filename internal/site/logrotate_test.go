package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
	"github.com/aprakasa/gow/internal/testmock"
)

func TestWriteLogrotateConfig(t *testing.T) {
	dir := t.TempDir()

	store, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	logDir := filepath.Join(dir, "logs")
	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "httpd_config.conf"), []byte(baseOLSConf), 0o644); err != nil { //nolint:gosec // test config
		t.Fatalf("write httpd_config: %v", err)
	}

	mgr := NewManager(store, ols.NewController(testmock.WriteMock(t, "exit 0")),
		system.Specs{TotalRAMMB: 8192, CPUCores: 4},
		allocator.DefaultPolicy(), confDir, filepath.Join(dir, "www"),
		&testmock.NoopRunner{})
	mgr.SetLogDir(logDir)

	// Override logrotate path so the test writes to temp dir.
	confPath := filepath.Join(dir, "etc", "logrotate.d", "gow")
	mgr.SetLogrotateConfPath(confPath)

	if err := mgr.writeLogrotateConfig(); err != nil {
		t.Fatalf("writeLogrotateConfig: %v", err)
	}

	data, err := os.ReadFile(confPath) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read logrotate config: %v", err)
	}
	got := string(data)

	// Per-site log block.
	sitePattern := filepath.Join(logDir, "*.log")
	if !strings.Contains(got, sitePattern) {
		t.Errorf("config should contain %q", sitePattern)
	}
	for _, want := range []string{"weekly", "rotate 4", "compress", "delaycompress",
		"missingok", "notifempty", "create 0644 root root", "sharedscripts",
		"postrotate", "systemctl reload lshttpd", "endscript"} {
		if !strings.Contains(got, want) {
			t.Errorf("config should contain %q", want)
		}
	}

	// PHP slow log block.
	slowPattern := filepath.Join(olsLogsDir, "php_slowlog_*.log")
	if !strings.Contains(got, slowPattern) {
		t.Errorf("config should contain %q", slowPattern)
	}
	for _, want := range []string{"rotate 2", "nocreate", "size 10M"} {
		if !strings.Contains(got, want) {
			t.Errorf("config should contain %q", want)
		}
	}
}
