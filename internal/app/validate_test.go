package app

import "testing"

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"blog.test", false},
		{"shop.example.com", false},
		{"a.b", false},
		{"my-site.test", false},
		{"", true},
		{"nolabel", true},
		{"BLOG.test", true},
		{"blog..test", true},
		{".blog.test", true},
		{"blog.test.", true},
		{"-blog.test", true},
		{"blog-.test", true},
		{"blog.test/evil", true},
		{"blog.test;DROP", true},
		{"../evil", true},
		{"blog .test", true},
	}
	for _, tc := range tests {
		err := ValidateDomain(tc.in)
		gotErr := err != nil
		if gotErr != tc.wantErr {
			t.Errorf("ValidateDomain(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestQuoteDBIdentifier(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"wp_blog_test", false},
		{"wp_123", false},
		{"_leading", false},
		{"", true},
		{"1leading_digit", true},
		{"has-hyphen", true},
		{"has space", true},
		{"has`backtick", true},
		{"drop;table", true},
	}
	for _, tc := range tests {
		_, err := quoteDBIdentifier(tc.in)
		gotErr := err != nil
		if gotErr != tc.wantErr {
			t.Errorf("quoteDBIdentifier(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestSQLEscapeString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a'b", `a\'b`},
		{`a\b`, `a\\b`},
		{"line\n", `line\n`},
	}
	for _, tc := range tests {
		got := sqlEscapeString(tc.in)
		if got != tc.want {
			t.Errorf("sqlEscapeString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
