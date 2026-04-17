package stack

import "fmt"

// Redis returns the Redis stack component.
func Redis() Component {
	return Component{
		Name: "redis",
		InstallFn: func(r Runner) error {
			// 1. Add Redis GPG key.
			if err := r.Run("sh", "-c",
				"curl -fsSL https://packages.redis.io/gpg | gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg"); err != nil {
				return fmt.Errorf("add gpg key: %w", err)
			}
			// 2. Add apt source.
			if err := r.Run("sh", "-c",
				"echo 'deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main' > /etc/apt/sources.list.d/redis.list"); err != nil {
				return fmt.Errorf("add apt source: %w", err)
			}
			// 3. Update package index.
			if err := r.Run("apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("apt update: %w", err)
			}
			// 4. Install.
			if err := r.Run("apt-get", "install", "-y", "redis"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			// 5. Enable and start.
			if err := r.Run("systemctl", "enable", "redis-server"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run("systemctl", "start", "redis-server"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			// 6. Verify.
			out, err := r.Output("redis-cli", "ping")
			if err != nil {
				return err
			}
			if out != "PONG\n" {
				return fmt.Errorf("redis ping returned %q, want PONG", out)
			}
			return nil
		},
		UninstallFn: func(r Runner) error {
			if err := r.Run("systemctl", "stop", "redis-server"); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y", "redis"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			return r.Run("apt-get", "autoremove", "-y")
		},
		VerifyFn: func(r Runner) error {
			out, err := r.Output("redis-cli", "ping")
			if err != nil {
				return err
			}
			if out != "PONG\n" {
				return fmt.Errorf("redis ping returned %q, want PONG", out)
			}
			return nil
		},
	}
}
