package allocator

import (
	"errors"
	"fmt"
)

// ErrInsufficientRAM is returned when the allocator cannot satisfy MinChildren
// for every site even after downgrading every site to the lightest preset. It
// signals that the server is saturated — the caller should refuse to provision
// the new site rather than yank RAM from running sites.
var ErrInsufficientRAM = errors.New("allocator: insufficient RAM to satisfy MinChildren across all sites")

// SiteInput is the per-site request handed to Compute. A blank Preset is
// resolved to Policy.DefaultPreset before any math runs.
type SiteInput struct {
	Name   string
	Preset string
}

// Allocation is the per-site result of Compute. PresetUsed may differ from
// the caller's requested preset if the allocator had to downgrade under
// memory pressure — Downgraded is then set so the caller can surface a
// warning to the operator.
type Allocation struct {
	Site             string
	PresetUsed       string
	Downgraded       bool
	Children         int
	PHPMemoryLimitMB uint64
	WorkerBudgetMB   uint64
	MemSoftMB        uint64
	MemHardMB        uint64
}

// Compute sizes every site's LSPHP cluster against the given hardware and
// policy. The function is pure — it reads no globals beyond the Presets
// catalog and performs no I/O.
//
// Algorithm:
//  1. Validate policy and resolve each site's preset (default if blank).
//  2. Compute PHPBudget by subtracting OS / MariaDB / Redis / OLS reservations
//     from total RAM and applying the safety factor.
//  3. Distribute PHPBudget proportionally to each site's WorkerBudgetMB
//     (weighted shares — heavier presets claim more of the budget).
//  4. Divide each site's share by its WorkerBudgetMB to get Children, clamped
//     by the CPU ceiling.
//  5. If any site lands below MinChildren, downgrade every failing site one
//     tier and recompute. Repeat until all sites fit or we hit the lite floor.
func Compute(totalRAMMB uint64, cpuCores int, sites []SiteInput, p Policy) ([]Allocation, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return []Allocation{}, nil
	}

	presets, err := resolvePresets(sites, p)
	if err != nil {
		return nil, err
	}
	downgraded := make([]bool, len(sites))

	budget := phpBudget(totalRAMMB, p)
	if budget == 0 {
		return nil, fmt.Errorf("%w: PHP budget is zero (reservations >= total RAM)", ErrInsufficientRAM)
	}
	cpuCeiling := cpuCores * p.MaxChildrenPerCPU

	// Iterate until stable. The chain length is the absolute upper bound on
	// downgrades per site, and we add slack for the intermediate recomputes.
	maxIterations := len(downgradeChain) * (len(sites) + 1)
	for range maxIterations {
		allocs := distribute(sites, presets, budget, cpuCeiling)

		failing := failingSites(allocs, p.MinChildren)
		if len(failing) == 0 {
			for i := range allocs {
				if downgraded[i] {
					allocs[i].Downgraded = true
				}
			}
			return allocs, nil
		}

		progressed := false
		for _, i := range failing {
			next, ok := DowngradePreset(presets[i].Name)
			if !ok {
				continue
			}
			nextPreset, err := LookupPreset(next)
			if err != nil {
				return nil, err
			}
			presets[i] = nextPreset
			downgraded[i] = true
			progressed = true
		}
		if !progressed {
			return nil, ErrInsufficientRAM
		}
	}
	return nil, ErrInsufficientRAM
}

// resolvePresets maps each SiteInput to its concrete Preset, substituting the
// policy default when the caller left Preset blank.
func resolvePresets(sites []SiteInput, p Policy) ([]Preset, error) {
	out := make([]Preset, len(sites))
	for i, s := range sites {
		name := s.Preset
		if name == "" {
			name = p.DefaultPreset
		}
		preset, err := LookupPreset(name)
		if err != nil {
			return nil, fmt.Errorf("site %q: %w", s.Name, err)
		}
		out[i] = preset
	}
	return out, nil
}

// phpBudget calculates the RAM available for PHP workers after server-level
// reservations and the safety factor. Returns zero when reservations alone
// meet or exceed total RAM.
func phpBudget(totalRAMMB uint64, p Policy) uint64 {
	osReserve := max(uint64(float64(totalRAMMB)*p.OSReservePct), p.OSReserveMinMB)
	maria := uint64(float64(totalRAMMB) * p.MariaDBPct)
	redis := max(uint64(float64(totalRAMMB)*p.RedisPct), p.RedisMinMB)
	reserved := osReserve + maria + redis + p.OLSCoreMB
	if reserved >= totalRAMMB {
		return 0
	}
	return uint64(float64(totalRAMMB-reserved) * p.SafetyFactor)
}

// distribute splits budget across sites proportional to WorkerBudgetMB, then
// derives per-site child counts subject to the CPU ceiling. Children may fall
// below Policy.MinChildren — the caller handles that via downgrading.
func distribute(sites []SiteInput, presets []Preset, budget uint64, cpuCeiling int) []Allocation {
	var totalWeight uint64
	for _, pr := range presets {
		totalWeight += pr.WorkerBudgetMB
	}

	// cpuCores is positive and MaxChildrenPerCPU is validated >=1, so cpuCeiling
	// is a small positive int that always fits in uint64.
	cpuCeilingU := uint64(cpuCeiling) //nolint:gosec // positive by construction
	out := make([]Allocation, len(sites))
	for i, s := range sites {
		pr := presets[i]
		share := budget * pr.WorkerBudgetMB / totalWeight

		raw := min(share/pr.WorkerBudgetMB, cpuCeilingU)
		// raw is now bounded by cpuCeiling (which fits in int), so the
		// narrowing conversion cannot overflow.
		children := int(raw) //nolint:gosec // bounded by cpuCeiling above

		memHard := raw * pr.PHPMemoryLimitMB
		memSoft := memHard * 80 / 100

		out[i] = Allocation{
			Site:             s.Name,
			PresetUsed:       pr.Name,
			Children:         children,
			PHPMemoryLimitMB: pr.PHPMemoryLimitMB,
			WorkerBudgetMB:   pr.WorkerBudgetMB,
			MemSoftMB:        memSoft,
			MemHardMB:        memHard,
		}
	}
	return out
}

// failingSites returns indices of sites whose Children count is below the
// MinChildren floor — the candidates for downgrade.
func failingSites(allocs []Allocation, minChildren int) []int {
	var out []int
	for i, a := range allocs {
		if a.Children < minChildren {
			out = append(out, i)
		}
	}
	return out
}
