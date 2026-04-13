package template

import (
	"strings"
	"testing"
)

// extAppGolden is the expected output for the canonical fixture used in both
// the field-presence test and the golden-file test.
const extAppGolden = `extprocessor lsphp-blog.test {
  type                    lsapi
  address                 uds://tmp/lshttpd/blog.test.sock
  maxConns                10
  env                     PHP_LSAPI_CHILDREN=10
  env                     PHP_LSAPI_MAX_REQUESTS=1000
  env                     PHP_MEMORY_LIMIT=256M
  env                     LSAPI_AVOID_FORK=200M
  initTimeout             60
  retryTimeout            0
  persistConn             1
  pcKeepAliveTimeout      30
  respBuffer              0
  autoStart               2
  path                    /usr/local/lsws/lsphp83/bin/lsphp
  backlog                 100
  instances               1
  priority                0
  memSoftLimit            2048M
  memHardLimit            2560M
  procSoftLimit           10
  procHardLimit           20
  runOnStartUp            1
}`

func fixtureExtApp() ExtAppData {
	return ExtAppData{
		Site:             "blog.test",
		PHPVer:           "83",
		Children:         10,
		PHPMemoryLimitMB: 256,
		MemSoftMB:        2048,
		MemHardMB:        2560,
	}
}

func TestRenderExtAppMatchesGolden(t *testing.T) {
	got, err := RenderExtApp(fixtureExtApp())
	if err != nil {
		t.Fatalf("RenderExtApp() error = %v", err)
	}
	if got != extAppGolden {
		t.Errorf("RenderExtApp() mismatch.\ngot:\n%s\n\nwant:\n%s", got, extAppGolden)
	}
}

func TestRenderExtAppContainsKeyFields(t *testing.T) {
	got, err := RenderExtApp(fixtureExtApp())
	if err != nil {
		t.Fatalf("RenderExtApp() error = %v", err)
	}
	checks := []struct{ label, substr string }{
		{"extprocessor name", "extprocessor lsphp-blog.test"},
		{"socket", "uds://tmp/lshttpd/blog.test.sock"},
		{"maxConns", "maxConns                10"},
		{"PHP_LSAPI_CHILDREN env", "PHP_LSAPI_CHILDREN=10"},
		{"PHP_MEMORY_LIMIT env", "PHP_MEMORY_LIMIT=256M"},
		{"memSoftLimit", "memSoftLimit            2048M"},
		{"memHardLimit", "memHardLimit            2560M"},
		{"procHardLimit", "procHardLimit           20"},
		{"php path", "/usr/local/lsws/lsphp83/bin/lsphp"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("output missing %s: %q", c.label, c.substr)
		}
	}
}

func TestRenderExtAppDifferentPHPVersion(t *testing.T) {
	data := fixtureExtApp()
	data.PHPVer = "82"
	got, err := RenderExtApp(data)
	if err != nil {
		t.Fatalf("RenderExtApp() error = %v", err)
	}
	if !strings.Contains(got, "lsphp82/bin/lsphp") {
		t.Error("output should reference lsphp82 for PHP version 82")
	}
}

func TestRenderExtAppSingleChild(t *testing.T) {
	data := fixtureExtApp()
	data.Children = 1
	data.MemSoftMB = 128
	data.MemHardMB = 256
	got, err := RenderExtApp(data)
	if err != nil {
		t.Fatalf("RenderExtApp() error = %v", err)
	}
	if !strings.Contains(got, "maxConns                1") {
		t.Error("maxConns should be 1")
	}
	if !strings.Contains(got, "procHardLimit           2") {
		t.Error("procHardLimit should be 2 (Children*2)")
	}
}

// --- VHost tests ---

func fixtureVHost() VHostData {
	return VHostData{
		Site:    "example.com",
		Domain:  "example.com",
		Aliases: "www.example.com",
		WebRoot: "/var/www/example.com",
		LogDir:  "/var/log/lsws",
		PHPVer:  "83",
	}
}

func TestRenderVHostContainsDocRoot(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "docRoot                   /var/www/example.com/htdocs") {
		t.Error("output should contain docRoot with WebRoot/htdocs")
	}
}

func TestRenderVHostContainsDomainAndAliases(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "vhDomain                  example.com") {
		t.Error("output should contain vhDomain")
	}
	if !strings.Contains(got, "vhAliases                 www.example.com") {
		t.Error("output should contain vhAliases")
	}
}

func TestRenderVHostContainsScriptHandler(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "lsapi:lsphp-example.com php") {
		t.Error("output should map php to lsapi:lsphp-{site}")
	}
}

func TestRenderVHostContainsExtProcessor(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "extprocessor lsphp-example.com") {
		t.Error("output should define extprocessor lsphp-{site}")
	}
	if !strings.Contains(got, "uds://tmp/lshttpd/example.com.sock") {
		t.Error("output should contain UDS socket path")
	}
}

func TestRenderVHostContainsSecurityContexts(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "context /wp-config.php") {
		t.Error("output should block wp-config.php")
	}
	if !strings.Contains(got, "context /xmlrpc.php") {
		t.Error("output should restrict xmlrpc.php")
	}
}

func TestRenderVHostContainsCacheRoot(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "cacheroot                 /var/cache/lshttpd/example.com") {
		t.Error("output should contain cacheroot for the site")
	}
}

func TestRenderVHostContainsLogs(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "/var/log/lsws/example.com.error.log") {
		t.Error("output should contain error log path")
	}
	if !strings.Contains(got, "/var/log/lsws/example.com.access.log") {
		t.Error("output should contain access log path")
	}
}

func TestRenderVHostContainsRewriteRules(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "RewriteRule . /index.php [L]") {
		t.Error("output should contain WordPress pretty-permalink rewrite")
	}
}

func TestRenderVHostContainsPHPPath(t *testing.T) {
	got, err := RenderVHost(fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "/usr/local/lsws/lsphp83/bin/lsphp") {
		t.Error("output should contain lsphp binary path")
	}
}

func TestRenderVHostUsesAllocatorValues(t *testing.T) {
	data := VHostData{
		Site:             "blog.test",
		Domain:           "blog.test",
		Aliases:          "www.blog.test",
		WebRoot:          "/var/www/blog.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         16,
		PHPMemoryLimitMB: 256,
		MemSoftMB:        3276,
		MemHardMB:        4096,
	}
	got, err := RenderVHost(data)
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	checks := []struct{ label, substr string }{
		{"maxConns from allocator", "maxConns                16"},
		{"PHP_LSAPI_CHILDREN from allocator", "PHP_LSAPI_CHILDREN=16"},
		{"PHP_MEMORY_LIMIT from allocator", "PHP_MEMORY_LIMIT=256M"},
		{"memSoftLimit from allocator", "memSoftLimit            3276M"},
		{"memHardLimit from allocator", "memHardLimit            4096M"},
		{"procSoftLimit from allocator", "procSoftLimit           16"},
		{"procHardLimit = Children*2", "procHardLimit           32"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("output missing %s: %q\ngot:\n%s", c.label, c.substr, got)
		}
	}
}
