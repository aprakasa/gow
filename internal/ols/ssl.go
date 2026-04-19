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
// If the entry already exists in the SSL listener, it returns nil without
// modifying the file.
func AddSSLMapEntry(httpdConfPath, siteName string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)

	mapEntry := "map                      " + siteName + " " + siteName
	sslBlock := extractListenerBlock(content, "SSL")
	if sslBlock != "" && strings.Contains(sslBlock, mapEntry) {
		return nil
	}

	content, err = insertMapEntryForListener(content, "SSL", siteName)
	if err != nil {
		return err
	}

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// RemoveSSLMapEntry removes the map entry for siteName from the SSL listener
// block. If the entry is not present, it returns nil without modifying the
// file.
func RemoveSSLMapEntry(httpdConfPath, siteName string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)
	mapLine := "    map                      " + siteName + " " + siteName + "\n"

	// Find the SSL listener block and remove the map line from within it.
	header := "listener SSL {"
	idx := strings.Index(content, header)
	if idx == -1 {
		return nil
	}
	end := idx + len(header)
	depth := 1
	for i := end; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	block := content[idx:end]
	newBlock := strings.Replace(block, mapLine, "", 1)
	if newBlock == block {
		return nil
	}
	content = content[:idx] + newBlock + content[end:]

	return os.WriteFile(httpdConfPath, []byte(content), 0o644) //nolint:gosec // config file
}

// extractListenerBlock returns the text of a named listener block (from its
// header through its closing brace). Returns empty string if not found.
func extractListenerBlock(content, listenerName string) string {
	header := "listener " + listenerName + " {"
	idx := strings.Index(content, header)
	if idx == -1 {
		return ""
	}
	depth := 0
	for i := idx; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				return content[idx : i+1]
			}
		}
	}
	return content[idx:]
}

// SetSSLListenerCerts adds certFile and keyFile to the SSL listener block if
// they are not already present. OLS requires listener-level certs as the SNI
// default even when per-vhost SSL is configured.
func SetSSLListenerCerts(httpdConfPath, certPath, keyPath string) error {
	data, err := os.ReadFile(httpdConfPath) //nolint:gosec // config file, not secret
	if err != nil {
		return fmt.Errorf("ols: read httpd config: %w", err)
	}

	content := string(data)
	sslBlock := extractListenerBlock(content, "SSL")
	if sslBlock == "" {
		return nil
	}
	if strings.Contains(sslBlock, "certFile") && strings.Contains(sslBlock, "keyFile") {
		return nil
	}

	certLine := "    certFile                 " + certPath + "\n"
	keyLine := "    keyFile                  " + keyPath + "\n"

	// Insert cert/key before the closing brace of the SSL listener.
	header := "listener SSL {"
	scanner := bufio.NewScanner(strings.NewReader(content))
	var buf bytes.Buffer
	inTarget := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		trimmedRight := strings.TrimRight(trimmed, " \t")

		if !inTarget && strings.HasPrefix(trimmed, header) {
			inTarget = true
		}

		if inTarget && trimmedRight == "}" {
			buf.WriteString(certLine)
			buf.WriteString(keyLine)
			inTarget = false
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ols: scan httpd config: %w", err)
	}

	return os.WriteFile(httpdConfPath, buf.Bytes(), 0o644) //nolint:gosec // config file
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
