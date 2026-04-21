package metrics

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/testmock"
)

func TestCollect_OLSRtreport_ParsesPerVhostRequests(t *testing.T) {
	rtreport := `VERSION: LiteSpeed Web Server/Open/1.8.5
UPTIME: 02:02:09
MAXCONN: 10000, MAXSSL_CONN: 10000, PLAINCONN: 2, AVAILCONN: 9998
REQ_RATE []: REQ_PROCESSING: 0, REQ_PER_SEC: 0, TOT_REQS: 7
REQ_RATE [blog.test]: REQ_PROCESSING: 3, REQ_PER_SEC: 1.2, TOT_REQS: 450, TOTAL_PUB_CACHE_HITS: 200
REQ_RATE [static.test]: REQ_PROCESSING: 0, REQ_PER_SEC: 0, TOT_REQS: 12
EOF
`
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": rtreport,
		},
	}
	c := NewCollector(rr, "/var/www")
	sites := []state.Site{
		{Name: "blog.test", Type: "wp"},
		{Name: "static.test", Type: "html"},
	}
	sm, siteM, err := c.Collect(context.Background(), sites)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	if sm.TotalReqs != 462 {
		t.Errorf("TotalReqs = %d, want 462", sm.TotalReqs)
	}
	blog := findSiteMetrics(t, siteM, "blog.test")
	if blog.ActiveReqs != 3 {
		t.Errorf("blog.test ActiveReqs = %d, want 3", blog.ActiveReqs)
	}
	if blog.CacheHits != 200 {
		t.Errorf("blog.test CacheHits = %d, want 200", blog.CacheHits)
	}
	static := findSiteMetrics(t, siteM, "static.test")
	if static.ActiveReqs != 0 {
		t.Errorf("static.test ActiveReqs = %d, want 0", static.ActiveReqs)
	}
}

func TestCollect_Redis_ParsesMemoryAndHitRate(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": "EOF\n",
		},
		redisInfo: map[string]string{
			"memory": "used_memory:5505024\nused_memory_human:5.25M\nmaxmemory_human:0B\n",
			"stats":  "keyspace_hits:942\nkeyspace_misses:58\n",
		},
	}
	c := NewCollector(rr, "/var/www")
	sm, _, err := c.Collect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	if sm.RedisUsedMB != 5.25 {
		t.Errorf("RedisUsedMB = %.2f, want 5.25", sm.RedisUsedMB)
	}
	wantHitRate := 942.0 / (942 + 58) * 100
	if diff := sm.RedisHitRate - wantHitRate; diff < -0.01 || diff > 0.01 {
		t.Errorf("RedisHitRate = %.2f, want %.2f", sm.RedisHitRate, wantHitRate)
	}
}

func TestCollect_Redis_NoHitsNoMisses(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": "EOF\n",
		},
		redisInfo: map[string]string{
			"memory": "used_memory:0\nused_memory_human:0B\n",
			"stats":  "keyspace_hits:0\nkeyspace_misses:0\n",
		},
	}
	c := NewCollector(rr, "/var/www")
	sm, _, err := c.Collect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	if sm.RedisHitRate != 0 {
		t.Errorf("RedisHitRate = %.2f, want 0 when no hits or misses", sm.RedisHitRate)
	}
}

func TestCollect_MariaDB_ParsesConnectionsAndDBSize(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": "EOF\n",
		},
		mariaDBStatus:  "Variable_name\tValue\nThreads_connected\t8\n",
		mariaDBMaxConn: "Variable_name\tValue\nmax_connections\t151\n",
		dbSizes:        map[string]float64{"wp_blog_test": 28.5},
	}
	c := NewCollector(rr, "/var/www")
	sites := []state.Site{
		{Name: "blog.test", Type: "wp"},
	}
	sm, siteM, err := c.Collect(context.Background(), sites)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	if sm.MariaDBConns != 8 {
		t.Errorf("MariaDBConns = %d, want 8", sm.MariaDBConns)
	}
	if sm.MariaDBMaxConns != 151 {
		t.Errorf("MariaDBMaxConns = %d, want 151", sm.MariaDBMaxConns)
	}
	blog := findSiteMetrics(t, siteM, "blog.test")
	if blog.DBSizeMB != 28.5 {
		t.Errorf("blog.test DBSizeMB = %.1f, want 28.5", blog.DBSizeMB)
	}
}

func TestCollect_Disk_ParsesDuOutput(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": "EOF\n",
		},
		diskSizes: map[string]int64{
			"/var/www/blog.test": 450971966, // ~430MB
		},
	}
	c := NewCollector(rr, "/var/www")
	sites := []state.Site{
		{Name: "blog.test", Type: "wp"},
	}
	_, siteM, err := c.Collect(context.Background(), sites)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	blog := findSiteMetrics(t, siteM, "blog.test")
	if blog.DiskMB < 429 || blog.DiskMB > 431 {
		t.Errorf("blog.test DiskMB = %.1f, want ~430", blog.DiskMB)
	}
}

func TestCollect_Slowlog_CountsLines(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport":                  "EOF\n",
			"/usr/local/lsws/logs/php_slowlog_blog.test.log": "line1\nline2\nline3\n",
		},
	}
	c := NewCollector(rr, "/var/www")
	sites := []state.Site{
		{Name: "blog.test", Type: "wp"},
	}
	_, siteM, err := c.Collect(context.Background(), sites)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	blog := findSiteMetrics(t, siteM, "blog.test")
	if blog.SlowLogCount != 3 {
		t.Errorf("blog.test SlowLogCount = %d, want 3", blog.SlowLogCount)
	}
}

func TestCollect_HTMLSite_SkipsDBAndRedis(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{
			"/dev/shm/ols/status/.rtreport": "EOF\n",
		},
		diskSizes: map[string]int64{
			"/var/www/static.test": 2555,
		},
	}
	c := NewCollector(rr, "/var/www")
	sites := []state.Site{
		{Name: "static.test", Type: "html"},
	}
	_, siteM, err := c.Collect(context.Background(), sites)
	if err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	s := findSiteMetrics(t, siteM, "static.test")
	if s.DBSizeMB != 0 {
		t.Errorf("html site DBSizeMB = %.1f, want 0", s.DBSizeMB)
	}
	if s.RedisHitRate != 0 {
		t.Errorf("html site RedisHitRate = %.2f, want 0", s.RedisHitRate)
	}
}

func TestCollect_OLSMissing_GracefulDegradation(t *testing.T) {
	rr := &outputRecordingRunner{
		fileContents: map[string]string{},
	}
	c := NewCollector(rr, "/var/www")
	sm, _, err := c.Collect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Collect() should succeed even without OLS data: %v", err)
	}
	if sm.TotalReqs != 0 {
		t.Errorf("TotalReqs = %d, want 0 when OLS missing", sm.TotalReqs)
	}
}

func findSiteMetrics(t *testing.T, metrics []SiteMetrics, name string) SiteMetrics {
	t.Helper()
	for _, m := range metrics {
		if m.Site == name {
			return m
		}
	}
	t.Fatalf("site %q not found in metrics", name)
	return SiteMetrics{}
}

// outputRecordingRunner implements stack.Runner and records calls. Returns
// canned outputs for known commands (redis-cli, mariadb, du, cat).
type outputRecordingRunner struct {
	testmock.NoopRunner
	fileContents   map[string]string
	redisInfo      map[string]string
	mariaDBStatus  string
	mariaDBMaxConn string
	dbSizes        map[string]float64
	diskSizes      map[string]int64
}

func (r *outputRecordingRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	joined := name + " " + strings.Join(args, " ")
	switch {
	case name == "cat":
		if len(args) > 0 {
			if content, ok := r.fileContents[args[0]]; ok {
				return content, nil
			}
		}
		return "", nil
	case strings.Contains(joined, "redis-cli") && strings.Contains(joined, "info"):
		for key, val := range r.redisInfo {
			if strings.Contains(joined, key) {
				return val, nil
			}
		}
		return "", nil
	case strings.Contains(joined, "mariadb") && strings.Contains(joined, "Threads_connected"):
		return r.mariaDBStatus, nil
	case strings.Contains(joined, "mariadb") && strings.Contains(joined, "max_connections"):
		return r.mariaDBMaxConn, nil
	case strings.Contains(joined, "mariadb") && strings.Contains(joined, "information_schema"):
		for db, size := range r.dbSizes {
			if strings.Contains(joined, db) {
				return "ROUND(SUM(data_length+index_length)/1024/1024, 1)\n" + formatFloatSimple(size) + "\n", nil
			}
		}
		return "ROUND(SUM(data_length+index_length)/1024/1024, 1)\n0.0\n", nil
	case name == "du":
		if len(args) >= 2 {
			path := args[len(args)-1]
			if size, ok := r.diskSizes[path]; ok {
				return formatInt64(size) + "\t" + path, nil
			}
		}
		return "0\t?", nil
	case name == "wc" && len(args) >= 2:
		path := args[len(args)-1]
		if content, ok := r.fileContents[path]; ok {
			return formatInt(strings.Count(content, "\n")) + " " + path, nil
		}
		return "0 " + path, nil
	}
	return "", nil
}

func formatFloatSimple(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}

func formatInt(i int) string {
	return strconv.Itoa(i)
}

func formatInt64(i int64) string {
	return strconv.FormatInt(i, 10)
}
