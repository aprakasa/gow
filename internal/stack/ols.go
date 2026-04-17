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
		UninstallFn: func(r Runner) error {
			if err := r.Run("apt-get", "purge", "-y", "openlitespeed"); err != nil {
				return fmt.Errorf("purge package: %w", err)
			}
			return r.Run("apt-get", "autoremove", "-y")
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
	}
}
