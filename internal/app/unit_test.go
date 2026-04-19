package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

func TestFormatSites_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := formatSites(&buf, nil); err != nil {
		t.Fatalf("formatSites() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "No sites") {
		t.Errorf("expected 'No sites' message, got %q", got)
	}
}

func TestFormatSites_ListsAllSites(t *testing.T) {
	sites := []state.Site{
		{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard", CreatedAt: time.Now()},
		{Name: "shop.test", Type: "wp", PHPVersion: "83", Preset: "woocommerce", CreatedAt: time.Now()},
	}
	var buf bytes.Buffer
	if err := formatSites(&buf, sites); err != nil {
		t.Fatalf("formatSites() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "blog.test") {
		t.Error("output should contain blog.test")
	}
	if !strings.Contains(got, "shop.test") {
		t.Error("output should contain shop.test")
	}
	if !strings.Contains(got, "online") {
		t.Error("output should contain status column with 'online'")
	}
}

func TestFormatSites_ShowsPreset(t *testing.T) {
	sites := []state.Site{
		{Name: "shop.test", PHPVersion: "83", Preset: "woocommerce", CreatedAt: time.Now()},
	}
	var buf bytes.Buffer
	if err := formatSites(&buf, sites); err != nil {
		t.Fatalf("formatSites() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "woocommerce") {
		t.Error("output should show preset")
	}
}

func TestFormatPresets_ListsAll(t *testing.T) {
	var buf bytes.Buffer
	if err := formatPresets(&buf); err != nil {
		t.Fatalf("formatPresets() = %v", err)
	}

	got := buf.String()
	for _, name := range []string{"lite", "standard", "business", "woocommerce", "heavy"} {
		if !strings.Contains(got, name) {
			t.Errorf("output should contain preset %q", name)
		}
	}
}

func TestFormatPresets_ShowsDescription(t *testing.T) {
	var buf bytes.Buffer
	if err := formatPresets(&buf); err != nil {
		t.Fatalf("formatPresets() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Typical blog") {
		t.Errorf("output should contain preset description, got %q", got)
	}
}

func TestFormatPresets_ShowsMemory(t *testing.T) {
	var buf bytes.Buffer
	if err := formatPresets(&buf); err != nil {
		t.Fatalf("formatPresets() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "256") {
		t.Error("output should show standard preset's memory limit")
	}
}

func TestFormatStatus_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := formatStatus(&buf, 8192, nil, allocator.DefaultPolicy()); err != nil {
		t.Fatalf("formatStatus() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "No sites") {
		t.Errorf("expected 'No sites' message, got %q", got)
	}
}

func TestFormatStatus_ShowsSiteAllocation(t *testing.T) {
	allocs := []allocator.Allocation{
		{Site: "blog.test", PresetUsed: "standard", Children: 10, PHPMemoryLimitMB: 256, MemHardMB: 2560},
	}
	var buf bytes.Buffer
	if err := formatStatus(&buf, 8192, allocs, allocator.DefaultPolicy()); err != nil {
		t.Fatalf("formatStatus() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "blog.test") {
		t.Error("output should contain site name")
	}
	if !strings.Contains(got, "standard") {
		t.Error("output should contain preset")
	}
}

func TestFormatStatus_ShowsHeadroom(t *testing.T) {
	allocs := []allocator.Allocation{
		{Site: "blog.test", PresetUsed: "standard", Children: 10, MemHardMB: 2560},
	}
	var buf bytes.Buffer
	if err := formatStatus(&buf, 8192, allocs, allocator.DefaultPolicy()); err != nil {
		t.Fatalf("formatStatus() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Headroom") {
		t.Error("output should show headroom")
	}
}

func TestFormatStatus_ShowsDowngrade(t *testing.T) {
	allocs := []allocator.Allocation{
		{Site: "big.test", PresetUsed: "standard", Downgraded: true, Children: 4, MemHardMB: 1024},
	}
	var buf bytes.Buffer
	if err := formatStatus(&buf, 8192, allocs, allocator.DefaultPolicy()); err != nil {
		t.Fatalf("formatStatus() = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "downgraded") {
		t.Error("output should note downgrade")
	}
}

func TestComponentDependents_NoSites(t *testing.T) {
	got := componentDependents("redis", nil)
	if len(got) != 0 {
		t.Errorf("expected no dependents, got %v", got)
	}
}

func TestComponentDependents_OLS(t *testing.T) {
	sites := []state.Site{
		{Name: "a.test", Type: "wp", PHPVersion: "83"},
		{Name: "b.test", Type: "html"},
	}
	got := componentDependents("ols", sites)
	if len(got) != 2 {
		t.Fatalf("expected 2 dependents, got %v", got)
	}
}

func TestComponentDependents_MariaDB(t *testing.T) {
	sites := []state.Site{
		{Name: "wp.test", Type: "wp"},
		{Name: "php.test", Type: "php"},
		{Name: "html.test", Type: "html"},
	}
	got := componentDependents("mariadb", sites)
	if len(got) != 2 {
		t.Fatalf("expected 2 dependents (wp+php), got %v", got)
	}
}

func TestComponentDependents_Redis(t *testing.T) {
	sites := []state.Site{
		{Name: "wp.test", Type: "wp"},
		{Name: "php.test", Type: "php"},
		{Name: "html.test", Type: "html"},
	}
	got := componentDependents("redis", sites)
	if len(got) != 1 || got[0] != "wp.test" {
		t.Fatalf("expected [wp.test], got %v", got)
	}
}

func TestComponentDependents_LSPHP(t *testing.T) {
	sites := []state.Site{
		{Name: "a.test", Type: "wp", PHPVersion: "83"},
		{Name: "b.test", Type: "wp", PHPVersion: "84"},
	}
	got := componentDependents("lsphp83", sites)
	if len(got) != 1 || got[0] != "a.test" {
		t.Fatalf("expected [a.test], got %v", got)
	}
}

func TestComponentDependents_LSPHP_NoMatch(t *testing.T) {
	sites := []state.Site{
		{Name: "a.test", Type: "wp", PHPVersion: "84"},
	}
	got := componentDependents("lsphp83", sites)
	if len(got) != 0 {
		t.Errorf("expected no dependents, got %v", got)
	}
}

func TestComponentDependents_UnknownComponent(t *testing.T) {
	sites := []state.Site{
		{Name: "a.test", Type: "wp"},
	}
	got := componentDependents("wpcli", sites)
	if len(got) != 0 {
		t.Errorf("expected no dependents for wpcli, got %v", got)
	}
}

func TestResolveTuneFlags(t *testing.T) {
	tests := []struct {
		name          string
		sf            SiteFlags
		wantPreset    string
		wantCustom    *state.CustomPreset
		wantErr       string
		wantNilCustom bool
	}{
		{
			name:          "empty tune returns empty preset",
			sf:            SiteFlags{Preset: ""},
			wantPreset:    "",
			wantNilCustom: true,
		},
		{
			name:          "blog maps to standard",
			sf:            SiteFlags{Preset: "blog"},
			wantPreset:    "standard",
			wantNilCustom: true,
		},
		{
			name:          "woocommerce maps to woocommerce",
			sf:            SiteFlags{Preset: "woocommerce"},
			wantPreset:    "woocommerce",
			wantNilCustom: true,
		},
		{
			name:    "custom without memory errors",
			sf:      SiteFlags{Preset: "custom"},
			wantErr: "--tune custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:    "custom without worker budget errors",
			sf:      SiteFlags{Preset: "custom", PHPMemory: 256},
			wantErr: "--tune custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:       "custom with both set returns custom preset",
			sf:         SiteFlags{Preset: "custom", PHPMemory: 512, WorkerBudget: 2048},
			wantPreset: "custom",
			wantCustom: &state.CustomPreset{PHPMemoryMB: 512, WorkerBudgetMB: 2048},
		},
		{
			name:          "unknown preset passes through",
			sf:            SiteFlags{Preset: "standard"},
			wantPreset:    "standard",
			wantNilCustom: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, custom, err := resolveTuneFlags(tt.sf)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if preset != tt.wantPreset {
				t.Errorf("preset = %q, want %q", preset, tt.wantPreset)
			}
			if tt.wantNilCustom {
				if custom != nil {
					t.Fatalf("expected nil custom, got %+v", custom)
				}
				return
			}
			if custom.PHPMemoryMB != tt.wantCustom.PHPMemoryMB || custom.WorkerBudgetMB != tt.wantCustom.WorkerBudgetMB {
				t.Fatalf("expected %+v, got %+v", tt.wantCustom, custom)
			}
		})
	}
}

// --- resolveStackFlags tests ---

func TestResolveStackFlags_DefaultWhenEmpty(t *testing.T) {
	sf := StackFlags{}
	names, phpVersions := resolveStackFlags(sf)
	wantNames := []string{"ols", "mariadb", "redis", "wpcli"}
	if len(names) != len(wantNames) {
		t.Fatalf("names = %v, want %v", names, wantNames)
	}
	for i, n := range wantNames {
		if names[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, names[i], n)
		}
	}
	if len(phpVersions) != 1 || phpVersions[0] != "83" {
		t.Errorf("phpVersions = %v, want [83]", phpVersions)
	}
}

func TestResolveStackFlags_IndividualFlags(t *testing.T) {
	sf := StackFlags{OLS: true, Redis: true}
	names, phpVersions := resolveStackFlags(sf)
	if len(names) != 2 {
		t.Fatalf("names = %d, want 2", len(names))
	}
	if names[0] != "ols" {
		t.Errorf("names[0] = %q, want %q", names[0], "ols")
	}
	if names[1] != "redis" {
		t.Errorf("names[1] = %q, want %q", names[1], "redis")
	}
	if len(phpVersions) != 0 {
		t.Errorf("phpVersions = %v, want empty", phpVersions)
	}
}

func TestResolveStackFlags_AllFlagsSet(t *testing.T) {
	sf := StackFlags{OLS: true, MariaDB: true, Redis: true, WPCLI: true, Composer: true}
	names, _ := resolveStackFlags(sf)
	if len(names) != 5 {
		t.Fatalf("names = %d, want 5", len(names))
	}
}

func TestResolveStackFlags_PHPCombinable(t *testing.T) {
	sf := StackFlags{PHP83: true, PHP84: true}
	names, phpVersions := resolveStackFlags(sf)
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
	if len(phpVersions) != 2 {
		t.Fatalf("phpVersions = %d, want 2", len(phpVersions))
	}
	if phpVersions[0] != "83" || phpVersions[1] != "84" {
		t.Errorf("phpVersions = %v, want [83 84]", phpVersions)
	}
}

func TestResolveStackFlags_PHPDedup(t *testing.T) {
	sf := StackFlags{PHP: "83", PHP83: true}
	_, phpVersions := resolveStackFlags(sf)
	if len(phpVersions) != 1 {
		t.Errorf("phpVersions = %v, want single [83]", phpVersions)
	}
}

func TestResolveStackFlags_PHPDefaultFlag(t *testing.T) {
	sf := StackFlags{PHP: "83"}
	_, phpVersions := resolveStackFlags(sf)
	if len(phpVersions) != 1 || phpVersions[0] != "83" {
		t.Errorf("phpVersions = %v, want [83]", phpVersions)
	}
}

func TestResolveStackFlags_WPCLIComposer(t *testing.T) {
	sf := StackFlags{WPCLI: true, Composer: true}
	names, phpVersions := resolveStackFlags(sf)
	if len(names) != 2 {
		t.Fatalf("names = %d, want 2", len(names))
	}
	if names[0] != "wpcli" {
		t.Errorf("names[0] = %q, want wpcli", names[0])
	}
	if names[1] != "composer" {
		t.Errorf("names[1] = %q, want composer", names[1])
	}
	if len(phpVersions) != 0 {
		t.Errorf("phpVersions = %v, want empty", phpVersions)
	}
}
