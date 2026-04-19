package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
)

// --- Stack operations ---

type stackOp struct {
	name     string
	fn       func(context.Context, stack.Component, stack.Runner) error
	reverse  bool
	validate bool // run Verify before fn, skip if not installed
	service  bool // skip components without service functions (HasService)
}

var (
	StackOpInstall = stackOp{
		name:     "install",
		fn:       func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Install(ctx, r) },
		validate: true,
	}
	StackOpUpgrade = stackOp{
		name: "upgrade",
		fn:   func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Upgrade(ctx, r) },
	}
	StackOpRemove = stackOp{
		name:     "remove",
		fn:       func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Remove(ctx, r) },
		reverse:  true,
		validate: true,
	}
	StackOpPurge = stackOp{
		name:     "purge",
		fn:       func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Purge(ctx, r) },
		reverse:  true,
		validate: true,
	}
	StackOpStart = stackOp{
		name:    "start",
		fn:      func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Start(ctx, r) },
		service: true,
	}
	StackOpStop = stackOp{
		name:    "stop",
		fn:      func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Stop(ctx, r) },
		service: true,
	}
	StackOpRestart = stackOp{
		name:    "restart",
		fn:      func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Restart(ctx, r) },
		service: true,
	}
	StackOpReload = stackOp{
		name:    "reload",
		fn:      func(ctx context.Context, c stack.Component, r stack.Runner) error { return c.Reload(ctx, r) },
		service: true,
	}
)

func stackFlagsEmpty(sf StackFlags) bool {
	return !sf.OLS && !sf.MariaDB && !sf.Redis && !sf.WPCLI && !sf.Composer && !sf.Certbot &&
		sf.PHP == "" && !sf.PHP81 && !sf.PHP82 && !sf.PHP83 && !sf.PHP84 && !sf.PHP85
}

// RunStackOp executes a stack operation (install, upgrade, remove, etc.).
func RunStackOp(sf StackFlags, op stackOp, d Deps) error {
	names, phpVersions := resolveStackFlags(sf)
	if stackFlagsEmpty(sf) && op.name != "install" {
		if detected := d.InstalledPHP(); len(detected) > 0 {
			phpVersions = detected
		}
	}
	components := stack.Lookup(names, phpVersions)
	r := d.NewRunner()
	ctx := d.Ctx

	if err := r.Run(ctx, "dpkg", "--configure", "-a"); err != nil {
		fmt.Fprintf(d.Stdout, "  warning: dpkg --configure -a failed: %v\n", err)
	}

	iter := components
	if op.reverse {
		iter = reverseComponents(components)
		iter = moveOLSBeforeLSPHP(iter)
	}

	for _, c := range iter {
		if op.service && !c.HasService() {
			continue
		}
		if op.service {
			if err := c.Verify(ctx, r); err != nil {
				fmt.Fprintf(d.Stdout, "  %s: not installed, skipping\n", c.Name)
				continue
			}
		}
		if op.validate && op.reverse {
			if err := c.Verify(ctx, r); err != nil {
				fmt.Fprintf(d.Stdout, "  %s: not installed, skipping\n", c.Name)
				continue
			}
		}
		if op.validate && !op.reverse {
			if c.VerifyFn != nil && !strings.HasPrefix(c.Name, "lsphp") {
				if err := c.Verify(ctx, r); err == nil {
					fmt.Fprintf(d.Stdout, "  %s: already installed, skipping\n", c.Name)
					continue
				}
			}
		}
		fmt.Fprintf(d.Stdout, "%s %s...\n", capitalize(op.name), c.Name)
		if err := op.fn(ctx, c, r); err != nil {
			return err
		}
		if op.name == "upgrade" && c.StatusFn != nil {
			if detail, err := c.Status(ctx, r); err == nil && detail != "" {
				fmt.Fprintf(d.Stdout, "  %s: %s\n", c.Name, detail)
				continue
			}
		}
		fmt.Fprintf(d.Stdout, "  %s: OK\n", c.Name)
	}
	return nil
}

// RunStackPurge purges stack packages and configs, blocking if sites depend on them.
func RunStackPurge(cfg CLIConfig, sf StackFlags, d Deps) error {
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	sites := store.Sites()

	names, phpVersions := resolveStackFlags(sf)
	if stackFlagsEmpty(sf) {
		if detected := d.InstalledPHP(); len(detected) > 0 {
			phpVersions = detected
		}
	}
	components := stack.Lookup(names, phpVersions)

	for _, c := range components {
		deps := componentDependents(c.Name, sites)
		if len(deps) > 0 {
			return fmt.Errorf("cannot purge %s: %d site(s) depend on it (%s). Delete sites first: gow site delete <domain>",
				c.Name, len(deps), strings.Join(deps, ", "))
		}
	}

	return RunStackOp(sf, StackOpPurge, d)
}

// componentDependents returns the names of sites that depend on a stack
// component. Returns empty slice if the component can be safely purged.
func componentDependents(componentName string, sites []state.Site) []string {
	var dependents []string
	for _, s := range sites {
		switch {
		case componentName == "ols":
			dependents = append(dependents, s.Name)
		case componentName == "mariadb" && s.Type != "html":
			dependents = append(dependents, s.Name)
		case componentName == "redis" && s.Type == "wp":
			dependents = append(dependents, s.Name)
		case strings.HasPrefix(componentName, "lsphp"):
			ver := strings.TrimPrefix(componentName, "lsphp")
			if s.PHPVersion == ver {
				dependents = append(dependents, s.Name)
			}
		}
	}
	return dependents
}

// RunStackMigrate migrates a stack component (e.g. MariaDB) to a new version.
func RunStackMigrate(sf StackFlags, d Deps) error {
	if sf.Target == "" {
		return fmt.Errorf("required flag: --target (e.g. --target 11.8)")
	}
	names, _ := resolveStackFlags(sf)
	if len(names) == 0 {
		names = []string{"mariadb"}
	}

	r := d.NewRunner()
	ctx := d.Ctx
	components := stack.Lookup(names, []string{"83"})

	for _, c := range components {
		if c.MigrateFn == nil {
			fmt.Fprintf(d.Stdout, "  %s: migrate not supported, skipping\n", c.Name)
			continue
		}
		fmt.Fprintf(d.Stdout, "Migrating %s to %s...\n", c.Name, sf.Target)
		if err := c.Migrate(ctx, r, sf.Target); err != nil {
			return err
		}
		fmt.Fprintf(d.Stdout, "  %s: migrated to %s\n", c.Name, sf.Target)
	}
	return nil
}

// RunStackStatus shows status for all installed stack components.
func RunStackStatus(w io.Writer, d Deps) error {
	components := stack.Registry(d.InstalledPHP())
	r := d.NewRunner()
	ctx := d.Ctx

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "COMPONENT\tSTATUS\tDETAIL")
	for _, c := range components {
		if err := c.Verify(ctx, r); err != nil {
			fmt.Fprintf(tw, "%s\tnot installed\t\n", c.Name)
			continue
		}
		detail, _ := c.Status(ctx, r)
		active, _ := c.Active(ctx, r)
		if c.HasService() {
			if active {
				fmt.Fprintf(tw, "%s\tactive\t%s\n", c.Name, detail)
			} else {
				fmt.Fprintf(tw, "%s\tstopped\t%s\n", c.Name, detail)
			}
		} else {
			fmt.Fprintf(tw, "%s\tinstalled\t%s\n", c.Name, detail)
		}
	}
	return tw.Flush()
}

// AddStackFlags registers stack component flags on a cobra command.
func AddStackFlags(cmd *cobra.Command, sf *StackFlags) {
	cmd.Flags().BoolVar(&sf.OLS, "ols", false, "OpenLiteSpeed")
	cmd.Flags().StringVar(&sf.PHP, "php", "", "PHP version (e.g. 83, 84)")
	cmd.Flags().BoolVar(&sf.PHP81, "php81", false, "LSPHP 8.1")
	cmd.Flags().BoolVar(&sf.PHP82, "php82", false, "LSPHP 8.2")
	cmd.Flags().BoolVar(&sf.PHP83, "php83", false, "LSPHP 8.3")
	cmd.Flags().BoolVar(&sf.PHP84, "php84", false, "LSPHP 8.4")
	cmd.Flags().BoolVar(&sf.PHP85, "php85", false, "LSPHP 8.5")
	cmd.Flags().BoolVar(&sf.MariaDB, "mariadb", false, "MariaDB")
	cmd.Flags().BoolVar(&sf.Redis, "redis", false, "Redis")
	cmd.Flags().BoolVar(&sf.WPCLI, "wpcli", false, "WP-CLI")
	cmd.Flags().BoolVar(&sf.Composer, "composer", false, "Composer")
	cmd.Flags().BoolVar(&sf.Certbot, "certbot", false, "Certbot (Let's Encrypt)")
}

func resolveStackFlags(sf StackFlags) ([]string, []string) {
	var names []string
	var phpVersions []string

	if sf.OLS {
		names = append(names, "ols")
	}
	if sf.MariaDB {
		names = append(names, "mariadb")
	}
	if sf.Redis {
		names = append(names, "redis")
	}
	if sf.WPCLI {
		names = append(names, "wpcli")
	}
	if sf.Composer {
		names = append(names, "composer")
	}
	if sf.Certbot {
		names = append(names, "certbot")
	}

	seen := make(map[string]bool)
	addVer := func(v string) {
		if !seen[v] {
			seen[v] = true
			phpVersions = append(phpVersions, v)
		}
	}
	if sf.PHP != "" {
		addVer(sf.PHP)
	}
	if sf.PHP81 {
		addVer("81")
	}
	if sf.PHP82 {
		addVer("82")
	}
	if sf.PHP83 {
		addVer("83")
	}
	if sf.PHP84 {
		addVer("84")
	}
	if sf.PHP85 {
		addVer("85")
	}

	if len(names) == 0 && len(phpVersions) == 0 {
		phpVersions = []string{"83"}
		names = []string{"ols", "mariadb", "redis", "wpcli"}
	}

	return names, phpVersions
}

func reverseComponents(cs []stack.Component) []stack.Component {
	n := len(cs)
	out := make([]stack.Component, n)
	for i, c := range cs {
		out[n-1-i] = c
	}
	return out
}

// moveOLSBeforeLSPHP reorders components so OLS comes before any lsphp
// entries. This prevents apt from removing OLS as a side effect of removing
// lsphp (OLS depends on lsphp).
func moveOLSBeforeLSPHP(cs []stack.Component) []stack.Component {
	var ols []stack.Component
	var rest []stack.Component
	for _, c := range cs {
		if c.Name == "ols" {
			ols = append(ols, c)
		} else {
			rest = append(rest, c)
		}
	}
	for i, c := range rest {
		if strings.HasPrefix(c.Name, "lsphp") {
			// rest[:i:i] uses a three-index slice with matching cap to
			// prevent the subsequent append from mutating rest's backing
			// array (which is still referenced via rest[i:]).
			return append(rest[:i:i], append(ols, rest[i:]...)...)
		}
	}
	return cs
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
