package allocator

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Policy captures the server-level reservation rules the allocator applies
// before splitting the remaining RAM across sites. Per-site memory assumptions
// live in Preset — Policy only covers what is subtracted from total RAM for
// OS, MariaDB, Redis, and OLS itself, plus global constraints on worker counts.
type Policy struct {
	OSReservePct      float64 `yaml:"os_reserve_pct"`
	OSReserveMinMB    uint64  `yaml:"os_reserve_min_mb"`
	MariaDBPct        float64 `yaml:"mariadb_pct"`
	RedisPct          float64 `yaml:"redis_pct"`
	RedisMinMB        uint64  `yaml:"redis_min_mb"`
	OLSCoreMB         uint64  `yaml:"ols_core_mb"`
	MinChildren       int     `yaml:"min_children"`
	MaxChildrenPerCPU int     `yaml:"max_children_per_cpu"`
	SafetyFactor      float64 `yaml:"safety_factor"`
	DefaultPreset     string  `yaml:"default_preset"`
}

// DefaultPolicy returns the built-in defaults tuned for a co-located OLS +
// MariaDB + Redis stack. Overrides live in /etc/gow/policy.yaml.
func DefaultPolicy() Policy {
	return Policy{
		OSReservePct:      0.10,
		OSReserveMinMB:    512,
		MariaDBPct:        0.25,
		RedisPct:          0.05,
		RedisMinMB:        128,
		OLSCoreMB:         256,
		MinChildren:       2,
		MaxChildrenPerCPU: 4,
		SafetyFactor:      0.85,
		DefaultPreset:     "standard",
	}
}

// Validate checks that the policy values are internally consistent. It does
// not touch Presets other than confirming DefaultPreset resolves.
func (p Policy) Validate() error {
	if p.OSReservePct < 0 || p.OSReservePct > 1 {
		return fmt.Errorf("allocator: OSReservePct=%v out of [0,1]", p.OSReservePct)
	}
	if p.MariaDBPct < 0 || p.MariaDBPct > 1 {
		return fmt.Errorf("allocator: MariaDBPct=%v out of [0,1]", p.MariaDBPct)
	}
	if p.RedisPct < 0 || p.RedisPct > 1 {
		return fmt.Errorf("allocator: RedisPct=%v out of [0,1]", p.RedisPct)
	}
	if p.OSReservePct+p.MariaDBPct+p.RedisPct >= 1 {
		return fmt.Errorf(
			"allocator: reservations sum to %.2f, leaves nothing for PHP",
			p.OSReservePct+p.MariaDBPct+p.RedisPct,
		)
	}
	if p.SafetyFactor <= 0 || p.SafetyFactor > 1 {
		return fmt.Errorf("allocator: SafetyFactor=%v out of (0,1]", p.SafetyFactor)
	}
	if p.MinChildren < 1 {
		return fmt.Errorf("allocator: MinChildren=%d must be >= 1", p.MinChildren)
	}
	if p.MaxChildrenPerCPU < 1 {
		return fmt.Errorf("allocator: MaxChildrenPerCPU=%d must be >= 1", p.MaxChildrenPerCPU)
	}
	if _, err := LookupPreset(p.DefaultPreset); err != nil {
		return fmt.Errorf("allocator: DefaultPreset invalid: %w", err)
	}
	return nil
}

// LoadPolicyFromFile reads a YAML override from path and merges it over
// DefaultPolicy. A missing file is not an error — the defaults are returned as
// the operator has simply chosen not to override. The loaded policy is
// validated before it is returned.
func LoadPolicyFromFile(path string) (Policy, error) {
	p := DefaultPolicy()

	data, err := os.ReadFile(path) //nolint:gosec // operator-controlled config path
	switch {
	case errors.Is(err, os.ErrNotExist):
		return p, nil
	case err != nil:
		return Policy{}, fmt.Errorf("allocator: read policy file: %w", err)
	}

	if err := yaml.Unmarshal(data, &p); err != nil {
		return Policy{}, fmt.Errorf("allocator: parse policy file: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}
