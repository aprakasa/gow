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
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
)

type cliConfig struct {
	confDir    string
	stateFile  string
	policyFile string
}

type siteFlags struct {
	preset       string
	php          string
	phpMemory    uint
	workerBudget uint
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

	rootCmd.AddCommand(siteCmd, presetsCmd, reconcileCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newManager(cfg cliConfig) (*site.Manager, func(), error) {
	specs, err := system.Detect()
	if err != nil {
		return nil, nil, fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := allocator.LoadPolicyFromFile(cfg.policyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load policy: %w", err)
	}

	store, err := state.Open(cfg.stateFile)
	if err != nil {
		return nil, nil, fmt.Errorf("open state: %w", err)
	}

	ctrl := ols.NewController(ols.DefaultBinPath)
	cleanup := func() {}
	return site.NewManager(store, ctrl, specs, policy, cfg.confDir), cleanup, nil
}

func runCreate(cfg cliConfig, sf siteFlags, domain string) error {
	custom, err := resolveCustom(sf.preset, sf.phpMemory, sf.workerBudget)
	if err != nil {
		return err
	}
	m, cleanup, err := newManager(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Create(domain, sf.php, sf.preset, custom)
}

func runDelete(cfg cliConfig, domain string) error {
	m, cleanup, err := newManager(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Delete(domain)
}

func runList(cfg cliConfig, w io.Writer) error {
	store, err := state.Open(cfg.stateFile)
	if err != nil {
		return err
	}

	return formatSites(w, store.Sites())
}

func runPresets(w io.Writer) error {
	return formatPresets(w)
}

func runReconcile(cfg cliConfig) error {
	m, cleanup, err := newManager(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Reconcile()
}

func runTune(cfg cliConfig, sf siteFlags, domain string) error {
	if sf.preset == "" {
		return fmt.Errorf("required flag: --preset")
	}
	custom, err := resolveCustom(sf.preset, sf.phpMemory, sf.workerBudget)
	if err != nil {
		return err
	}
	m, cleanup, err := newManager(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Tune(domain, sf.preset, custom)
}

func runStatus(cfg cliConfig, w io.Writer) error {
	specs, err := system.Detect()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := allocator.LoadPolicyFromFile(cfg.policyFile)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	store, err := state.Open(cfg.stateFile)
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
	var used uint64
	for _, a := range allocs {
		used += a.MemHardMB
	}
	headroom := budget - min(used, budget)

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
	if _, err := fmt.Fprintf(tw, "\nPHP Budget: %dMB\tAllocated: %dMB\tHeadroom: %dMB\n", budget, used, headroom); err != nil {
		return err
	}
	return tw.Flush()
}
