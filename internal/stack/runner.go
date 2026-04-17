// Package stack manages the installation and removal of the GOW server
// stack: OpenLiteSpeed, LSPHP, MariaDB, and Redis.
package stack

import (
	"fmt"
	"os"
	"os/exec"
)

// Runner abstracts shell command execution for testability.
// ShellRunner is the production implementation; tests supply a mock.
type Runner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) (string, error)
}

// ShellRunner executes real commands via exec.Command.
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
func (ShellRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // binary and args are hardcoded constants
	cmd.Env = noninteractiveEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stack: %s %v: %s", name, args, out)
	}
	return nil
}

// Output executes a command and returns its stdout.
func (ShellRunner) Output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // binary and args are hardcoded constants
	cmd.Env = noninteractiveEnv()
	out, err := cmd.Output()
	return string(out), err
}
