package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/stack"
)

// installRunner is a stack.Runner used by the WordPress install regression
// test. It records every Run/Output/Stream call and the stdin payload of any
// Stream call. It also fakes the side effects the test depends on: writing a
// stub wp-config.php when it sees `wp config create`, and returning the
// requested admin username when it sees `wp user get`.
type installRunner struct {
	docRoot  string
	commands [][]string
	stdins   []string
}

func (r *installRunner) record(name string, args []string) {
	r.commands = append(r.commands, append([]string{name}, args...))
}

// stubWPConfig is what wp-cli would write when `wp config create` runs. The
// test only needs the DB_PASSWORD line present so writeWPConfigPasswordDirect
// can replace it with the real password.
const stubWPConfig = "<?php\ndefine('DB_PASSWORD', 'placeholder');\n"

func (r *installRunner) Run(_ context.Context, name string, args ...string) error {
	r.record(name, args)
	if isWPConfigCreate(name, args) {
		if err := os.MkdirAll(r.docRoot, 0o755); err != nil { //nolint:gosec // test dir
			return err
		}
		return os.WriteFile(filepath.Join(r.docRoot, "wp-config.php"), []byte(stubWPConfig), 0o644) //nolint:gosec // test stub
	}
	return nil
}

func (r *installRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	r.record(name, args)
	// installWordPress verifies the admin user via:
	//   wp user get <user> --field=user_login
	// Return the requested username so the verify check passes.
	if isWPUserGet(name, args) {
		for _, a := range args {
			if a == "user" || a == "get" || strings.HasPrefix(a, "--") || strings.HasPrefix(a, "/") || a == "wp" {
				continue
			}
			return a + "\n", nil
		}
	}
	return "", nil
}

func (r *installRunner) Stream(_ context.Context, stdin io.Reader, _, _ io.Writer, name string, args ...string) error {
	r.record(name, args)
	if stdin != nil {
		b, _ := io.ReadAll(stdin) //nolint:errcheck // test mock
		r.stdins = append(r.stdins, string(b))
	}
	return nil
}

var _ stack.Runner = (*installRunner)(nil)

func isWPConfigCreate(name string, args []string) bool {
	if name != stack.WPCLIBinPath && name != "wp" {
		return false
	}
	return len(args) >= 2 && args[0] == "config" && args[1] == "create"
}

func isWPUserGet(name string, args []string) bool {
	if name != stack.WPCLIBinPath && name != "wp" {
		return false
	}
	return len(args) >= 2 && args[0] == "user" && args[1] == "get"
}

var dbPassRE = regexp.MustCompile(`define\(\s*'DB_PASSWORD'\s*,\s*'([^']+)'\s*\)`)

// TestInstallWordPress_DBPasswordNeverInArgv is the regression test for commit
// 16c262e ("avoid exposing DB password in argv during WordPress install").
// The generated DB password must:
//   - never appear in any recorded argv
//   - appear inside a Stream stdin payload alongside `IDENTIFIED BY`
//
// If this fails, someone reverted the hardening — likely by changing the
// CREATE USER SQL back to `mariadb -e <sql>` or by switching to
// `wp config set DB_PASSWORD <pass>`.
func TestInstallWordPress_DBPasswordNeverInArgv(t *testing.T) {
	dir := t.TempDir()
	webRoot := filepath.Join(dir, "www")
	docRoot := filepath.Join(webRoot, "test.example", "htdocs")
	if err := os.MkdirAll(docRoot, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatalf("mkdir docRoot: %v", err)
	}

	// Redirect cron writes to a tempdir so the test does not touch /etc/cron.d.
	origCronDir := cronDir
	cronDir = filepath.Join(dir, "cron.d")
	t.Cleanup(func() { cronDir = origCronDir })

	r := &installRunner{docRoot: docRoot}

	// cacheMode "none" skips the LSCache plugin install, keeping the test
	// focused on the credential-handling path.
	if err := installWordPress(io.Discard, context.Background(), r, "test.example", webRoot, "none", ""); err != nil {
		t.Fatalf("installWordPress() = %v", err)
	}

	// Read the password the install actually generated and persisted.
	cfg, err := os.ReadFile(filepath.Join(docRoot, "wp-config.php")) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read wp-config: %v", err)
	}
	match := dbPassRE.FindSubmatch(cfg)
	if match == nil {
		t.Fatalf("DB_PASSWORD not found in wp-config.php; content:\n%s", cfg)
	}
	dbPass := string(match[1])
	if dbPass == "" || dbPass == "placeholder" {
		t.Fatalf("expected freshly generated DB password in wp-config.php, got %q", dbPass)
	}

	// Negative: must not leak into any subprocess argv.
	for _, cmd := range r.commands {
		for i, a := range cmd {
			if strings.Contains(a, dbPass) {
				t.Fatalf("DB password leaked in argv at index %d:\n  cmd: %v\n  password must be delivered via stdin only", i, cmd)
			}
		}
	}

	// Positive: must appear inside a Stream stdin payload, in a CREATE USER
	// statement. Confirms the credential really did travel via stdin.
	var found bool
	for _, s := range r.stdins {
		if strings.Contains(s, dbPass) && strings.Contains(s, "IDENTIFIED BY") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("DB password not found in any Stream stdin; expected CREATE USER ... IDENTIFIED BY '<pass>' delivered via stdin")
	}
}
