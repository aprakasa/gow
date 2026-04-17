package stack

import "fmt"

// lsphpExtensions lists the PHP extensions required for WordPress.
var lsphpExtensions = []string{
	"common", "mysql", "curl", "gd", "imap",
	"mbstring", "xml", "zip", "redis", "opcache",
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
			// LiteSpeed repo already added by OLS install; just install packages.
			args := append([]string{"install", "-y"}, pkgs...)
			if err := r.Run("apt-get", args...); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			// Verify.
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
	}
}
