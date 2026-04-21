// Package main is the gow CLI entrypoint. It wires cobra commands to the
// internal app package, which contains all business logic.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aprakasa/gow/internal/app"
)

func main() {
	d := app.DefaultDeps()

	rootCmd := &cobra.Command{
		Use:           "gow",
		Short:         "WordPress on OpenLiteSpeed, simplified.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var cfg app.CLIConfig
	rootCmd.PersistentFlags().StringVar(&cfg.ConfDir, "conf-dir", "/usr/local/lsws/conf", "OLS config base directory")
	rootCmd.PersistentFlags().StringVar(&cfg.StateFile, "state-file", "/etc/gow/state.json", "Site registry file")
	rootCmd.PersistentFlags().StringVar(&cfg.PolicyFile, "policy-file", "/etc/gow/policy.yaml", "Allocator policy override")
	rootCmd.PersistentFlags().StringVar(&cfg.WebRoot, "web-root", "/var/www", "Base directory for site document roots")

	// --- Site commands ---

	siteCmd := &cobra.Command{
		Use:   "site",
		Short: "Manage WordPress sites",
	}

	var sCreateFlags app.SiteFlags
	createCmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunCreate(cfg, sCreateFlags, args[0], d)
		},
	}
	createCmd.Flags().StringVar(&sCreateFlags.SiteType, "type", "wp", "Site type (html, php, wp)")
	createCmd.Flags().StringVar(&sCreateFlags.PHP, "php", "", "PHP major version (default: latest installed)")
	createCmd.Flags().StringVar(&sCreateFlags.Preset, "tune", "blog", "Tuning template (blog, woocommerce, custom)")
	createCmd.Flags().UintVar(&sCreateFlags.PHPMemory, "php-memory", 0, "PHP memory limit in MB (custom only)")
	createCmd.Flags().UintVar(&sCreateFlags.WorkerBudget, "worker-budget", 0, "Worker budget in MB (custom only)")
	createCmd.Flags().BoolVar(&sCreateFlags.NoCache, "no-cache", false, "Disable LSCache page cache + plugin (wp only)")
	createCmd.Flags().StringVar(&sCreateFlags.Multisite, "multisite", "", "Enable multisite (subdirectory or subdomain, wp only)")

	var sUpdateFlags app.SiteFlags
	updateCmd := &cobra.Command{
		Use:   "update <domain>",
		Short: "Update a site's PHP version or tuning",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunUpdate(cfg, sUpdateFlags, args[0], d)
		},
	}
	updateCmd.Flags().StringVar(&sUpdateFlags.PHP, "php", "", "PHP major version (empty = no change)")
	updateCmd.Flags().StringVar(&sUpdateFlags.Preset, "tune", "", "Tuning template (blog, woocommerce, custom)")
	updateCmd.Flags().UintVar(&sUpdateFlags.PHPMemory, "php-memory", 0, "PHP memory limit in MB (custom only)")
	updateCmd.Flags().UintVar(&sUpdateFlags.WorkerBudget, "worker-budget", 0, "Worker budget in MB (custom only)")
	updateCmd.Flags().BoolVar(&sUpdateFlags.Isolate, "isolate", false, "Isolate site with dedicated system user")

	var sInfoFlags app.SiteFlags
	infoCmd := &cobra.Command{
		Use:   "info <domain>",
		Short: "Show site details",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return app.RunInfo(cfg, sInfoFlags, args[0], c.OutOrStdout(), d)
		},
	}
	infoCmd.Flags().BoolVar(&sInfoFlags.Verbose, "verbose", false, "Show allocation details")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List managed sites",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.RunList(cfg, cmd.OutOrStdout(), d)
		},
	}

	onlineCmd := &cobra.Command{
		Use:   "online <domain>",
		Short: "Bring a site online (exit maintenance mode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunOnline(cfg, args[0], d)
		},
	}

	offlineCmd := &cobra.Command{
		Use:   "offline <domain>",
		Short: "Put a site into maintenance mode (503 page)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunOffline(cfg, args[0], d)
		},
	}

	var sDeleteFlags app.SiteFlags
	deleteCmd := &cobra.Command{
		Use:   "delete <domain>",
		Short: "Delete a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunDelete(cfg, sDeleteFlags, args[0], d)
		},
	}
	deleteCmd.Flags().BoolVar(&sDeleteFlags.NoPrompt, "no-prompt", false, "Skip confirmation prompt")

	var sSSLFlags app.SiteFlags
	sslCmd := &cobra.Command{
		Use:   "ssl <domain>",
		Short: "Enable SSL with Let's Encrypt",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunSSL(cfg, sSSLFlags, args[0], d)
		},
	}
	sslCmd.Flags().StringVar(&sSSLFlags.SSLEmail, "email", "", "Let's Encrypt registration email")
	sslCmd.Flags().BoolVar(&sSSLFlags.SSLStaging, "staging", false, "Use Let's Encrypt staging server")
	sslCmd.Flags().BoolVar(&sSSLFlags.SSLWildcard, "wildcard", false, "Issue wildcard cert (*.domain); requires --dns")
	sslCmd.Flags().StringVar(&sSSLFlags.SSLDNS, "dns", "", "DNS provider for DNS-01 (cloudflare)")
	sslCmd.Flags().BoolVar(&sSSLFlags.SSLHSTS, "hsts", false, "Emit Strict-Transport-Security header")
	_ = sslCmd.MarkFlagRequired("email")

	cloneCmd := &cobra.Command{
		Use:   "clone <src> <dest>",
		Short: "Clone a site to a new domain",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunClone(cfg, args[0], args[1], d)
		},
	}

	backupCmd := &cobra.Command{
		Use:   "backup <domain>",
		Short: "Create a backup archive of a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunBackup(cfg, args[0], d)
		},
	}

	var restoreFlags app.SiteFlags
	restoreCmd := &cobra.Command{
		Use:   "restore <domain>",
		Short: "Restore a site from a backup archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunRestore(cfg, args[0], restoreFlags.RestoreFile, d)
		},
	}
	restoreCmd.Flags().StringVar(&restoreFlags.RestoreFile, "file", "", "Path to backup archive (required)")
	_ = restoreCmd.MarkFlagRequired("file")

	flushCmd := &cobra.Command{
		Use:   "flush <domain>",
		Short: "Purge WordPress object cache and LSCache",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunFlush(cfg, args[0], d)
		},
	}

	var logFlags app.LogFlags
	logCmd := &cobra.Command{
		Use:   "log <domain>",
		Short: "Tail the site's access or error log",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.RunLog(cfg, logFlags, args[0], d)
		},
	}
	logCmd.Flags().BoolVar(&logFlags.Access, "access", false, "Tail the access log")
	logCmd.Flags().BoolVar(&logFlags.Error, "error", false, "Tail the error log (default)")
	logCmd.Flags().BoolVarP(&logFlags.Follow, "follow", "f", false, "Follow the log as it grows")
	logCmd.Flags().IntVarP(&logFlags.Lines, "lines", "n", 100, "Number of lines from the end to show")

	wpCmd := &cobra.Command{
		Use:                "wp <domain> [-- <wp-cli args>...]",
		Short:              "Run wp-cli scoped to a site's document root",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			domain := args[0]
			wpArgs := args[1:]
			if len(wpArgs) > 0 && wpArgs[0] == "--" {
				wpArgs = wpArgs[1:]
			}
			return app.RunWP(cfg, domain, wpArgs, d)
		},
	}

	siteCmd.AddCommand(createCmd, updateCmd, infoCmd, listCmd, onlineCmd, offlineCmd, deleteCmd, sslCmd, cloneCmd, backupCmd, restoreCmd, flushCmd, logCmd, wpCmd)

	// --- Stack commands ---

	stackCmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the server stack (OLS, LSPHP, MariaDB, Redis, WP-CLI, Composer)",
	}

	var installFlags app.StackFlags
	stackInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(installFlags, app.StackOpInstall, d)
		},
	}
	app.AddStackFlags(stackInstallCmd, &installFlags)

	var upgradeFlags app.StackFlags
	stackUpgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade stack components",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(upgradeFlags, app.StackOpUpgrade, d)
		},
	}
	app.AddStackFlags(stackUpgradeCmd, &upgradeFlags)

	var migrateFlags app.StackFlags
	stackMigrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate MariaDB to a new major version",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackMigrate(migrateFlags, d)
		},
	}
	app.AddStackFlags(stackMigrateCmd, &migrateFlags)
	stackMigrateCmd.Flags().StringVar(&migrateFlags.Target, "target", "", "Target MariaDB version (e.g. 11.8)")

	var removeFlags app.StackFlags
	stackRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove stack packages (keeps configs)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(removeFlags, app.StackOpRemove, d)
		},
	}
	app.AddStackFlags(stackRemoveCmd, &removeFlags)

	var purgeFlags app.StackFlags
	stackPurgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge stack packages and configs",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackPurge(cfg, purgeFlags, d)
		},
	}
	app.AddStackFlags(stackPurgeCmd, &purgeFlags)

	var startFlags app.StackFlags
	stackStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Start stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(startFlags, app.StackOpStart, d)
		},
	}
	app.AddStackFlags(stackStartCmd, &startFlags)

	var stopFlags app.StackFlags
	stackStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(stopFlags, app.StackOpStop, d)
		},
	}
	app.AddStackFlags(stackStopCmd, &stopFlags)

	var restartFlags app.StackFlags
	stackRestartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(restartFlags, app.StackOpRestart, d)
		},
	}
	app.AddStackFlags(stackRestartCmd, &restartFlags)

	var reloadFlags app.StackFlags
	stackReloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload stack services",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStackOp(reloadFlags, app.StackOpReload, d)
		},
	}
	app.AddStackFlags(stackReloadCmd, &reloadFlags)

	stackStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show stack component status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.RunStackStatus(cmd.OutOrStdout(), d)
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
			return app.RunPresets(cmd.OutOrStdout())
		},
	}

	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Recompute allocations and reload OLS",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunReconcile(cfg, d)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current allocations and resource headroom",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.RunStatus(cfg, cmd.OutOrStdout(), d)
		},
	}

	rootCmd.AddCommand(siteCmd, stackCmd, presetsCmd, reconcileCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
