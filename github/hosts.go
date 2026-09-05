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

// enterpriseBaseURLOverride lets tests redirect a specific enterprise host's
// API traffic (both REST and GraphQL) to an httptest.Server, mirroring the
// GhBaseURL seam for github.com. Keyed by normalized host, value is the
// server root including a trailing slash; empty/absent falls back to the
// real GHES API paths. RestBaseURLForHost and graphQLURLForHost must both
// consult this map — if only one does, tests that set it get a false sense
// of isolation while the other call type still dials the real host. Guarded
// by hostConfigMu (http_client.go) — access only via
// SetEnterpriseBaseURLOverride/enterpriseBaseURLOverrideFor.
var enterpriseBaseURLOverride = map[string]string{}

// SetEnterpriseBaseURLOverride registers a per-host API override for tests
// (pass url == "" to clear it), mirroring GhBaseURL's test seam for
// github.com. host is normalized the same way RestBaseURLForHost/
// graphQLURLForHost normalize their lookup key, so callers may pass a raw
// host (with scheme, casing, or trailing slash) and still hit the override.
func SetEnterpriseBaseURLOverride(host, url string) {
	host = NormalizeHost(host)
	hostConfigMu.Lock()
	defer hostConfigMu.Unlock()
	if url == "" {
		delete(enterpriseBaseURLOverride, host)
	} else {
		enterpriseBaseURLOverride[host] = url
	}
}

func enterpriseBaseURLOverrideFor(host string) (string, bool) {
	hostConfigMu.RLock()
	defer hostConfigMu.RUnlock()
	v, ok := enterpriseBaseURLOverride[host]
	return v, ok
}

// RestBaseURLForHost returns the REST API base URL for host, including a
// trailing slash. For github.com this returns GhBaseURL unchanged, preserving
// the test seam that overrides it directly.
func RestBaseURLForHost(host string) string {
	host = NormalizeHost(host)
	if host == defaultHost {
		return GhBaseURL()
	}
	if override, ok := enterpriseBaseURLOverrideFor(host); ok {
		return override
	}
	return "https://" + host + "/api/v3/"
}

// graphQLURLForHost returns the GraphQL endpoint URL for host.
func graphQLURLForHost(host string) string {
	host = NormalizeHost(host)
	if host == defaultHost {
		return GhBaseURL() + "graphql"
	}
	if override, ok := enterpriseBaseURLOverrideFor(host); ok {
		return strings.TrimSuffix(override, "/") + "/api/graphql"
	}
	return "https://" + host + "/api/graphql"
}

// webBaseURLForHost returns the web (non-API) base URL for host, no trailing slash.
func webBaseURLForHost(host string) string {
	return "https://" + NormalizeHost(host)
}
