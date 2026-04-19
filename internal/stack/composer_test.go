package stack

import (
	"testing"
)

func TestComposer_Install(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Composer()
	if err := c.Install(ctx, mr); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	foundDownload, foundInstall, foundCleanup := false, false, false
	for _, c := range calls {
		if c.name == "sh" && containsAny(c.args, "wget") && containsAny(c.args, "composer-setup.php") {
			foundDownload = true
		}
		if c.name == "sh" && containsAny(c.args, "php") && containsAny(c.args, "composer-setup.php") {
			foundInstall = true
		}
		if c.name == "rm" && containsAny(c.args, "composer-setup.php") {
			foundCleanup = true
		}
	}
	if !foundDownload {
		t.Error("expected wget download call")
	}
	if !foundInstall {
		t.Error("expected php installer call")
	}
	if !foundCleanup {
		t.Error("expected rm cleanup call")
	}
}

func TestComposer_Upgrade(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Composer()
	if err := c.Upgrade(ctx, mr); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "/usr/local/bin/composer" && containsAny(c.args, "self-update") {
			found = true
		}
	}
	if !found {
		t.Error("expected composer self-update call")
	}
}

func TestComposer_Remove(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Composer()
	if err := c.Remove(ctx, mr); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "rm" && containsAny(c.args, "/usr/local/bin/composer") {
			found = true
		}
	}
	if !found {
		t.Error("expected rm /usr/local/bin/composer call")
	}
}

func TestComposer_Purge(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Composer()
	if err := c.Purge(ctx, mr); err != nil {
		t.Fatalf("Purge() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "rm" && containsAny(c.args, "/usr/local/bin/composer") {
			found = true
		}
	}
	if !found {
		t.Error("expected rm /usr/local/bin/composer call")
	}
}

func TestComposer_Verify(t *testing.T) {
	var calls []call
	mr := &loggingRunner{calls: &calls}

	c := Composer()
	if err := c.Verify(ctx, mr); err != nil {
		t.Fatalf("Verify() = %v", err)
	}

	found := false
	for _, c := range calls {
		if c.name == "test" && containsAny(c.args, "-x") && containsAny(c.args, "/usr/local/bin/composer") {
			found = true
		}
	}
	if !found {
		t.Error("expected test -x /usr/local/bin/composer call")
	}
}

func TestComposer_ServiceFnsAreNil(t *testing.T) {
	c := Composer()
	if c.StartFn != nil || c.StopFn != nil || c.RestartFn != nil || c.ReloadFn != nil {
		t.Error("Composer should not have service functions")
	}
	if c.MigrateFn != nil {
		t.Error("Composer should not have MigrateFn")
	}
}
