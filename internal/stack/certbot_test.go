package stack

import "testing"

func TestCertbot_Component(t *testing.T) {
	c := Certbot()
	if c.Name != "certbot" {
		t.Errorf("Name = %q, want %q", c.Name, "certbot")
	}
	if c.InstallFn == nil {
		t.Error("InstallFn should not be nil")
	}
	if c.VerifyFn == nil {
		t.Error("VerifyFn should not be nil")
	}
	if c.RemoveFn == nil {
		t.Error("RemoveFn should not be nil")
	}
	if c.PurgeFn == nil {
		t.Error("PurgeFn should not be nil")
	}
}

func TestCertbot_InRegistry(t *testing.T) {
	components := Registry(nil)
	found := false
	for _, c := range components {
		if c.Name == "certbot" {
			found = true
			break
		}
	}
	if !found {
		t.Error("certbot should be in Registry")
	}
}

func TestCertbot_Lookup(t *testing.T) {
	components := Lookup([]string{"certbot"}, nil)
	if len(components) != 1 {
		t.Fatalf("Lookup returned %d components, want 1", len(components))
	}
	if components[0].Name != "certbot" {
		t.Errorf("Lookup returned %q, want certbot", components[0].Name)
	}
}
