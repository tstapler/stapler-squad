package domain

import "testing"

// GHE hosts are free-text config, so a host registered or pasted in mixed case
// must compare equal to its lowercase form everywhere NormalizeHost is used.
func TestNormalizeHost_Lowercases(t *testing.T) {
	t.Parallel()
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
