package allocator

import (
	"errors"
	"strconv"
	"testing"
)

// ramMedium / cpuMedium are the representative server size used for the
// worked examples in the plan document (section 4.7).
const (
	ramMedium = uint64(8192) // MB
	cpuMedium = 4
)

func mustCompute(t *testing.T, sites []SiteInput) []Allocation {
	t.Helper()
	allocs, err := Compute(ramMedium, cpuMedium, sites, DefaultPolicy())
	if err != nil {
		t.Fatalf("Compute(...) error = %v, want nil", err)
	}
	return allocs
}

func findAlloc(t *testing.T, allocs []Allocation, name string) Allocation {
	t.Helper()
	for _, a := range allocs {
		if a.Site == name {
			return a
		}
	}
	t.Fatalf("no allocation for site %q", name)
	return Allocation{}
}

func TestComputeEmptySitesReturnsEmpty(t *testing.T) {
	allocs, err := Compute(ramMedium, cpuMedium, nil, DefaultPolicy())
	if err != nil {
		t.Fatalf("Compute(nil sites) error = %v, want nil", err)
	}
	if len(allocs) != 0 {
		t.Errorf("len(allocs) = %d, want 0", len(allocs))
	}
}

func TestComputeSingleStandardHitsCPUCeiling(t *testing.T) {
	allocs := mustCompute(t,
		[]SiteInput{{Name: "blog.test", Preset: presetStandard}})

	a := findAlloc(t, allocs, "blog.test")
	// 3961/128 = 30 raw; CPU ceiling = 4*4 = 16.
	if a.Children != 16 {
		t.Errorf("Children = %d, want 16 (CPU ceiling)", a.Children)
	}
	if a.PresetUsed != presetStandard {
		t.Errorf("PresetUsed = %q, want %q", a.PresetUsed, presetStandard)
	}
	if a.Downgraded {
		t.Error("Downgraded = true, want false")
	}
	if a.PHPMemoryLimitMB != 256 {
		t.Errorf("PHPMemoryLimitMB = %d, want 256", a.PHPMemoryLimitMB)
	}
	if a.WorkerBudgetMB != 128 {
		t.Errorf("WorkerBudgetMB = %d, want 128", a.WorkerBudgetMB)
	}
	// MemHard = 16 × 256 = 4096
	if a.MemHardMB != 4096 {
		t.Errorf("MemHardMB = %d, want 4096", a.MemHardMB)
	}
	// MemSoft = 4096 × 80 / 100 = 3276
	if a.MemSoftMB != 3276 {
		t.Errorf("MemSoftMB = %d, want 3276", a.MemSoftMB)
	}
}

func TestComputeWeightedSharesStandardPlusWoocommerce(t *testing.T) {
	allocs := mustCompute(t, []SiteInput{
		{Name: "blog.test", Preset: presetStandard},
		{Name: "shop.test", Preset: "woocommerce"},
	})

	std := findAlloc(t, allocs, "blog.test")
	woo := findAlloc(t, allocs, "shop.test")

	// Weighted math (section 4.7 scenario B):
	//   totalWeight = 128 + 256 = 384
	//   stdShare = 3961 * 128 / 384 = 1320 → Children = 1320/128 = 10
	//   wooShare = 3961 * 256 / 384 = 2640 → Children = 2640/256 = 10
	if std.Children != 10 {
		t.Errorf("standard Children = %d, want 10", std.Children)
	}
	if woo.Children != 10 {
		t.Errorf("woocommerce Children = %d, want 10", woo.Children)
	}
}

func TestComputeTwoLitePlusHeavy(t *testing.T) {
	allocs := mustCompute(t, []SiteInput{
		{Name: "a.test", Preset: "lite"},
		{Name: "b.test", Preset: "lite"},
		{Name: "c.test", Preset: "heavy"},
	})

	// Scenario C: totalWeight = 64+64+384 = 512
	//   lite share   = 3961 * 64 / 512 = 495 → 495/64 = 7 (under CPU cap 16)
	//   heavy share  = 3961 * 384 / 512 = 2970 → 2970/384 = 7
	for _, name := range []string{"a.test", "b.test", "c.test"} {
		if c := findAlloc(t, allocs, name).Children; c != 7 {
			t.Errorf("%s Children = %d, want 7", name, c)
		}
	}
}

func TestComputeAutoDowngradesSaturatedServer(t *testing.T) {
	sites := make([]SiteInput, 8)
	for i := range sites {
		sites[i] = SiteInput{Name: siteName(i), Preset: "heavy"}
	}

	allocs := mustCompute(t, sites)

	// Scenario D: 8× heavy on 8GB does not fit (1 child each).
	// Downgrade heavy→woo→business; business at 192 MB yields 2 children per site.
	for i, a := range allocs {
		if a.PresetUsed != "business" {
			t.Errorf("site %d PresetUsed = %q, want business", i, a.PresetUsed)
		}
		if !a.Downgraded {
			t.Errorf("site %d Downgraded = false, want true", i)
		}
		if a.Children != 2 {
			t.Errorf("site %d Children = %d, want 2", i, a.Children)
		}
	}
}

func TestComputeReturnsErrInsufficientRAMWhenLiteCannotFit(t *testing.T) {
	// 2 GB box (PHPBudget ≈ 544 MB) with 10 heavy sites: even after
	// downgrading everyone to lite (64 MB worker budget), each site only gets
	// 544/10 = 54 MB share → 0 children, which is below MinChildren.
	sites := make([]SiteInput, 10)
	for i := range sites {
		sites[i] = SiteInput{Name: siteName(i), Preset: "heavy"}
	}
	_, err := Compute(2048, 2, sites, DefaultPolicy())
	if !errors.Is(err, ErrInsufficientRAM) {
		t.Errorf("err = %v, want ErrInsufficientRAM", err)
	}
}

func TestComputeReturnsErrInsufficientRAMWhenReservationsExceedRAM(t *testing.T) {
	_, err := Compute(1024, 2, []SiteInput{{Name: "a.test"}}, DefaultPolicy())
	if !errors.Is(err, ErrInsufficientRAM) {
		t.Errorf("err = %v, want ErrInsufficientRAM (1 GB is below reservation floor)", err)
	}
}

func TestComputeEmptyPresetUsesPolicyDefault(t *testing.T) {
	allocs := mustCompute(t,
		[]SiteInput{{Name: "blog.test"}})
	if allocs[0].PresetUsed != presetStandard {
		t.Errorf("PresetUsed = %q, want %q (policy default)", allocs[0].PresetUsed, presetStandard)
	}
}

func TestComputeRejectsUnknownPreset(t *testing.T) {
	_, err := Compute(ramMedium, cpuMedium,
		[]SiteInput{{Name: "blog.test", Preset: "nonesuch"}},
		DefaultPolicy())
	if err == nil {
		t.Fatal("Compute(unknown preset) = nil error, want error")
	}
	if errors.Is(err, ErrInsufficientRAM) {
		t.Errorf("err = %v, should not be ErrInsufficientRAM (bad input, not capacity)", err)
	}
}

func TestComputeRejectsInvalidPolicy(t *testing.T) {
	bad := DefaultPolicy()
	bad.SafetyFactor = 2.0
	_, err := Compute(ramMedium, cpuMedium,
		[]SiteInput{{Name: "a.test", Preset: presetStandard}}, bad)
	if err == nil {
		t.Error("Compute(bad policy) = nil error, want validation failure")
	}
}

func TestComputeOOMInvariantHolds(t *testing.T) {
	// Property check: across a sweep of server sizes and site mixes, the sum
	// of Children_i × WorkerBudgetMB_i never exceeds the allocator's PHPBudget.
	// This is the load-bearing safety invariant.
	mixes := [][]string{
		{presetStandard},
		{presetStandard, "woocommerce"},
		{"lite", "lite", "heavy"},
		{"business", "business", "business"},
		{presetStandard, presetStandard, presetStandard, presetStandard},
	}
	for _, ram := range []uint64{4096, 8192, 16384} {
		for _, mix := range mixes {
			sites := make([]SiteInput, len(mix))
			for i, preset := range mix {
				sites[i] = SiteInput{Name: siteName(i), Preset: preset}
			}
			allocs, err := Compute(ram, 4, sites, DefaultPolicy())
			if err != nil {
				continue // capacity exhaustion is expected in some sweeps
			}
			budget := phpBudgetFor(ram, DefaultPolicy())
			var used uint64
			for _, a := range allocs {
				if a.Children < 0 {
					t.Fatalf("negative Children=%d", a.Children)
				}
				used += uint64(a.Children) * a.WorkerBudgetMB //nolint:gosec // guarded above
			}
			if used > budget {
				t.Errorf("ram=%d mix=%v used=%d > budget=%d (OOM invariant violated)",
					ram, mix, used, budget)
			}
		}
	}
}

func TestComputeRespectsMinChildrenOnSuccess(t *testing.T) {
	allocs := mustCompute(t,
		[]SiteInput{{Name: "a.test", Preset: presetStandard}})
	if allocs[0].Children < DefaultPolicy().MinChildren {
		t.Errorf("Children = %d, want >= %d", allocs[0].Children, DefaultPolicy().MinChildren)
	}
}

// siteName is a tiny helper so tests can generate deterministic distinct names.
func siteName(i int) string {
	return "s" + strconv.Itoa(i)
}

// --- Custom preset ---

func TestComputeCustomPresetSingleSite(t *testing.T) {
	allocs := mustCompute(t, []SiteInput{{
		Name:                 "custom.test",
		Preset:               "custom",
		CustomPHPMemoryMB:    320,
		CustomWorkerBudgetMB: 160,
	}})

	a := findAlloc(t, allocs, "custom.test")
	if a.PresetUsed != "custom" {
		t.Errorf("PresetUsed = %q, want %q", a.PresetUsed, "custom")
	}
	if a.PHPMemoryLimitMB != 320 {
		t.Errorf("PHPMemoryLimitMB = %d, want 320", a.PHPMemoryLimitMB)
	}
	if a.WorkerBudgetMB != 160 {
		t.Errorf("WorkerBudgetMB = %d, want 160", a.WorkerBudgetMB)
	}
	if a.Downgraded {
		t.Error("Downgraded = true, want false")
	}
	if a.Children < DefaultPolicy().MinChildren {
		t.Errorf("Children = %d, want >= %d", a.Children, DefaultPolicy().MinChildren)
	}
}

func TestComputeCustomPresetMixedWithNamed(t *testing.T) {
	allocs := mustCompute(t, []SiteInput{
		{Name: "blog.test", Preset: presetStandard},
		{Name: "custom.test", Preset: "custom", CustomPHPMemoryMB: 320, CustomWorkerBudgetMB: 160},
	})

	std := findAlloc(t, allocs, "blog.test")
	cus := findAlloc(t, allocs, "custom.test")

	// Weighted math: totalWeight = 128 + 160 = 288
	// stdShare = 3961 * 128 / 288 ≈ 1760 → Children = 1760/128 = 13
	// cusShare = 3961 * 160 / 288 ≈ 2200 → Children = 2200/160 = 13
	if std.Children <= 0 {
		t.Errorf("standard Children = %d, want > 0", std.Children)
	}
	if cus.Children <= 0 {
		t.Errorf("custom Children = %d, want > 0", cus.Children)
	}
}

func TestComputeCustomPresetNotDowngraded(t *testing.T) {
	// Pack a small server so the custom site can't fit MinChildren.
	_, err := Compute(2048, 2, []SiteInput{
		{Name: "a.test", Preset: "heavy"},
		{Name: "b.test", Preset: "heavy"},
		{Name: "c.test", Preset: "heavy"},
		{Name: "custom.test", Preset: "custom", CustomPHPMemoryMB: 768, CustomWorkerBudgetMB: 384},
	}, DefaultPolicy())

	if !errors.Is(err, ErrInsufficientRAM) {
		t.Errorf("err = %v, want ErrInsufficientRAM (custom should not be downgraded)", err)
	}
}

func TestComputeCustomPresetZeroValuesRejected(t *testing.T) {
	_, err := Compute(ramMedium, cpuMedium, []SiteInput{
		{Name: "bad.test", Preset: "custom"},
	}, DefaultPolicy())
	if err == nil {
		t.Fatal("custom preset with zero values should return error")
	}
	if errors.Is(err, ErrInsufficientRAM) {
		t.Errorf("err = %v, should be validation error not capacity error", err)
	}
}

// phpBudgetFor mirrors the allocator's PHPBudget calculation so the invariant
// test has an independent reference. It intentionally uses the same float
// coercions as the production phpBudget so rounding stays aligned.
func phpBudgetFor(ramMB uint64, p Policy) uint64 {
	osReserve := max(uint64(float64(ramMB)*p.OSReservePct), p.OSReserveMinMB)
	maria := uint64(float64(ramMB) * p.MariaDBPct)
	redis := max(uint64(float64(ramMB)*p.RedisPct), p.RedisMinMB)
	reserved := osReserve + maria + redis + p.OLSCoreMB
	if reserved >= ramMB {
		return 0
	}
	return uint64(float64(ramMB-reserved) * p.SafetyFactor)
}
