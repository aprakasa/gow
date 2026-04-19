package stack

import (
	"context"
	"fmt"
	"strings"
)

// lsphpExtensions lists the PHP extensions required and recommended for
// WordPress. bcmath, gd, mbstring, xml, zip are compiled into the
// base/common package and don't need separate installation.
var lsphpExtensions = []string{
	"common", "mysql", "curl", "imap", "redis", "opcache",
	"intl", "imagick", "sqlite3",
}

// lsphpPackages returns the full package list for a given PHP version.
func lsphpPackages(ver string) []string {
	base := "lsphp" + ver
	pkgs := []string{base}
	for _, ext := range lsphpExtensions {
		pkgs = append(pkgs, base+"-"+ext)
	}
	return pkgs
}

// lsphpBinPath returns the LSPHP binary path for a given version.
func lsphpBinPath(ver string) string {
	return "/usr/local/lsws/lsphp" + ver + "/bin/lsphp"
}

// ensurePHPInPath ensures a CLI PHP binary with mysqli is available for tools
// like WP-CLI and Composer. LSPHP uses the litespeed SAPI which these tools
// reject, so we install the system php-cli package instead.
func ensurePHPInPath(ctx context.Context, r Runner) {
	_ = r.Run(ctx, "apt-get", "install", "-y", "php-cli", "php-mysql")
}

// LSPHP returns the LSPHP stack component for the given PHP version.
func LSPHP(phpVer string) Component {
	pkgs := lsphpPackages(phpVer)
	binPath := lsphpBinPath(phpVer)

	return Component{
		Name: "lsphp" + phpVer,
		InstallFn: func(ctx context.Context, r Runner) error {
			args := append([]string{"install", "-y"}, pkgs...)
			if err := r.Run(ctx, "apt-get", args...); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			_, err := r.Output(ctx, binPath, "-v")
			return err
		},
		UpgradeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			args := append([]string{"upgrade", "-y"}, pkgs...)
			return r.Run(ctx, "apt-get", args...)
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			args := append([]string{"remove", "-y"}, pkgs...)
			return r.Run(ctx, "apt-get", args...)
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			args := append([]string{"purge", "-y"}, pkgs...)
			if err := r.Run(ctx, "apt-get", args...); err != nil {
				return fmt.Errorf("purge packages: %w", err)
			}
			return r.Run(ctx, "apt-get", "autoremove", "-y")
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			_, err := r.Output(ctx, binPath, "-v")
			return err
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, binPath, "-v")
			if err != nil {
				return "", err
			}
			line := strings.Split(out, "\n")[0]
			return strings.TrimSpace(line), nil
		},
	}
}
