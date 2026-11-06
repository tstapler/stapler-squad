package redact

import (
	"strings"
	"testing"
)

func TestGit(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"userinfo with token", "fatal: unable to access 'https://x-access-token:ghp_abc123@github.com/o/r.git/'", "fatal: unable to access 'https://***@github.com/o/r.git/'"},
		{"ssh userinfo", "ssh://user@host/p", "ssh://***@host/p"},
		{"password containing at and slash", "https://u:p@ss/w@github.com/o/r", "https://***@github.com/o/r"},
		{"scp style", "fatal: u:tok123@github.com:o/r.git", "fatal: ***@github.com:o/r.git"},
		{"no userinfo untouched", "https://github.com/o/r.git", "https://github.com/o/r.git"},
		{"authorization header", "Authorization: Bearer abc.def", "Authorization: ***"},
		{"basic header lower", "proxy-authorization: Basic Zm9v", "proxy-authorization: ***"},
		{"bare token", "token ghp_abcdef123456 leaked", "token *** leaked"},
		{"fine grained", "github_pat_11ABCDEF0_xyz", "***"},
		{"helper output", "protocol=https\nhost=github.com\nusername=bot\npassword=hunter2\n", "protocol=https\nhost=github.com\nusername=bot\npassword=***\n"},
		{"inline password", "fetch failed password=hunter2 for bot", "fetch failed password=*** for bot"},
		{"bare bearer", "sent Bearer abcdef123456 upstream", "sent Bearer *** upstream"},
		{"inline helper secret", "out: username=bot; password=hunter2; done", "out: username=bot; password=***; done"},
		{"token query parameter", "GET https://h/api?access_token=abc123&x=1", "GET https://h/api?access_token=***&x=1"},
		{"helper line with spaces around equals", "password = hunter two\nnext=1", "password =***\nnext=1"},
		{"inline bearer key", "sent bearer=abc123def", "sent bearer=***"},
		{"inline passwd", "passwd=hunter2 x", "passwd=*** x"},
		{"inline oauth refresh token", "out: oauth_refresh_token=r3fr35h x", "out: oauth_refresh_token=*** x"},
		{"inline authtoken", "out: authtoken=abc123 x", "out: authtoken=*** x"},
		{"inline secret", "secret=s3cr3t x", "secret=*** x"},
		{"inline token", "token=abc123 more", "token=*** more"},
		{"inline passphrase", "passphrase=correct-horse x", "passphrase=*** x"},
		{"gitlab private_token query", "https://h/api?private_token=abcd1234", "https://h/api?private_token=***"},
		{"prefixed secret key", "client_secret=abc&x=1", "client_secret=***&x=1"},
		{"refresh_token query", "?refresh_token=abc123&x=1", "?refresh_token=***&x=1"},
		{"api key", "api_key=abc123 x", "api_key=*** x"},
		{"helper oauth refresh token line", "oauth_refresh_token=r3fr35h\nhost=h", "oauth_refresh_token=***\nhost=h"},
		{"helper authtoken line", "authtoken=abc\nhost=h", "authtoken=***\nhost=h"},
		{"proxy authorization mixed case", "Proxy-Authorization: Basic Zm9v", "Proxy-Authorization: ***"},
		{"gitlab pat", "using glpat-abcDEF123_xyz456 now", "using *** now"},
		{"spaces around equals", "remote: token = abc123 done", "remote: token = *** done"},
		{"double quoted value with spaces", `token="abc def" x`, `token=*** x`},
		{"single quoted value", "secret='a b' x", "secret=*** x"},
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
