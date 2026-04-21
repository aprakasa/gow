package stack

import (
	"context"
	"errors"
	"io"
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

func (m *mockRunner) Run(_ context.Context, name string, args ...string) error {
	m.runCalls = append(m.runCalls, call{name, args})
	return m.runErr
}

func (m *mockRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	m.outputCalls = append(m.outputCalls, call{name, args})
	return m.outVal, m.outErr
}

func (m *mockRunner) Stream(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
	m.runCalls = append(m.runCalls, call{name, args})
	return m.runErr
}

var ctx = context.Background()

// --- Component delegation ---

func TestComponent_Install_DelegatesToInstallFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		InstallFn: func(_ context.Context, _ Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Install(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	if !called {
		t.Error("InstallFn was not called")
	}
}

func TestComponent_Install_WrapsError(t *testing.T) {
	c := Component{
		Name: "ols",
		InstallFn: func(_ context.Context, _ Runner) error {
			return errors.New("repo failed")
		},
	}
	err := c.Install(ctx, &mockRunner{})
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

func TestComponent_Install_NilIsNoop(t *testing.T) {
	c := Component{Name: "test"}
	if err := c.Install(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Install() with nil InstallFn = %v", err)
	}
}

func TestComponent_Purge_DelegatesToPurgeFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		PurgeFn: func(_ context.Context, _ Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Purge(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Purge() = %v", err)
	}
	if !called {
		t.Error("PurgeFn was not called")
	}
}

func TestComponent_Purge_WrapsError(t *testing.T) {
	c := Component{
		Name: "redis",
		PurgeFn: func(_ context.Context, _ Runner) error {
			return errors.New("purge failed")
		},
	}
	err := c.Purge(ctx, &mockRunner{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "redis") {
		t.Errorf("error should contain component name, got %q", err.Error())
	}
}

func TestComponent_Purge_NilIsNoop(t *testing.T) {
	c := Component{Name: "test"}
	if err := c.Purge(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Purge() with nil PurgeFn = %v", err)
	}
}

func TestComponent_Upgrade_DelegatesToUpgradeFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		UpgradeFn: func(_ context.Context, _ Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Upgrade(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Upgrade() = %v", err)
	}
	if !called {
		t.Error("UpgradeFn was not called")
	}
}

func TestComponent_Upgrade_NilIsNoop(t *testing.T) {
	c := Component{Name: "test"}
	if err := c.Upgrade(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Upgrade() with nil UpgradeFn = %v", err)
	}
}

func TestComponent_Remove_DelegatesToRemoveFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		RemoveFn: func(_ context.Context, _ Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Remove(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	if !called {
		t.Error("RemoveFn was not called")
	}
}

func TestComponent_Remove_NilIsNoop(t *testing.T) {
	c := Component{Name: "test"}
	if err := c.Remove(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Remove() with nil RemoveFn = %v", err)
	}
}

func TestComponent_Verify_DelegatesToVerifyFn(t *testing.T) {
	called := false
	c := Component{
		Name: "test",
		VerifyFn: func(_ context.Context, _ Runner) error {
			called = true
			return nil
		},
	}
	if err := c.Verify(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if !called {
		t.Error("VerifyFn was not called")
	}
}

func TestComponent_Start_NilIsNoop(t *testing.T) {
	c := Component{Name: "wpcli"}
	if err := c.Start(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Start() with nil StartFn = %v", err)
	}
}

func TestComponent_Stop_NilIsNoop(t *testing.T) {
	c := Component{Name: "wpcli"}
	if err := c.Stop(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Stop() with nil StopFn = %v", err)
	}
}

func TestComponent_Restart_NilIsNoop(t *testing.T) {
	c := Component{Name: "wpcli"}
	if err := c.Restart(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Restart() with nil RestartFn = %v", err)
	}
}

func TestComponent_Reload_NilIsNoop(t *testing.T) {
	c := Component{Name: "wpcli"}
	if err := c.Reload(ctx, &mockRunner{}); err != nil {
		t.Fatalf("Reload() with nil ReloadFn = %v", err)
	}
}

func TestComponent_Start_WrapsError(t *testing.T) {
	c := Component{
		Name: "ols",
		StartFn: func(_ context.Context, _ Runner) error {
			return errors.New("already running")
		},
	}
	err := c.Start(ctx, &mockRunner{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ols") {
		t.Errorf("error should contain component name, got %q", err.Error())
	}
}

func TestComponent_Migrate_NilReturnsError(t *testing.T) {
	c := Component{Name: "redis"}
	err := c.Migrate(ctx, &mockRunner{}, "11.8")
	if err == nil {
		t.Fatal("expected error for component without MigrateFn")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error should say not supported, got %q", err.Error())
	}
}

func TestComponent_Migrate_DelegatesToMigrateFn(t *testing.T) {
	var gotTarget string
	c := Component{
		Name: "mariadb",
		MigrateFn: func(_ context.Context, _ Runner, target string) error {
			gotTarget = target
			return nil
		},
	}
	if err := c.Migrate(ctx, &mockRunner{}, "11.8"); err != nil {
		t.Fatalf("Migrate() = %v", err)
	}
	if gotTarget != "11.8" {
		t.Errorf("target = %q, want %q", gotTarget, "11.8")
	}
}

// --- Registry and Lookup ---

func TestRegistry_ReturnsAllComponents(t *testing.T) {
	components := Registry([]string{"81"})
	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Name
	}
	for _, want := range []string{"ols", "lsphp81", "mariadb", "redis", "wpcli", "composer", "certbot"} {
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

func TestRegistry_MultiplePHPVersions(t *testing.T) {
	components := Registry([]string{"81", "83", "84"})
	lsphpCount := 0
	for _, c := range components {
		if strings.HasPrefix(c.Name, "lsphp") {
			lsphpCount++
		}
	}
	if lsphpCount != 3 {
		t.Errorf("expected 3 LSPHP components, got %d", lsphpCount)
	}
}

func TestLookup_FullRegistryWhenBothEmpty(t *testing.T) {
	components := Lookup(nil, nil)
	if len(components) != 6 {
		t.Errorf("Lookup(nil, nil) returned %d components, want 6", len(components))
	}
}

func TestLookup_PHPOnly(t *testing.T) {
	components := Lookup(nil, []string{"84"})
	if len(components) != 1 {
		t.Fatalf("Lookup(nil, [84]) returned %d components, want 1", len(components))
	}
	if components[0].Name != "lsphp84" {
		t.Errorf("component = %q, want %q", components[0].Name, "lsphp84")
	}
}

func TestLookup_Specific(t *testing.T) {
	components := Lookup([]string{"ols", "redis"}, nil)
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

func TestLookup_SpecificWithPHP(t *testing.T) {
	components := Lookup([]string{"ols", "redis"}, []string{"84"})
	if len(components) != 3 {
		t.Fatalf("Lookup returned %d components, want 3", len(components))
	}
	if components[0].Name != "ols" {
		t.Errorf("first = %q, want ols", components[0].Name)
	}
	if components[1].Name != "lsphp84" {
		t.Errorf("second = %q, want lsphp84", components[1].Name)
	}
	if components[2].Name != "redis" {
		t.Errorf("third = %q, want redis", components[2].Name)
	}
}

func TestLookup_EmptyForUnknown(t *testing.T) {
	components := Lookup([]string{"nginx"}, nil)
	if len(components) != 0 {
		t.Errorf("Lookup(unknown) returned %d components, want 0", len(components))
	}
}

func TestLookup_LSPHPByVersion(t *testing.T) {
	components := Lookup([]string{"lsphp83", "lsphp84"}, []string{"83", "84"})
	if len(components) != 2 {
		t.Fatalf("Lookup returned %d components, want 2", len(components))
	}
	if components[0].Name != "lsphp83" {
		t.Errorf("first = %q, want lsphp83", components[0].Name)
	}
	if components[1].Name != "lsphp84" {
		t.Errorf("second = %q, want lsphp84", components[1].Name)
	}
}
