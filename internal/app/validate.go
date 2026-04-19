package app

import (
	"fmt"
	"regexp"
	"strings"
)

// domainLabel matches a single DNS label: starts and ends with an
// alphanumeric, may contain hyphens in between, length 1-63.
var domainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateDomain rejects inputs that cannot safely be used as a site name.
// The string becomes a system user name, database name, filesystem path
// component, and OLS config identifier — anything containing shell meta,
// path separators, SQL meta, or whitespace is rejected here so downstream
// code can assume clean input.
func ValidateDomain(s string) error {
	if s == "" {
		return fmt.Errorf("domain is empty")
	}
	if len(s) > 253 {
		return fmt.Errorf("domain %q exceeds 253 characters", s)
	}
	if s != strings.ToLower(s) {
		return fmt.Errorf("domain %q must be lowercase", s)
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain %q must have at least two labels (e.g. example.com)", s)
	}
	for _, l := range labels {
		if !domainLabel.MatchString(l) {
			return fmt.Errorf("domain %q contains invalid label %q", s, l)
		}
	}
	return nil
}

// dbIdentifier matches names safe to interpolate between MariaDB backticks.
// Backticks themselves are forbidden, as are quotes, semicolons, and
// whitespace. Underscores and digits are allowed.
var dbIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)

// quoteDBIdentifier wraps an identifier in backticks after verifying it
// matches dbIdentifier. Callers that build SQL via fmt.Sprintf should use
// this instead of hand-quoting so a future unvalidated caller cannot slip
// an injection through.
func quoteDBIdentifier(name string) (string, error) {
	if !dbIdentifier.MatchString(name) {
		return "", fmt.Errorf("unsafe db identifier %q", name)
	}
	return "`" + name + "`", nil
}

// sqlEscapeString escapes a MariaDB string literal for use between single
// quotes. Callers still supply the surrounding quotes. It escapes the four
// characters MariaDB recognizes inside a single-quoted string: backslash,
// single quote, NUL, and newline.
func sqlEscapeString(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		"\x00", `\0`,
		"\n", `\n`,
	)
	return r.Replace(s)
}
