// Package stack defines installable LiteSpeed/LSPHP/MariaDB components and
// their lifecycle hooks (install, upgrade, start, status, …) used by the CLI.
package stack

import (
	"context"
	"fmt"
	"strings"
)

// Certbot returns the certbot stack component.
func Certbot() Component {
	return Component{
		Name: "certbot",
		InstallFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "install", "-y", "certbot"); err != nil {
				return fmt.Errorf("install package: %w", err)
			}
			// Deploy hook: reload OLS after cert renewal.
			hookDir := "/etc/letsencrypt/renewal-hooks/deploy"
			if err := r.Run(ctx, "mkdir", "-p", hookDir); err != nil {
				return fmt.Errorf("create hook dir: %w", err)
			}
			if err := r.Run(ctx, "sh", "-c",
				"printf '#!/bin/bash\\nsystemctl reload lsws\\n' > "+hookDir+"/reload-lsws.sh"); err != nil {
				return fmt.Errorf("write deploy hook: %w", err)
			}
			return r.Run(ctx, "chmod", "+x", hookDir+"/reload-lsws.sh")
		},
		RemoveFn: func(ctx context.Context, r Runner) error {
			return r.Run(ctx, "apt-get", "remove", "-y", "certbot")
		},
		PurgeFn: func(ctx context.Context, r Runner) error {
			if err := r.Run(ctx, "apt-get", "purge", "-y", "certbot"); err != nil {
				return err
			}
			return r.Run(ctx, "rm", "-rf", "/etc/letsencrypt")
		},
		VerifyFn: func(ctx context.Context, r Runner) error {
			out, err := r.Output(ctx, "dpkg-query", "-W", "-f", "${Status}", "certbot")
			if err != nil {
				return err
			}
			if !strings.Contains(out, "install ok installed") {
				return fmt.Errorf("certbot not installed: %s", strings.TrimSpace(out))
			}
			return nil
		},
		StatusFn: func(ctx context.Context, r Runner) (string, error) {
			out, err := r.Output(ctx, "certbot", "--version")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(out), nil
		},
	}
}
