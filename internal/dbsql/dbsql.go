// Package dbsql holds the small helpers for generating and quoting MariaDB
// identifiers, literals, and site-derived names. These live in one place so
// every caller uses the same strict validation rules — a lenient fallback in
// one package silently mangling names while another errors out is the exact
// foot-gun this package exists to prevent.
package dbsql

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// identifier matches names safe to interpolate between MariaDB backticks:
// starts with letter or underscore, 1–64 chars, no backticks or whitespace.
var identifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)

// QuoteIdent wraps an identifier in backticks after verifying it matches a
// conservative allowlist. Callers that build SQL via fmt.Sprintf should use
// this instead of hand-quoting so an unvalidated caller cannot slip an
// injection through.
func QuoteIdent(name string) (string, error) {
	if !identifier.MatchString(name) {
		return "", fmt.Errorf("dbsql: unsafe identifier %q", name)
	}
	return "`" + name + "`", nil
}

// Escape escapes a MariaDB string literal for use between single quotes.
// Callers still supply the surrounding quotes.
func Escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		"\x00", `\0`,
		"\n", `\n`,
	)
	return r.Replace(s)
}

// DBName returns the MariaDB database name for a domain. The callers validate
// the domain via app.ValidateDomain before we get here, so the result is
// always a safe identifier.
func DBName(domain string) string {
	return "wp_" + strings.ReplaceAll(domain, ".", "_")
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Password returns a cryptographically random alphanumeric string of length n.
func Password(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		buf[i] = passwordChars[x.Int64()]
	}
	return string(buf)
}
