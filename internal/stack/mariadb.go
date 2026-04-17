package stack

import (
	"fmt"
	"strings"
)

// MariaDB returns the MariaDB stack component (11.4 LTS).
func MariaDB() Component {
	return Component{
		Name: "mariadb",
		InstallFn: func(r Runner) error {
			if err := r.Run("sh", "-c",
				"curl -sS https://downloads.mariadb.com/MariaDB/mariadb_repo_setup | bash -s -- --mariadb-server-version=mariadb-11.4"); err != nil {
				return fmt.Errorf("add repo: %w", err)
			}
			if err := r.Run("apt-get", "install", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			if err := r.Run("systemctl", "enable", "mariadb"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run("systemctl", "start", "mariadb"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			return r.Run("systemctl", "is-active", "mariadb")
		},
		UninstallFn: func(r Runner) error {
			// Pre-seed debconf so purge doesn't block on "remove databases?" prompt.
			if err := r.Run("sh", "-c",
				"echo 'mariadb-server mariadb-server/remove_db boolean true' | debconf-set-selections"); err != nil {
				return fmt.Errorf("debconf seed: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("purge packages: %w", err)
			}
			return r.Run("apt-get", "autoremove", "-y")
		},
		VerifyFn: func(r Runner) error {
			return r.Run("systemctl", "is-active", "mariadb")
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output("mariadb", "--version")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		},
	}
}
