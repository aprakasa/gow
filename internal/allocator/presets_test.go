package allocator

import "testing"

const presetStandard = "standard"

func TestPresetsCatalogContainsNamedTiers(t *testing.T) {
	want := []string{"lite", presetStandard, "business", "woocommerce", "heavy"}
	for _, name := range want {
		if _, ok := presets[name]; !ok {
			t.Errorf("presets catalog missing %q", name)
		}
	}
}

func TestPresetWorkerBudgetNeverExceedsPHPMemoryLimit(t *testing.T) {
	for name, p := range presets {
		if p.WorkerBudgetMB > p.PHPMemoryLimitMB {
			t.Errorf("%s: WorkerBudgetMB=%d must be <= PHPMemoryLimitMB=%d",
				name, p.WorkerBudgetMB, p.PHPMemoryLimitMB)
		}
	}
}

func TestPresetsAreMonotonicallyHeavier(t *testing.T) {
	chain := []string{"lite", presetStandard, "business", "woocommerce", "heavy"}
	for i := 1; i < len(chain); i++ {
		prev, curr := presets[chain[i-1]], presets[chain[i]]
		if curr.PHPMemoryLimitMB <= prev.PHPMemoryLimitMB {
			t.Errorf("%s PHPMemoryLimitMB=%d not > %s=%d",
				chain[i], curr.PHPMemoryLimitMB, chain[i-1], prev.PHPMemoryLimitMB)
		}
		if curr.WorkerBudgetMB <= prev.WorkerBudgetMB {
			t.Errorf("%s WorkerBudgetMB=%d not > %s=%d",
				chain[i], curr.WorkerBudgetMB, chain[i-1], prev.WorkerBudgetMB)
		}
	}
}

func TestLookupPresetReturnsErrorForUnknown(t *testing.T) {
	if _, err := LookupPreset("bogus"); err == nil {
		t.Fatal("LookupPreset(\"bogus\") = nil error, want error")
	}
}

func TestLookupPresetReturnsDefinitionForKnown(t *testing.T) {
	p, err := LookupPreset(presetStandard)
	if err != nil {
		t.Fatalf("LookupPreset(%q) error = %v", presetStandard, err)
	}
	if p.Name != presetStandard {
		t.Errorf("Name = %q, want %q", p.Name, presetStandard)
	}
}

func TestDowngradeChainReturnsLighterPreset(t *testing.T) {
	cases := []struct{ from, want string }{
		{"heavy", "woocommerce"},
		{"woocommerce", "business"},
		{"business", presetStandard},
		{presetStandard, "lite"},
	}
	for _, c := range cases {
		got, ok := DowngradePreset(c.from)
		if !ok {
			t.Errorf("DowngradePreset(%q) ok = false, want true", c.from)
			continue
		}
		if got != c.want {
			t.Errorf("DowngradePreset(%q) = %q, want %q", c.from, got, c.want)
		}
	}
}

func TestDowngradeFromLiteReturnsNotOK(t *testing.T) {
	if _, ok := DowngradePreset("lite"); ok {
		t.Error("DowngradePreset(\"lite\") ok = true, want false (no lighter preset exists)")
	}
}
