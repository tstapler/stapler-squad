package domain

import "strings"

// GitHubComHost is the GitHub.com hostname used when no host is specified.
const GitHubComHost = "github.com"

// NormalizeHost returns the canonical form of a GitHub host: lowercased, no
// scheme, no trailing slash, and "" mapped to github.com. Hostnames are
// case-insensitive (DNS), and GHE hosts are free-text admin/user input, so
// lowercasing here keeps registration and URL-match comparisons consistent
// regardless of how a host was typed.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return GitHubComHost
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

// IsGitHubCom reports whether host (after normalization) is github.com.
func IsGitHubCom(host string) bool {
	return NormalizeHost(host) == GitHubComHost
}
