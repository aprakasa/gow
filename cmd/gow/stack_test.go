package main

import (
	"testing"
)

func TestResolveStackFlags_DefaultWhenEmpty(t *testing.T) {
	sf := stackFlags{}
	names, phpVersions := resolveStackFlags(sf)
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
	if len(phpVersions) != 1 || phpVersions[0] != "83" {
		t.Errorf("phpVersions = %v, want [83]", phpVersions)
	}
}

func TestResolveStackFlags_IndividualFlags(t *testing.T) {
	sf := stackFlags{ols: true, redis: true}
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
	sf := stackFlags{ols: true, mariadb: true, redis: true, wpcli: true, composer: true}
	names, _ := resolveStackFlags(sf)
	if len(names) != 5 {
		t.Fatalf("names = %d, want 5", len(names))
	}
}

func TestResolveStackFlags_PHPCombinable(t *testing.T) {
	sf := stackFlags{php83: true, php84: true}
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
	sf := stackFlags{php: true, php83: true}
	_, phpVersions := resolveStackFlags(sf)
	if len(phpVersions) != 1 {
		t.Errorf("phpVersions = %v, want single [83]", phpVersions)
	}
}

func TestResolveStackFlags_PHPDefaultFlag(t *testing.T) {
	sf := stackFlags{php: true}
	_, phpVersions := resolveStackFlags(sf)
	if len(phpVersions) != 1 || phpVersions[0] != "83" {
		t.Errorf("phpVersions = %v, want [83]", phpVersions)
	}
}

func TestResolveStackFlags_WPCLIComposer(t *testing.T) {
	sf := stackFlags{wpcli: true, composer: true}
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
