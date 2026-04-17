package stack

import (
	"fmt"
	"strings"
)

// lsphpExtensions lists the PHP extensions required for WordPress.
// gd, mbstring, xml, zip are compiled into the base/common package.
var lsphpExtensions = []string{
	"common", "mysql", "curl", "imap", "redis", "opcache",
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

// LSPHP returns the LSPHP stack component for the given PHP version.
func LSPHP(phpVer string) Component {
	pkgs := lsphpPackages(phpVer)
	binPath := lsphpBinPath(phpVer)

	return Component{
		Name: "lsphp",
		InstallFn: func(r Runner) error {
			args := append([]string{"install", "-y"}, pkgs...)
			if err := r.Run("apt-get", args...); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			_, err := r.Output(binPath, "-v")
			return err
		},
		UninstallFn: func(r Runner) error {
			args := append([]string{"purge", "-y"}, pkgs...)
			if err := r.Run("apt-get", args...); err != nil {
				return fmt.Errorf("purge packages: %w", err)
			}
			return r.Run("apt-get", "autoremove", "-y")
		},
		VerifyFn: func(r Runner) error {
			_, err := r.Output(binPath, "-v")
			return err
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output(binPath, "-v")
			if err != nil {
				return "", err
			}
			line := strings.Split(out, "\n")[0]
			return strings.TrimSpace(line), nil
		},
	}
}
