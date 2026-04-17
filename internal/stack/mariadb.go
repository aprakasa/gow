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
		UpgradeFn: func(r Runner) error {
			if err := r.Run("apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run("apt-get", "upgrade", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common")
		},
		RemoveFn: func(r Runner) error {
			return r.Run("apt-get", "remove", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common")
		},
		PurgeFn: func(r Runner) error {
			if err := r.Run("sh", "-c",
				"echo 'mariadb-server mariadb-server/remove_db boolean true' | debconf-set-selections"); err != nil {
				return fmt.Errorf("debconf seed: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("purge packages: %w", err)
			}
			if err := r.Run("rm", "-rf", "/var/lib/mysql"); err != nil {
				return fmt.Errorf("remove data dir: %w", err)
			}
			if err := r.Run("rm", "-rf", "/var/log/mysql"); err != nil {
				return fmt.Errorf("remove log dir: %w", err)
			}
			if err := r.Run("rm", "-rf", "/etc/mysql"); err != nil {
				return fmt.Errorf("remove config dir: %w", err)
			}
			if err := r.Run("rm", "-f", "/etc/apt/sources.list.d/mariadb.list"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run("rm", "-f",
				"/usr/share/keyrings/mariadb-keyring.gpg",
				"/usr/share/keyrings/mariadb.gpg",
			); err != nil {
				return fmt.Errorf("remove repo keys: %w", err)
			}
			if err := r.Run("apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run("apt-get", "update", "-y")
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
		StartFn: func(r Runner) error {
			return r.Run("systemctl", "start", "mariadb")
		},
		StopFn: func(r Runner) error {
			return r.Run("systemctl", "stop", "mariadb")
		},
		RestartFn: func(r Runner) error {
			return r.Run("systemctl", "restart", "mariadb")
		},
		ReloadFn: func(r Runner) error {
			return r.Run("systemctl", "reload", "mariadb")
		},
		ActiveFn: func(r Runner) error {
			return r.Run("systemctl", "is-active", "mariadb")
		},
		MigrateFn: func(r Runner, targetVer string) error {
			if err := r.Run("sh", "-c",
				"mariadb-dump --all-databases --routines --triggers --events > /tmp/gow-migration.sql"); err != nil {
				return fmt.Errorf("dump databases: %w", err)
			}
			if err := r.Run("systemctl", "stop", "mariadb"); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			if err := r.Run("sh", "-c",
				"echo 'mariadb-server mariadb-server/remove_db boolean true' | debconf-set-selections"); err != nil {
				return fmt.Errorf("debconf seed: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("purge old version: %w", err)
			}
			_ = r.Run("rm", "-rf", "/var/lib/mysql", "/var/log/mysql", "/etc/mysql")
			if err := r.Run("sh", "-c",
				"curl -sS https://downloads.mariadb.com/MariaDB/mariadb_repo_setup | bash -s -- --mariadb-server-version=mariadb-"+targetVer); err != nil {
				return fmt.Errorf("add repo %s: %w", targetVer, err)
			}
			if err := r.Run("apt-get", "install", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("install %s: %w", targetVer, err)
			}
			if err := r.Run("systemctl", "start", "mariadb"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			if err := r.Run("sh", "-c",
				"mariadb < /tmp/gow-migration.sql"); err != nil {
				return fmt.Errorf("import dump: %w", err)
			}
			return r.Run("rm", "-f", "/tmp/gow-migration.sql")
		},
	}
}
