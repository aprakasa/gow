package app

import (
	"fmt"
	"regexp"
	"strings"
)

// domainLabel matches a single DNS label: starts and ends with an
// alphanumeric, may contain hyphens in between, length 1-63.
var domainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// wpUsernameRe matches WordPress-compatible usernames: alphanumeric, dots,
// hyphens, underscores. Same chars that WordPress's sanitize_user() preserves.
var wpUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

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

// ValidateWPUsername rejects usernames that WordPress's sanitize_user() would
// alter. Only alphanumeric, dots, hyphens, and underscores are allowed.
func ValidateWPUsername(s string) error {
	if s == "" {
		return fmt.Errorf("username is empty")
	}
	if !wpUsernameRe.MatchString(s) {
		return fmt.Errorf("username %q contains invalid characters (use letters, digits, dots, hyphens, underscores)", s)
	}
	return nil
}
