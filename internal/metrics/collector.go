// Package metrics collects live server metrics from OLS, Redis, MariaDB, and disk.
package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aprakasa/gow/internal/dbsql"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
)

const olsReportPath = "/dev/shm/ols/status/.rtreport"

// ServerMetrics holds aggregate server-wide metrics.
type ServerMetrics struct {
	TotalReqs       int
	ActiveReqs      int
	RedisUsedMB     float64
	RedisMaxMB      float64
	RedisHitRate    float64
	MariaDBConns    int
	MariaDBMaxConns int
	TotalDiskMB     float64
}

// SiteMetrics holds per-site live metrics.
type SiteMetrics struct {
	Site         string
	ActiveReqs   int
	TotalReqs    int
	CacheHits    int
	DiskMB       float64
	DBSizeMB     float64
	SlowLogCount int
	RedisHitRate float64
}

// Collector gathers live server metrics from OLS rtreport, Redis, MariaDB,
// and disk usage. Each source degrades gracefully on failure.
type Collector struct {
	runner  stack.Runner
	webRoot string
}

// NewCollector creates a Collector with the given runner and web root.
func NewCollector(runner stack.Runner, webRoot string) *Collector {
	return &Collector{runner: runner, webRoot: webRoot}
}

// Collect gathers all metrics. It runs data sources in parallel and merges
// results. Returns server-wide aggregate and per-site metrics.
func (c *Collector) Collect(ctx context.Context, sites []state.Site) (ServerMetrics, []SiteMetrics, error) {
	siteM := make([]SiteMetrics, len(sites))
	for i, s := range sites {
		siteM[i] = SiteMetrics{Site: s.Name}
	}

	var sm ServerMetrics

	// OLS rtreport — parse per-vhost request rates.
	c.collectOLS(ctx, sites, &sm, siteM)

	// Redis info.
	c.collectRedis(ctx, &sm)

	// MariaDB connections + per-site DB size.
	c.collectMariaDB(ctx, sites, &sm, siteM)

	// Disk + slowlog per site.
	c.collectDiskAndSlowlog(ctx, sites, &sm, siteM)

	return sm, siteM, nil
}

func (c *Collector) collectOLS(ctx context.Context, sites []state.Site, sm *ServerMetrics, siteM []SiteMetrics) {
	out, err := c.runner.Output(ctx, "cat", olsReportPath)
	if err != nil {
		return
	}
	vhostData := parseRtreport(out)
	for i, s := range sites {
		if vd, ok := vhostData[s.Name]; ok {
			siteM[i].ActiveReqs = vd.activeReqs
			siteM[i].TotalReqs = vd.totalReqs
			siteM[i].CacheHits = vd.cacheHits
			sm.TotalReqs += vd.totalReqs
			sm.ActiveReqs += vd.activeReqs
		}
	}
}

func (c *Collector) collectRedis(ctx context.Context, sm *ServerMetrics) {
	memOut, err := c.runner.Output(ctx, "redis-cli", "info", "memory")
	if err != nil {
		return
	}
	sm.RedisUsedMB = parseRedisMemory(memOut)

	statsOut, err := c.runner.Output(ctx, "redis-cli", "info", "stats")
	if err != nil {
		return
	}
	sm.RedisHitRate = parseRedisHitRate(statsOut)
}

func (c *Collector) collectMariaDB(ctx context.Context, sites []state.Site, sm *ServerMetrics, siteM []SiteMetrics) {
	// Connection count.
	connOut, err := c.runner.Output(ctx, "mariadb", "-e",
		"SHOW STATUS LIKE 'Threads_connected'")
	if err == nil {
		sm.MariaDBConns = parseMariaDBValue(connOut)
	}
	maxOut, err := c.runner.Output(ctx, "mariadb", "-e",
		"SHOW VARIABLES LIKE 'max_connections'")
	if err == nil {
		sm.MariaDBMaxConns = parseMariaDBValue(maxOut)
	}

	// Per-site DB size.
	for i, s := range sites {
		if s.Type == "html" {
			continue
		}
		db := dbsql.DBName(s.Name)
		out, err := c.runner.Output(ctx, "mariadb", "-e",
			fmt.Sprintf("SELECT ROUND(SUM(data_length+index_length)/1024/1024, 1) FROM information_schema.TABLES WHERE table_schema='%s'", db))
		if err == nil {
			siteM[i].DBSizeMB = parseMariaDBFloat(out)
		}
		sm.TotalDiskMB += siteM[i].DBSizeMB
	}
}

func (c *Collector) collectDiskAndSlowlog(ctx context.Context, sites []state.Site, sm *ServerMetrics, siteM []SiteMetrics) {
	for i, s := range sites {
		path := c.webRoot + "/" + s.Name
		out, err := c.runner.Output(ctx, "du", "-sb", path)
		if err == nil {
			siteM[i].DiskMB = parseDuBytes(out)
			sm.TotalDiskMB += siteM[i].DiskMB
		}

		if s.Type != "html" {
			slowPath := "/usr/local/lsws/logs/php_slowlog_" + s.Name + ".log"
			out, err := c.runner.Output(ctx, "wc", "-l", slowPath)
			if err == nil {
				siteM[i].SlowLogCount = parseWcLines(out)
			}
		}
	}
}

// --- Parsing helpers ---

type vhostStats struct {
	activeReqs int
	totalReqs  int
	cacheHits  int
}

func parseRtreport(content string) map[string]vhostStats {
	result := map[string]vhostStats{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "REQ_RATE [") {
			continue
		}
		// Extract vhost name between [ and ].
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		name := line[start+1 : end]
		stats := vhostStats{}
		remainder := strings.TrimPrefix(line[end+1:], ":")
		for _, field := range strings.Split(remainder, ",") {
			field = strings.TrimSpace(field)
			kv := strings.SplitN(field, ":", 2)
			if len(kv) != 2 {
				continue
			}
			val := strings.TrimSpace(kv[1])
			switch strings.TrimSpace(kv[0]) {
			case "REQ_PROCESSING":
				stats.activeReqs, _ = strconv.Atoi(val)
			case "TOT_REQS":
				stats.totalReqs, _ = strconv.Atoi(val)
			case "TOTAL_PUB_CACHE_HITS":
				stats.cacheHits, _ = strconv.Atoi(val)
			}
		}
		result[name] = stats
	}
	return result
}

func parseRedisMemory(info string) float64 {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory_human:") {
			return parseHumanBytes(strings.TrimPrefix(line, "used_memory_human:"))
		}
	}
	return 0
}

func parseRedisHitRate(info string) float64 {
	var hits, misses float64
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "keyspace_hits:") {
			hits, _ = strconv.ParseFloat(strings.TrimPrefix(line, "keyspace_hits:"), 64)
		}
		if strings.HasPrefix(line, "keyspace_misses:") {
			misses, _ = strconv.ParseFloat(strings.TrimPrefix(line, "keyspace_misses:"), 64)
		}
	}
	if hits+misses == 0 {
		return 0
	}
	return hits / (hits + misses) * 100
}

func parseMariaDBValue(output string) int {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			v, err := strconv.Atoi(fields[len(fields)-1])
			if err == nil {
				return v
			}
		}
	}
	return 0
}

func parseMariaDBFloat(output string) float64 {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

func parseDuBytes(output string) float64 {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 1 {
		return 0
	}
	bytes, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return float64(bytes) / 1024 / 1024
}

func parseWcLines(output string) int {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 1 {
		return 0
	}
	n, _ := strconv.Atoi(fields[0])
	return n
}

// parseHumanBytes converts strings like "5.25M", "1.32G", "832K" to MB.
func parseHumanBytes(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" || s == "0" {
		return 0
	}
	last := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	switch last {
	case 'G':
		return num * 1024
	case 'M':
		return num
	case 'K':
		return num / 1024
	case 'B':
		return num / 1024 / 1024
	}
	return 0
}
