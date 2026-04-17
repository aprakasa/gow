package stack

import (
	"strings"
	"testing"
)

func TestLSPHP_Install_InstallsAllPackages(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := LSPHP("84")
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && len(c.args) > 0 && c.args[0] == "install" {
			found = true
			// Verify version-aware packages
			for _, pkg := range []string{"lsphp84", "lsphp84-common", "lsphp84-mysql", "lsphp84-curl"} {
				if !containsAny(c.args, pkg) {
					t.Errorf("apt-get install missing package %q, args = %v", pkg, c.args)
				}
			}
		}
	}
	if !found {
		t.Error("expected apt-get install call")
	}
}

func TestLSPHP_Install_VersionParametrized(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := LSPHP("83")
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	for _, c := range calls {
		if c.name == "apt-get" && len(c.args) > 0 && c.args[0] == "install" {
			if containsAny(c.args, "lsphp84") {
				t.Error("should not install lsphp84 when version is 83")
			}
			if !containsAny(c.args, "lsphp83") {
				t.Error("should install lsphp83 when version is 83")
			}
		}
	}
}

func TestLSPHP_Install_VerifiesPhpVersion(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := LSPHP("84")
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	// Should call lsphp binary with -v to verify
	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "lsphp") && containsAny(c.args, "-v") {
			found = true
		}
	}
	if !found {
		t.Error("expected lsphp -v verification call")
	}
}

func TestLSPHP_Install_Fails(t *testing.T) {
	mr := &mockRunner{runErr: errGeneric}

	c := LSPHP("84")
	err := c.Install(mr)
	if err == nil {
		t.Fatal("expected error when install fails")
	}
}

func TestLSPHP_Uninstall_PurgesAllPackages(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := LSPHP("84")
	if err := c.Uninstall(mr); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "purge") {
			found = true
			if !containsAny(c.args, "lsphp84") {
				t.Errorf("apt-get purge missing lsphp84, args = %v", c.args)
			}
		}
	}
	if !found {
		t.Error("expected apt-get purge call")
	}
}
