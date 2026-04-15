// Package template renders OpenLiteSpeed configuration files from Go templates
// embedded at compile time. The allocator produces Allocation structs; this
// package turns them into the vhconf.conf and extprocessor blocks that OLS
// reads at startup.
package template

import (
	"bytes"
	"embed"
	"fmt"
	"sync"
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
	WebRoot          string
	LogDir           string
	PHPVer           string
	Children         int
	PHPMemoryLimitMB uint64
	MemSoftMB        uint64
	MemHardMB        uint64
}

// Renderer lazily parses embedded templates once and caches them for reuse.
type Renderer struct {
	once       sync.Once
	cachedTmpl *template.Template
	initErr    error
}

var defaultRenderer Renderer

var templateFuncs = template.FuncMap{
	"mul": func(a, b int) int { return a * b },
}

func (r *Renderer) render(name string, data any) (string, error) {
	r.once.Do(func() {
		r.cachedTmpl, r.initErr = template.New("").Funcs(templateFuncs).ParseFS(tmplFS, "tmpl/*.tmpl")
	})
	if r.initErr != nil {
		return "", fmt.Errorf("template: parse %s: %w", name, r.initErr)
	}
	var buf bytes.Buffer
	if err := r.cachedTmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("template: execute %s: %w", name, err)
	}
	return buf.String(), nil
}

// RenderExtApp renders the LSPHP extprocessor block for a single site.
func RenderExtApp(data ExtAppData) (string, error) {
	return defaultRenderer.render("lsphp_extapp.conf.tmpl", data)
}

// RenderVHost renders the full virtual host configuration for a site.
func RenderVHost(data VHostData) (string, error) {
	return defaultRenderer.render("vhost.conf.tmpl", data)
}
