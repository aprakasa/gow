package ols

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

const sslListenerBlock = "\nlistener SSL {\n" +
	"    address                  *:443\n" +
	"    secure                   1\n" +
	"    map                      Example *\n" +
	"}\n"

// EnsureSSLListener adds an SSL listener block to httpd_config.conf if one does
// not already exist. It is idempotent.
func EnsureSSLListener(httpdConfPath string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)

	if strings.Contains(content, "listener SSL {") {
		return nil
	}

	content += sslListenerBlock

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// AddSSLMapEntry adds a map entry for siteName inside the SSL listener block.
// If the entry already exists anywhere in the file, it returns nil without
// modifying the file.
func AddSSLMapEntry(httpdConfPath, siteName string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)

	mapEntry := "map                      " + siteName + " " + siteName
	if strings.Contains(content, mapEntry) {
		return nil
	}

	content, err = insertMapEntryForListener(content, "SSL", siteName)
	if err != nil {
		return err
	}

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// RemoveSSLMapEntry removes one occurrence of the map entry for siteName from
// the file. If the entry is not present, it returns nil without modifying the
// file.
func RemoveSSLMapEntry(httpdConfPath, siteName string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)
	original := content

	mapLine := "    map                      " + siteName + " " + siteName + "\n"
	content = strings.Replace(content, mapLine, "", 1)

	if content == original {
		return nil
	}

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// insertMapEntryForListener finds a named listener block and inserts a map
// entry before its closing brace.
func insertMapEntryForListener(content, listenerName, siteName string) (string, error) {
	header := "listener " + listenerName + " {"
	scanner := bufio.NewScanner(strings.NewReader(content))
	var buf bytes.Buffer
	inTarget := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		trimmedRight := strings.TrimRight(trimmed, " \t")

		// Detect target listener block start.
		if !inTarget && strings.HasPrefix(trimmed, header) {
			inTarget = true
		}

		// Insert map entry before closing brace of target listener.
		if inTarget && trimmedRight == "}" {
			mapEntry := "    map                      " + siteName + " " + siteName + "\n"
			buf.WriteString(mapEntry)
			inTarget = false
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("ols: scan httpd config: %w", err)
	}

	return buf.String(), nil
}
