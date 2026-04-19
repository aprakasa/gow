package ols

import "strings"

// EnsureSSLListener adds an SSL listener block to httpd_config.conf if one
// does not already exist. Idempotent.
func EnsureSSLListener(httpdConfPath string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.ensureSSLListener()
	return c.save()
}

// AddSSLMapEntry adds a map entry for siteName inside the SSL listener block.
// Idempotent.
func AddSSLMapEntry(httpdConfPath, siteName string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.ensureListenerMap("SSL", siteName)
	return c.save()
}

// RemoveSSLMapEntry removes the map entry for siteName from the SSL listener
// block. Idempotent.
func RemoveSSLMapEntry(httpdConfPath, siteName string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.removeLineInBlock("listener SSL {", vhostMapLine(siteName))
	return c.save()
}

// SetSSLListenerCerts adds certFile and keyFile to the SSL listener block if
// they are not already present. OLS requires listener-level certs as the SNI
// default even when per-vhost SSL is configured.
func SetSSLListenerCerts(httpdConfPath, certPath, keyPath string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.setSSLListenerCerts(certPath, keyPath)
	return c.save()
}

// SetVHostSSL adds a vhssl block with certFile and keyFile inside a
// virtualHost block. OLS reads SSL certs from the virtualHost block for SNI,
// not from the included vhconf.conf. Idempotent.
func SetVHostSSL(httpdConfPath, siteName, certPath, keyPath string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.setVHostSSL(siteName, certPath, keyPath)
	return c.save()
}

// extractListenerBlock returns the text of a named listener block (from its
// header through its closing brace). Kept here as a test helper used by
// ssl_test.go; production code should use httpdConf.findBlock directly.
func extractListenerBlock(content, listenerName string) string {
	header := "listener " + listenerName + " {"
	idx := strings.Index(content, header)
	if idx == -1 {
		return ""
	}
	depth := 0
	for i := idx; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[idx : i+1]
			}
		}
	}
	return content[idx:]
}
