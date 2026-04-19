// Package template renders OpenLiteSpeed configuration files from Go templates
// embedded at compile time. The allocator produces Allocation structs; this
// package turns them into the vhconf.conf and extprocessor blocks that OLS
// reads at startup.
package template

import (
	"bytes"
	"embed"
	"fmt"
	htmltmpl "html/template"
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

// VHostData holds the values injected into the vhost template variants.
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
	SSLEnabled       bool
	CertPath         string
	KeyPath          string
}

// Renderer lazily parses embedded templates once and caches them for reuse.
// Two caches are kept: text templates for OLS configs (where Go's HTML
// escaping would corrupt syntax), and html/template for HTML output (where
// auto-escaping is the defense against XSS in case a rendered field ever
// takes untrusted input).
type Renderer struct {
	once     sync.Once
	textTmpl *template.Template
	htmlTmpl *htmltmpl.Template
	initErr  error
}

var defaultRenderer Renderer

var templateFuncs = template.FuncMap{
	"mul": func(a, b int) int { return a * b },
}

// htmlTemplateFiles lists the html/template files — everything that ends up
// served to a browser. OLS config templates stay on text/template because
// HTML escaping would mangle the `$REQUEST_URI` and similar syntax.
var htmlTemplateFiles = map[string]bool{
	"index-html.html.tmpl":  true,
	"maintenance.html.tmpl": true,
}

func (r *Renderer) init() {
	r.once.Do(func() {
		r.textTmpl, r.initErr = template.New("").Funcs(templateFuncs).ParseFS(tmplFS, "tmpl/*.tmpl")
		if r.initErr != nil {
			return
		}
		r.htmlTmpl = htmltmpl.New("")
		for name := range htmlTemplateFiles {
			data, err := tmplFS.ReadFile("tmpl/" + name)
			if err != nil {
				r.initErr = fmt.Errorf("read %s: %w", name, err)
				return
			}
			if _, err := r.htmlTmpl.New(name).Parse(string(data)); err != nil {
				r.initErr = fmt.Errorf("parse %s: %w", name, err)
				return
			}
		}
	})
}

func (r *Renderer) render(name string, data any) (string, error) {
	r.init()
	if r.initErr != nil {
		return "", fmt.Errorf("template: parse %s: %w", name, r.initErr)
	}
	var buf bytes.Buffer
	if htmlTemplateFiles[name] {
		if err := r.htmlTmpl.ExecuteTemplate(&buf, name, data); err != nil {
			return "", fmt.Errorf("template: execute %s: %w", name, err)
		}
		return buf.String(), nil
	}
	if err := r.textTmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("template: execute %s: %w", name, err)
	}
	return buf.String(), nil
}

// RenderExtApp renders the LSPHP extprocessor block for a single site.
func RenderExtApp(data ExtAppData) (string, error) {
	return defaultRenderer.render("lsphp_extapp.conf.tmpl", data)
}

// RenderVHost renders the full virtual host configuration for a site.
// siteType selects the template variant: "wp", "php", or "html".
func RenderVHost(siteType string, data VHostData) (string, error) {
	tmplName := "vhost-" + siteType + ".conf.tmpl"
	return defaultRenderer.render(tmplName, data)
}

// RenderIndexPHP renders a PHP info page for PHP sites.
func RenderIndexPHP(domain string) (string, error) {
	return defaultRenderer.render("index-php.php.tmpl", struct{ Domain string }{Domain: domain})
}

// RenderIndexHTML renders a placeholder index page for HTML sites.
func RenderIndexHTML(domain string) (string, error) {
	return defaultRenderer.render("index-html.html.tmpl", struct{ Domain string }{Domain: domain})
}

// RenderMaintenance renders a static 503 maintenance page for a domain.
func RenderMaintenance(domain string) (string, error) {
	return defaultRenderer.render("maintenance.html.tmpl", struct{ Domain string }{Domain: domain})
}
