package stack

import (
	"context"
	"fmt"
	"strings"
)

const composerBinPath = "/usr/local/bin/composer"

// Composer returns the Composer stack component.
func Composer() Component {
	return Component{
		Name: "composer",
		InstallFn: func(ctx context.Context, r Runner) error {
			ensurePHPInPath(ctx, r)
			if err := r.Run(ctx, "sh", "-c",
				"wget -qO /tmp/composer-setup.php https://getcomposer.org/installer"); err != nil {
				return fmt.Errorf("download installer: %w", err)
			}
			if err := r.Run(ctx, "sh", "-c",
				"php /tmp/composer-setup.php --install-dir=/usr/local/bin --filename=composer"); err != nil {
				return fmt.Errorf("run installer: %w", err)
			}
			return r.Run(ctx, "rm", "-f", "/tmp/composer-setup.php")
		},
		UpgradeFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, composerBinPath, "self-update")
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "rm", "-f", composerBinPath)
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "rm", "-f", composerBinPath)
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "test", "-x", composerBinPath)
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, composerBinPath, "--version")
			if err != nil {
				return "", nil
			}
			return strings.TrimSpace(out), nil
		},
	}
}
