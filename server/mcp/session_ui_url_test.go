package mcp

import "testing"

func TestSessionUIURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, base, id, want string }{
		{"workspace app url", "https://8543--main--ws--me.apps.example.com", "fix bug/1", "https://8543--main--ws--me.apps.example.com/?session=fix+bug%2F1"},
		{"path prefix kept", "http://localhost:8543/ssq/", "a", "http://localhost:8543/ssq/?session=a"},
		{"path prefix without slash", "https://gw.example.com/ssq", "a", "https://gw.example.com/ssq/?session=a"},
		{"query and fragment dropped", "https://gw.example.com/?x=1#f", "a", "https://gw.example.com/?session=a"},
		{"userinfo rejected", "https://admin:s3cret@gw.example.com", "a", ""},
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
