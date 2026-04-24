// Package app contains the business logic for the gow CLI, separated from
// the cobra command wiring in cmd/gow. All side-effecting operations are
// injected via the Deps struct, making every operation testable without
// real hardware, filesystem, or network access.
package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/ols"
	"github.com/aprakasa/gow/internal/stack"
	"github.com/aprakasa/gow/internal/state"
	"github.com/aprakasa/gow/internal/system"
)

// CLIConfig holds paths and flags resolved from the CLI.
type CLIConfig struct {
	ConfDir    string
	StateFile  string
	PolicyFile string
	WebRoot    string
	LogDir     string // per-site log directory (default: /var/log/lsws)
}

// SiteFlags holds per-command flags for site operations.
type SiteFlags struct {
	SiteType       string // --type (create only: html, php, wp)
	PHP            string // --php (version string)
	Preset         string // --tune (blog, woocommerce, custom)
	PHPMemory      uint   // --php-memory
	WorkerBudget   uint   // --worker-budget
	NoCache        bool   // --no-cache (create only, wp only: disable LSCache)
	Multisite      string // --multisite (create only, wp only: subdirectory or subdomain)
	Verbose        bool   // --verbose (info only)
	NoPrompt       bool   // --no-prompt (delete only)
	Isolate        bool   // --isolate (update only)
	SSLEmail       string // --email (ssl only)
	SSLStaging     bool   // --staging (ssl only)
	SSLWildcard    bool   // --wildcard (ssl only, requires --dns)
	SSLDNS         string // --dns (ssl only, currently: "cloudflare")
	SSLHSTS        bool   // --hsts (ssl only)
	RestoreFile    string // --file (restore only)
	BackupSchedule string // --schedule (backup-schedule: daily or weekly)
	BackupRetain   int    // --retain (backup-schedule: number of backups to keep)
	Force          bool   // --force (create: delete and recreate if site already exists)
}

// StackFlags holds per-command flags for stack operations.
type StackFlags struct {
	OLS      bool
	PHP      string // --php (version string, e.g. "83", "84")
	PHP81    bool
	PHP82    bool
	PHP83    bool
	PHP84    bool
	PHP85    bool
	MariaDB  bool
	Redis    bool
	WPCLI    bool
	Composer bool
	Certbot  bool
	Target   string // --target for migrate
}

// Deps groups the side-effecting operations that app depends on.
// Production code uses DefaultDeps; tests inject mocks via this struct.
type Deps struct {
	Ctx          context.Context
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	DetectSpecs  func() (system.Specs, error)
	LoadPolicy   func(string) (allocator.Policy, error)
	OpenStore    func(string) (*state.Store, error)
	NewOLS       func() ols.Controller
	NewRunner    func() stack.Runner
	InstalledPHP func() []string
	PHPAvailable func(string) bool
	WPInstall    func(domain, webRoot, cacheMode, multisite string) error
	DBCleanup    func(domain string) error
}

// DefaultDeps returns a Deps struct wired to production implementations.
func DefaultDeps() Deps {
	d := Deps{
		Ctx:          context.Background(),
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		DetectSpecs:  system.Detect,
		LoadPolicy:   allocator.LoadPolicyFromFile,
		OpenStore:    state.Open,
		NewOLS:       func() ols.Controller { return ols.NewController(ols.DefaultBinPath) },
		NewRunner:    func() stack.Runner { return &stack.ShellRunner{} },
		InstalledPHP: detectInstalledPHP,
		PHPAvailable: phpVersionInstalled,
	}
	d.DBCleanup = func(domain string) error { return dropSiteDB(d.Ctx, domain) }
	d.WPInstall = func(domain, webRoot, cacheMode, multisite string) error {
		return installWordPress(d.Stdout, d.Ctx, domain, webRoot, cacheMode, multisite)
	}
	return d
}

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
