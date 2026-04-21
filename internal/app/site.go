package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/site"
	"github.com/aprakasa/gow/internal/state"
)

// isTerminal reports whether f is attached to a terminal (not a pipe or file).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// NewManager constructs a site.Manager using the injected dependencies.
func NewManager(cfg CLIConfig, d Deps) (*site.Manager, error) {
	specs, err := d.DetectSpecs()
	if err != nil {
		return nil, fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := d.LoadPolicy(cfg.PolicyFile)
	if err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}

	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}

	ctrl := d.NewOLS()
	mgr := site.NewManager(store, ctrl, specs, policy, cfg.ConfDir, cfg.WebRoot, d.NewRunner())
	if cfg.LogDir != "" {
		mgr.SetLogDir(cfg.LogDir)
	}
	return mgr, nil
}

// RunCreate creates a new site.
func RunCreate(cfg CLIConfig, sf SiteFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	preset, custom, err := resolveTuneFlags(sf)
	if err != nil {
		return err
	}

	// HTML sites don't need PHP.
	if sf.SiteType == "html" {
		if sf.NoCache {
			return fmt.Errorf("--no-cache only applies to --type wp")
		}
		m, err := NewManager(cfg, d)
		if err != nil {
			return err
		}
		if err := m.Create(d.Ctx, domain, sf.SiteType, "", "standard", "", "", nil); err != nil {
			return err
		}
		fmt.Fprintf(d.Stdout, "Site %s created.\n", domain)
		return nil
	}

	phpVer := sf.PHP
	installed := d.InstalledPHP()

	if len(installed) == 0 {
		return fmt.Errorf("no LSPHP versions found. Install one first: sudo gow stack install --php")
	}

	if phpVer == "" {
		phpVer = installed[len(installed)-1]
	} else if !d.PHPAvailable(phpVer) {
		return fmt.Errorf("PHP %s is not installed. Install it first: sudo gow stack install --php%s", phpVer, phpVer)
	}

	cacheMode := ""
	if sf.SiteType == "wp" {
		cacheMode = "lscache"
		if sf.NoCache {
			cacheMode = "none"
		}
	} else if sf.NoCache {
		return fmt.Errorf("--no-cache only applies to --type wp")
	}

	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Create(d.Ctx, domain, sf.SiteType, phpVer, preset, cacheMode, sf.Multisite, custom); err != nil {
		return err
	}
	if sf.SiteType == "wp" {
		if err := d.WPInstall(domain, cfg.WebRoot, cacheMode, sf.Multisite); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(d.Stdout, "Site %s created.\n", domain)
	}
	return nil
}

// RunUpdate updates a site's PHP version or tuning.
func RunUpdate(cfg CLIConfig, sf SiteFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	preset, custom, err := resolveTuneFlags(sf)
	if err != nil {
		return err
	}
	if sf.PHP != "" {
		if !d.PHPAvailable(sf.PHP) {
			return fmt.Errorf("PHP %s is not available; install it first with: sudo gow stack install --php%s", sf.PHP, sf.PHP)
		}
		if !slices.Contains(d.InstalledPHP(), sf.PHP) {
			return fmt.Errorf("PHP %s is not installed; install it first with: sudo gow stack install --php%s", sf.PHP, sf.PHP)
		}
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Update(d.Ctx, domain, sf.PHP, preset, custom, sf.Isolate); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Site %s updated.\n", domain)
	return nil
}

// RunInfo shows details for a site.
func RunInfo(cfg CLIConfig, sf SiteFlags, domain string, w io.Writer, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	store, err := d.OpenStore(cfg.StateFile)
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

	if !sf.Verbose {
		return nil
	}

	specs, err := d.DetectSpecs()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}
	policy, err := d.LoadPolicy(cfg.PolicyFile)
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

// RunOnline brings a site online.
func RunOnline(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Online(d.Ctx, domain); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Site %s is now online.\n", domain)
	return nil
}

// RunOffline puts a site into maintenance mode.
func RunOffline(cfg CLIConfig, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	if err := m.Offline(d.Ctx, domain); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "Site %s is now offline (maintenance mode).\n", domain)
	return nil
}

// RunDelete deletes a site.
func RunDelete(cfg CLIConfig, sf SiteFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return err
	}
	if _, ok := store.Find(domain); !ok {
		return fmt.Errorf("site %q not found", domain)
	}

	if !sf.NoPrompt {
		if !isTerminal(os.Stdin) {
			return fmt.Errorf("refusing to delete %q on non-interactive stdin; re-run with --no-prompt to confirm", domain)
		}
		fmt.Fprintf(d.Stdout, "This will permanently delete:\n")
		fmt.Fprintf(d.Stdout, "  - OLS virtual host configuration\n")
		fmt.Fprintf(d.Stdout, "  - Site state from gow registry\n")
		fmt.Fprintf(d.Stdout, "  - %s/%s/ (including uploads, themes, plugins)\n", cfg.WebRoot, domain)
		fmt.Fprintf(d.Stdout, "\nAre you sure? [y/N]: ")
		var resp string
		if _, err := fmt.Scanln(&resp); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if resp != "y" && resp != "Y" {
			return fmt.Errorf("aborted")
		}
	}

	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}

	site, _ := store.Find(domain)
	sType := site.Type
	if sType == "" {
		sType = "wp"
	}

	if err := m.Delete(d.Ctx, domain); err != nil {
		return err
	}

	// Clean up database for WP/PHP sites.
	if sType != "html" {
		if err := d.DBCleanup(domain); err != nil {
			fmt.Fprintf(d.Stdout, "  warning: database cleanup failed: %v\n", err)
		}
	}
	fmt.Fprintf(d.Stdout, "Site %s deleted.\n", domain)
	return nil
}

// RunSSL enables SSL for a site.
func RunSSL(cfg CLIConfig, sf SiteFlags, domain string, d Deps) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	opts := site.SSLOptions{
		Email:    sf.SSLEmail,
		Staging:  sf.SSLStaging,
		Wildcard: sf.SSLWildcard,
		DNS:      sf.SSLDNS,
		HSTS:     sf.SSLHSTS,
	}
	if err := m.EnableSSL(d.Ctx, domain, opts); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "SSL enabled for %s.\n", domain)
	return nil
}

// RunList lists all managed sites.
func RunList(cfg CLIConfig, w io.Writer, d Deps) error {
	store, err := d.OpenStore(cfg.StateFile)
	if err != nil {
		return err
	}

	return formatSites(w, store.Sites())
}

// RunPresets lists available resource presets.
func RunPresets(w io.Writer) error {
	return formatPresets(w)
}

// RunReconcile recomputes allocations and reloads OLS.
func RunReconcile(cfg CLIConfig, d Deps) error {
	m, err := NewManager(cfg, d)
	if err != nil {
		return err
	}
	return m.Reconcile(d.Ctx)
}

// RunStatus shows current allocations and resource headroom.
func RunStatus(cfg CLIConfig, w io.Writer, d Deps) error {
	specs, err := d.DetectSpecs()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}

	policy, err := d.LoadPolicy(cfg.PolicyFile)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	store, err := d.OpenStore(cfg.StateFile)
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

func resolveTuneFlags(sf SiteFlags) (string, *state.CustomPreset, error) {
	tune := sf.Preset
	if tune == "" {
		return "", nil, nil
	}

	switch tune {
	case "blog":
		return "standard", nil, nil
	case "woocommerce":
		return "woocommerce", nil, nil
	case "custom":
		if sf.PHPMemory == 0 || sf.WorkerBudget == 0 {
			return "", nil, fmt.Errorf("--tune custom requires --php-memory and --worker-budget > 0")
		}
		return "custom", &state.CustomPreset{
			PHPMemoryMB:    uint64(sf.PHPMemory),
			WorkerBudgetMB: uint64(sf.WorkerBudget),
		}, nil
	default:
		return tune, nil, nil
	}
}
