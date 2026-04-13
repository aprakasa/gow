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
	presetFlag     string
	phpFlag        string
	confDirFlag    string
	stateFileFlag  string
	policyFileFlag string
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

	siteCmd.AddCommand(createCmd, deleteCmd, listCmd)

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

	rootCmd.AddCommand(siteCmd, presetsCmd, reconcileCmd)

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
	m, cleanup, err := newManager()
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Create(args[0], phpFlag, presetFlag)
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
