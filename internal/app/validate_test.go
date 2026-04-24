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
