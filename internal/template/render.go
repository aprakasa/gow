// Package template renders OpenLiteSpeed configuration files from Go templates
// embedded at compile time. The allocator produces Allocation structs; this
// package turns them into the vhconf.conf and extprocessor blocks that OLS
// reads at startup.
package template

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed tmpl/*.tmpl
var tmplFS embed.FS

// ExtAppData holds the values injected into the lsphp_extapp.conf.tmpl
// template. It is a subset of allocator.Allocation plus site metadata.
type ExtAppData struct {
	Site             string // domain used in socket path and extprocessor name
	PHPVer           string // major version string, e.g. "83"
	Children         int
	PHPMemoryLimitMB uint64
	MemSoftMB        uint64
	MemHardMB        uint64
}

// VHostData holds the values injected into the vhost.conf.tmpl template.
// The allocator-derived fields (Children, PHPMemoryLimitMB, MemSoftMB,
// MemHardMB) populate the extprocessor block so each site gets its own
// resource-isolated LSPHP cluster.
type VHostData struct {
	Site             string
	Domain           string
	Aliases          string
	WebRoot          string
	LogDir           string
	PHPVer           string
	Children         int
	PHPMemoryLimitMB uint64
	MemSoftMB        uint64
	MemHardMB        uint64
}

// RenderExtApp renders the LSPHP external-app block for a single site.
func RenderExtApp(data ExtAppData) (string, error) {
	return renderTmpl("lsphp_extapp.conf.tmpl", data)
}

// RenderVHost renders the OLS virtual-host configuration for a single site.
func RenderVHost(data VHostData) (string, error) {
	return renderTmpl("vhost.conf.tmpl", data)
}

// templateFuncs returns the custom functions available inside all templates.
var templateFuncs = template.FuncMap{
	"mul": func(a, b int) int { return a * b },
}

// renderTmpl parses the named template from the embedded filesystem and
// executes it with the given data object.
func renderTmpl(name string, data any) (string, error) {
	tmpl, err := template.New(name).Funcs(templateFuncs).ParseFS(tmplFS, "tmpl/"+name)
	if err != nil {
		return "", fmt.Errorf("template: parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("template: execute %s: %w", name, err)
	}
	return buf.String(), nil
}
