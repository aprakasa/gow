package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/aprakasa/gow/internal/stack"
)

func dropSiteDB(ctx context.Context, domain string) error {
	dbName := dbNameFromDomain(domain)
	qDB, err := quoteDBIdentifier(dbName)
	if err != nil {
		return err
	}
	qUser, err := quoteDBIdentifier(dbName)
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

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generatePassword(length int) string {
	chars := make([]byte, length)
	for i := range chars {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		chars[i] = passwordChars[n.Int64()]
	}
	return string(chars)
}

func dbNameFromDomain(domain string) string {
	return "wp_" + strings.ReplaceAll(domain, ".", "_")
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
	adminEmail := promptDefault(w, "Admin email", "admin@"+domain)
	adminPass := promptDefault(w, "Admin password", "auto-generated")
	if adminPass == "auto-generated" {
		adminPass = generatePassword(16)
	}

	// Download WordPress.
	fmt.Fprint(w, "  Downloading WordPress...")
	if err := r.Run(ctx, stack.WPCLIBinPath, "core", "download", "--path="+docRoot, "--allow-root"); err != nil {
		return fmt.Errorf("wp core download: %w", err)
	}
	fmt.Fprintln(w, " OK")
	dbName := dbNameFromDomain(domain)
	dbUser := dbName
	dbPass := generatePassword(20)
	qDB, err := quoteDBIdentifier(dbName)
	if err != nil {
		return err
	}
	qUser, err := quoteDBIdentifier(dbUser)
	if err != nil {
		return err
	}
	qPass := sqlEscapeString(dbPass)

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

	// Add multisite constants to wp-config.php.
	if multisite != "" {
		subdomain := "false"
		if multisite == "subdomain" {
			subdomain = "true"
		}
		consts := []struct{ name, val string; raw bool }{
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

	fmt.Fprintf(w, "\n  URL:      http://%s\n", domain)
	fmt.Fprintf(w, "  Username: %s\n", adminUser)
	fmt.Fprintf(w, "  Password: %s\n", adminPass)

	return nil
}

// configureObjectCache sets up LSCache to use Redis via Unix socket as object
// cache and copies the object-cache.php drop-in.
func configureObjectCache(ctx context.Context, r stack.Runner, docRoot, domain, multisite string) error {
	phpEval := `$conf = get_option('litespeed-cache-conf', array());
if (!is_array($conf)) $conf = array();
$conf['object'] = true;
$conf['object-kind'] = true;
$conf['object-host'] = '/var/run/redis/redis.sock';
$conf['object-port'] = 0;
$conf['object-life'] = 360;
$conf['object-persistent'] = true;
$conf['object-admin'] = true;
$conf['object-db_id'] = 0;
update_option('litespeed-cache-conf', $conf);
// Write .litespeed_conf.dat so the object-cache.php drop-in can read
// settings before plugins are loaded (early bootstrap).
$dat = array(
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
		return fmt.Errorf("configure object cache: %w", err)
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
