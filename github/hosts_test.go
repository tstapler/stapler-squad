package github

import "testing"

// TestNormalizeHost_Lowercases guards the round-2 review finding: GHE hosts
// are free-text config (unlike the hardcoded "github.com" constant), so a
// host registered or pasted in mixed case (e.g. "Github.Corp.com") must
// still compare equal to its lowercase form everywhere NormalizeHost is used
// (keychain lookups, URL-match host capture).
func TestNormalizeHost_Lowercases(t *testing.T) {
	cases := map[string]string{
		"Github.Corp.com":         "github.corp.com",
		"HTTPS://GITHUB.CORP.COM": "github.corp.com",
		"github.com":              "github.com",
		"":                        "github.com",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
