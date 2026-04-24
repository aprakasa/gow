// Package allocator computes per-site LSPHP cluster sizing for OpenLiteSpeed.
package allocator

import "fmt"

// Preset captures the memory expectations of a class of WordPress workload.
//
// PHPMemoryLimitMB is what the allocator writes to PHP_MEMORY_LIMIT and to the
// OLS extprocessor memHardLimit: the ceiling a single request is allowed to
// reach before PHP kills it. WorkerBudgetMB is the realistic resident-set
// estimate used for budgeting against available RAM — workers rarely settle
// at the hard ceiling, so budgeting against it would waste capacity.
type Preset struct {
	Name             string
	PHPMemoryLimitMB uint64
	WorkerBudgetMB   uint64
	Description      string
}

// presets is the catalog of named workload tiers, ordered from lightest to
// heaviest. The ordering is load-bearing: DowngradePreset walks it in reverse
// when the allocator needs to shed memory pressure.
var presets = map[string]Preset{
	"lite": {
		Name:             "lite",
		PHPMemoryLimitMB: 128,
		WorkerBudgetMB:   64,
		Description:      "Static / brochure WP, no heavy plugins",
	},
	"standard": {
		Name:             "standard",
		PHPMemoryLimitMB: 256,
		WorkerBudgetMB:   128,
		Description:      "Typical blog: Yoast, LSCache, a handful of plugins",
	},
	"business": {
		Name:             "business",
		PHPMemoryLimitMB: 384,
		WorkerBudgetMB:   192,
		Description:      "LMS, membership, forum, page builder",
	},
	"woocommerce": {
		Name:             "woocommerce",
		PHPMemoryLimitMB: 512,
		WorkerBudgetMB:   256,
		Description:      "WooCommerce shop with payment and shipping plugins",
	},
	"heavy": {
		Name:             "heavy",
		PHPMemoryLimitMB: 768,
		WorkerBudgetMB:   384,
		Description:      "WooCommerce + Elementor + Jetpack + analytics bloat",
	},
}

// downgradeChain orders presets from lightest to heaviest. DowngradePreset
// uses it to find the next-lighter neighbour when shedding memory pressure.
var downgradeChain = []string{"lite", "standard", "business", "woocommerce", "heavy"}

// LookupPreset returns the named preset or an error if it does not exist.
func LookupPreset(name string) (Preset, error) {
	p, ok := presets[name]
	if !ok {
		return Preset{}, fmt.Errorf("allocator: unknown preset %q", name)
	}
	return p, nil
}

// DowngradePreset returns the next-lighter preset in the canonical chain.
// It returns ("", false) when called on the lightest preset.
func DowngradePreset(name string) (string, bool) {
	for i := len(downgradeChain) - 1; i > 0; i-- {
		if downgradeChain[i] == name {
			return downgradeChain[i-1], true
		}
	}
	return "", false
}
