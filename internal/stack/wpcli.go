package stack

import (
	"fmt"
	"strings"
)

const wpcliBinPath = "/usr/local/bin/wp"

// WPCLI returns the WP-CLI stack component.
func WPCLI() Component {
	return Component{
		Name: "wpcli",
		InstallFn: func(r Runner) error {
			ensurePHPInPath(r)
			if err := r.Run("sh", "-c",
				"wget -qO "+wpcliBinPath+" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"); err != nil {
				return fmt.Errorf("download: %w", err)
			}
			return r.Run("chmod", "+x", wpcliBinPath)
		},
		UpgradeFn: func(r Runner) error {
			if err := r.Run("sh", "-c",
				"wget -qO "+wpcliBinPath+" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"); err != nil {
				return fmt.Errorf("download: %w", err)
			}
			return r.Run("chmod", "+x", wpcliBinPath)
		},
		RemoveFn: func(r Runner) error {
			return r.Run("rm", "-f", wpcliBinPath)
		},
		PurgeFn: func(r Runner) error {
			return r.Run("rm", "-f", wpcliBinPath)
		},
		VerifyFn: func(r Runner) error {
			return r.Run("test", "-x", wpcliBinPath)
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output("sh", "-c", "php "+wpcliBinPath+" --version --allow-root 2>&1 | head -1")
			if err != nil {
				return "", nil
			}
			return strings.TrimSpace(out), nil
		},
	}
}
