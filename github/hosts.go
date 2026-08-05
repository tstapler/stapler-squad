package github

import "strings"

// defaultHost is the GitHub.com hostname used when no host is specified.
const defaultHost = "github.com"

// NormalizeHost returns the canonical form of a GitHub host: lowercased, no
// scheme, no trailing slash, and "" mapped to github.com. Hostnames are
// case-insensitive (DNS), and GHE hosts are free-text admin/user input, so
// lowercasing here keeps registration and URL-match comparisons consistent
// regardless of how a host was typed.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return defaultHost
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

// IsGitHubCom reports whether host (after normalization) is github.com.
func IsGitHubCom(host string) bool {
	return NormalizeHost(host) == defaultHost
}

// EnterpriseBaseURLOverride lets tests redirect a specific enterprise host's
// REST API base URL to an httptest.Server, mirroring the GhBaseURL seam for
// github.com. Keyed by normalized host; empty/absent falls back to the real
// GHES API path.
var EnterpriseBaseURLOverride = map[string]string{}

// RestBaseURLForHost returns the REST API base URL for host, including a
// trailing slash. For github.com this returns the existing GhBaseURL package
// var unchanged, preserving the test seam that overrides it directly.
func RestBaseURLForHost(host string) string {
	host = NormalizeHost(host)
	if host == defaultHost {
		return GhBaseURL
	}
	if override, ok := EnterpriseBaseURLOverride[host]; ok {
		return override
	}
	return "https://" + host + "/api/v3/"
}

// graphQLURLForHost returns the GraphQL endpoint URL for host.
func graphQLURLForHost(host string) string {
	host = NormalizeHost(host)
	if host == defaultHost {
		return GhBaseURL + "graphql"
	}
	return "https://" + host + "/api/graphql"
}

// webBaseURLForHost returns the web (non-API) base URL for host, no trailing slash.
func webBaseURLForHost(host string) string {
	return "https://" + NormalizeHost(host)
}
