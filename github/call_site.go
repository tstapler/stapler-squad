package github

import "context"

// githubCallSiteKey is the unexported context key for the tagged call site.
type githubCallSiteKey struct{}

// WithGitHubCallSite returns a copy of ctx tagged with callSite, the fixed,
// bounded name of the wrapper function issuing a GitHub call (e.g.
// "pr.view.graphql"). Mirrors WithGitHubCallOrigin's pattern (call_origin.go)
// but for the orthogonal "which wrapper" dimension: runGHCLICommand
// (gh_exec.go) sets this per gh-CLI call site directly on its own span, and
// githubTelemetryTransport.RoundTrip (telemetry_transport.go) reads it via
// GitHubCallSiteFrom for every native HTTP call site, so both paths populate
// plan.md's Observability Plan `call_site` label the same way.
func WithGitHubCallSite(ctx context.Context, callSite string) context.Context {
	return context.WithValue(ctx, githubCallSiteKey{}, callSite)
}

// GitHubCallSiteFrom returns the call site tagged on ctx, or "" ("unattributed")
// when none was set — an untagged native HTTP call is still counted, just not
// attributable to a specific wrapper.
func GitHubCallSiteFrom(ctx context.Context) string {
	if callSite, ok := ctx.Value(githubCallSiteKey{}).(string); ok {
		return callSite
	}
	return ""
}
