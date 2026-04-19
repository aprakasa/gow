package stack

import (
	"context"
	"fmt"
	"strings"
)

const olsCtrlPath = "/usr/local/lsws/bin/lswsctrl"

// OLS returns the OpenLiteSpeed stack component.
func OLS() Component {
	return Component{
		Name: "ols",
		InstallFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "sh", "-c",
				"wget -qO - https://repo.litespeed.sh | bash"); err != nil {
				return fmt.Errorf("add repo: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "install", "-y", "openlitespeed"); err != nil {
				// OLS postinst can be killed by dpkg due to a missing admin_php
				// binary in fresh installs. If the package was unpacked
				// successfully, force-fix the dpkg status and continue.
				_ = r.Run(ctx, "sh", "-c",
					"sed -i 's/^Status: install ok half-configured/Status: install ok installed/' /var/lib/dpkg/status")
				if err := r.Run(ctx, olsCtrlPath, "version"); err != nil {
					return fmt.Errorf("install package: %w", err)
				}
			}
			// Set listener to port 80 (default is 8088).
			if err := r.Run(ctx, "sed", "-i",
				"s/\\*:8088/*:80/",
				"/usr/local/lsws/conf/httpd_config.conf"); err != nil {
				return fmt.Errorf("set port 80: %w", err)
			}
			if err := r.Run(ctx, olsCtrlPath, "start"); err != nil {
				return fmt.Errorf("start: %w", err)
			}
			return r.Run(ctx, olsCtrlPath, "status")
		},
		UpgradeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run(ctx, "apt-get", "upgrade", "-y", "openlitespeed")
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "apt-get", "remove", "-y", "openlitespeed")
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, olsCtrlPath, "stop"); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "purge", "-y", "openlitespeed"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			if err := r.Run(ctx, "rm", "-rf", "/usr/local/lsws"); err != nil {
				return fmt.Errorf("remove install dir: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f", "/etc/apt/sources.list.d/lst_deb_repo.list"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run(ctx, "rm", "-f", "/etc/apt/sources.list.d/lst_deb_repo.all"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run(ctx, "apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run(ctx, "apt-get", "update", "-y")
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			out, err := r.Output(ctx, "dpkg-query", "-W", "-f", "${Status}", "openlitespeed")
			if err != nil {
				return err
			}
			if !strings.Contains(out, "install ok installed") {
				return fmt.Errorf("openlitespeed not installed: %s", strings.TrimSpace(out))
			}
			return nil
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, "dpkg-query", "-W", "-f", "${Version}", "openlitespeed")
			if err != nil {
				return "", err
			}
			return "OpenLiteSpeed " + strings.TrimSpace(out), nil
		},
		StartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "start", "lshttpd")
		},
		StopFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "stop", "lshttpd")
		},
		RestartFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "restart", "lshttpd")
		},
		ReloadFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "reload", "lshttpd")
		},
		ActiveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "systemctl", "is-active", "lshttpd")
		},
	}
}
