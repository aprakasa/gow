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

// ensurePHPInPath ensures a CLI PHP binary with the extensions required by
// WP-CLI and Composer (phar, mbstring, iconv, mysqli, xml) is available.
// LSPHP uses the litespeed SAPI which these tools reject, so we install the
// system php-cli package instead.
//
// On Ubuntu 24.04, some extensions ship inside php8.x-common/php8.x-mysql as
// .so files but without corresponding .ini entries in conf.d. We scan for any
// installed .so that lacks a conf.d ini and create one.
func ensurePHPInPath(ctx context.Context, r Runner) {
	_ = r.Run(ctx, "apt-get", "install", "-y", "php-cli", "php-mbstring", "php-xml", "php-mysql")
	// Scan for .so files in the extension dir that don't have a corresponding
	// .ini in the CLI conf.d. Ubuntu 24.04 omits ini files for bundled
	// extensions (phar, iconv) and the mysql family (mysqli, mysqlnd, pdo_mysql).
	_ = r.Run(ctx, "sh", "-c",
		`confd=$(ls -d /etc/php/*/cli/conf.d 2>/dev/null | head -1) &&
		 extdir=$(php -r 'echo ini_get("extension_dir");' 2>/dev/null) &&
		 [ -n "$confd" ] && [ -n "$extdir" ] &&
		 for so in "$extdir"/*.so; do
		     name=$(basename "$so" .so)
		     [ "$name" = "opcache" ] && continue
		     [ -f "$confd"/??-"$name".ini ] && continue
		     echo "extension=${name}.so" > "$confd/20-${name}.ini"
		 done || true`)
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
