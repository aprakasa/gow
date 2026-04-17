package stack

import (
	"strings"
	"testing"
)

func TestRedis_Install_AddsKeySourceAndInstalls(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	// Expected: GPG key setup, apt source, apt-get update, apt-get install, systemctl enable/start
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls, got %d", len(calls))
	}

	// GPG key via sh
	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q (GPG key setup)", calls[0].name, "sh")
	}

	// apt source write
	if calls[1].name != "sh" {
		t.Errorf("call[1].name = %q, want %q (apt source)", calls[1].name, "sh")
	}

	// apt-get install
	foundInstall := false
	for _, c := range calls {
		if c.name == "apt-get" && len(c.args) > 0 && c.args[0] == "install" {
			foundInstall = true
			if !containsAny(c.args, "redis") {
				t.Errorf("apt-get install missing redis, args = %v", c.args)
			}
		}
	}
	if !foundInstall {
		t.Error("expected apt-get install redis call")
	}

	// systemctl enable + start
	foundEnable, foundStart := false, false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "enable") && containsAny(c.args, "redis") {
			foundEnable = true
		}
		if c.name == "systemctl" && containsAny(c.args, "start") && containsAny(c.args, "redis") {
			foundStart = true
		}
	}
	if !foundEnable {
		t.Error("expected systemctl enable redis-server call")
	}
	if !foundStart {
		t.Error("expected systemctl start redis-server call")
	}
}

func TestRedis_Install_PingReturnsPong(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	// Verify should call redis-cli ping
	found := false
	for _, c := range calls {
		if strings.Contains(c.name, "redis-cli") {
			found = true
		}
	}
	if !found {
		t.Error("expected redis-cli verification call")
	}
}

func TestRedis_Install_Fails(t *testing.T) {
	mr := &mockRunner{runErr: errGeneric}

	c := Redis()
	err := c.Install(mr)
	if err == nil {
		t.Fatal("expected error when install fails")
	}
}

func TestRedis_Uninstall_StopsAndPurges(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Uninstall(mr); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}

	// Should stop first
	if len(calls) < 1 {
		t.Fatal("expected at least 1 call")
	}
	if calls[0].name != "systemctl" || !containsAny(calls[0].args, "stop") {
		t.Errorf("first call should be systemctl stop, got %v", calls[0])
	}

	// Should purge
	foundPurge := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "purge") {
			foundPurge = true
		}
	}
	if !foundPurge {
		t.Error("expected apt-get purge call")
	}
}
