package stack

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Runner abstracts shell command execution for testability.
// ShellRunner is the production implementation; tests supply a mock.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) (string, error)
	Stream(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error
}

// ShellRunner executes real commands via exec.CommandContext.
type ShellRunner struct{}

// NewShellRunner returns a Runner that executes real shell commands.
func NewShellRunner() *ShellRunner {
	return &ShellRunner{}
}

// noninteractiveEnv returns os.Environ with DEBIAN_FRONTEND=noninteractive set
// to prevent debconf prompts from blocking apt-get operations.
func noninteractiveEnv() []string {
	return append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
}

// Run executes a command, returning an error if it exits non-zero.
func (ShellRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // binary and args are hardcoded constants
	cmd.Env = noninteractiveEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stack: %s %v: %s", name, args, out)
	}
	return nil
}

// Output executes a command and returns its stdout.
func (ShellRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // binary and args are hardcoded constants
	cmd.Env = noninteractiveEnv()
	out, err := cmd.Output()
	return string(out), err
}

// Stream executes a command with live stdio passthrough. Use for interactive
// commands (wp shell, wp db cli) and long-running follow operations (tail -f).
func (ShellRunner) Stream(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // binary and args are hardcoded constants
	cmd.Env = noninteractiveEnv()
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stack: %s %v: %w", name, args, err)
	}
	return nil
}
