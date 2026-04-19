package stack

import (
	"strings"
	"testing"
)

func TestRedis_Install_AddsKeySourceAndInstalls(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Install(ctx, mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if len(calls) < 7 {
		t.Fatalf("expected at least 7 calls, got %d", len(calls))
	}

	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q (GPG key setup)", calls[0].name, "sh")
	}

	if calls[1].name != "sh" {
		t.Errorf("call[1].name = %q, want %q (apt source)", calls[1].name, "sh")
	}

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

	foundSocket, foundUsermod, foundRestart := false, false, false
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "unixsocket") {
			foundSocket = true
		}
		if c.name == "usermod" && containsAny(c.args, "nobody") && containsAny(c.args, "redis") {
			foundUsermod = true
		}
		if c.name == "systemctl" && containsAny(c.args, "restart") && containsAny(c.args, "redis") {
			foundRestart = true
		}
	}
	if !foundSocket {
		t.Error("expected sed unixsocket config call")
	}
	if !foundUsermod {
		t.Error("expected usermod nobody redis call")
	}
	if !foundRestart {
		t.Error("expected systemctl restart redis-server call")
	}
}

func TestRedis_Install_PingReturnsPong(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Install(ctx, mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

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
	err := c.Install(ctx, mr)
	if err == nil {
		t.Fatal("expected error when install fails")
	}
}

func TestRedis_Purge_DeepCleans(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Purge(ctx, mr); err != nil {
		t.Fatalf("Purge() = %v", err)
	}

	checks := map[string]bool{
		"stop":       false,
		"purge":      false,
		"rm_data":    false,
		"rm_config":  false,
		"rm_gpg":     false,
		"rm_source":  false,
		"autoremove": false,
		"update":     false,
	}
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "stop") {
			checks["stop"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "purge") {
			checks["purge"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/var/lib/redis") {
			checks["rm_data"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/etc/redis") {
			checks["rm_config"] = true
		}
		if c.name == "rm" && containsAny(c.args, "redis-archive-keyring.gpg") {
			checks["rm_gpg"] = true
		}
		if c.name == "rm" && containsAny(c.args, "redis.list") {
			checks["rm_source"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "autoremove") {
			checks["autoremove"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "update") {
			checks["update"] = true
		}
	}
	for k, v := range checks {
		if !v {
			t.Errorf("expected %s call", k)
		}
	}
}

func TestRedis_Start(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Start(ctx, mr); err != nil {
		t.Fatalf("Start() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "start") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl start redis-server call")
	}
}

func TestRedis_Stop(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Stop(ctx, mr); err != nil {
		t.Fatalf("Stop() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "stop") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl stop redis-server call")
	}
}

func TestRedis_Restart(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Restart(ctx, mr); err != nil {
		t.Fatalf("Restart() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "restart") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl restart redis-server call")
	}
}

func TestRedis_Reload(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Reload(ctx, mr); err != nil {
		t.Fatalf("Reload() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "restart") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl restart redis-server call")
	}
}

func TestRedis_Upgrade(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Upgrade(ctx, mr); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "upgrade") && containsAny(c.args, "redis") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get upgrade redis call")
	}
}

func TestRedis_Remove(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if err := c.Remove(ctx, mr); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "remove") && containsAny(c.args, "redis") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get remove redis call")
	}
}

func TestRedis_Active_UsesSocketPath(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if _, err := c.Active(ctx, mr); err != nil {
		t.Fatalf("Active() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "redis-cli" && containsAny(c.args, "/var/run/redis/redis.sock") {
			found = true
		}
	}
	if !found {
		t.Error("expected redis-cli with socket path")
	}
}

func TestRedis_Status_ChecksSocket(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Redis()
	if _, err := c.Status(ctx, mr); err != nil {
		t.Fatalf("Status() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "test" && containsAny(c.args, "-S") && containsAny(c.args, "/var/run/redis/redis.sock") {
			found = true
		}
	}
	if !found {
		t.Error("expected test -S /var/run/redis/redis.sock call")
	}
}
