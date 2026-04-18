package ols

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// RegisterVHost adds a virtualHost block and listener map entry for the site
// to the OLS httpd_config.conf. If the site is already registered, it returns
// nil without modifying the file.
func RegisterVHost(httpdConfPath, siteName, vhRoot, configFile string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)
	modified := false

	// Add virtualHost block if missing.
	vhostHeader := "virtualHost " + siteName + " {"
	if !strings.Contains(content, vhostHeader) {
		block := fmt.Sprintf("\nvirtualHost %s {\n"+
			"    vhRoot                   %s\n"+
			"    allowSymbolLink          1\n"+
			"    enableScript             1\n"+
			"    restrained               1\n"+
			"    setUIDMode               2\n"+
			"    configFile               %s\n"+
			"}\n", siteName, vhRoot, configFile)
		content += block
		modified = true
	}

	// Add listener map entry if missing.
	mapEntry := "map                      " + siteName + " " + siteName
	if !strings.Contains(content, mapEntry) {
		content, err = insertMapEntry(content, siteName)
		if err != nil {
			return err
		}
		modified = true
	}

	if !modified {
		return nil
	}

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// UnregisterVHost removes the virtualHost block and listener map entry for the
// site from the OLS httpd_config.conf. If the site is not registered, it
// returns nil without modifying the file.
func UnregisterVHost(httpdConfPath, siteName string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)
	original := content

	// Remove virtualHost block.
	content = removeBlock(content, "virtualHost "+siteName)

	// Remove listener map entry.
	mapLine := "map                      " + siteName + " " + siteName + "\n"
	content = strings.Replace(content, mapLine, "", 1)

	if content == original {
		return nil
	}

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// insertMapEntry finds the first listener block and adds a map entry before
// its closing brace.
func insertMapEntry(content, siteName string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var buf bytes.Buffer
	found := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimRight(trimmed, " \t")

		// Look for closing brace of listener block.
		if found && trimmed == "}" {
			mapEntry := "    map                      " + siteName + " " + siteName + "\n"
			buf.WriteString(mapEntry)
			found = false
		}

		// Detect listener block start.
		if strings.HasPrefix(trimmed, "listener ") && strings.Contains(line, "{") {
			found = true
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("ols: scan httpd config: %w", err)
	}

	return buf.String(), nil
}

// removeBlock removes a top-level block starting with the given header prefix
// (e.g., "virtualHost blog.test") including its opening and closing braces.
func removeBlock(content, headerPrefix string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var buf bytes.Buffer
	depth := 0
	skipping := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		trimmedRight := strings.TrimRight(trimmed, " \t")

		if !skipping && strings.HasPrefix(trimmed, headerPrefix) && strings.Contains(line, "{") {
			skipping = true
			depth = 1
			continue
		}

		if skipping {
			if strings.Contains(trimmedRight, "{") {
				depth++
			}
			if trimmedRight == "}" {
				depth--
			}
			if depth == 0 {
				skipping = false
			}
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	return buf.String()
}
