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

// TestGraphQLURLForHost is the regression test for the CI hang this fixes:
// graphQLURLForHost used to ignore EnterpriseBaseURLOverride entirely, so a
// test that pointed the override at an httptest.Server (mirroring
// RestBaseURLForHost's test seam) still had its GraphQL calls fall through
// to the real enterprise host.
func TestGraphQLURLForHost(t *testing.T) {
	const host = "github.example.test"

	tests := []struct {
		name     string
		override string // "" = no override set
		want     string
	}{
		{
			name: "no_override_falls_back_to_real_host",
			want: "https://github.example.test/api/graphql",
		},
		{
			name:     "honors_enterprise_override",
			override: "http://127.0.0.1:9",
			want:     "http://127.0.0.1:9/api/graphql",
		},
		{
			// Guards against a double slash when the override is stored with a
			// trailing slash, which is how tests set it
			// (EnterpriseBaseURLOverride[host] = ts.URL + "/") to match
			// RestBaseURLForHost's existing convention.
			name:     "override_trailing_slash_handled_like_rest",
			override: "http://127.0.0.1:9/",
			want:     "http://127.0.0.1:9/api/graphql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.override != "" {
				SetEnterpriseBaseURLOverride(host, tt.override)
				defer SetEnterpriseBaseURLOverride(host, "")
			}

			if got := graphQLURLForHost(host); got != tt.want {
				t.Errorf("graphQLURLForHost(%q) = %q, want %q", host, got, tt.want)
			}
		})
	}
}
