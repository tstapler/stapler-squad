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

// TestGraphQLURLForHost_HonorsEnterpriseBaseURLOverride is the regression
// test for the CI hang this fixes: graphQLURLForHost used to ignore
// EnterpriseBaseURLOverride entirely, so a test that pointed the override at
// an httptest.Server (mirroring RestBaseURLForHost's test seam) still had
// its GraphQL calls fall through to the real enterprise host.
func TestGraphQLURLForHost_HonorsEnterpriseBaseURLOverride(t *testing.T) {
	const host = "github.example.test"
	EnterpriseBaseURLOverride[host] = "http://127.0.0.1:9"
	defer delete(EnterpriseBaseURLOverride, host)

	got := graphQLURLForHost(host)
	want := "http://127.0.0.1:9/api/graphql"
	if got != want {
		t.Errorf("graphQLURLForHost(%q) = %q, want %q", host, got, want)
	}
}

// TestGraphQLURLForHost_OverrideTrailingSlashHandledLikeRest guards against
// a double slash when the override is stored with a trailing slash, which
// is how tests set it (EnterpriseBaseURLOverride[host] = ts.URL + "/") to
// match RestBaseURLForHost's existing convention.
func TestGraphQLURLForHost_OverrideTrailingSlashHandledLikeRest(t *testing.T) {
	const host = "github.example.test"
	EnterpriseBaseURLOverride[host] = "http://127.0.0.1:9/"
	defer delete(EnterpriseBaseURLOverride, host)

	got := graphQLURLForHost(host)
	want := "http://127.0.0.1:9/api/graphql"
	if got != want {
		t.Errorf("graphQLURLForHost(%q) = %q, want %q", host, got, want)
	}
}

// TestGraphQLURLForHost_NoOverride_FallsBackToRealHost confirms the
// non-overridden path (production behavior) is unchanged.
func TestGraphQLURLForHost_NoOverride_FallsBackToRealHost(t *testing.T) {
	const host = "github.example.test"
	got := graphQLURLForHost(host)
	want := "https://github.example.test/api/graphql"
	if got != want {
		t.Errorf("graphQLURLForHost(%q) = %q, want %q", host, got, want)
	}
}
