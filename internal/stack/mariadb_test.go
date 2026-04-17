package stack

import (
	"testing"
)

func TestMariaDB_Install_AddsRepoInstallsEnablesStarts(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	// Expected sequence: repo setup, apt-get install, systemctl enable, systemctl start, verify
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls, got %d", len(calls))
	}

	// First call: repo setup via sh
	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q", calls[0].name, "sh")
	}

	// Should have apt-get install
	foundInstall := false
	for _, c := range calls {
		if c.name == "apt-get" && len(c.args) > 0 && c.args[0] == "install" {
			foundInstall = true
			if !containsAny(c.args, "mariadb-server") {
				t.Errorf("apt-get install missing mariadb-server, args = %v", c.args)
			}
		}
	}
	if !foundInstall {
		t.Error("expected apt-get install call")
	}

	// Should enable and start systemd service
	foundEnable, foundStart := false, false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "enable") && containsAny(c.args, "mariadb") {
			foundEnable = true
		}
		if c.name == "systemctl" && containsAny(c.args, "start") && containsAny(c.args, "mariadb") {
			foundStart = true
		}
	}
	if !foundEnable {
		t.Error("expected systemctl enable mariadb call")
	}
	if !foundStart {
		t.Error("expected systemctl start mariadb call")
	}
}

func TestMariaDB_Install_StartFails(t *testing.T) {
	mr := &mockRunner{runErr: errGeneric}

	c := MariaDB()
	err := c.Install(mr)
	if err == nil {
		t.Fatal("expected error when install fails")
	}
}

func TestMariaDB_Uninstall_StopsAndPurges(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Uninstall(mr); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}

	// Should stop service first (via debconf pre-seed)
	if len(calls) < 1 {
		t.Fatal("expected at least 1 call")
	}
	if calls[0].name != "sh" || !containsAny(calls[0].args, "debconf-set-selections") {
		t.Errorf("first call should be debconf pre-seed, got %v", calls[0])
	}

	// Should purge packages
	foundPurge := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "purge") && containsAny(c.args, "mariadb-server") {
			foundPurge = true
		}
	}
	if !foundPurge {
		t.Error("expected apt-get purge mariadb-server call")
	}
}

func TestMariaDB_Verify_ChecksSystemctlIsActive(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Verify(mr); err != nil {
		t.Fatalf("Verify() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "is-active") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl is-active verification call")
	}
}
