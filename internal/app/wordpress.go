package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aprakasa/gow/internal/dbsql"
	"github.com/aprakasa/gow/internal/site"
	"github.com/aprakasa/gow/internal/stack"
)

// cronDir is the system cron drop-in directory. Overridden in tests.
var cronDir = "/etc/cron.d"

// writeCronFile creates /etc/cron.d/gow-<domain> with a WP cron event runner.
func writeCronFile(domain, docRoot string) error {
	path := filepath.Join(cronDir, "gow-"+domain)
	content := fmt.Sprintf(
		"*/5 * * * * root cd %s && %s cron event run --due-now --allow-root >/dev/null 2>&1\n",
		docRoot, stack.WPCLIBinPath,
	)
	if err := os.MkdirAll(cronDir, 0o755); err != nil { //nolint:gosec // cron.d must be world-readable
		return fmt.Errorf("create %s: %w", cronDir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // cron entry, not secret
		return fmt.Errorf("write cron %s: %w", domain, err)
	}
	return nil
}

// removeCronFile removes the per-site cron entry. Tolerates already-gone files.
func removeCronFile(domain string) error {
	path := filepath.Join(cronDir, "gow-"+domain)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cron %s: %w", domain, err)
	}
	return nil
}

func dropSiteDB(ctx context.Context, domain string) error {
	if err := removeCronFile(domain); err != nil {
		return err
	}
	dbName := dbsql.DBName(domain)
	qDB, err := dbsql.QuoteIdent(dbName)
	if err != nil {
		return err
	}
	qUser, err := dbsql.QuoteIdent(dbName)
	if err != nil {
		return err
	}
	r := stack.NewShellRunner()
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS %s; DROP USER IF EXISTS %s@'localhost'; FLUSH PRIVILEGES;",
		qDB, qUser,
	)
	return r.Run(ctx, "mariadb", "-e", sql)
}

func promptDefault(w io.Writer, label, def string) string {
	fmt.Fprintf(w, "  %s [%s]: ", label, def)
	var input string
	if _, err := fmt.Scanln(&input); err != nil && !errors.Is(err, io.EOF) {
		return def
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return def
	}
	return input
}

func installWordPress(w io.Writer, ctx context.Context, domain, webRoot, cacheMode, multisite string) error {
	docRoot := filepath.Join(webRoot, domain, "htdocs")
	r := stack.NewShellRunner()

	// Prompt for WP admin credentials.
	fmt.Fprintln(w, "\n  WordPress setup:")
	adminUser := promptDefault(w, "Admin username", "admin")
	if err := ValidateWPUsername(adminUser); err != nil {
		return err
	}
	adminEmail := promptDefault(w, "Admin email", "admin@"+domain)
	adminPass := promptDefault(w, "Admin password", "auto-generated")
	if adminPass == "auto-generated" {
		adminPass = dbsql.Password(16)
	}

	// Download WordPress.
	fmt.Fprint(w, "  Downloading WordPress...")
	if err := r.Run(ctx, stack.WPCLIBinPath, "core", "download", "--path="+docRoot, "--allow-root"); err != nil {
		return fmt.Errorf("wp core download: %w", err)
	}
	fmt.Fprintln(w, " OK")

	// wp core download runs as root, so extracted files are owned by root.
	// Fix ownership so PHP (running as the site's isolated user) can write.
	unixUser := site.UserName(domain)
	if err := r.Run(ctx, "chown", "-R", unixUser+":"+unixUser, docRoot); err != nil {
		return fmt.Errorf("chown docroot: %w", err)
	}
	dbName := dbsql.DBName(domain)
	dbUser := dbName
	dbPass := dbsql.Password(20)
	qDB, err := dbsql.QuoteIdent(dbName)
	if err != nil {
		return err
	}
	qUser, err := dbsql.QuoteIdent(dbUser)
	if err != nil {
		return err
	}
	qPass := dbsql.Escape(dbPass)

	fmt.Fprint(w, "  Creating database...")
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS %s; CREATE USER IF NOT EXISTS %s@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON %s.* TO %s@'localhost'; FLUSH PRIVILEGES;",
		qDB, qUser, qPass, qDB, qUser,
	)
	if err := r.Run(ctx, "mariadb", "-e", sql); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	fmt.Fprintln(w, " OK")
	if err := r.Run(ctx, stack.WPCLIBinPath, "config", "create",
		"--dbname="+dbName, "--dbuser="+dbUser, "--dbpass="+dbPass,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config create: %w", err)
	}

	// Use direct filesystem access — no FTP prompt for plugin/theme installs.
	if err := r.Run(ctx, stack.WPCLIBinPath, "config", "set", "FS_METHOD", "direct",
		"--type=constant", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config set FS_METHOD: %w", err)
	}

	// Disable WP-Cron — replaced by system crontab.
	if err := r.Run(ctx, stack.WPCLIBinPath, "config", "set", "DISABLE_WP_CRON", "true",
		"--type=constant", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config set DISABLE_WP_CRON: %w", err)
	}

	// Prevent theme/plugin file editing in the admin dashboard.
	if err := r.Run(ctx, stack.WPCLIBinPath, "config", "set", "DISALLOW_FILE_EDIT", "true",
		"--type=constant", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config set DISALLOW_FILE_EDIT: %w", err)
	}

	// Enable automatic core updates (minor + major).
	if err := r.Run(ctx, stack.WPCLIBinPath, "config", "set", "WP_AUTO_UPDATE_CORE", "true",
		"--type=constant", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config set WP_AUTO_UPDATE_CORE: %w", err)
	}

	// Add multisite constants to wp-config.php.
	if multisite != "" {
		subdomain := "false"
		if multisite == "subdomain" {
			subdomain = "true"
		}
		consts := []struct {
			name, val string
			raw       bool
		}{
			{"MULTISITE", "true", false},
			{"SUBDOMAIN_INSTALL", subdomain, false},
			{"DOMAIN_CURRENT_SITE", "'" + domain + "'", true},
			{"PATH_CURRENT_SITE", "'/'", true},
			{"SITE_ID_CURRENT_SITE", "1", false},
			{"BLOG_ID_CURRENT_SITE", "1", false},
		}
		for _, c := range consts {
			args := []string{"config", "set", c.name, c.val,
				"--type=constant", "--allow-root", "--path=" + docRoot}
			if c.raw {
				args = append(args, "--raw")
			}
			if err := r.Run(ctx, stack.WPCLIBinPath, args...); err != nil {
				return fmt.Errorf("wp config set %s: %w", c.name, err)
			}
		}
	}

	// Install WordPress (single or multisite).
	fmt.Fprint(w, "  Installing WordPress...")
	if multisite != "" {
		msArgs := []string{"core", "multisite-install",
			"--url=" + domain, "--title=" + domain,
			"--admin_user=" + adminUser, "--admin_password=" + adminPass,
			"--admin_email=" + adminEmail,
			"--allow-root", "--path=" + docRoot,
		}
		if multisite == "subdomain" {
			msArgs = append(msArgs, "--subdomains")
		}
		if err := r.Run(ctx, stack.WPCLIBinPath, msArgs...); err != nil {
			return fmt.Errorf("wp core multisite-install: %w", err)
		}
	} else {
		if err := r.Run(ctx, stack.WPCLIBinPath, "core", "install",
			"--url="+domain, "--title="+domain,
			"--admin_user="+adminUser, "--admin_password="+adminPass,
			"--admin_email="+adminEmail,
			"--allow-root", "--path="+docRoot,
		); err != nil {
			return fmt.Errorf("wp core install: %w", err)
		}
	}
	fmt.Fprintln(w, " OK")

	// Verify the admin user was actually created.
	fmt.Fprint(w, "  Verifying admin user...")
	out, err := r.Output(ctx, stack.WPCLIBinPath, "user", "get", adminUser,
		"--field=user_login", "--allow-root", "--path="+docRoot)
	if err != nil {
		return fmt.Errorf("admin user %q not found after install: %w", adminUser, err)
	}
	actualUser := strings.TrimSpace(out)
	if actualUser != adminUser {
		return fmt.Errorf("admin user mismatch: requested %q, got %q", adminUser, actualUser)
	}
	fmt.Fprintln(w, " OK")

	// Create writable .htaccess so WP can write rewrite rules without warnings.
	// OLS ignores .htaccess (rules are in vhconf.conf), but the file needs to
	// exist and be writable to suppress the wp-admin "update your .htaccess" notice.
	// Own it by the site's isolated user so PHP can write without world-writable perms.
	htaccess := filepath.Join(docRoot, ".htaccess")
	if err := r.Run(ctx, "touch", htaccess); err != nil {
		return fmt.Errorf("create .htaccess: %w", err)
	}
	unixUser = site.UserName(domain)
	if err := r.Run(ctx, "chown", unixUser+":"+unixUser, htaccess); err != nil {
		return fmt.Errorf("chown .htaccess: %w", err)
	}
	if err := r.Run(ctx, "chmod", "644", htaccess); err != nil {
		return fmt.Errorf("chmod .htaccess: %w", err)
	}

	// Remove files that expose the WordPress version.
	if err := r.Run(ctx, "rm", "-f",
		filepath.Join(docRoot, "readme.html"),
		filepath.Join(docRoot, "license.txt"),
	); err != nil {
		return fmt.Errorf("remove version files: %w", err)
	}

	if cacheMode == "lscache" {
		fmt.Fprint(w, "  Installing LiteSpeed Cache...")
		wpPluginArgs := []string{"plugin", "install", "litespeed-cache",
			"--activate", "--allow-root", "--path=" + docRoot}
		if multisite != "" {
			wpPluginArgs = append(wpPluginArgs, "--url="+domain)
		}
		if err := r.Run(ctx, stack.WPCLIBinPath, wpPluginArgs...); err != nil {
			return fmt.Errorf("install lscache: %w", err)
		}
		fmt.Fprintln(w, " OK")
		fmt.Fprint(w, "  Configuring object cache...")
		if err := configureObjectCache(ctx, r, docRoot, domain, multisite); err != nil {
			return err
		}
		fmt.Fprintln(w, " OK")
	}

	if err := writeCronFile(domain, docRoot); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n  URL:      http://%s\n", domain)
	fmt.Fprintf(w, "  Username: %s\n", adminUser)
	fmt.Fprintf(w, "  Password: %s\n", adminPass)

	return nil
}

// configureObjectCache sets up LSCache to use Redis via Unix socket as object
// cache and copies the object-cache.php drop-in.
func configureObjectCache(ctx context.Context, r stack.Runner, docRoot, domain, multisite string) error {
	// Set individual litespeed.conf.* options (LSCache v5+ format).
	opts := []struct{ key, val string }{
		{"object", "1"},
		{"object-kind", "1"},
		{"object-host", "/var/run/redis/redis.sock"},
		{"object-port", "0"},
		{"object-life", "360"},
		{"object-persistent", "1"},
		{"object-admin", "1"},
		{"object-db_id", "0"},
	}
	for _, o := range opts {
		args := []string{"option", "update", "litespeed.conf." + o.key, o.val,
			"--allow-root", "--path=" + docRoot}
		if multisite != "" {
			args = append(args, "--url="+domain)
		}
		if err := r.Run(ctx, stack.WPCLIBinPath, args...); err != nil {
			return fmt.Errorf("set litespeed.conf.%s: %w", o.key, err)
		}
	}

	// Write .litespeed_conf.dat so the object-cache.php drop-in can read
	// settings before plugins are loaded (early bootstrap).
	phpEval := `$dat = array(
    'object' => true,
    'object-kind' => true,
    'object-host' => '/var/run/redis/redis.sock',
    'object-port' => 0,
    'object-life' => 360,
    'object-persistent' => true,
    'object-admin' => true,
    'object-db_id' => 0,
);
file_put_contents(WP_CONTENT_DIR . '/.litespeed_conf.dat', wp_json_encode($dat));
`
	evalArgs := []string{"eval", phpEval,
		"--allow-root", "--path=" + docRoot}
	if multisite != "" {
		evalArgs = append(evalArgs, "--url="+domain)
	}
	if err := r.Run(ctx, stack.WPCLIBinPath, evalArgs...); err != nil {
		return fmt.Errorf("write litespeed_conf.dat: %w", err)
	}
	// Copy object-cache.php drop-in (LSCache may not auto-create via CLI).
	if err := r.Run(ctx, "cp", "-n",
		docRoot+"/wp-content/plugins/litespeed-cache/lib/object-cache.php",
		docRoot+"/wp-content/object-cache.php",
	); err != nil {
		return fmt.Errorf("copy object-cache drop-in: %w", err)
	}
	return nil
}
