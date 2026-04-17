package stack

import "fmt"

// MariaDB returns the MariaDB stack component (11.4 LTS).
func MariaDB() Component {
	return Component{
		Name: "mariadb",
		InstallFn: func(r Runner) error {
			// 1. Add MariaDB repository.
			if err := r.Run("sh", "-c",
				"curl -sS https://downloads.mariadb.com/MariaDB/mariadb_repo_setup | bash -s -- --mariadb-server-version=mariadb-11.4"); err != nil {
				return fmt.Errorf("add repo: %w", err)
			}
			// 2. Install packages.
			if err := r.Run("apt-get", "install", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			// 3. Enable and start via systemd.
			if err := r.Run("systemctl", "enable", "mariadb"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run("systemctl", "start", "mariadb"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			// 4. Verify.
			return r.Run("systemctl", "is-active", "mariadb")
		},
		UninstallFn: func(r Runner) error {
			if err := r.Run("systemctl", "stop", "mariadb"); err != nil {
				return fmt.Errorf("stop service: %w", err)
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
	}
}
