package redact

import (
	"strings"
	"testing"
)

func TestGit(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"userinfo with token", "fatal: unable to access 'https://x-access-token:ghp_abc123@github.com/o/r.git/'", "fatal: unable to access 'https://***@github.com/o/r.git/'"},
		{"ssh userinfo", "ssh://user@host/p", "ssh://***@host/p"},
		{"no userinfo untouched", "https://github.com/o/r.git", "https://github.com/o/r.git"},
		{"authorization header", "Authorization: Bearer abc.def", "Authorization: ***"},
		{"basic header lower", "proxy-authorization: Basic Zm9v", "proxy-authorization: ***"},
		{"bare token", "token ghp_abcdef123456 leaked", "token *** leaked"},
		{"fine grained", "github_pat_11ABCDEF0_xyz", "***"},
		{"plain text", "nothing secret here", "nothing secret here"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Git(c.in)
			if got != c.want {
				t.Fatalf("Git(%q) = %q, want %q", c.in, got, c.want)
			}
			if strings.Contains(got, "ghp_") {
				t.Fatalf("token survived: %q", got)
			}
		})
	}
}
