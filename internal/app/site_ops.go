package app

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/aprakasa/gow/internal/metrics"
	"github.com/aprakasa/gow/internal/state"
)

// LogFlags holds flags for the `site log` command.
type LogFlags struct {
	Access bool // --access
	Error  bool // --error
	Follow bool // --follow / -f
	Lines  int  // --lines / -n
}

// RunFlush purges a site's caches via Manager.Flush.
func RunFlush(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Flush(d.Ctx, domain); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Flushed caches for %s.\n", domain)
	return nil
}

// RunWP passes arguments through to wp-cli scoped to a site's document root.
func RunWP(cfg CLIConfig, domain string, args []string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	return m.WP(d.Ctx, domain, d.Stdin, d.Stdout, d.Stderr, args)
}

// RunLog tails the site's log file to stdout/stderr.
func RunLog(cfg CLIConfig, lf LogFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	if lf.Access && lf.Error {
		return fmt.Errorf("--access and --error are mutually exclusive")
	}
	mode := "error"
	if lf.Access {
		mode = "access"
	}
	lines := lf.Lines
	if lines <= 0 {
		lines = 100
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	return m.Log(d.Ctx, domain, mode, lines, lf.Follow, d.Stdout, d.Stderr)
}

// RunMetrics collects and displays live server metrics.
func RunMetrics(cfg CLIConfig, w io.Writer, d Deps, jsonOutput bool) error {
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return err
	}
	sites := store.Sites()
	c := metrics.NewCollector(d.NewRunner(), cfg.WebRoot)
	sm, siteM, err := c.Collect(d.Ctx, sites)
	if err != nil {
		return err
	}
	if jsonOutput {
		return formatMetricsJSON(w, sites, sm, siteM)
	}
	return formatMetricsTable(w, sm, siteM)
}

func formatMetricsTable(w io.Writer, sm metrics.ServerMetrics, siteM []metrics.SiteMetrics) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SITE\tACTIVE\tREQS\tCACHE\tDISK\tDB\tSLOW")
	for _, m := range siteM {
		active := fmt.Sprintf("%d", m.ActiveReqs)
		cache := fmt.Sprintf("%d", m.CacheHits)
		disk := formatMB(m.DiskMB)
		db := formatMB(m.DBSizeMB)
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%d\n",
			m.Site, active, m.TotalReqs, cache, disk, db, m.SlowLogCount)
	}
	fmt.Fprintf(tw, "\nRedis: %.1fMB (%.1f%% hit) | MariaDB: %d/%d conn | Disk: %s\n",
		sm.RedisUsedMB, sm.RedisHitRate, sm.MariaDBConns, sm.MariaDBMaxConns, formatMB(sm.TotalDiskMB))
	return tw.Flush()
}

func formatMetricsJSON(w io.Writer, _ []state.Site, sm metrics.ServerMetrics, siteM []metrics.SiteMetrics) error {
	type jsonOutput struct {
		Server metrics.ServerMetrics `json:"server"`
		Sites  []metrics.SiteMetrics `json:"sites"`
	}
	return json.NewEncoder(w).Encode(jsonOutput{Server: sm, Sites: siteM})
}

func formatMB(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1fGB", mb/1024)
	}
	if mb >= 1 {
		return fmt.Sprintf("%.0fMB", mb)
	}
	return fmt.Sprintf("%.0fKB", mb*1024)
}
