package ols

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baseHttpdConf returns a minimal httpd_config.conf with an Example virtualHost
// and a Default listener, mimicking a real OLS installation.
func baseHttpdConf() string {
	return `serverName localhost
user nobody
group nogroup

virtualHost Example {
    vhRoot                   Example/
    allowSymbolLink          1
    enableScript             1
    restrained               1
    setUIDMode               0
    configFile               conf/vhosts/Example/vhconf.conf
}

listener Default {
    address                  *:8088
    secure                   0
    map                      Example *
}
`
}

// writeHttpdConf writes the base config to a temp file and returns its path.
func writeHttpdConf(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "httpd_config.conf")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil { //nolint:gosec // test config file
		t.Fatalf("write httpd conf: %v", err)
	}
	return p
}

func TestRegisterVHost_AddsVirtualHostAndMap(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	err := RegisterVHost(p, "blog.test", "/var/www/blog.test", "conf/vhosts/blog.test/vhconf.conf")
	if err != nil {
		t.Fatalf("RegisterVHost() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test reads from temp dir
	content := string(got)

	// Verify virtualHost block added.
	if !strings.Contains(content, "virtualHost blog.test {") {
		t.Error("missing virtualHost block for blog.test")
	}
	if !strings.Contains(content, "configFile               conf/vhosts/blog.test/vhconf.conf") {
		t.Error("missing configFile in virtualHost block")
	}

	// Verify listener map entry added.
	if !strings.Contains(content, "map                      blog.test blog.test") {
		t.Error("missing listener map entry for blog.test")
	}

	if !strings.Contains(content, "restrained               1") {
		t.Error("virtualHost should have restrained 1")
	}
}

func TestRegisterVHost_Idempotent(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	if err := RegisterVHost(p, "blog.test", "/var/www/blog.test", "conf/vhosts/blog.test/vhconf.conf"); err != nil {
		t.Fatalf("first RegisterVHost() = %v", err)
	}
	if err := RegisterVHost(p, "blog.test", "/var/www/blog.test", "conf/vhosts/blog.test/vhconf.conf"); err != nil {
		t.Fatalf("second RegisterVHost() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test reads from temp dir
	content := string(got)

	count := strings.Count(content, "virtualHost blog.test {")
	if count != 1 {
		t.Errorf("virtualHost block appears %d times, want 1", count)
	}
	count = strings.Count(content, "map                      blog.test blog.test")
	if count != 1 {
		t.Errorf("map entry appears %d times, want 1", count)
	}
}

func TestUnregisterVHost_RemovesVirtualHostAndMap(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	// Register first.
	if err := RegisterVHost(p, "blog.test", "/var/www/blog.test", "conf/vhosts/blog.test/vhconf.conf"); err != nil {
		t.Fatalf("RegisterVHost() = %v", err)
	}

	// Now unregister.
	if err := UnregisterVHost(p, "blog.test"); err != nil {
		t.Fatalf("UnregisterVHost() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test reads from temp dir
	content := string(got)

	if strings.Contains(content, "virtualHost blog.test") {
		t.Error("virtualHost block should be removed")
	}
	if strings.Contains(content, "blog.test blog.test") {
		t.Error("listener map entry should be removed")
	}
	// Example should still be intact.
	if !strings.Contains(content, "virtualHost Example {") {
		t.Error("Example virtualHost should still be present")
	}
	if !strings.Contains(content, "map                      Example *") {
		t.Error("Example map entry should still be present")
	}
}

func TestUnregisterVHost_Idempotent(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	// Unregister site that was never registered — should not error.
	if err := UnregisterVHost(p, "nope.test"); err != nil {
		t.Fatalf("UnregisterVHost() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test reads from temp dir
	if !strings.Contains(string(got), "virtualHost Example {") {
		t.Error("existing virtualHosts should be untouched")
	}
}

func TestUpdateVHostRestrained(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	// Register a site.
	if err := RegisterVHost(p, "blog.test", "/var/www/blog.test", "conf/vhosts/blog.test/vhconf.conf"); err != nil {
		t.Fatalf("RegisterVHost() = %v", err)
	}

	// Manually set restrained to 0 to simulate an old site.
	data, _ := os.ReadFile(p) //nolint:gosec // test
	content := strings.ReplaceAll(string(data),
		"restrained               1\n"+
			"    setUIDMode               2",
		"restrained               0\n"+
			"    setUIDMode               2",
	)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil { //nolint:gosec // test
		t.Fatalf("write httpd_config: %v", err)
	}

	// Update it to 1.
	if err := UpdateVHostRestrained(p, "blog.test", 1); err != nil {
		t.Fatalf("UpdateVHostRestrained() = %v", err)
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test
	s := string(got)
	if !strings.Contains(s, "restrained               1") {
		t.Error("expected restrained 1 after update")
	}
	if strings.Contains(s, "restrained               0") {
		t.Error("should not contain restrained 0 after update")
	}
}

func TestRegisterVHost_MultipleSites(t *testing.T) {
	p := writeHttpdConf(t, baseHttpdConf())

	for _, site := range []string{"blog.test", "shop.test", "api.test"} {
		confPath := "conf/vhosts/" + site + "/vhconf.conf"
		if err := RegisterVHost(p, site, "/var/www/"+site, confPath); err != nil {
			t.Fatalf("RegisterVHost(%s) = %v", site, err)
		}
	}

	got, _ := os.ReadFile(p) //nolint:gosec // test reads from temp dir
	content := string(got)

	for _, site := range []string{"blog.test", "shop.test", "api.test"} {
		if !strings.Contains(content, "virtualHost "+site+" {") {
			t.Errorf("missing virtualHost block for %s", site)
		}
		if !strings.Contains(content, "map                      "+site+" "+site) {
			t.Errorf("missing listener map for %s", site)
		}
	}
}
