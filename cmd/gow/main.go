// Package main is the gow CLI entrypoint. It wires cobra commands to the
// internal site lifecycle, allocator, and OLS control packages.
package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/site"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
)

type cliConfig struct {
	confDir    string
	stateFile  string
	policyFile string
	webRoot    string
}

type siteFlags struct {
	preset       string
	php          string
	phpMemory    uint
	workerBudget uint
}

type stackFlags struct {
	ols     bool
	lsphp   bool
	mariadb bool
	redis   bool
	php     string
}

func main() {
	rootCmd := &cobra.Command{
		Use:           "gow",
		Short:         "WordPress on OpenLiteSpeed, simplified.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var cfg cliConfig
	rootCmd.PersistentFlags().StringVar(&cfg.confDir, "conf-dir", "/usr/local/lsws/conf", "OLS config base directory")
	rootCmd.PersistentFlags().StringVar(&cfg.stateFile, "state-file", "/etc/gow/state.json", "Site registry file")
	rootCmd.PersistentFlags().StringVar(&cfg.policyFile, "policy-file", "/etc/gow/policy.yaml", "Allocator policy override")
	rootCmd.PersistentFlags().StringVar(&cfg.webRoot, "web-root", "/var/www", "Base directory for site document roots")

	siteCmd := &cobra.Command{
		Use:   "site",
		Short: "Manage WordPress sites",
	}

	var createFlags siteFlags
	createCmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new WordPress site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runCreate(cfg, createFlags, args[0])
		},
	}
	createCmd.Flags().StringVar(&createFlags.preset, "preset", "standard", "Resource preset (lite/standard/business/woocommerce/heavy)")
	createCmd.Flags().StringVar(&createFlags.php, "php", "83", "PHP major version")
	createCmd.Flags().UintVar(&createFlags.phpMemory, "php-memory", 0, "PHP memory limit in MB (only with --preset custom)")
	createCmd.Flags().UintVar(&createFlags.workerBudget, "worker-budget", 0, "Worker budget in MB (only with --preset custom)")

	deleteCmd := &cobra.Command{
		Use:   "delete <domain>",
		Short: "Delete a WordPress site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDelete(cfg, args[0])
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List managed sites",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cfg, cmd.OutOrStdout())
		},
	}

	var tuneFlags siteFlags
	tuneCmd := &cobra.Command{
		Use:   "tune <domain>",
		Short: "Change the resource preset for an existing site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runTune(cfg, tuneFlags, args[0])
		},
	}
	tuneCmd.Flags().StringVar(&tuneFlags.preset, "preset", "", "New resource preset (required)")
	tuneCmd.Flags().UintVar(&tuneFlags.phpMemory, "php-memory", 0, "PHP memory limit in MB (only with --preset custom)")
	tuneCmd.Flags().UintVar(&tuneFlags.workerBudget, "worker-budget", 0, "Worker budget in MB (only with --preset custom)")

	siteCmd.AddCommand(createCmd, deleteCmd, listCmd, tuneCmd)

	var stackInstallFlags stackFlags
	stackInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackInstall(stackInstallFlags)
		},
	}
	stackInstallCmd.Flags().BoolVar(&stackInstallFlags.ols, "ols", false, "Install OpenLiteSpeed")
	stackInstallCmd.Flags().BoolVar(&stackInstallFlags.lsphp, "lsphp", false, "Install LSPHP")
	stackInstallCmd.Flags().BoolVar(&stackInstallFlags.mariadb, "mariadb", false, "Install MariaDB")
	stackInstallCmd.Flags().BoolVar(&stackInstallFlags.redis, "redis", false, "Install Redis")
	stackInstallCmd.Flags().StringVar(&stackInstallFlags.php, "php", "81", "PHP major version (81-85)")

	var stackUninstallFlags stackFlags
	stackUninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackUninstall(stackUninstallFlags)
		},
	}
	stackUninstallCmd.Flags().BoolVar(&stackUninstallFlags.ols, "ols", false, "Uninstall OpenLiteSpeed")
	stackUninstallCmd.Flags().BoolVar(&stackUninstallFlags.lsphp, "lsphp", false, "Uninstall LSPHP")
	stackUninstallCmd.Flags().BoolVar(&stackUninstallFlags.mariadb, "mariadb", false, "Uninstall MariaDB")
	stackUninstallCmd.Flags().BoolVar(&stackUninstallFlags.redis, "redis", false, "Uninstall Redis")
	stackUninstallCmd.Flags().StringVar(&stackUninstallFlags.php, "php", "81", "PHP major version (81-85)")

	stackStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show stack component status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStackStatus(stackInstallFlags.php, cmd.OutOrStdout())
		},
	}
	stackStatusCmd.Flags().StringVar(&stackInstallFlags.php, "php", "81", "PHP major version (81-85)")

	stackCmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the server stack (OLS, LSPHP, MariaDB, Redis)",
	}
	stackCmd.AddCommand(stackInstallCmd, stackUninstallCmd, stackStatusCmd)

	presetsCmd := &cobra.Command{
		Use:   "presets",
		Short: "List available resource presets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPresets(cmd.OutOrStdout())
		},
	}

	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Recompute allocations and reload OLS",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runReconcile(cfg)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current allocations and resource headroom",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cfg, cmd.OutOrStdout())
		},
	}

	rootCmd.AddCommand(siteCmd, stackCmd, presetsCmd, reconcileCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// deps groups the side-effecting operations that cmd/gow depends on.
// Production code uses defaultDeps; tests inject mocks via this struct.
type deps struct {
	detectSpecs func() (system.Specs, error)
	loadPolicy  func(string) (allocator.Policy, error)
	openStore   func(string) (*state.Store, error)
	newOLS      func() ols.Controller
}

var defaultDeps = deps{
	detectSpecs: system.Detect,
	loadPolicy:  allocator.LoadPolicyFromFile,
	openStore:   state.Open,
	newOLS:      func() ols.Controller { return ols.NewController(ols.DefaultBinPath) },
}

func newManagerWithDeps(cfg cliConfig, d deps) (*site.Manager, error) {
	specs, err := d.detectSpecs()
	if err != nil {
		return nil, fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := d.loadPolicy(cfg.policyFile)
	if err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}

	store, err := d.openStore(cfg.stateFile)
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}

	ctrl := d.newOLS()
	return site.NewManager(store, ctrl, specs, policy, cfg.confDir, cfg.webRoot), nil
}

func runCreate(cfg cliConfig, sf siteFlags, domain string) error {
	return runCreateWithDeps(cfg, sf, domain, defaultDeps)
}

func runCreateWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	custom, err := resolveCustom(sf.preset, sf.phpMemory, sf.workerBudget)
	if err != nil {
		return err
	}
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	return m.Create(domain, sf.php, sf.preset, custom)
}

func runDelete(cfg cliConfig, domain string) error {
	return runDeleteWithDeps(cfg, domain, defaultDeps)
}

func runDeleteWithDeps(cfg cliConfig, domain string, d deps) error {
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	return m.Delete(domain)
}

func runList(cfg cliConfig, w io.Writer) error {
	return runListWithDeps(cfg, w, defaultDeps)
}

func runListWithDeps(cfg cliConfig, w io.Writer, d deps) error {
	store, err := d.openStore(cfg.stateFile)
	if err != nil {
		return err
	}

	return formatSites(w, store.Sites())
}

func runPresets(w io.Writer) error {
	return formatPresets(w)
}

func runReconcile(cfg cliConfig) error {
	return runReconcileWithDeps(cfg, defaultDeps)
}

func runReconcileWithDeps(cfg cliConfig, d deps) error {
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	return m.Reconcile()
}

func runTune(cfg cliConfig, sf siteFlags, domain string) error {
	return runTuneWithDeps(cfg, sf, domain, defaultDeps)
}

func runTuneWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	if sf.preset == "" {
		return fmt.Errorf("required flag: --preset")
	}
	custom, err := resolveCustom(sf.preset, sf.phpMemory, sf.workerBudget)
	if err != nil {
		return err
	}
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	return m.Tune(domain, sf.preset, custom)
}

func runStatus(cfg cliConfig, w io.Writer) error {
	return runStatusWithDeps(cfg, w, defaultDeps)
}

func runStatusWithDeps(cfg cliConfig, w io.Writer, d deps) error {
	specs, err := d.detectSpecs()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := d.loadPolicy(cfg.policyFile)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	store, err := d.openStore(cfg.stateFile)
	if err != nil {
		return err
	}

	sites := store.Sites()
	if len(sites) == 0 {
		return formatStatus(w, specs.TotalRAMMB, nil, policy)
	}

	inputs := make([]allocator.SiteInput, len(sites))
	for i, s := range sites {
		inputs[i] = allocator.SiteInput{Name: s.Name, Preset: s.Preset}
	}

	allocs, err := allocator.Compute(specs.TotalRAMMB, specs.CPUCores, inputs, policy)
	if err != nil {
		return fmt.Errorf("compute allocations: %w", err)
	}

	return formatStatus(w, specs.TotalRAMMB, allocs, policy)
}

func resolveCustom(preset string, phpMem, workerBudget uint) (*state.CustomPreset, error) {
	if preset != "custom" {
		if phpMem != 0 || workerBudget != 0 {
			return nil, fmt.Errorf("--php-memory and --worker-budget require --preset custom")
		}
		return nil, nil
	}
	if phpMem == 0 || workerBudget == 0 {
		return nil, fmt.Errorf("--preset custom requires --php-memory and --worker-budget > 0")
	}
	return &state.CustomPreset{PHPMemoryMB: uint64(phpMem), WorkerBudgetMB: uint64(workerBudget)}, nil
}

func runStackInstall(sf stackFlags) error {
	phpVer := sf.php
	if phpVer == "" {
		phpVer = "81"
	}
	if err := validatePHPVersion(phpVer); err != nil {
		return err
	}
	names := resolveStackFlags(sf)
	components := stack.Lookup(names, phpVer)
	r := stack.NewShellRunner()
	// Repair dpkg if previously interrupted (e.g. Ctrl+C during purge).
	_ = r.Run("dpkg", "--configure", "-a")
	for _, c := range components {
		if err := c.Verify(r); err == nil {
			fmt.Printf("  %s: already installed, skipping\n", c.Name)
			continue
		}
		fmt.Printf("Installing %s...\n", c.Name)
		if err := c.Install(r); err != nil {
			return err
		}
		fmt.Printf("  %s: OK\n", c.Name)
	}
	return nil
}

func runStackUninstall(sf stackFlags) error {
	phpVer := sf.php
	if phpVer == "" {
		phpVer = "81"
	}
	names := resolveStackFlags(sf)
	components := stack.Lookup(names, phpVer)
	r := stack.NewShellRunner()
	// Repair dpkg if previously interrupted (e.g. Ctrl+C during purge).
	_ = r.Run("dpkg", "--configure", "-a")
	// Uninstall in reverse order.
	for i := len(components) - 1; i >= 0; i-- {
		c := components[i]
		if err := c.Verify(r); err != nil {
			fmt.Printf("  %s: not installed, skipping\n", c.Name)
			continue
		}
		fmt.Printf("Uninstalling %s...\n", c.Name)
		if err := c.Uninstall(r); err != nil {
			return err
		}
		fmt.Printf("  %s: removed\n", c.Name)
	}
	return nil
}

func runStackStatus(phpVer string, w io.Writer) error {
	if phpVer == "" {
		phpVer = "81"
	}
	components := stack.Registry(phpVer)
	r := stack.NewShellRunner()

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "COMPONENT\tSTATUS\tDETAIL")
	for _, c := range components {
		if err := c.Verify(r); err != nil {
			fmt.Fprintf(tw, "%s\tnot installed\t\n", c.Name)
			continue
		}
		detail, _ := c.Status(r)
		fmt.Fprintf(tw, "%s\tinstalled\t%s\n", c.Name, detail)
	}
	return tw.Flush()
}

func resolveStackFlags(sf stackFlags) []string {
	var names []string
	if sf.ols {
		names = append(names, "ols")
	}
	if sf.lsphp {
		names = append(names, "lsphp")
	}
	if sf.mariadb {
		names = append(names, "mariadb")
	}
	if sf.redis {
		names = append(names, "redis")
	}
	return names // empty => all components
}

func validatePHPVersion(v string) error {
	for _, valid := range []string{"81", "82", "83", "84", "85"} {
		if v == valid {
			return nil
		}
	}
	return fmt.Errorf("unsupported PHP version %q; choose from 81, 82, 83, 84, 85", v)
}

func formatSites(w io.Writer, sites []state.Site) error {
	if len(sites) == 0 {
		_, err := fmt.Fprintln(w, "No sites configured.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SITE\tPHP\tPRESET"); err != nil {
		return err
	}
	for _, s := range sites {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.PHPVersion, s.Preset); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatPresets(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "PRESET\tMEM\tWORKER\tDESCRIPTION"); err != nil {
		return err
	}
	for _, name := range []string{"lite", "standard", "business", "woocommerce", "heavy"} {
		p, _ := allocator.LookupPreset(name)
		if _, err := fmt.Fprintf(tw, "%s\t%dMB\t%dMB\t%s\n", p.Name, p.PHPMemoryLimitMB, p.WorkerBudgetMB, p.Description); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatStatus(w io.Writer, totalRAMMB uint64, allocs []allocator.Allocation, p allocator.Policy) error {
	if len(allocs) == 0 {
		_, err := fmt.Fprintln(w, "No sites configured.")
		return err
	}

	budget := allocator.PHPBudget(totalRAMMB, p)
	var budgeted uint64
	for _, a := range allocs {
		budgeted += uint64(a.Children) * a.WorkerBudgetMB //nolint:gosec // Children is bounded by cpuCeiling
	}
	headroom := budget - min(budgeted, budget)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SITE\tPRESET\tCHILDREN\tHARD LIMIT\tNOTE"); err != nil {
		return err
	}
	for _, a := range allocs {
		note := ""
		if a.Downgraded {
			note = "(downgraded)"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%dMB\t%s\n", a.Site, a.PresetUsed, a.Children, a.MemHardMB, note); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(tw, "\nPHP Budget: %dMB\tBudgeted: %dMB\tHeadroom: %dMB\n", budget, budgeted, headroom); err != nil {
		return err
	}
	return tw.Flush()
}
