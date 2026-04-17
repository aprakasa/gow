package stack

import (
	"fmt"
	"strings"
)

const olsCtrlPath = "/usr/local/lsws/bin/lswsctrl"

// OLS returns the OpenLiteSpeed stack component.
func OLS() Component {
	return Component{
		Name: "ols",
		InstallFn: func(r Runner) error {
			if err := r.Run("sh", "-c",
				"wget -qO - https://repo.litespeed.sh | bash"); err != nil {
				return fmt.Errorf("add repo: %w", err)
			}
			if err := r.Run("apt-get", "install", "-y", "openlitespeed"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			if err := r.Run(olsCtrlPath, "start"); err != nil {
				return fmt.Errorf("start: %w", err)
			}
			return r.Run(olsCtrlPath, "status")
		},
		UpgradeFn: func(r Runner) error {
			if err := r.Run("apt-get", "update", "-y"); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return r.Run("apt-get", "upgrade", "-y", "openlitespeed")
		},
		RemoveFn: func(r Runner) error {
			return r.Run("apt-get", "remove", "-y", "openlitespeed")
		},
		PurgeFn: func(r Runner) error {
			if err := r.Run(olsCtrlPath, "stop"); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			if err := r.Run("apt-get", "purge", "-y", "openlitespeed"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			if err := r.Run("rm", "-rf", "/usr/local/lsws"); err != nil {
				return fmt.Errorf("remove install dir: %w", err)
			}
			if err := r.Run("rm", "-f", "/etc/apt/sources.list.d/lst_deb_repo.list"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run("rm", "-f", "/etc/apt/sources.list.d/lst_deb_repo.all"); err != nil {
				return fmt.Errorf("remove repo list: %w", err)
			}
			if err := r.Run("apt-get", "autoremove", "-y"); err != nil {
				return fmt.Errorf("autoremove: %w", err)
			}
			return r.Run("apt-get", "update", "-y")
		},
		VerifyFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "status")
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output("dpkg-query", "-W", "-f", "${Version}", "openlitespeed")
			if err != nil {
				return "", err
			}
			return "OpenLiteSpeed " + strings.TrimSpace(out), nil
		},
		StartFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "start")
		},
		StopFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "stop")
		},
		RestartFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "restart")
		},
		ReloadFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "reload")
		},
		ActiveFn: func(r Runner) error {
			return r.Run(olsCtrlPath, "status")
		},
	}
}
