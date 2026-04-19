package stack

import (
	"fmt"
	"strings"
)

// Certbot returns the certbot stack component.
func Certbot() Component {
	return Component{
		Name: "certbot",
		InstallFn: func(r Runner) error {
			if err := r.Run("apt-get", "install", "-y", "certbot"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			// Deploy hook: reload OLS after cert renewal.
			hookDir := "/etc/letsencrypt/renewal-hooks/deploy"
			if err := r.Run("mkdir", "-p", hookDir); err != nil {
				return fmt.Errorf("create hook dir: %w", err)
			}
			if err := r.Run("sh", "-c",
				"printf '#!/bin/bash\\nsystemctl reload lsws\\n' > "+hookDir+"/reload-lsws.sh"); err != nil {
				return fmt.Errorf("write deploy hook: %w", err)
			}
			return r.Run("chmod", "+x", hookDir+"/reload-lsws.sh")
		},
		RemoveFn: func(r Runner) error {
			return r.Run("apt-get", "remove", "-y", "certbot")
		},
		PurgeFn: func(r Runner) error {
			if err := r.Run("apt-get", "purge", "-y", "certbot"); err != nil {
				return err
			}
			return r.Run("rm", "-rf", "/etc/letsencrypt")
		},
		VerifyFn: func(r Runner) error {
			out, err := r.Output("dpkg-query", "-W", "-f", "${Status}", "certbot")
			if err != nil {
				return err
			}
			if !strings.Contains(out, "install ok installed") {
				return fmt.Errorf("certbot not installed: %s", strings.TrimSpace(out))
			}
			return nil
		},
		StatusFn: func(r Runner) (string, error) {
			out, err := r.Output("certbot", "--version")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		},
	}
}
