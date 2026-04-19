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
		WebRoot: "/var/www/example.com",
		LogDir:  "/var/log/lsws",
		PHPVer:  "83",
	}
}

func TestRenderVHostContainsDocRoot(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "docRoot                   /var/www/example.com/htdocs") {
		t.Error("output should contain docRoot with WebRoot/htdocs")
	}
}

func TestRenderVHostContainsDomain(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "vhDomain                  example.com") {
		t.Error("output should contain vhDomain")
	}
}

func TestRenderVHostContainsScriptHandler(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "lsapi:lsphp-example.com php") {
		t.Error("output should map php to lsapi:lsphp-{site}")
	}
}

func TestRenderVHostContainsExtProcessor(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
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
	got, err := RenderVHost("wp", fixtureVHost())
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
	got, err := RenderVHost("wp", fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "cacheroot                 /var/cache/lshttpd/example.com") {
		t.Error("output should contain cacheroot for the site")
	}
}

func TestRenderVHostContainsLogs(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
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
	got, err := RenderVHost("wp", fixtureVHost())
	if err != nil {
		t.Fatalf("RenderVHost() error = %v", err)
	}
	if !strings.Contains(got, "RewriteRule . /index.php [L]") {
		t.Error("output should contain WordPress pretty-permalink rewrite")
	}
}

func TestRenderVHostContainsPHPPath(t *testing.T) {
	got, err := RenderVHost("wp", fixtureVHost())
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
		WebRoot:          "/var/www/blog.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         16,
		PHPMemoryLimitMB: 256,
		MemSoftMB:        3276,
		MemHardMB:        4096,
	}
	got, err := RenderVHost("wp", data)
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

func TestRendererCachesTemplate(t *testing.T) {
	var r Renderer
	data := fixtureExtApp()
	got1, err := r.render("lsphp_extapp.conf.tmpl", data)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	if r.textTmpl == nil {
		t.Fatal("textTmpl should be set after first render")
	}
	cached := r.textTmpl
	got2, err := r.render("lsphp_extapp.conf.tmpl", data)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if r.textTmpl != cached {
		t.Error("textTmpl should not change on subsequent calls")
	}
	if got1 != got2 {
		t.Error("output should be identical across calls")
	}
}

func BenchmarkRenderExtApp(b *testing.B) {
	data := fixtureExtApp()
	for i := 0; i < b.N; i++ {
		_, err := RenderExtApp(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- VHost variant tests ---

func TestRenderVHostHTML(t *testing.T) {
	data := VHostData{
		Site:    "static.test",
		Domain:  "static.test",
		WebRoot: "/var/www/static.test",
		LogDir:  "/var/log/lsws",
	}
	got, err := RenderVHost("html", data)
	if err != nil {
		t.Fatalf("RenderVHost(html) error = %v", err)
	}
	if strings.Contains(got, "scripthandler") {
		t.Error("html template should not contain scripthandler")
	}
	if strings.Contains(got, "extprocessor") {
		t.Error("html template should not contain extprocessor")
	}
	if !strings.Contains(got, "docRoot                   /var/www/static.test/htdocs") {
		t.Error("html template should contain docRoot")
	}
}

func TestRenderVHostPHP(t *testing.T) {
	data := VHostData{
		Site:             "app.test",
		Domain:           "app.test",
		WebRoot:          "/var/www/app.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         4,
		PHPMemoryLimitMB: 256,
		MemSoftMB:        512,
		MemHardMB:        640,
	}
	got, err := RenderVHost("php", data)
	if err != nil {
		t.Fatalf("RenderVHost(php) error = %v", err)
	}
	if !strings.Contains(got, "scripthandler") {
		t.Error("php template should contain scripthandler")
	}
	if strings.Contains(got, "RewriteRule") {
		t.Error("php template should not contain WordPress rewrite rules")
	}
}

func TestRenderMaintenance(t *testing.T) {
	got, err := RenderMaintenance("example.com")
	if err != nil {
		t.Fatalf("RenderMaintenance() error = %v", err)
	}
	if !strings.Contains(got, "Under Maintenance") {
		t.Error("maintenance page should contain heading")
	}
	if !strings.Contains(got, "example.com") {
		t.Error("maintenance page should contain domain")
	}
	if !strings.Contains(got, "<!DOCTYPE html>") {
		t.Error("maintenance page should be valid HTML")
	}
}

const htmlVHostGolden = `docRoot                   /var/www/static.test/htdocs
vhDomain                  static.test
enableGzip                1

index  {
  useServer               0
  indexFiles              index.html, index.htm
}

errorlog /var/log/lsws/static.test.error.log {
  useServer               0
  logLevel                WARN
}

accesslog /var/log/lsws/static.test.access.log {
  useServer               0
  logFormat               "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\""
}
`

func TestRenderVHostHTMLMatchesGolden(t *testing.T) {
	data := VHostData{
		Site:    "static.test",
		Domain:  "static.test",
		WebRoot: "/var/www/static.test",
		LogDir:  "/var/log/lsws",
	}
	got, err := RenderVHost("html", data)
	if err != nil {
		t.Fatalf("RenderVHost(html) error = %v", err)
	}
	if got != htmlVHostGolden {
		t.Errorf("mismatch.\ngot:\n%s\nwant:\n%s", got, htmlVHostGolden)
	}
}

func TestRenderVHostWP(t *testing.T) {
	data := fixtureVHost()
	got, err := RenderVHost("wp", data)
	if err != nil {
		t.Fatalf("RenderVHost(wp) error = %v", err)
	}
	if !strings.Contains(got, "RewriteRule . /index.php [L]") {
		t.Error("wp template should contain WordPress rewrite rules")
	}
	if !strings.Contains(got, "context /wp-config.php") {
		t.Error("wp template should block wp-config.php")
	}
}

func TestRenderVHost_WPWithSSL(t *testing.T) {
	data := VHostData{
		Site:             "blog.test",
		Domain:           "blog.test",
		WebRoot:          "/var/www/blog.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         4,
		PHPMemoryLimitMB: 128,
		MemSoftMB:        256,
		MemHardMB:        512,
		SSLEnabled:       true,
		CertPath:         "/etc/letsencrypt/live/blog.test/fullchain.pem",
		KeyPath:          "/etc/letsencrypt/live/blog.test/privkey.pem",
	}
	got, err := RenderVHost("wp", data)
	if err != nil {
		t.Fatalf("RenderVHost() = %v", err)
	}
	if !strings.Contains(got, "ssl {") {
		t.Error("missing ssl block")
	}
	if !strings.Contains(got, data.CertPath) {
		t.Error("missing certFile path")
	}
	if !strings.Contains(got, data.KeyPath) {
		t.Error("missing keyFile path")
	}
	if !strings.Contains(got, "certChain                1") {
		t.Error("missing certChain 1")
	}
	if !strings.Contains(got, "SERVER_PORT") {
		t.Error("missing HTTPS redirect rule")
	}
	if !strings.Contains(got, "acme-challenge") {
		t.Error("missing ACME challenge exclusion in redirect")
	}
}

func TestRenderVHost_WPNoSSL(t *testing.T) {
	data := VHostData{
		Site:             "blog.test",
		Domain:           "blog.test",
		WebRoot:          "/var/www/blog.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         4,
		PHPMemoryLimitMB: 128,
		MemSoftMB:        256,
		MemHardMB:        512,
	}
	got, err := RenderVHost("wp", data)
	if err != nil {
		t.Fatalf("RenderVHost() = %v", err)
	}
	if strings.Contains(got, "ssl {") {
		t.Error("should not contain ssl block when SSLEnabled is false")
	}
	if strings.Contains(got, "SERVER_PORT") {
		t.Error("should not contain redirect when SSLEnabled is false")
	}
}

func TestRenderVHost_HTMLWithSSL(t *testing.T) {
	data := VHostData{
		Site:       "static.test",
		Domain:     "static.test",
		WebRoot:    "/var/www/static.test",
		LogDir:     "/var/log/lsws",
		SSLEnabled: true,
		CertPath:   "/etc/letsencrypt/live/static.test/fullchain.pem",
		KeyPath:    "/etc/letsencrypt/live/static.test/privkey.pem",
	}
	got, err := RenderVHost("html", data)
	if err != nil {
		t.Fatalf("RenderVHost() = %v", err)
	}
	if !strings.Contains(got, "ssl {") {
		t.Error("missing ssl block")
	}
	if !strings.Contains(got, "SERVER_PORT") {
		t.Error("missing HTTPS redirect")
	}
}

func TestRenderVHost_PHPWithSSL(t *testing.T) {
	data := VHostData{
		Site:             "app.test",
		Domain:           "app.test",
		WebRoot:          "/var/www/app.test",
		LogDir:           "/var/log/lsws",
		PHPVer:           "83",
		Children:         4,
		PHPMemoryLimitMB: 128,
		MemSoftMB:        256,
		MemHardMB:        512,
		SSLEnabled:       true,
		CertPath:         "/etc/letsencrypt/live/app.test/fullchain.pem",
		KeyPath:          "/etc/letsencrypt/live/app.test/privkey.pem",
	}
	got, err := RenderVHost("php", data)
	if err != nil {
		t.Fatalf("RenderVHost() = %v", err)
	}
	if !strings.Contains(got, "ssl {") {
		t.Error("missing ssl block")
	}
	if !strings.Contains(got, "SERVER_PORT") {
		t.Error("missing HTTPS redirect")
	}
}
