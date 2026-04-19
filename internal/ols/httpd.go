package ols

// Public functions in this file are thin wrappers around the httpdConf helper
// in conf.go. Each opens the file, performs one mutation, and saves. For
// multi-edit flows (e.g. reconcile) the caller should batch via openConf
// directly to avoid redundant read/write cycles.

// RegisterVHost adds a virtualHost block and Default-listener map entry for
// the site. Idempotent.
func RegisterVHost(httpdConfPath, siteName, vhRoot, configFile string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.ensureVHostBlock(siteName, vhRoot, configFile)
	c.ensureListenerMap("Default", siteName)
	return c.save()
}

// UnregisterVHost removes the virtualHost block and Default-listener map
// entry for the site. Idempotent.
func UnregisterVHost(httpdConfPath, siteName string) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	c.removeBlock("virtualHost " + siteName + " {")
	c.removeLineInBlock("listener Default {", vhostMapLine(siteName))
	return c.save()
}

// UpdateVHostRestrained changes the restrained value for a site's virtualHost
// block. Returns an error when the site or the restrained line is missing —
// this operation is not idempotent: callers rely on the error to detect a
// missing vhost.
func UpdateVHostRestrained(httpdConfPath, siteName string, value int) error {
	c, err := openConf(httpdConfPath)
	if err != nil {
		return err
	}
	if err := c.setVHostRestrained(siteName, value); err != nil {
		return err
	}
	return c.save()
}
