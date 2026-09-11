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

// graphQLURLForHost stands in for the real github.graphQLURLForHost
// (github/hosts.go) — a wrapped constructor that itself calls GhBaseURL one
// function-call deep, outside the URL arg's own AST subtree.
func graphQLURLForHost(host string) string { return GhBaseURL() + "graphql" }

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

// BAD3: the URL is built via a known wrapped helper (graphQLURLForHost) that
// itself calls GhBaseURL one function-call deep — outside the URL arg's own
// AST subtree, so a naive walk of the arg alone would miss it.
func bad3(ctx context.Context) {
	req, _ := http.NewRequestWithContext(ctx, "POST", graphQLURLForHost(""), nil) // want `direct call to http\.NewRequestWithContext`
	_ = req
}

// BAD4: the URL is built via a local variable assigned from GhBaseURL,
// rather than passed as a call expression directly — urlArg is a bare
// *ast.Ident with no nested CallExpr.
func bad4(ctx context.Context) {
	url := GhBaseURL() + "repos/foo/bar"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil) // want `direct call to http\.NewRequestWithContext`
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

// GOOD4: newGHGraphQLRequestForHostWithToken is also an approved constructor
// — exempted by function-declaration containment, same as GOOD2.
func newGHGraphQLRequestForHostWithToken(ctx context.Context, host string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, "POST", graphQLURLForHost(host), nil)
}

// GOOD5: a local variable assigned from a non-GitHub URL is never flagged,
// even though BAD4 proves the analyzer now resolves local variables.
func notGitHubViaVar() {
	url := "https://example.com/"
	req, _ := http.NewRequest("GET", url, nil)
	_ = req
}
