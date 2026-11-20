package mcp

import "testing"

func TestSessionUIURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, base, id, want string }{
		{"workspace app url", "https://8543--main--ws--me.apps.example.com", "fix bug/1", "https://8543--main--ws--me.apps.example.com/?session=fix+bug%2F1"},
		{"trailing slash and path dropped", "http://localhost:8543/ignored/", "a", "http://localhost:8543/?session=a"},
		{"empty base", "", "a", ""},
		{"empty id", "http://localhost:8543", "", ""},
		{"non-http scheme", "javascript:alert(1)", "a", ""},
		{"no host", "https://", "a", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sessionUIURL(tc.base, tc.id); got != tc.want {
				t.Fatalf("sessionUIURL(%q, %q) = %q, want %q", tc.base, tc.id, got, tc.want)
			}
		})
	}
}
