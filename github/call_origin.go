package github

import "context"

// githubCallOriginKey is the unexported context key for the tagged CallOrigin.
type githubCallOriginKey struct{}

// CallOrigin identifies which code path is making a GitHub call, so telemetry
// (Phase 1) and admission control (Phase 3) can attribute and prioritize
// traffic by source.
type CallOrigin string

const (
	// OriginInteractive is a call made synchronously on behalf of a user-facing
	// RPC (e.g. GetPRInfo, MergePR triggered from the UI).
	OriginInteractive CallOrigin = "interactive"
	// OriginPRStatusPoller is a call made by the background PR status poller.
	OriginPRStatusPoller CallOrigin = "pr_status_poller"
	// OriginWorktreePRPoller is a call made by the background worktree PR poller.
	OriginWorktreePRPoller CallOrigin = "worktree_pr_poller"
	// OriginBacklogSync is a call made by the backlog GitHub-issue sync loop.
	OriginBacklogSync CallOrigin = "backlog_sync"
	// OriginWebhookReconcile is a call made in response to a GitHub webhook
	// event reconciling local state.
	OriginWebhookReconcile CallOrigin = "webhook_reconcile"
)

// WithGitHubCallOrigin returns a copy of ctx tagged with the given CallOrigin.
func WithGitHubCallOrigin(ctx context.Context, origin CallOrigin) context.Context {
	return context.WithValue(ctx, githubCallOriginKey{}, origin)
}

// GitHubCallOriginFrom returns the CallOrigin tagged on ctx. Per ADR-002, an
// untagged context defaults to OriginPRStatusPoller (the background tier)
// rather than OriginInteractive, so an un-instrumented call path is treated
// as background traffic instead of silently getting interactive priority.
func GitHubCallOriginFrom(ctx context.Context) CallOrigin {
	if origin, ok := ctx.Value(githubCallOriginKey{}).(CallOrigin); ok {
		return origin
	}
	return OriginPRStatusPoller
}
