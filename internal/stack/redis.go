package stack

import (
	"fmt"
	"strings"
)

// Redis returns the Redis stack component.
func Redis() Component {
	return Component{
		Name: "redis",
		InstallFn: func(r Runner) error {
			if err := r.Run("sh", "-c",
				"curl -fsSL https://packages.redis.io/gpg | gpg --yes --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg"); err != nil {
				return fmt.Errorf("add gpg key: %w", err)
			}
			if err := r.Run("sh", "-c",
				"echo \"deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main\" > /etc/apt/sources.list.d/redis.list"); err != nil {
				return fmt.Errorf("add apt source: %w", err)
			}
			if err := r.Run("apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("apt update: %w", err)
			}
			if err := r.Run("apt-get", "install", "-y", "redis"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			if err := r.Run("systemctl", "enable", "redis-server"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run("systemctl", "start", "redis-server"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			out, err := r.Output("redis-cli", "ping")
			if err != nil {
				return err
			}
			if !strings.HasPrefix(out, "PONG") {
				return fmt.Errorf("redis ping returned %q, want PONG", out)
			}
			return nil
		},
		UpgradeFn: func(r Runner) error {
			if err := r.Run("apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run("apt-get", "upgrade", "-y", "redis-server")
		},
		RemoveFn: func(r Runner) error {
			return r.Run("apt-get", "remove", "-y", "redis-server", "redis-tools")
		},
		PurgeFn: func(r Runner) error {
			if err := r.Run("systemctl", "stop", "redis-server"); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y", "redis"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			if err := r.Run("rm", "-rf", "/var/lib/redis"); err != nil {
				return fmt.Errorf("remove data dir: %w", err)
			}
			if err := r.Run("rm", "-rf", "/etc/redis"); err != nil {
				return fmt.Errorf("remove config dir: %w", err)
			}
			if err := r.Run("rm", "-f", "/usr/share/keyrings/redis-archive-keyring.gpg"); err != nil {
				return fmt.Errorf("remove gpg key: %w", err)
			}
			if err := r.Run("rm", "-f", "/etc/apt/sources.list.d/redis.list"); err != nil {
				return fmt.Errorf("remove apt source: %w", err)
			}
			if err := r.Run("apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run("apt-get", "update", "-y")
		},
		VerifyFn: func(r Runner) error {
			out, err := r.Output("redis-cli", "ping")
			if err != nil {
				return err
			}
			if !strings.HasPrefix(out, "PONG") {
				return fmt.Errorf("redis ping returned %q, want PONG", out)
			}
			return nil
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output("redis-server", "--version")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		},
		StartFn: func(r Runner) error {
			return r.Run("systemctl", "start", "redis-server")
		},
		StopFn: func(r Runner) error {
			return r.Run("systemctl", "stop", "redis-server")
		},
		RestartFn: func(r Runner) error {
			return r.Run("systemctl", "restart", "redis-server")
		},
		ReloadFn: func(r Runner) error {
			return r.Run("systemctl", "reload", "redis-server")
		},
		ActiveFn: func(r Runner) error {
			out, err := r.Output("redis-cli", "ping")
			if err != nil {
				return err
			}
			if !strings.HasPrefix(out, "PONG") {
				return fmt.Errorf("redis not responding")
			}
			return nil
		},
	}
}
