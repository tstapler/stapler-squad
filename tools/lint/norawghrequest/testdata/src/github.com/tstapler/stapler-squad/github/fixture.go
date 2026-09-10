// Package github contains test fixtures for the norawghrequest analyzer,
// resolved via analysistest's GOPATH-style testdata overlay at exactly the
// real import path (github.com/tstapler/stapler-squad/github) so the
// analyzer's self-exemption for newGHRequestForHostWithToken — gated on that
// exact package path — activates during the test, the same way
// norawgitopen's testdata resolves go-git/go-git/v5 at its real import path.
package github

import (
	"context"
	"net/http"
)

// GhBaseURL and RestBaseURLForHost stand in for the real github.GhBaseURL()/
// RestBaseURLForHost() (github/http_client.go, github/hosts.go).
func GhBaseURL() string                     { return "https://api.github.com/" }
func RestBaseURLForHost(host string) string { return GhBaseURL() }

// BAD1: a raw http.NewRequestWithContext call to a GitHub host, outside the
// approved constructor.
func bad1(ctx context.Context) {
	req, _ := http.NewRequestWithContext(ctx, "GET", GhBaseURL()+"repos/foo/bar", nil) // want `direct call to http\.NewRequestWithContext`
	_ = req
}

// BAD2: same violation via the context-less http.NewRequest.
func bad2() {
	req, _ := http.NewRequest("GET", GhBaseURL()+"user", nil) // want `direct call to http\.NewRequest `
	_ = req
}

// GOOD1: a //nolint comment on the same line suppresses the finding.
func good1(ctx context.Context) {
	req, _ := http.NewRequestWithContext(ctx, "GET", GhBaseURL()+"repos/foo/bar", nil) //nolint:norawghrequest test fixture, not a real call
	_ = req
}

// GOOD2: the approved constructor itself — exempted by function-declaration
// containment, since it sits inside newGHRequestForHostWithToken in this
// (the real) github package.
func newGHRequestForHostWithToken(ctx context.Context, host, path string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, "GET", RestBaseURLForHost(host)+path, nil)
}

// GOOD3: a request to a non-GitHub URL is never flagged.
func notGitHub() {
	req, _ := http.NewRequest("GET", "https://example.com/", nil)
	_ = req
}
