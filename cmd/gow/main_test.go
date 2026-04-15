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
		{Name: "blog.test", PHPVersion: "83", Preset: "standard", CreatedAt: time.Now()},
		{Name: "shop.test", PHPVersion: "83", Preset: "woocommerce", CreatedAt: time.Now()},
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

func TestResolveCustom(t *testing.T) {
	tests := []struct {
		name         string
		preset       string
		phpMem       uint
		workerBudget uint
		wantErr      string
		wantNil      bool
		wantCustom   *state.CustomPreset
	}{
		{
			name:    "non-custom preset returns nil",
			preset:  "standard",
			wantNil: true,
		},
		{
			name:    "non-custom preset with phpMem",
			preset:  "standard",
			phpMem:  256,
			wantErr: "--php-memory and --worker-budget require --preset custom",
		},
		{
			name:         "non-custom preset with workerBudget",
			preset:       "standard",
			workerBudget: 1024,
			wantErr:      "--php-memory and --worker-budget require --preset custom",
		},
		{
			name:         "non-custom preset with both set",
			preset:       "standard",
			phpMem:       256,
			workerBudget: 1024,
			wantErr:      "--php-memory and --worker-budget require --preset custom",
		},
		{
			name:    "custom preset with zero phpMem",
			preset:  "custom",
			wantErr: "--preset custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:    "custom preset with zero workerBudget",
			preset:  "custom",
			phpMem:  256,
			wantErr: "--preset custom requires --php-memory and --worker-budget > 0",
		},
		{
			name:         "custom preset with both set",
			preset:       "custom",
			phpMem:       512,
			workerBudget: 2048,
			wantCustom:   &state.CustomPreset{PHPMemoryMB: 512, WorkerBudgetMB: 2048},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCustom(tt.preset, tt.phpMem, tt.workerBudget)
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
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got.PHPMemoryMB != tt.wantCustom.PHPMemoryMB || got.WorkerBudgetMB != tt.wantCustom.WorkerBudgetMB {
				t.Fatalf("expected %+v, got %+v", tt.wantCustom, got)
			}
		})
	}
}
