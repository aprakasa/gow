package stack

import (
	"fmt"
	"strings"
)

const composerBinPath = "/usr/local/bin/composer"

// Composer returns the Composer stack component.
func Composer() Component {
	return Component{
		Name: "composer",
		InstallFn: func(r Runner) error {
			ensurePHPInPath(r)
			if err := r.Run("sh", "-c",
				"wget -qO /tmp/composer-setup.php https://getcomposer.org/installer"); err != nil {
				return fmt.Errorf("download installer: %w", err)
			}
			if err := r.Run("sh", "-c",
				"php /tmp/composer-setup.php --install-dir=/usr/local/bin --filename=composer"); err != nil {
				return fmt.Errorf("run installer: %w", err)
			}
			return r.Run("rm", "-f", "/tmp/composer-setup.php")
		},
		UpgradeFn: func(r Runner) error {
			return r.Run(composerBinPath, "self-update")
		},
		RemoveFn: func(r Runner) error {
			return r.Run("rm", "-f", composerBinPath)
		},
		PurgeFn: func(r Runner) error {
			return r.Run("rm", "-f", composerBinPath)
		},
		VerifyFn: func(r Runner) error {
			return r.Run("test", "-x", composerBinPath)
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output(composerBinPath, "--version")
			if err != nil {
				return "", nil
			}
			return strings.TrimSpace(out), nil
		},
	}
}
