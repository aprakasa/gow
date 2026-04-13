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

var (
	presetFlag       string
	phpFlag          string
	phpMemoryFlag    uint
	workerBudgetFlag uint
	tunePresetFlag   string
	tunePHPMemory    uint
	tuneWorkerBudget uint
	confDirFlag      string
	stateFileFlag    string
	policyFileFlag   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:           "gow",
		Short:         "WordPress on OpenLiteSpeed, simplified.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&confDirFlag, "conf-dir", "/usr/local/lsws/conf", "OLS config base directory")
	rootCmd.PersistentFlags().StringVar(&stateFileFlag, "state-file", "/etc/gow/state.json", "Site registry file")
	rootCmd.PersistentFlags().StringVar(&policyFileFlag, "policy-file", "/etc/gow/policy.yaml", "Allocator policy override")

	// --- site subcommands ---
	siteCmd := &cobra.Command{
		Use:   "site",
		Short: "Manage WordPress sites",
	}

	createCmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new WordPress site",
		Args:  cobra.ExactArgs(1),
		RunE:  runCreate,
	}
	createCmd.Flags().StringVar(&presetFlag, "preset", "standard", "Resource preset (lite/standard/business/woocommerce/heavy)")
	createCmd.Flags().StringVar(&phpFlag, "php", "83", "PHP major version")
	createCmd.Flags().UintVar(&phpMemoryFlag, "php-memory", 0, "PHP memory limit in MB (only with --preset custom)")
	createCmd.Flags().UintVar(&workerBudgetFlag, "worker-budget", 0, "Worker budget in MB (only with --preset custom)")

	deleteCmd := &cobra.Command{
		Use:   "delete <domain>",
		Short: "Delete a WordPress site",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List managed sites",
		RunE:  runList,
	}

	tuneCmd := &cobra.Command{
		Use:   "tune <domain>",
		Short: "Change the resource preset for an existing site",
		Args:  cobra.ExactArgs(1),
		RunE:  runTune,
	}
	tuneCmd.Flags().StringVar(&tunePresetFlag, "preset", "", "New resource preset (required)")
	tuneCmd.Flags().UintVar(&tunePHPMemory, "php-memory", 0, "PHP memory limit in MB (only with --preset custom)")
	tuneCmd.Flags().UintVar(&tuneWorkerBudget, "worker-budget", 0, "Worker budget in MB (only with --preset custom)")

	siteCmd.AddCommand(createCmd, deleteCmd, listCmd, tuneCmd)

	// --- presets ---
	presetsCmd := &cobra.Command{
		Use:   "presets",
		Short: "List available resource presets",
		RunE:  runPresets,
	}

	// --- reconcile ---
	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Recompute allocations and reload OLS",
		RunE:  runReconcile,
	}

	// --- status ---
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current allocations and resource headroom",
		RunE:  runStatus,
	}

	rootCmd.AddCommand(siteCmd, presetsCmd, reconcileCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newManager constructs a site.Manager from the global flag values.
func newManager() (*site.Manager, func(), error) {
	specs, err := system.Detect()
	if err != nil {
		return nil, nil, fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := allocator.LoadPolicyFromFile(policyFileFlag)
	if err != nil {
		return nil, nil, fmt.Errorf("load policy: %w", err)
	}

	store, err := state.Open(stateFileFlag)
	if err != nil {
		return nil, nil, fmt.Errorf("open state: %w", err)
	}

	ctrl := ols.NewController(ols.DefaultBinPath)
	cleanup := func() { store.Close() }
	return site.NewManager(store, ctrl, specs, policy, confDirFlag), cleanup, nil
}

func runCreate(cmd *cobra.Command, args []string) error {
	custom, err := resolveCustom(presetFlag, phpMemoryFlag, workerBudgetFlag)
	if err != nil {
		return err
	}
	m, cleanup, err := newManager()
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Create(args[0], phpFlag, presetFlag, custom)
}

func runDelete(cmd *cobra.Command, args []string) error {
	m, cleanup, err := newManager()
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Delete(args[0])
}

func runList(cmd *cobra.Command, args []string) error {
	store, err := state.Open(stateFileFlag)
	if err != nil {
		return err
	}
	defer store.Close()

	return formatSites(cmd.OutOrStdout(), store.Sites())
}

func runPresets(cmd *cobra.Command, args []string) error {
	return formatPresets(cmd.OutOrStdout())
}

func runReconcile(cmd *cobra.Command, args []string) error {
	m, cleanup, err := newManager()
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Reconcile()
}

func runTune(cmd *cobra.Command, args []string) error {
	if tunePresetFlag == "" {
		return fmt.Errorf("required flag: --preset")
	}
	custom, err := resolveCustom(tunePresetFlag, tunePHPMemory, tuneWorkerBudget)
	if err != nil {
		return err
	}
	m, cleanup, err := newManager()
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Tune(args[0], tunePresetFlag, custom)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	specs, err := system.Detect()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := allocator.LoadPolicyFromFile(policyFileFlag)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	store, err := state.Open(stateFileFlag)
	if err != nil {
		return err
	}
	defer store.Close()

	sites := store.Sites()
	if len(sites) == 0 {
		return formatStatus(cmd.OutOrStdout(), specs.TotalRAMMB, nil, policy)
	}

	inputs := make([]allocator.SiteInput, len(sites))
	for i, s := range sites {
		inputs[i] = allocator.SiteInput{Name: s.Name, Preset: s.Preset}
	}

	allocs, err := allocator.Compute(specs.TotalRAMMB, specs.CPUCores, inputs, policy)
	if err != nil {
		return fmt.Errorf("compute allocations: %w", err)
	}

	return formatStatus(cmd.OutOrStdout(), specs.TotalRAMMB, allocs, policy)
}

// resolveCustom validates custom-preset flags and returns a CustomPreset when
// preset is "custom", or nil for named presets.
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

// formatSites writes a human-readable site listing to w.
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

// formatPresets writes the preset catalog to w.
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

// formatStatus writes a status overview showing per-site allocations and
// overall PHP budget headroom.
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
