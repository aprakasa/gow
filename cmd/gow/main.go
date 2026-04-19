// Package main is the gow CLI entrypoint. It wires cobra commands to the
// internal site lifecycle, allocator, and OLS control packages.
package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/site"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
)

// detectInstalledPHP scans the OLS lsphp directory for installed LSPHP
// versions. Returns sorted version strings (e.g., ["81", "83", "84"]).
func detectInstalledPHP() []string {
	olsPHPDir := "/usr/local/lsws"
	entries, err := os.ReadDir(olsPHPDir)
	if err != nil {
		return nil
	}
	var versions []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "lsphp") {
			continue
		}
		ver := strings.TrimPrefix(e.Name(), "lsphp")
		bin := filepath.Join(olsPHPDir, e.Name(), "bin", "lsphp")
		if _, err := os.Stat(bin); err == nil {
			versions = append(versions, ver)
		}
	}
	sort.Strings(versions)
	return versions
}

// phpVersionInstalled checks whether a specific LSPHP version is installed.
func phpVersionInstalled(ver string) bool {
	bin := "/usr/local/lsws/lsphp" + ver + "/bin/lsphp"
	_, err := os.Stat(bin)
	return err == nil
}

type cliConfig struct {
	confDir    string
	stateFile  string
	policyFile string
	webRoot    string
}

type siteFlags struct {
	siteType     string // --type (create only: html, php, wp)
	php          string // --php (version string)
	preset       string // --tune (blog, woocommerce, custom)
	phpMemory    uint   // --php-memory
	workerBudget uint   // --worker-budget
	verbose      bool   // --verbose (info only)
	noPrompt     bool   // --no-prompt (delete only)
	isolate      bool   // --isolate (update only)
	sslEmail     string // --email (ssl only)
	sslStaging   bool   // --staging (ssl only)
}

type stackFlags struct {
	ols      bool
	php      string // --php (version string, e.g. "83", "84")
	php81    bool
	php82    bool
	php83    bool
	php84    bool
	php85    bool
	mariadb  bool
	redis    bool
	wpcli    bool
	composer bool
	certbot  bool
	target   string // --target for migrate
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

	var sCreateFlags siteFlags
	createCmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runCreate(cfg, sCreateFlags, args[0])
		},
	}
	createCmd.Flags().StringVar(&sCreateFlags.siteType, "type", "wp", "Site type (html, php, wp)")
	createCmd.Flags().StringVar(&sCreateFlags.php, "php", "", "PHP major version (default: latest installed)")
	createCmd.Flags().StringVar(&sCreateFlags.preset, "tune", "blog", "Tuning template (blog, woocommerce, custom)")
	createCmd.Flags().UintVar(&sCreateFlags.phpMemory, "php-memory", 0, "PHP memory limit in MB (custom only)")
	createCmd.Flags().UintVar(&sCreateFlags.workerBudget, "worker-budget", 0, "Worker budget in MB (custom only)")

	var sUpdateFlags siteFlags
	updateCmd := &cobra.Command{
		Use:   "update <domain>",
		Short: "Update a site's PHP version or tuning",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runUpdate(cfg, sUpdateFlags, args[0])
		},
	}
	updateCmd.Flags().StringVar(&sUpdateFlags.php, "php", "", "PHP major version (empty = no change)")
	updateCmd.Flags().StringVar(&sUpdateFlags.preset, "tune", "", "Tuning template (blog, woocommerce, custom)")
	updateCmd.Flags().UintVar(&sUpdateFlags.phpMemory, "php-memory", 0, "PHP memory limit in MB (custom only)")
	updateCmd.Flags().UintVar(&sUpdateFlags.workerBudget, "worker-budget", 0, "Worker budget in MB (custom only)")
	updateCmd.Flags().BoolVar(&sUpdateFlags.isolate, "isolate", false, "Isolate site with dedicated system user")

	var sInfoFlags siteFlags
	infoCmd := &cobra.Command{
		Use:   "info <domain>",
		Short: "Show site details",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runInfo(cfg, sInfoFlags, args[0], c.OutOrStdout())
		},
	}
	infoCmd.Flags().BoolVar(&sInfoFlags.verbose, "verbose", false, "Show allocation details")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List managed sites",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cfg, cmd.OutOrStdout())
		},
	}

	onlineCmd := &cobra.Command{
		Use:   "online <domain>",
		Short: "Bring a site online (exit maintenance mode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runOnline(cfg, args[0])
		},
	}

	offlineCmd := &cobra.Command{
		Use:   "offline <domain>",
		Short: "Put a site into maintenance mode (503 page)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runOffline(cfg, args[0])
		},
	}

	var sDeleteFlags siteFlags
	deleteCmd := &cobra.Command{
		Use:   "delete <domain>",
		Short: "Delete a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDelete(cfg, sDeleteFlags, args[0])
		},
	}
	deleteCmd.Flags().BoolVar(&sDeleteFlags.noPrompt, "no-prompt", false, "Skip confirmation prompt")

	var sSSLFlags siteFlags
	sslCmd := &cobra.Command{
		Use:   "ssl <domain>",
		Short: "Enable SSL with Let's Encrypt",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runSSL(cfg, sSSLFlags, args[0])
		},
	}
	sslCmd.Flags().StringVar(&sSSLFlags.sslEmail, "email", "", "Let's Encrypt registration email")
	sslCmd.Flags().BoolVar(&sSSLFlags.sslStaging, "staging", false, "Use Let's Encrypt staging server")
	_ = sslCmd.MarkFlagRequired("email")

	siteCmd.AddCommand(createCmd, updateCmd, infoCmd, listCmd, onlineCmd, offlineCmd, deleteCmd, sslCmd)

	// --- Stack commands ---

	stackCmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the server stack (OLS, LSPHP, MariaDB, Redis, WP-CLI, Composer)",
	}

	var installFlags stackFlags
	stackInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(installFlags, stackOpInstall)
		},
	}
	addStackFlags(stackInstallCmd, &installFlags)

	var upgradeFlags stackFlags
	stackUpgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(upgradeFlags, stackOpUpgrade)
		},
	}
	addStackFlags(stackUpgradeCmd, &upgradeFlags)

	var migrateFlags stackFlags
	stackMigrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate MariaDB to a new major version",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackMigrate(migrateFlags)
		},
	}
	addStackFlags(stackMigrateCmd, &migrateFlags)
	stackMigrateCmd.Flags().StringVar(&migrateFlags.target, "target", "", "Target MariaDB version (e.g. 11.8)")

	var removeFlags stackFlags
	stackRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove stack packages (keeps configs)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(removeFlags, stackOpRemove)
		},
	}
	addStackFlags(stackRemoveCmd, &removeFlags)

	var purgeFlags stackFlags
	stackPurgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge stack packages and configs",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackPurge(purgeFlags, cfg)
		},
	}
	addStackFlags(stackPurgeCmd, &purgeFlags)

	var startFlags stackFlags
	stackStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Start stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(startFlags, stackOpStart)
		},
	}
	addStackFlags(stackStartCmd, &startFlags)

	var stopFlags stackFlags
	stackStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(stopFlags, stackOpStop)
		},
	}
	addStackFlags(stackStopCmd, &stopFlags)

	var restartFlags stackFlags
	stackRestartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(restartFlags, stackOpRestart)
		},
	}
	addStackFlags(stackRestartCmd, &restartFlags)

	var reloadFlags stackFlags
	stackReloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStackOp(reloadFlags, stackOpReload)
		},
	}
	addStackFlags(stackReloadCmd, &reloadFlags)

	stackStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show stack component status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStackStatus(cmd.OutOrStdout())
		},
	}

	stackCmd.AddCommand(
		stackInstallCmd, stackUpgradeCmd, stackMigrateCmd,
		stackRemoveCmd, stackPurgeCmd,
		stackStartCmd, stackStopCmd, stackRestartCmd, stackReloadCmd,
		stackStatusCmd,
	)

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
	detectSpecs   func() (system.Specs, error)
	loadPolicy    func(string) (allocator.Policy, error)
	openStore     func(string) (*state.Store, error)
	newOLS        func() ols.Controller
	newRunner     func() stack.Runner
	installedPHP  func() []string
	phpAvailable  func(string) bool
	wpInstall     func(domain, webRoot string) error
	dbCleanup     func(domain string) error
}

var defaultDeps = deps{
	detectSpecs:   system.Detect,
	loadPolicy:    allocator.LoadPolicyFromFile,
	openStore:     state.Open,
	newOLS:        func() ols.Controller { return ols.NewController(ols.DefaultBinPath) },
	newRunner:     func() stack.Runner { return &stack.ShellRunner{} },
	installedPHP:  detectInstalledPHP,
	phpAvailable:  phpVersionInstalled,
	wpInstall:     installWordPress,
	dbCleanup:     dropSiteDB,
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
	return site.NewManager(store, ctrl, specs, policy, cfg.confDir, cfg.webRoot, d.newRunner()), nil
}

func runCreate(cfg cliConfig, sf siteFlags, domain string) error {
	return runCreateWithDeps(cfg, sf, domain, defaultDeps)
}

func runCreateWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	preset, custom, err := resolveTuneFlags(sf)
	if err != nil {
		return err
	}

	// HTML sites don't need PHP.
	if sf.siteType == "html" {
		m, err := newManagerWithDeps(cfg, d)
		if err != nil {
			return err
		}
		if err := m.Create(domain, sf.siteType, "", "standard", nil); err != nil {
			return err
		}
		fmt.Printf("Site %s created.\n", domain)
		return nil
	}

	phpVer := sf.php
	installed := d.installedPHP()

	if len(installed) == 0 {
		return fmt.Errorf("no LSPHP versions found. Install one first: sudo gow stack install --php")
	}

	if phpVer == "" {
		phpVer = installed[len(installed)-1]
	} else if !d.phpAvailable(phpVer) {
		return fmt.Errorf("PHP %s is not installed. Install it first: sudo gow stack install --php%s", phpVer, phpVer)
	}

	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Create(domain, sf.siteType, phpVer, preset, custom); err != nil {
		return err
	}
	if sf.siteType == "wp" {
		if err := d.wpInstall(domain, cfg.webRoot); err != nil {
			return err
		}
	} else {
		fmt.Printf("Site %s created.\n", domain)
	}
	return nil
}

func runUpdate(cfg cliConfig, sf siteFlags, domain string) error {
	return runUpdateWithDeps(cfg, sf, domain, defaultDeps)
}

func runUpdateWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	preset, custom, err := resolveTuneFlags(sf)
	if err != nil {
		return err
	}
	if sf.php != "" {
		if !d.phpAvailable(sf.php) {
			return fmt.Errorf("PHP %s is not available; install it first with: sudo gow stack install --php%s", sf.php, sf.php)
		}
		if !slices.Contains(d.installedPHP(), sf.php) {
			return fmt.Errorf("PHP %s is not installed; install it first with: sudo gow stack install --php%s", sf.php, sf.php)
		}
	}
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Update(domain, sf.php, preset, custom, sf.isolate); err != nil {
		return err
	}
	fmt.Printf("Site %s updated.\n", domain)
	return nil
}

func runInfo(cfg cliConfig, sf siteFlags, domain string, w io.Writer) error {
	return runInfoWithDeps(cfg, sf, domain, w, defaultDeps)
}

func runInfoWithDeps(cfg cliConfig, sf siteFlags, domain string, w io.Writer, d deps) error {
	store, err := d.openStore(cfg.stateFile)
	if err != nil {
		return err
	}
	s, ok := store.Find(domain)
	if !ok {
		return fmt.Errorf("site %q not found", domain)
	}

	sType := s.Type
	if sType == "" {
		sType = "wp"
	}
	status := "online"
	if s.Maintenance {
		status = "offline"
	}

	php := s.PHPVersion
	if php == "" {
		php = "-"
	}
	preset := s.Preset
	if preset == "" {
		preset = "-"
	}
	fmt.Fprintf(w, "Site:     %s\n", s.Name)
	fmt.Fprintf(w, "Type:     %s\n", sType)
	fmt.Fprintf(w, "PHP:      %s\n", php)
	fmt.Fprintf(w, "Preset:   %s\n", preset)
	fmt.Fprintf(w, "Status:   %s\n", status)
	if s.UnixUser != "" {
		fmt.Fprintf(w, "User:     %s\n", s.UnixUser)
	}
	fmt.Fprintf(w, "Created:  %s\n", s.CreatedAt.Format("2006-01-02 15:04:05"))

	if !sf.verbose {
		return nil
	}

	specs, err := d.detectSpecs()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}
	policy, err := d.loadPolicy(cfg.policyFile)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	sites := store.Sites()
	inputs := make([]allocator.SiteInput, 0, len(sites))
	for _, s := range sites {
		if s.Type == "html" {
			continue
		}
		in := allocator.SiteInput{Name: s.Name, Preset: s.Preset}
		if s.CustomPreset != nil {
			in.CustomPHPMemoryMB = s.CustomPreset.PHPMemoryMB
			in.CustomWorkerBudgetMB = s.CustomPreset.WorkerBudgetMB
		}
		inputs = append(inputs, in)
	}
	allocs, err := allocator.Compute(specs.TotalRAMMB, specs.CPUCores, inputs, policy)
	if err != nil {
		return fmt.Errorf("compute allocations: %w", err)
	}
	for _, a := range allocs {
		if a.Site == domain {
			fmt.Fprintf(w, "\nAllocation:\n")
			fmt.Fprintf(w, "  Children:   %d\n", a.Children)
			fmt.Fprintf(w, "  PHP Memory: %dMB\n", a.PHPMemoryLimitMB)
			fmt.Fprintf(w, "  Mem Soft:   %dMB\n", a.MemSoftMB)
			fmt.Fprintf(w, "  Mem Hard:   %dMB\n", a.MemHardMB)
			if a.Downgraded {
				fmt.Fprintf(w, "  Note:       (downgraded from requested preset)\n")
			}
			break
		}
	}
	return nil
}

func runOnline(cfg cliConfig, domain string) error {
	return runOnlineWithDeps(cfg, domain, defaultDeps)
}

func runOnlineWithDeps(cfg cliConfig, domain string, d deps) error {
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Online(domain); err != nil {
		return err
	}
	fmt.Printf("Site %s is now online.\n", domain)
	return nil
}

func runOffline(cfg cliConfig, domain string) error {
	return runOfflineWithDeps(cfg, domain, defaultDeps)
}

func runOfflineWithDeps(cfg cliConfig, domain string, d deps) error {
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Offline(domain); err != nil {
		return err
	}
	fmt.Printf("Site %s is now offline (maintenance mode).\n", domain)
	return nil
}

func runDelete(cfg cliConfig, sf siteFlags, domain string) error {
	return runDeleteWithDeps(cfg, sf, domain, defaultDeps)
}

func runDeleteWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	store, err := d.openStore(cfg.stateFile)
	if err != nil {
		return err
	}
	if _, ok := store.Find(domain); !ok {
		return fmt.Errorf("site %q not found", domain)
	}

	if !sf.noPrompt {
		fmt.Printf("This will permanently delete:\n")
		fmt.Printf("  - OLS virtual host configuration\n")
		fmt.Printf("  - Site state from gow registry\n")
		fmt.Printf("  - %s/%s/ (including uploads, themes, plugins)\n", cfg.webRoot, domain)
		fmt.Printf("\nAre you sure? [y/N]: ")
		var resp string
		fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" {
			return fmt.Errorf("aborted")
		}
	}

	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}

	site, _ := store.Find(domain)
	sType := site.Type
	if sType == "" {
		sType = "wp"
	}

	if err := m.Delete(domain); err != nil {
		return err
	}

	// Clean up database for WP/PHP sites.
	if sType != "html" {
		if err := d.dbCleanup(domain); err != nil {
			fmt.Printf("  warning: database cleanup failed: %v\n", err)
		}
	}
	fmt.Printf("Site %s deleted.\n", domain)
	return nil
}

func runSSL(cfg cliConfig, sf siteFlags, domain string) error {
	return runSSLWithDeps(cfg, sf, domain, defaultDeps)
}

func runSSLWithDeps(cfg cliConfig, sf siteFlags, domain string, d deps) error {
	m, err := newManagerWithDeps(cfg, d)
	if err != nil {
		return err
	}
	if err := m.EnableSSL(domain, sf.sslEmail, sf.sslStaging); err != nil {
		return err
	}
	fmt.Printf("SSL enabled for %s.\n", domain)
	return nil
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

func resolveTuneFlags(sf siteFlags) (string, *state.CustomPreset, error) {
	tune := sf.preset
	if tune == "" {
		return "", nil, nil
	}

	switch tune {
	case "blog":
		return "standard", nil, nil
	case "woocommerce":
		return "woocommerce", nil, nil
	case "custom":
		if sf.phpMemory == 0 || sf.workerBudget == 0 {
			return "", nil, fmt.Errorf("--tune custom requires --php-memory and --worker-budget > 0")
		}
		return "custom", &state.CustomPreset{
			PHPMemoryMB:    uint64(sf.phpMemory),
			WorkerBudgetMB: uint64(sf.workerBudget),
		}, nil
	default:
		return tune, nil, nil
	}
}

// --- Stack operations ---

type stackOp struct {
	name     string
	fn       func(stack.Component, stack.Runner) error
	reverse  bool
	validate bool    // run Verify before fn, skip if not installed/not installed
	service  bool    // skip components without service functions (HasService)
}

var (
	stackOpInstall = stackOp{
		name:     "install",
		fn:       func(c stack.Component, r stack.Runner) error { return c.Install(r) },
		validate: true,
	}
	stackOpUpgrade = stackOp{
		name: "upgrade",
		fn:   func(c stack.Component, r stack.Runner) error { return c.Upgrade(r) },
	}
	stackOpRemove = stackOp{
		name:     "remove",
		fn:       func(c stack.Component, r stack.Runner) error { return c.Remove(r) },
		reverse:  true,
		validate: true,
	}
	stackOpPurge = stackOp{
		name:     "purge",
		fn:       func(c stack.Component, r stack.Runner) error { return c.Purge(r) },
		reverse:  true,
		validate: true,
	}
	stackOpStart = stackOp{
		name:    "start",
		fn:      func(c stack.Component, r stack.Runner) error { return c.Start(r) },
		service: true,
	}
	stackOpStop = stackOp{
		name:    "stop",
		fn:      func(c stack.Component, r stack.Runner) error { return c.Stop(r) },
		service: true,
	}
	stackOpRestart = stackOp{
		name:    "restart",
		fn:      func(c stack.Component, r stack.Runner) error { return c.Restart(r) },
		service: true,
	}
	stackOpReload = stackOp{
		name:    "reload",
		fn:      func(c stack.Component, r stack.Runner) error { return c.Reload(r) },
		service: true,
	}
)

func stackFlagsEmpty(sf stackFlags) bool {
	return !sf.ols && !sf.mariadb && !sf.redis && !sf.wpcli && !sf.composer && !sf.certbot &&
		sf.php == "" && !sf.php81 && !sf.php82 && !sf.php83 && !sf.php84 && !sf.php85
}

func runStackOp(sf stackFlags, op stackOp) error {
	names, phpVersions := resolveStackFlags(sf)
	// For non-install operations with default flags, detect all installed
	// PHP versions instead of only the default (83). Install keeps the
	// default since its purpose is to set up a known-good stack.
	if stackFlagsEmpty(sf) && op.name != "install" {
		if detected := detectInstalledPHP(); len(detected) > 0 {
			phpVersions = detected
		}
	}
	components := stack.Lookup(names, phpVersions)
	r := stack.NewShellRunner()

	_ = r.Run("dpkg", "--configure", "-a")

	iter := components
	if op.reverse {
		iter = reverseComponents(components)
		// OLS depends on lsphp, so removing lsphp first kills OLS.
		// Move OLS before any lsphp in the remove order.
		iter = moveOLSBeforeLSPHP(iter)
	}

	for _, c := range iter {
		if op.service && !c.HasService() {
			continue
		}
		if op.service {
			if err := c.Verify(r); err != nil {
				fmt.Printf("  %s: not installed, skipping\n", c.Name)
				continue
			}
		}
		if op.validate && op.reverse {
			if err := c.Verify(r); err != nil {
				fmt.Printf("  %s: not installed, skipping\n", c.Name)
				continue
			}
		}
		if op.validate && !op.reverse {
			// Don't skip LSPHP — apt-get install is idempotent and
			// ensures all extension packages are present even if the
			// base was installed as an OLS dependency.
			if c.VerifyFn != nil && !strings.HasPrefix(c.Name, "lsphp") {
				if err := c.Verify(r); err == nil {
					fmt.Printf("  %s: already installed, skipping\n", c.Name)
					continue
				}
			}
		}
		fmt.Printf("%s %s...\n", capitalize(op.name), c.Name)
		if err := op.fn(c, r); err != nil {
			return err
		}
		if op.name == "upgrade" && c.StatusFn != nil {
			if detail, err := c.Status(r); err == nil && detail != "" {
				fmt.Printf("  %s: %s\n", c.Name, detail)
				continue
			}
		}
		fmt.Printf("  %s: OK\n", c.Name)
	}
	return nil
}

func runStackPurge(sf stackFlags, cfg cliConfig) error {
	store, err := state.Open(cfg.stateFile)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	sites := store.Sites()

	names, phpVersions := resolveStackFlags(sf)
	if stackFlagsEmpty(sf) {
		if detected := detectInstalledPHP(); len(detected) > 0 {
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

	return runStackOp(sf, stackOpPurge)
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

func runStackMigrate(sf stackFlags) error {
	if sf.target == "" {
		return fmt.Errorf("required flag: --target (e.g. --target 11.8)")
	}
	names, _ := resolveStackFlags(sf)
	if len(names) == 0 {
		names = []string{"mariadb"}
	}

	r := stack.NewShellRunner()
	components := stack.Lookup(names, []string{"83"})

	for _, c := range components {
		if c.MigrateFn == nil {
			fmt.Printf("  %s: migrate not supported, skipping\n", c.Name)
			continue
		}
		fmt.Printf("Migrating %s to %s...\n", c.Name, sf.target)
		if err := c.Migrate(r, sf.target); err != nil {
			return err
		}
		fmt.Printf("  %s: migrated to %s\n", c.Name, sf.target)
	}
	return nil
}

func runStackStatus(w io.Writer) error {
	components := stack.Registry(detectInstalledPHP())
	r := stack.NewShellRunner()

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "COMPONENT\tSTATUS\tDETAIL")
	for _, c := range components {
		if err := c.Verify(r); err != nil {
			fmt.Fprintf(tw, "%s\tnot installed\t\n", c.Name)
			continue
		}
		detail, _ := c.Status(r)
		active, _ := c.Active(r)
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

func addStackFlags(cmd *cobra.Command, sf *stackFlags) {
	cmd.Flags().BoolVar(&sf.ols, "ols", false, "OpenLiteSpeed")
	cmd.Flags().StringVar(&sf.php, "php", "", "PHP version (e.g. 83, 84)")
	cmd.Flags().BoolVar(&sf.php81, "php81", false, "LSPHP 8.1")
	cmd.Flags().BoolVar(&sf.php82, "php82", false, "LSPHP 8.2")
	cmd.Flags().BoolVar(&sf.php83, "php83", false, "LSPHP 8.3")
	cmd.Flags().BoolVar(&sf.php84, "php84", false, "LSPHP 8.4")
	cmd.Flags().BoolVar(&sf.php85, "php85", false, "LSPHP 8.5")
	cmd.Flags().BoolVar(&sf.mariadb, "mariadb", false, "MariaDB")
	cmd.Flags().BoolVar(&sf.redis, "redis", false, "Redis")
	cmd.Flags().BoolVar(&sf.wpcli, "wpcli", false, "WP-CLI")
	cmd.Flags().BoolVar(&sf.composer, "composer", false, "Composer")
	cmd.Flags().BoolVar(&sf.certbot, "certbot", false, "Certbot (Let's Encrypt)")
}

func resolveStackFlags(sf stackFlags) ([]string, []string) {
	var names []string
	var phpVersions []string

	if sf.ols {
		names = append(names, "ols")
	}
	if sf.mariadb {
		names = append(names, "mariadb")
	}
	if sf.redis {
		names = append(names, "redis")
	}
	if sf.wpcli {
		names = append(names, "wpcli")
	}
	if sf.composer {
		names = append(names, "composer")
	}
	if sf.certbot {
		names = append(names, "certbot")
	}

	seen := make(map[string]bool)
	addVer := func(v string) {
		if !seen[v] {
			seen[v] = true
			phpVersions = append(phpVersions, v)
		}
	}
	if sf.php != "" {
		addVer(sf.php)
	}
	if sf.php81 {
		addVer("81")
	}
	if sf.php82 {
		addVer("82")
	}
	if sf.php83 {
		addVer("83")
	}
	if sf.php84 {
		addVer("84")
	}
	if sf.php85 {
		addVer("85")
	}

	// If any component flag was set but no PHP, no LSPHP.
	// If no flags at all, default to PHP 83 + core components (no composer).
	if len(names) == 0 && len(phpVersions) == 0 {
		phpVersions = []string{"83"}
		names = []string{"ols", "mariadb", "redis", "wpcli"}
	}

	// If component flags set but no PHP, skip LSPHP (user chose specific non-PHP components).
	// If no component flags but PHP set, include only LSPHP versions.

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
			return append(rest[:i:i], append(ols, rest[i:]...)...)
		}
	}
	return cs
}

func dropSiteDB(domain string) error {
	dbName := dbNameFromDomain(domain)
	r := stack.NewShellRunner()
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS `%s`@'localhost'; FLUSH PRIVILEGES;",
		dbName, dbName,
	)
	return r.Run("mariadb", "-e", sql)
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generatePassword(length int) string {
	chars := make([]byte, length)
	for i := range chars {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		chars[i] = passwordChars[n.Int64()]
	}
	return string(chars)
}

func dbNameFromDomain(domain string) string {
	return "wp_" + strings.ReplaceAll(domain, ".", "_")
}

func promptDefault(label, def string) string {
	fmt.Printf("  %s [%s]: ", label, def)
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)
	if input == "" {
		return def
	}
	return input
}

func installWordPress(domain, webRoot string) error {
	docRoot := filepath.Join(webRoot, domain, "htdocs")
	r := stack.NewShellRunner()

	// Prompt for WP admin credentials.
	fmt.Println("\n  WordPress setup:")
	adminUser := promptDefault("Admin username", "admin")
	adminEmail := promptDefault("Admin email", "admin@"+domain)
	adminPass := promptDefault("Admin password", "auto-generated")
	if adminPass == "auto-generated" {
		adminPass = generatePassword(16)
	}

	// Download WordPress.
	fmt.Print("  Downloading WordPress...")
	if err := r.Run(stack.WPCLIBinPath, "core", "download", "--path="+docRoot, "--allow-root"); err != nil {
		return fmt.Errorf("wp core download: %w", err)
	}
	fmt.Println(" OK")

	// Create database and dedicated user.
	dbName := dbNameFromDomain(domain)
	dbUser := dbName
	dbPass := generatePassword(20)

	fmt.Print("  Creating database...")
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; CREATE USER IF NOT EXISTS `%s`@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO `%s`@'localhost'; FLUSH PRIVILEGES;",
		dbName, dbUser, dbPass, dbName, dbUser,
	)
	if err := r.Run("mariadb", "-e", sql); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	fmt.Println(" OK")

	// Generate wp-config.php.
	if err := r.Run(stack.WPCLIBinPath, "config", "create",
		"--dbname="+dbName, "--dbuser="+dbUser, "--dbpass="+dbPass,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config create: %w", err)
	}

	// Install WordPress.
	fmt.Print("  Installing WordPress...")
	if err := r.Run(stack.WPCLIBinPath, "core", "install",
		"--url="+domain, "--title="+domain,
		"--admin_user="+adminUser, "--admin_password="+adminPass,
		"--admin_email="+adminEmail,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp core install: %w", err)
	}
	fmt.Println(" OK")

	// Install and activate LiteSpeed Cache.
	fmt.Print("  Installing LiteSpeed Cache...")
	if err := r.Run(stack.WPCLIBinPath, "plugin", "install", "litespeed-cache",
		"--activate", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("install lscache: %w", err)
	}
	fmt.Println(" OK")

		// Configure LSCache object cache (Redis via Unix socket).
		fmt.Print("  Configuring object cache...")
		if err := configureObjectCache(r, docRoot); err != nil {
			return err
		}
		fmt.Println(" OK")

	fmt.Printf("\n  URL:      http://%s\n", domain)
	fmt.Printf("  Username: %s\n", adminUser)
	fmt.Printf("  Password: %s\n", adminPass)

	return nil
}

// configureObjectCache sets up LSCache to use Redis via Unix socket as object
// cache and copies the object-cache.php drop-in.
func configureObjectCache(r stack.Runner, docRoot string) error {
	phpEval := `$conf = get_option('litespeed-cache-conf', array());
if (!is_array($conf)) $conf = array();
$conf['object'] = true;
$conf['object-kind'] = true;
$conf['object-host'] = '/var/run/redis/redis.sock';
$conf['object-port'] = 0;
$conf['object-life'] = 360;
$conf['object-persistent'] = true;
$conf['object-admin'] = true;
$conf['object-db_id'] = 0;
update_option('litespeed-cache-conf', $conf);
// Write .litespeed_conf.dat so the object-cache.php drop-in can read
// settings before plugins are loaded (early bootstrap).
$dat = array(
    'object' => true,
    'object-kind' => true,
    'object-host' => '/var/run/redis/redis.sock',
    'object-port' => 0,
    'object-life' => 360,
    'object-persistent' => true,
    'object-admin' => true,
    'object-db_id' => 0,
);
file_put_contents(WP_CONTENT_DIR . '/.litespeed_conf.dat', wp_json_encode($dat));
`
	if err := r.Run(stack.WPCLIBinPath, "eval", phpEval,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("configure object cache: %w", err)
	}
	// Copy object-cache.php drop-in (LSCache may not auto-create via CLI).
	if err := r.Run("cp", "-n",
		docRoot+"/wp-content/plugins/litespeed-cache/lib/object-cache.php",
		docRoot+"/wp-content/object-cache.php",
	); err != nil {
		return fmt.Errorf("copy object-cache drop-in: %w", err)
	}
	return nil
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func formatSites(w io.Writer, sites []state.Site) error {
	if len(sites) == 0 {
		_, err := fmt.Fprintln(w, "No sites configured.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SITE\tTYPE\tPHP\tPRESET\tSTATUS"); err != nil {
		return err
	}
	for _, s := range sites {
		status := "online"
		if s.Maintenance {
			status = "offline"
		}
		sType := s.Type
		if sType == "" {
			sType = "wp"
		}
		php := s.PHPVersion
		preset := s.Preset
		if sType == "html" {
			php = "-"
			preset = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, sType, php, preset, status); err != nil {
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
