package stack

import (
	"context"
	"fmt"
	"strings"
)

// MariaDB returns the MariaDB stack component (11.4 LTS).
func MariaDB() Component {
	return Component{
		Name: "mariadb",
		InstallFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "sh", "-c",
				"curl -sSL https://downloads.mariadb.com/MariaDB/mariadb_repo_setup | bash -s -- --mariadb-server-version=mariadb-11.4"); err != nil {
				return fmt.Errorf("add repo: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "install", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("install packages: %w", err)
			}
			if err := r.Run(ctx, "systemctl", "enable", "mariadb"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run(ctx, "systemctl", "start", "mariadb"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			return r.Run(ctx, "systemctl", "is-active", "mariadb")
		},
		UpgradeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run(ctx, "apt-get", "upgrade", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common")
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "apt-get", "remove", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common")
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "sh", "-c",
				"echo 'mariadb-server mariadb-server/remove_db boolean true' | debconf-set-selections"); err != nil {
				return fmt.Errorf("debconf seed: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "purge", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("purge packages: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/var/lib/mysql"); err != nil {
				return fmt.Errorf("remove data dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/var/log/mysql"); err != nil {
				return fmt.Errorf("remove log dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/etc/mysql"); err != nil {
				return fmt.Errorf("remove config dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f", "/etc/apt/sources.list.d/mariadb.list"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f",
				"/usr/share/keyrings/mariadb-keyring.gpg",
				"/usr/share/keyrings/mariadb.gpg",
			); err != nil {
				return fmt.Errorf("remove repo keys: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run(ctx, "apt-get", "update", "-y")
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			out, err := r.Output(ctx, "dpkg-query", "-W", "-f", "${Status}", "mariadb-server")
			if err != nil {
				return err
			}
			if !strings.Contains(out, "install ok installed") {
				return fmt.Errorf("mariadb-server not installed: %s", strings.TrimSpace(out))
			}
			return nil
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, "mariadb", "--version")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		},
		StartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "start", "mariadb")
		},
		StopFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "stop", "mariadb")
		},
		RestartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "restart", "mariadb")
		},
		ReloadFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "restart", "mariadb")
		},
		ActiveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "is-active", "mariadb")
		},
		MigrateFn: func(ctx context.Context, r Runner, targetVer string) error {
			if err := r.Run(ctx, "sh", "-c",
				"mariadb-dump --all-databases --routines --triggers --events > /tmp/gow-migration.sql"); err != nil {
				return fmt.Errorf("dump databases: %w", err)
			}
			if err := r.Run(ctx, "systemctl", "stop", "mariadb"); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			if err := r.Run(ctx, "sh", "-c",
				"echo 'mariadb-server mariadb-server/remove_db boolean true' | debconf-set-selections"); err != nil {
				return fmt.Errorf("debconf seed: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "purge", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("purge old version: %w", err)
			}
			_ = r.Run(ctx, "rm", "-rf", "/var/lib/mysql", "/var/log/mysql", "/etc/mysql")
			_ = r.Run(ctx, "mkdir", "-p", "/etc/mysql/conf.d", "/etc/mysql/mariadb.conf.d")
			if err := r.Run(ctx, "sh", "-c",
				"curl -sSL https://downloads.mariadb.com/MariaDB/mariadb_repo_setup | bash -s -- --mariadb-server-version=mariadb-"+targetVer); err != nil {
				return fmt.Errorf("add repo %s: %w", targetVer, err)
			}
			if err := r.Run(ctx, "apt-get", "install", "-y",
				"mariadb-server", "mariadb-client", "mariadb-common"); err != nil {
				return fmt.Errorf("install %s: %w", targetVer, err)
			}
			_ = r.Run(ctx, "mysql_install_db", "--user=mysql", "--datadir=/var/lib/mysql")
			if err := r.Run(ctx, "systemctl", "start", "mariadb"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			if err := r.Run(ctx, "sh", "-c",
				"mariadb < /tmp/gow-migration.sql"); err != nil {
				return fmt.Errorf("import dump: %w", err)
			}
			return r.Run(ctx, "rm", "-f", "/tmp/gow-migration.sql")
		},
	}
}
