package session

import (
	"context"
	"strings"
)

// PollerInvalidationAdapter satisfies server/services.GitHubPollerInvalidator,
// translating a webhook-delivered (repoFullName, prNumber) into both
// PRStatusPoller's and WorktreePRPoller's own (owner, repo, prNumber)
// invalidation calls. Both pollers share one underlying *github.ETagCache (see
// PRStatusPoller.ETagCache's doc comment), so invalidating via both is a
// harmless double-invalidate of the same cache entry, not two cache systems
// that need to be kept in sync.
type PollerInvalidationAdapter struct {
	PRPoller       *PRStatusPoller
	WorktreePoller *WorktreePRPoller
}

// NewPollerInvalidationAdapter constructs a PollerInvalidationAdapter. Either
// poller may be nil (e.g. WorktreePRPoller is not constructed when no GitHub
// token is available at startup) — InvalidateForEvent skips a nil poller's
// call rather than panicking.
func NewPollerInvalidationAdapter(prPoller *PRStatusPoller, worktreePoller *WorktreePRPoller) *PollerInvalidationAdapter {
	return &PollerInvalidationAdapter{PRPoller: prPoller, WorktreePoller: worktreePoller}
}

// InvalidateForEvent parses repoFullName ("owner/repo") and invalidates both
// pollers' cache entries for prNumber. A malformed repoFullName (no "/", or an
// empty owner/repo) is a no-op rather than an error — the caller (the webhook
// handler) has already validated the payload it extracted this from, so this
// is purely defensive. InvalidateAndRefresh dispatches its out-of-band fetch
// tagged OriginWebhookReconcile internally; InvalidateCache makes no outbound
// GitHub call at all — so no origin tagging happens in this method itself.
func (a *PollerInvalidationAdapter) InvalidateForEvent(ctx context.Context, repoFullName string, prNumber int) {
	owner, repo, ok := strings.Cut(repoFullName, "/")
	if !ok || owner == "" || repo == "" {
		return
	}
	if a.PRPoller != nil {
		a.PRPoller.InvalidateAndRefresh(ctx, owner, repo, prNumber)
	}
	if a.WorktreePoller != nil {
		a.WorktreePoller.InvalidateCache(owner, repo, prNumber)
	}
}
