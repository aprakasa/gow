package dbsql

import "testing"

func TestQuoteIdent(t *testing.T) {
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
		_, err := QuoteIdent(tc.in)
		gotErr := err != nil
		if gotErr != tc.wantErr {
			t.Errorf("QuoteIdent(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestEscape(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a'b", `a\'b`},
		{`a\b`, `a\\b`},
		{"line\n", `line\n`},
	}
	for _, tc := range tests {
		got := Escape(tc.in)
		if got != tc.want {
			t.Errorf("Escape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDBName(t *testing.T) {
	tests := []struct {
		domain, want string
	}{
		{"blog.test", "wp_blog_test"},
		{"shop.example.com", "wp_shop_example_com"},
		{"a.b", "wp_a_b"},
	}
	for _, tc := range tests {
		if got := DBName(tc.domain); got != tc.want {
			t.Errorf("DBName(%q) = %q, want %q", tc.domain, got, tc.want)
		}
	}
}

func TestPassword(t *testing.T) {
	p := Password(20)
	if len(p) != 20 {
		t.Fatalf("Password(20) len=%d", len(p))
	}
	for _, c := range p {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			t.Fatalf("Password produced non-alphanumeric rune %q", c)
		}
	}
	// Very low probability of collision for a 20-char alphanumeric.
	if Password(20) == p {
		t.Fatalf("Password(20) returned same value twice in a row")
	}
}
