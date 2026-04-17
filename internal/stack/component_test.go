package stack

import (
	"errors"
	"strings"
	"testing"
)

// mockRunner records calls for assertion in tests.
type mockRunner struct {
	runCalls    []call
	outputCalls []call
	runErr      error
	outErr      error
	outVal      string
}

type call struct {
	name string
	args []string
}

func (m *mockRunner) Run(name string, args ...string) error {
	m.runCalls = append(m.runCalls, call{name, args})
	return m.runErr
}

func (m *mockRunner) Output(name string, args ...string) (string, error) {
	m.outputCalls = append(m.outputCalls, call{name, args})
	return m.outVal, m.outErr
}

// --- Component delegation ---

func TestComponent_Install_DelegatesToInstallFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		InstallFn: func(Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Install(&mockRunner{}); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	if !called {
		t.Error("InstallFn was not called")
	}
}

func TestComponent_Install_WrapsError(t *testing.T) {
	c := Component{
		Name: "ols",
		InstallFn: func(Runner) error {
			return errors.New("repo failed")
		},
	}
	err := c.Install(&mockRunner{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ols") {
		t.Errorf("error should contain component name, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "repo failed") {
		t.Errorf("error should contain inner error, got %q", err.Error())
	}
}

func TestComponent_Uninstall_DelegatesToUninstallFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		UninstallFn: func(Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Uninstall(&mockRunner{}); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}
	if !called {
		t.Error("UninstallFn was not called")
	}
}

func TestComponent_Uninstall_WrapsError(t *testing.T) {
	c := Component{
		Name: "redis",
		UninstallFn: func(Runner) error {
			return errors.New("purge failed")
		},
	}
	err := c.Uninstall(&mockRunner{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "redis") {
		t.Errorf("error should contain component name, got %q", err.Error())
	}
}

func TestComponent_Verify_DelegatesToVerifyFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		VerifyFn: func(Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Verify(&mockRunner{}); err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if !called {
		t.Error("VerifyFn was not called")
	}
}

// --- Registry and Lookup ---

func TestRegistry_ReturnsAllComponents(t *testing.T) {
	components := Registry("81")
	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Name
	}
	for _, want := range []string{"ols", "lsphp", "mariadb", "redis"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Registry() missing component %q, got %v", want, names)
		}
	}
}

func TestLookup_AllWhenEmpty(t *testing.T) {
	components := Lookup(nil, "81")
	if len(components) != 4 {
		t.Errorf("Lookup(nil) returned %d components, want 4", len(components))
	}
}

func TestLookup_Specific(t *testing.T) {
	components := Lookup([]string{"ols", "redis"}, "81")
	if len(components) != 2 {
		t.Fatalf("Lookup returned %d components, want 2", len(components))
	}
	if components[0].Name != "ols" {
		t.Errorf("first component = %q, want %q", components[0].Name, "ols")
	}
	if components[1].Name != "redis" {
		t.Errorf("second component = %q, want %q", components[1].Name, "redis")
	}
}

func TestLookup_EmptyForUnknown(t *testing.T) {
	components := Lookup([]string{"nginx"}, "81")
	if len(components) != 0 {
		t.Errorf("Lookup(unknown) returned %d components, want 0", len(components))
	}
}
