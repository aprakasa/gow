package stack

import (
	"context"
	"fmt"
	"strings"
)

// Redis returns the Redis stack component.
func Redis() Component {
	return Component{
		Name: "redis",
		InstallFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "sh", "-c",
				"curl -fsSL https://packages.redis.io/gpg | gpg --yes --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg"); err != nil {
				return fmt.Errorf("add gpg key: %w", err)
			}
			if err := r.Run(ctx, "sh", "-c",
				"echo \"deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main\" > /etc/apt/sources.list.d/redis.list"); err != nil {
				return fmt.Errorf("add apt source: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("apt update: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "install", "-y", "redis"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			if err := r.Run(ctx, "systemctl", "enable", "redis-server"); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			if err := r.Run(ctx, "systemctl", "start", "redis-server"); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			out, err := r.Output(ctx, "redis-cli", "ping")
			if err != nil {
				return err
			}
			if !strings.HasPrefix(out, "PONG") {
				return fmt.Errorf("redis ping returned %q, want PONG", out)
			}
			return configureRedisSocket(ctx, r)
		},
		ConfigureFn: configureRedisSocket,
		UpgradeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run(ctx, "apt-get", "upgrade", "-y", "redis-server")
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "apt-get", "remove", "-y", "redis-server", "redis-tools")
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "systemctl", "stop", "redis-server"); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "purge", "-y", "redis-server", "redis-tools"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/var/lib/redis"); err != nil {
				return fmt.Errorf("remove data dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/etc/redis"); err != nil {
				return fmt.Errorf("remove config dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f", "/usr/share/keyrings/redis-archive-keyring.gpg"); err != nil {
				return fmt.Errorf("remove gpg key: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f", "/etc/apt/sources.list.d/redis.list"); err != nil {
				return fmt.Errorf("remove apt source: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run(ctx, "apt-get", "update", "-y")
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			out, err := r.Output(ctx, "dpkg-query", "-W", "-f", "${Status}", "redis-server")
			if err != nil {
				return err
			}
			if !strings.Contains(out, "install ok installed") {
				return fmt.Errorf("redis-server not installed: %s", strings.TrimSpace(out))
			}
			return nil
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, "redis-server", "--version")
			if err != nil {
				return "", err
			}
			_ = r.Run(ctx, "test", "-S", "/var/run/redis/redis.sock")
			return strings.TrimSpace(out), nil
		},
		StartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "start", "redis-server")
		},
		StopFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "stop", "redis-server")
		},
		RestartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "restart", "redis-server")
		},
		ReloadFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "restart", "redis-server")
		},
		ActiveFn: func(ctx context.Context, r Runner) error {
			out, err := r.Output(ctx, "redis-cli", "-s", "/var/run/redis/redis.sock", "ping")
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

func configureRedisSocket(ctx context.Context, r Runner) error {
	if err := r.Run(ctx, "sh", "-c",
		"sed -i 's|^# \\?unixsocket .*|unixsocket /var/run/redis/redis.sock|;s|^# \\?unixsocketperm .*|unixsocketperm 770|' /etc/redis/redis.conf"); err != nil {
		return fmt.Errorf("configure unix socket: %w", err)
	}
	// Add all site users and nobody to the redis group so PHP processes can
	// reach the socket. Site users follow the "site-<domain>" convention.
	if err := r.Run(ctx, "sh", "-c",
		"for u in $(cut -d: -f1 /etc/passwd | grep '^site-'); do usermod -aG redis \"$u\" 2>/dev/null; done"); err != nil {
		return fmt.Errorf("add site users to redis group: %w", err)
	}
	if err := r.Run(ctx, "usermod", "-aG", "redis", "nobody"); err != nil {
		return fmt.Errorf("add nobody to redis group: %w", err)
	}
	return r.Run(ctx, "systemctl", "restart", "redis-server")
}
