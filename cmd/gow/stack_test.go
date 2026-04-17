package main

import (
	"testing"
)

func TestResolveStackFlags_AllWhenEmpty(t *testing.T) {
	sf := stackFlags{}
	names := resolveStackFlags(sf)
	if len(names) != 0 {
		t.Errorf("resolveStackFlags(empty) = %v, want empty (means install all)", names)
	}
}

func TestResolveStackFlags_IndividualFlags(t *testing.T) {
	sf := stackFlags{ols: true, redis: true}
	names := resolveStackFlags(sf)
	if len(names) != 2 {
		t.Fatalf("resolveStackFlags() = %d names, want 2", len(names))
	}
	if names[0] != "ols" {
		t.Errorf("names[0] = %q, want %q", names[0], "ols")
	}
	if names[1] != "redis" {
		t.Errorf("names[1] = %q, want %q", names[1], "redis")
	}
}

func TestResolveStackFlags_AllFlagsSet(t *testing.T) {
	sf := stackFlags{ols: true, lsphp: true, mariadb: true, redis: true}
	names := resolveStackFlags(sf)
	if len(names) != 4 {
		t.Fatalf("resolveStackFlags() = %d names, want 4", len(names))
	}
}

func TestValidatePHPVersion(t *testing.T) {
	tests := []struct {
		php    string
		wantOK bool
	}{
		{"81", true},
		{"82", true},
		{"83", true},
		{"84", true},
		{"85", true},
		{"80", false},
		{"86", false},
		{"", false},
		{"php84", false},
	}
	for _, tt := range tests {
		t.Run(tt.php, func(t *testing.T) {
			err := validatePHPVersion(tt.php)
			if tt.wantOK && err != nil {
				t.Errorf("validatePHPVersion(%q) = %v, want nil", tt.php, err)
			}
			if !tt.wantOK && err == nil {
				t.Errorf("validatePHPVersion(%q) = nil, want error", tt.php)
			}
		})
	}
}
