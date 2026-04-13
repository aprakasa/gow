package allocator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicyValues(t *testing.T) {
	p := DefaultPolicy()

	if p.OSReservePct != 0.10 {
		t.Errorf("OSReservePct = %v, want 0.10", p.OSReservePct)
	}
	if p.OSReserveMinMB != 512 {
		t.Errorf("OSReserveMinMB = %d, want 512", p.OSReserveMinMB)
	}
	if p.MariaDBPct != 0.25 {
		t.Errorf("MariaDBPct = %v, want 0.25", p.MariaDBPct)
	}
	if p.RedisPct != 0.05 {
		t.Errorf("RedisPct = %v, want 0.05", p.RedisPct)
	}
	if p.RedisMinMB != 128 {
		t.Errorf("RedisMinMB = %d, want 128", p.RedisMinMB)
	}
	if p.OLSCoreMB != 256 {
		t.Errorf("OLSCoreMB = %d, want 256", p.OLSCoreMB)
	}
	if p.MinChildren != 2 {
		t.Errorf("MinChildren = %d, want 2", p.MinChildren)
	}
	if p.MaxChildrenPerCPU != 4 {
		t.Errorf("MaxChildrenPerCPU = %d, want 4", p.MaxChildrenPerCPU)
	}
	if p.SafetyFactor != 0.85 {
		t.Errorf("SafetyFactor = %v, want 0.85", p.SafetyFactor)
	}
	if p.DefaultPreset != "standard" {
		t.Errorf("DefaultPreset = %q, want standard", p.DefaultPreset)
	}
}

func TestDefaultPolicyValidates(t *testing.T) {
	if err := DefaultPolicy().Validate(); err != nil {
		t.Fatalf("DefaultPolicy().Validate() = %v, want nil", err)
	}
}

func TestPolicyValidateRejectsBadValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Policy)
	}{
		{"OSReservePct > 1", func(p *Policy) { p.OSReservePct = 1.5 }},
		{"MariaDBPct negative", func(p *Policy) { p.MariaDBPct = -0.1 }},
		{"reservations sum > 1", func(p *Policy) {
			p.OSReservePct = 0.4
			p.MariaDBPct = 0.5
			p.RedisPct = 0.3
		}},
		{"SafetyFactor zero", func(p *Policy) { p.SafetyFactor = 0 }},
		{"SafetyFactor > 1", func(p *Policy) { p.SafetyFactor = 1.2 }},
		{"MinChildren zero", func(p *Policy) { p.MinChildren = 0 }},
		{"MaxChildrenPerCPU zero", func(p *Policy) { p.MaxChildrenPerCPU = 0 }},
		{"DefaultPreset unknown", func(p *Policy) { p.DefaultPreset = "nonesuch" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := DefaultPolicy()
			c.mutate(&p)
			if err := p.Validate(); err == nil {
				t.Errorf("Validate() = nil, want error")
			}
		})
	}
}

func TestLoadPolicyFromFileMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	yaml := []byte(`
mariadb_pct: 0.15
default_preset: business
min_children: 3
`)
	if err := os.WriteFile(path, yaml, 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicyFromFile(path)
	if err != nil {
		t.Fatalf("LoadPolicyFromFile error = %v", err)
	}

	if p.MariaDBPct != 0.15 {
		t.Errorf("MariaDBPct = %v, want 0.15 (override)", p.MariaDBPct)
	}
	if p.DefaultPreset != "business" {
		t.Errorf("DefaultPreset = %q, want business (override)", p.DefaultPreset)
	}
	if p.MinChildren != 3 {
		t.Errorf("MinChildren = %d, want 3 (override)", p.MinChildren)
	}
	// Non-overridden values should still equal the defaults.
	if p.OSReservePct != 0.10 {
		t.Errorf("OSReservePct = %v, want 0.10 (default preserved)", p.OSReservePct)
	}
	if p.SafetyFactor != 0.85 {
		t.Errorf("SafetyFactor = %v, want 0.85 (default preserved)", p.SafetyFactor)
	}
}

func TestLoadPolicyFromMissingFileReturnsDefaults(t *testing.T) {
	p, err := LoadPolicyFromFile(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadPolicyFromFile(missing) error = %v, want nil", err)
	}
	if p.DefaultPreset != "standard" {
		t.Errorf("DefaultPreset = %q, want standard", p.DefaultPreset)
	}
}

func TestLoadPolicyRejectsInvalidOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("mariadb_pct: 2.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicyFromFile(path); err == nil {
		t.Error("LoadPolicyFromFile(invalid) = nil error, want validation failure")
	}
}
