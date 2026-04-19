package app

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/aprakasa/gow/internal/stack"
)

func dropSiteDB(domain string) error {
	dbName := dbNameFromDomain(domain)
	r := stack.NewShellRunner()
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS `%s`@'localhost'; FLUSH PRIVILEGES;",
		dbName, dbName,
	)
	return r.Run("mariadb", "-e", sql)
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

func promptDefault(label, def string) string {
	fmt.Printf("  %s [%s]: ", label, def)
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)
	if input == "" {
		return def
	}
	return input
}

func installWordPress(domain, webRoot string) error {
	docRoot := filepath.Join(webRoot, domain, "htdocs")
	r := stack.NewShellRunner()

	// Prompt for WP admin credentials.
	fmt.Println("\n  WordPress setup:")
	adminUser := promptDefault("Admin username", "admin")
	adminEmail := promptDefault("Admin email", "admin@"+domain)
	adminPass := promptDefault("Admin password", "auto-generated")
	if adminPass == "auto-generated" {
		adminPass = generatePassword(16)
	}

	// Download WordPress.
	fmt.Print("  Downloading WordPress...")
	if err := r.Run(stack.WPCLIBinPath, "core", "download", "--path="+docRoot, "--allow-root"); err != nil {
		return fmt.Errorf("wp core download: %w", err)
	}
	fmt.Println(" OK")

	// Create database and dedicated user.
	dbName := dbNameFromDomain(domain)
	dbUser := dbName
	dbPass := generatePassword(20)

	fmt.Print("  Creating database...")
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; CREATE USER IF NOT EXISTS `%s`@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO `%s`@'localhost'; FLUSH PRIVILEGES;",
		dbName, dbUser, dbPass, dbName, dbUser,
	)
	if err := r.Run("mariadb", "-e", sql); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	fmt.Println(" OK")

	// Generate wp-config.php.
	if err := r.Run(stack.WPCLIBinPath, "config", "create",
		"--dbname="+dbName, "--dbuser="+dbUser, "--dbpass="+dbPass,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp config create: %w", err)
	}

	// Install WordPress.
	fmt.Print("  Installing WordPress...")
	if err := r.Run(stack.WPCLIBinPath, "core", "install",
		"--url="+domain, "--title="+domain,
		"--admin_user="+adminUser, "--admin_password="+adminPass,
		"--admin_email="+adminEmail,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("wp core install: %w", err)
	}
	fmt.Println(" OK")

	// Install and activate LiteSpeed Cache.
	fmt.Print("  Installing LiteSpeed Cache...")
	if err := r.Run(stack.WPCLIBinPath, "plugin", "install", "litespeed-cache",
		"--activate", "--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("install lscache: %w", err)
	}
	fmt.Println(" OK")

	// Configure LSCache object cache (Redis via Unix socket).
	fmt.Print("  Configuring object cache...")
	if err := configureObjectCache(r, docRoot); err != nil {
		return err
	}
	fmt.Println(" OK")

	fmt.Printf("\n  URL:      http://%s\n", domain)
	fmt.Printf("  Username: %s\n", adminUser)
	fmt.Printf("  Password: %s\n", adminPass)

	return nil
}

// configureObjectCache sets up LSCache to use Redis via Unix socket as object
// cache and copies the object-cache.php drop-in.
func configureObjectCache(r stack.Runner, docRoot string) error {
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
	if err := r.Run(stack.WPCLIBinPath, "eval", phpEval,
		"--allow-root", "--path="+docRoot,
	); err != nil {
		return fmt.Errorf("configure object cache: %w", err)
	}
	// Copy object-cache.php drop-in (LSCache may not auto-create via CLI).
	if err := r.Run("cp", "-n",
		docRoot+"/wp-content/plugins/litespeed-cache/lib/object-cache.php",
		docRoot+"/wp-content/object-cache.php",
	); err != nil {
		return fmt.Errorf("copy object-cache drop-in: %w", err)
	}
	return nil
}
