package app

import (
	"fmt"
)

// LogFlags holds flags for the `site log` command.
type LogFlags struct {
	Access bool // --access
	Error  bool // --error
	Follow bool // --follow / -f
	Lines  int  // --lines / -n
}

// RunFlush purges a site's caches via Manager.Flush.
func RunFlush(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Flush(d.Ctx, domain); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Flushed caches for %s.\n", domain)
	return nil
}

// RunWP passes arguments through to wp-cli scoped to a site's document root.
func RunWP(cfg CLIConfig, domain string, args []string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	return m.WP(d.Ctx, domain, d.Stdin, d.Stdout, d.Stderr, args)
}

// RunLog tails the site's log file to stdout/stderr.
func RunLog(cfg CLIConfig, lf LogFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	if lf.Access && lf.Error {
		return fmt.Errorf("--access and --error are mutually exclusive")
	}
	mode := "error"
	if lf.Access {
		mode = "access"
	}
	lines := lf.Lines
	if lines <= 0 {
		lines = 100
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	return m.Log(d.Ctx, domain, mode, lines, lf.Follow, d.Stdout, d.Stderr)
}
