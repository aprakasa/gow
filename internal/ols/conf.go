// Package ols manages OpenLiteSpeed configuration and lifecycle.
package ols

import (
	"fmt"
	"os"
	"strings"
)

// httpdConf is an in-memory editable copy of httpd_config.conf. Use OpenConf
// to load, mutate via methods, then call Save to flush if anything changed.
// This lets callers batch many edits into a single read/write cycle; the
// legacy one-shot wrappers (RegisterVHost, EnsureSSLListener, …) use it
// internally to share the parsing logic.
type httpdConf struct {
	path    string
	content string
	dirty   bool
}

func openConf(path string) (*httpdConf, error) {
	data, err := os.ReadFile(path) //nolint:gosec // config file, not secret
	if err != nil {
		return nil, fmt.Errorf("ols: read httpd config: %w", err)
	}
	return &httpdConf{path: path, content: string(data)}, nil
}

// save writes the current content back to disk if any edit has been made.
func (c *httpdConf) save() error {
	if !c.dirty {
		return nil
	}
	if err := os.WriteFile(c.path, []byte(c.content), 0o644); err != nil { //nolint:gosec // config file
		return fmt.Errorf("ols: write httpd config: %w", err)
	}
	return nil
}

// findBlock locates a top-level block by its exact opening header (e.g.
// "listener SSL {" or "virtualHost blog.test {") and returns the byte range
// [start, end) covering header through closing brace. Returns -1, -1 when
// the header is not present.
func (c *httpdConf) findBlock(header string) (int, int) {
	idx := strings.Index(c.content, header)
	if idx < 0 {
		return -1, -1
	}
	depth := 0
	for i := idx; i < len(c.content); i++ {
		switch c.content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return idx, i + 1
			}
		}
	}
	return idx, len(c.content)
}

// blockContains reports whether needle appears between the named block's
// opening and closing braces.
func (c *httpdConf) blockContains(header, needle string) bool {
	s, e := c.findBlock(header)
	if s < 0 {
		return false
	}
	return strings.Contains(c.content[s:e], needle)
}

// insertBeforeClose inserts line (which must already include its own
// indentation and trailing newline) immediately before the closing brace of
// the named block. Returns false if the block is missing; otherwise marks
// the conf dirty.
func (c *httpdConf) insertBeforeClose(header, line string) bool {
	_, end := c.findBlock(header)
	if end < 0 {
		return false
	}
	brace := end - 1
	c.content = c.content[:brace] + line + c.content[brace:]
	c.dirty = true
	return true
}

// removeLineInBlock removes the first occurrence of line from within the
// named block. Returns false when the block is missing or the line is not
// present; otherwise marks the conf dirty.
func (c *httpdConf) removeLineInBlock(header, line string) bool {
	s, e := c.findBlock(header)
	if s < 0 {
		return false
	}
	block := c.content[s:e]
	replaced := strings.Replace(block, line, "", 1)
	if replaced == block {
		return false
	}
	c.content = c.content[:s] + replaced + c.content[e:]
	c.dirty = true
	return true
}

// removeBlock removes the entire block with the given header, including its
// opening and closing braces.
func (c *httpdConf) removeBlock(header string) bool {
	s, e := c.findBlock(header)
	if s < 0 {
		return false
	}
	// Also strip one trailing newline so the file doesn't accumulate blanks
	// across register/unregister cycles.
	if e < len(c.content) && c.content[e] == '\n' {
		e++
	}
	c.content = c.content[:s] + c.content[e:]
	c.dirty = true
	return true
}

// append adds text to the end of the configuration.
func (c *httpdConf) append(text string) {
	c.content += text
	c.dirty = true
}

// --- vhost helpers ---

const vhostMapIndent = "    "

func vhostMapLine(site string) string {
	return vhostMapIndent + "map                      " + site + " " + site + "\n"
}

// ensureVHostBlock appends a virtualHost block for the site if missing.
func (c *httpdConf) ensureVHostBlock(siteName, vhRoot, configFile string) {
	header := "virtualHost " + siteName + " {"
	if s, _ := c.findBlock(header); s >= 0 {
		return
	}
	block := fmt.Sprintf("\nvirtualHost %s {\n"+
		"    vhRoot                   %s\n"+
		"    allowSymbolLink          1\n"+
		"    enableScript             1\n"+
		"    restrained               1\n"+
		"    setUIDMode               2\n"+
		"    configFile               %s\n"+
		"}\n", siteName, vhRoot, configFile)
	c.append(block)
}

// ensureListenerMap adds a map entry for site inside the named listener
// block if it is not already present. No-op when the listener is missing.
func (c *httpdConf) ensureListenerMap(listenerName, siteName string) {
	header := "listener " + listenerName + " {"
	line := vhostMapLine(siteName)
	if c.blockContains(header, strings.TrimSpace(line)) {
		return
	}
	c.insertBeforeClose(header, line)
}

// ensureSSLListener appends the SSL listener block if missing. Idempotent.
func (c *httpdConf) ensureSSLListener() {
	if strings.Contains(c.content, "listener SSL {") {
		return
	}
	c.append("\nlistener SSL {\n" +
		"    address                  *:443\n" +
		"    secure                   1\n" +
		"    map                      Example *\n" +
		"}\n")
}

// setSSLListenerCerts adds certFile and keyFile to the SSL listener block
// if not already present. No-op when the SSL listener is missing.
func (c *httpdConf) setSSLListenerCerts(certPath, keyPath string) {
	header := "listener SSL {"
	if s, _ := c.findBlock(header); s < 0 {
		return
	}
	if c.blockContains(header, "certFile") && c.blockContains(header, "keyFile") {
		return
	}
	c.insertBeforeClose(header,
		"    certFile                 "+certPath+"\n"+
			"    keyFile                  "+keyPath+"\n")
}

// setVHostRestrained rewrites the restrained line inside a site's virtualHost
// block. Returns an error when the site or the restrained line is missing.
func (c *httpdConf) setVHostRestrained(siteName string, value int) error {
	header := "virtualHost " + siteName + " {"
	s, e := c.findBlock(header)
	if s < 0 {
		return fmt.Errorf("ols: virtualHost %s not found", siteName)
	}
	block := c.content[s:e]
	// Match the whole restrained line (indentation + value + newline) to avoid
	// substring collisions with other directives.
	newLine := fmt.Sprintf("restrained               %d", value)
	replaced := ""
	for _, line := range strings.SplitAfter(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "restrained") && replaced == "" {
			replaced = "    " + newLine + "\n"
			block = strings.Replace(block, line, replaced, 1)
			break
		}
	}
	if replaced == "" {
		return fmt.Errorf("ols: restrained line not found for %s", siteName)
	}
	c.content = c.content[:s] + block + c.content[e:]
	c.dirty = true
	return nil
}

// HttpdConf is the public facade over httpdConf for callers that need to
// batch many mutations into a single read/write cycle (e.g. Reconcile).
// One-shot callers should keep using the package-level wrappers
// (RegisterVHost, AddSSLMapEntry, …).
type HttpdConf struct{ inner *httpdConf }

// OpenHttpd loads the config file for batched editing. Call Save to flush.
func OpenHttpd(path string) (*HttpdConf, error) {
	c, err := openConf(path)
	if err != nil {
		return nil, err
	}
	return &HttpdConf{inner: c}, nil
}

// Save flushes pending edits to disk (no-op if nothing changed).
func (h *HttpdConf) Save() error { return h.inner.save() }

// RegisterVHost adds the virtualHost block and default listener map entry.
func (h *HttpdConf) RegisterVHost(siteName, vhRoot, configFile string) {
	h.inner.ensureVHostBlock(siteName, vhRoot, configFile)
	h.inner.ensureListenerMap("Default", siteName)
}

// EnsureSSLListener adds the SSL listener block if missing.
func (h *HttpdConf) EnsureSSLListener() { h.inner.ensureSSLListener() }

// SetSSLListenerCerts sets the default SSL listener's cert and key paths.
func (h *HttpdConf) SetSSLListenerCerts(cert, key string) { h.inner.setSSLListenerCerts(cert, key) }

// AddSSLMapEntry adds the site to the SSL listener's virtualHost map.
func (h *HttpdConf) AddSSLMapEntry(siteName string) { h.inner.ensureListenerMap("SSL", siteName) }

// SetVHostSSL installs a vhssl block on the site's virtualHost for SNI.
func (h *HttpdConf) SetVHostSSL(siteName, cert, key string) { h.inner.setVHostSSL(siteName, cert, key) }

// setVHostSSL inserts a vhssl block into the site's virtualHost. No-op when
// the block already contains a certFile entry or when the virtualHost is
// missing.
func (c *httpdConf) setVHostSSL(siteName, certPath, keyPath string) {
	header := "virtualHost " + siteName + " {"
	if s, _ := c.findBlock(header); s < 0 {
		return
	}
	if c.blockContains(header, "certFile") {
		return
	}
	c.insertBeforeClose(header, "\n    vhssl {\n"+
		"        certFile                 "+certPath+"\n"+
		"        keyFile                  "+keyPath+"\n"+
		"        certChain                1\n"+
		"    }\n")
}
