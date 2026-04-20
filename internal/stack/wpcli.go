package stack

import (
	"context"
	"fmt"
	"strings"
)

// WPCLIBinPath is the on-disk location of the wp-cli binary.
const WPCLIBinPath = "/usr/local/bin/wp"

// WPCLI returns the WP-CLI stack component.
func WPCLI() Component {
	return Component{
		Name: "wpcli",
		InstallFn: func(ctx context.Context, r Runner) error {
			ensurePHPInPath(ctx, r)
			if err := r.Run(ctx, "sh", "-c",
				"wget -qO "+WPCLIBinPath+" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"); err != nil {
				return fmt.Errorf("download: %w", err)
			}
			return r.Run(ctx, "chmod", "+x", WPCLIBinPath)
		},
		UpgradeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "sh", "-c",
				"wget -qO "+WPCLIBinPath+" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"); err != nil {
				return fmt.Errorf("download: %w", err)
			}
			return r.Run(ctx, "chmod", "+x", WPCLIBinPath)
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "rm", "-f", WPCLIBinPath)
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "rm", "-f", WPCLIBinPath)
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "test", "-x", WPCLIBinPath)
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, "sh", "-c", "php "+WPCLIBinPath+" --version --allow-root 2>&1 | head -1")
			if err != nil {
				return "", nil
			}
			return strings.TrimSpace(out), nil
		},
	}
}
