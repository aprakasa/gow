package main

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

func TestResolveTuneFlags(t *testing.T) {
	tests := []struct {
		name           string
		sf             siteFlags
		wantPreset     string
		wantCustom     *state.CustomPreset
		wantErr        string
		wantNilCustom  bool
	}{
		{
			name:          "empty tune returns empty preset",
			sf:            siteFlags{preset: ""},
			wantPreset:    "",
			wantNilCustom: true,
		},
		{
			name:          "blog maps to standard",
			sf:            siteFlags{preset: "blog"},
			wantPreset:    "standard",
			wantNilCustom: true,
		},
		{
			name:          "woocommerce maps to woocommerce",
			sf:            siteFlags{preset: "woocommerce"},
			wantPreset:    "woocommerce",
			wantNilCustom: true,
		},
		{
			name:    "custom without memory errors",
			sf:      siteFlags{preset: "custom"},
			wantErr: "--tune custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:    "custom without worker budget errors",
			sf:      siteFlags{preset: "custom", phpMemory: 256},
			wantErr: "--tune custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:       "custom with both set returns custom preset",
			sf:         siteFlags{preset: "custom", phpMemory: 512, workerBudget: 2048},
			wantPreset: "custom",
			wantCustom: &state.CustomPreset{PHPMemoryMB: 512, WorkerBudgetMB: 2048},
		},
		{
			name:          "unknown preset passes through",
			sf:            siteFlags{preset: "standard"},
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
