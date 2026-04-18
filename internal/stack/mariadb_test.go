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

	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls, got %d", len(calls))
	}

	if calls[0].name != "sh" {
		t.Errorf("call[0].name = %q, want %q", calls[0].name, "sh")
	}

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

func TestMariaDB_Purge_DeepCleans(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Purge(mr); err != nil {
		t.Fatalf("Purge() = %v", err)
	}

	checks := map[string]bool{
		"debconf":    false,
		"purge":      false,
		"rm_data":    false,
		"rm_log":     false,
		"rm_config":  false,
		"rm_repo":    false,
		"rm_keyring": false,
		"autoremove": false,
		"update":     false,
	}
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "debconf-set-selections") {
			checks["debconf"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "purge") && containsAny(c.args, "mariadb-server") {
			checks["purge"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/var/lib/mysql") {
			checks["rm_data"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/var/log/mysql") {
			checks["rm_log"] = true
		}
		if c.name == "rm" && containsAny(c.args, "/etc/mysql") {
			checks["rm_config"] = true
		}
		if c.name == "rm" && containsAny(c.args, "mariadb.list") {
			checks["rm_repo"] = true
		}
		if c.name == "rm" && containsAny(c.args, "mariadb-keyring.gpg") {
			checks["rm_keyring"] = true
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

func TestMariaDB_Verify_ChecksPackageInstalled(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Verify(mr); err != nil {
		t.Fatalf("Verify() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "dpkg-query" {
			found = true
		}
	}
	if !found {
		t.Error("expected dpkg-query verification call")
	}
}

func TestMariaDB_Start(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Start(mr); err != nil {
		t.Fatalf("Start() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "start") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl start mariadb call")
	}
}

func TestMariaDB_Stop(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Stop(mr); err != nil {
		t.Fatalf("Stop() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "stop") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl stop mariadb call")
	}
}

func TestMariaDB_Restart(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Restart(mr); err != nil {
		t.Fatalf("Restart() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "restart") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl restart mariadb call")
	}
}

func TestMariaDB_Reload(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Reload(mr); err != nil {
		t.Fatalf("Reload() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "systemctl" && containsAny(c.args, "restart") {
			found = true
		}
	}
	if !found {
		t.Error("expected systemctl restart mariadb call")
	}
}

func TestMariaDB_Upgrade(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Upgrade(mr); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "upgrade") && containsAny(c.args, "mariadb-server") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get upgrade mariadb-server call")
	}
}

func TestMariaDB_Remove(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Remove(mr); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "apt-get" && containsAny(c.args, "remove") && containsAny(c.args, "mariadb-server") {
			found = true
		}
	}
	if !found {
		t.Error("expected apt-get remove mariadb-server call")
	}
}

func TestMariaDB_Migrate(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := MariaDB()
	if err := c.Migrate(mr, "11.8"); err != nil {
		t.Fatalf("Migrate() = %v", err)
	}

	checks := map[string]bool{
		"dump":    false,
		"stop":    false,
		"debconf": false,
		"purge":   false,
		"install": false,
		"start":   false,
		"import":  false,
		"cleanup": false,
	}
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "mariadb-dump") {
			checks["dump"] = true
		}
		if c.name == "systemctl" && containsAny(c.args, "stop") {
			checks["stop"] = true
		}
		if c.name == "sh" && containsAny(c.args, "debconf-set-selections") {
			checks["debconf"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "purge") {
			checks["purge"] = true
		}
		if c.name == "apt-get" && containsAny(c.args, "install") {
			checks["install"] = true
		}
		if c.name == "systemctl" && containsAny(c.args, "start") {
			checks["start"] = true
		}
		if c.name == "sh" && containsAny(c.args, "gow-migration.sql") && !containsAny(c.args, "rm") {
			checks["import"] = true
		}
		if c.name == "rm" && containsAny(c.args, "gow-migration.sql") {
			checks["cleanup"] = true
		}
	}
	for k, v := range checks {
		if !v {
			t.Errorf("expected %s call", k)
		}
	}
}
