# ADR-001: Self-Actor Filter for `issue_comment`/`pull_request_review` Bot-Loop Avoidance

**Status**: Accepted
**Date**: 2026-08-23

## Context

`pitfalls.md` §1 establishes that `AutoReopenForPRFix`'s own fix sessions run
`/github:pr-ship`, which pushes commits and posts PR comments
(`server/services/github_service.go:137` `PostPRComment` → `session/pr_tracking.go:71`
`Instance.PostComment` → `github/client.go:624` `github.PostPRComment`, which shells
out to `gh pr comment`). Those comments are themselves `issue_comment` webhook
deliveries on the same PR this feature listens to. Without a filter, the fix
loop can re-trigger itself on its own status-update comments.

Two candidate mechanisms exist, confirmed by research
(`research/pitfalls.md` §5, and a targeted follow-up investigation):

1. **Filter by `sender.type == "Bot"`** — cheap, zero new code beyond a field
   read, but wrong: it would also silently drop a legitimate CI bot's failure
   summary comment or a `dependabot`/reviewer-bot's actionable feedback,
   which a human would want acted on. `session/git/worktree_git.go:469-479`'s
   `isExcludedBotAuthor` already draws exactly this distinction for a
   different purpose (excluding noise bots like `codecov[bot]` from
   `HasReviewFeedback`, while carving out `copilot-pull-request-reviewer[bot]`
   as legitimate feedback) — the same "not all bots" principle applies here.
2. **Filter by self-login equality** — compare the webhook payload's
   `sender.login` (or `comment.user.login`/`review.user.login`) against this
   instance's own authenticated GitHub identity. Narrower and correct: it
   only suppresses the instance's own output, not any other bot's.

No existing mechanism in this repo answers "what GitHub login does this
instance's token authenticate as," queryable from a webhook-handling context:
`github.CheckGHAuth()` (`github/client.go:150`) only returns
success/failure, not a login. `github.GetCurrentUserLogin(ctx)`
(`github/client.go:212`) does the actual `GET /user` call and returns the
login string, and `github/user_pr_cache.go`'s `GetCachedLogin()`
(`:228`, TTL-backed via `LoginCacheTTL`, `:92`) establishes the caching
pattern already used elsewhere for the identical primitive — but neither is
wired into the webhook handler today, and `config.Config` has no stored
`GitHubUsername` field (this repo relies entirely on `gh`'s/keychain's
ambient auth state, not a persisted identity value).

## Decision

Filter `issue_comment`/`pull_request_review` events by **self-login
equality**, not by bot-type. Add a small, process-lifetime cache
(TTL-based, mirroring `user_pr_cache.go`'s existing pattern rather than
inventing a new caching idiom) around `github.GetCurrentUserLogin(ctx)`,
scoped to `GitHubWebhookHandler` (or a small helper it owns), and compare
the cached login (case-insensitively — GitHub logins are
case-insensitive) against the event's actor field before treating the event
as actionable feedback.

`check_run`/`workflow_run` events do **not** get this filter — per
`architecture.md` §4, a `success` conclusion is never actionable regardless
of actor, and the item's own status (`pr_pending` → `in_progress`) already
backstops repeat CI-completion events for the same fix cycle
(`AutoReopenForPRFix`'s `"item %s is not pr_pending"` rejection,
`server/services/backlog_service_triage.go:2027-2029`). Adding actor
filtering there would require resolving a commit SHA to its pusher — a
second API call this repo has no existing helper for — for a race the
status check already closes.

## Consequences

- One new small cache (a `sync.Mutex`/`atomic.Value`-guarded string + TTL, or
  direct reuse of `user_pr_cache.go`'s existing cache if its lifecycle fits)
  is added to the webhook-handling path. It costs at most one
  `GET /user` call per TTL window, not per webhook delivery.
- If `GetCurrentUserLogin` returns `""` (unauthenticated) or errors, the
  filter must fail **open toward filtering out nothing** (never silently
  drop a real reviewer's comment because identity lookup failed) but should
  log at `Warn` — an unauthenticated instance can't post its own comments
  either, so the loop risk this ADR addresses doesn't exist in that state.
- A future GitHub App migration (noted as out of scope in
  `research/build-vs-buy.md` §2) would get a stable App-level login for
  free; this ADR's cache becomes redundant at that point, not wrong.
