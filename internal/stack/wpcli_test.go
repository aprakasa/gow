package stack

import (
	"testing"
)

func TestWPCLI_Install(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := WPCLI()
	if err := c.Install(mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	foundDownload, foundChmod := false, false
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "wget") && containsAny(c.args, "wp-cli.phar") {
			foundDownload = true
		}
		if c.name == "chmod" && containsAny(c.args, "+x") {
			foundChmod = true
		}
	}
	if !foundDownload {
		t.Error("expected wget download call")
	}
	if !foundChmod {
		t.Error("expected chmod +x call")
	}
}

func TestWPCLI_Upgrade(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := WPCLI()
	if err := c.Upgrade(mr); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "wget") {
			found = true
		}
	}
	if !found {
		t.Error("expected wget re-download call")
	}
}

func TestWPCLI_Remove(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := WPCLI()
	if err := c.Remove(mr); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "rm" && containsAny(c.args, "/usr/local/bin/wp") {
			found = true
		}
	}
	if !found {
		t.Error("expected rm /usr/local/bin/wp call")
	}
}

func TestWPCLI_Purge(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := WPCLI()
	if err := c.Purge(mr); err != nil {
		t.Fatalf("Purge() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "rm" && containsAny(c.args, "/usr/local/bin/wp") {
			found = true
		}
	}
	if !found {
		t.Error("expected rm /usr/local/bin/wp call")
	}
}

func TestWPCLI_Verify(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := WPCLI()
	if err := c.Verify(mr); err != nil {
		t.Fatalf("Verify() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "test" && containsAny(c.args, "-x") && containsAny(c.args, "/usr/local/bin/wp") {
			found = true
		}
	}
	if !found {
		t.Error("expected test -x /usr/local/bin/wp call")
	}
}

func TestWPCLI_ServiceFnsAreNil(t *testing.T) {
	c := WPCLI()
	if c.StartFn != nil || c.StopFn != nil || c.RestartFn != nil || c.ReloadFn != nil {
		t.Error("WPCLI should not have service functions")
	}
	if c.MigrateFn != nil {
		t.Error("WPCLI should not have MigrateFn")
	}
}
