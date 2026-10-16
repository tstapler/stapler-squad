# Requirements: Event-Driven GitHub PR Reaction (CI Failure / Review Comments)

**Source**: backlog item `1f6150ad-1eef-481a-b16a-76153b037762`. No interactive
ideation interview was run — no user is present in this session (non-interactive
SDD pipeline mode) — this doc is derived directly from the backlog item's title,
description, and codebase investigation.

## Problem

When a backlog-shipped PR fails CI or gets review feedback after its work session
has already ended (or gone idle), nothing immediately steers a session back into
fixing it. The operator currently has to notice and manually re-engage the session.
This defeats the point of the automation: a human is still the trigger for the fix
loop, not just the approver of the result. Detection today depends entirely on
`PRStatusPoller` (`session/pr_status_poller.go`) noticing a state change on its next
60s tick (`DefaultPRStatusPollerConfig.PollInterval`) — real latency between "CI
actually failed" / "reviewer actually commented" and the fix loop starting, and a
risk of missing a transient state transition between polls.

## What already exists (confirmed by reading the code)

- **`PRFixSpawner`** (`session/backlog_lifecycle_pr.go:19-22`) is a consumer-defined
  interface with one method, `AutoReopenForPRFix(ctx, itemID, fixContext) error`.
  `*BacklogService` implements it (`server/services/backlog_service_triage.go:2018`,
  `AutoReopenForPRFix`) — reopens a `pr_pending` item for rework, respects an active
  work-session guard (`notifyRespawnBlockedByActiveSession`) and a rework cap
  (`workCount`/`reworkCap`), then drives the fix through `/backlog/ship` →
  `/github:pr-ship` (local CI, code review, remote CI, merge-conflict resolution —
  see `session/backlog_lifecycle_pr.go:43-72`'s doc comment on `agentShipPrompt`).
- **`AutoReopenAfterFailedReview`** (`server/services/backlog_service_triage.go:1667`)
  is the equivalent path for a failed *internal* review verdict, with its own
  double-failure and stale-active-session guards.
- **`OneShotShipRunner`/`RunOneShotForSession`** (`session/backlog_lifecycle_pr.go:24-49`)
  lets the reopen path run `/backlog/ship` as a headless one-shot even when the
  original session's tmux process has already exited, closing the gap PR #189
  flagged as out of scope.
- **`PRStatusPoller`** (`session/pr_status_poller.go`) is the only trigger today: a
  single workspace-level ticker (default 60s, `DefaultPRStatusPollerConfig`), ETag-
  cached so unchanged PRs cost zero rate-limit quota, calling `onUpdated` when a
  session's PR priority changes. It has no push-based counterpart.
- **`GitHubWebhookHandler`** (`server/services/github_webhook_handler.go`) already
  exists at `POST /webhooks/github`, gated behind the `webhook_triggers` feature
  flag (see `project_plans/webhook-triggers/`, which built this for a *different*
  purpose: triggering new session **creation** from `push` events matched against
  `github_push`-type `Workflow` rows). It:
  - Verifies `X-Hub-Signature-256` HMAC via `readAndDecodeWebhookBody` (shared
    helper — signature verification, delivery-ID dedup, structured JSON parse).
  - Extracts `repository.full_name` + `ref` via `extractGitHubRepoAndBranch`.
  - Matches only against `github_push`-type `Workflow` rows via
    `WorkflowRepository.ListByTriggerType`.
  - Persists `TriggerFireEvent` rows for audit (`persistTriggerFireEvent`).
  - **Does not** parse or route `check_run`, `workflow_run`, `pull_request_review`,
    or `issue_comment` event payloads at all, and is not wired to `PRFixSpawner`
    in any way — confirmed via `grep -rn "check_run\|workflow_run\|pull_request_review\|issue_comment"`
    across `server/services/*.go` and `proto/`: zero matches outside this doc.
  - Route is registered at boot time only if `webhook_triggers` is enabled
    (`server/server.go`); the handler itself also re-checks the flag as defense in
    depth.
- **Public reachability** is a solved, documented problem for exactly this shape of
  requirement: `.claude/docs/slack-phase2-public-reachability.md` documents scoping
  a tunnel/reverse-proxy to forward *only* the one signed webhook path
  (`/api/hooks/slack-interactive` there) to the internet, never the whole port. The
  existing `/webhooks/github` route needs the identical treatment if a user wants to
  receive real GitHub deliveries rather than only same-host test posts.

## The gap

There is no push-based path from a GitHub CI-completion or review-feedback event
into `PRFixSpawner`. The fix loop only starts on the next `PRStatusPoller` tick.

## Goals

1. Extend the existing GitHub webhook receiver to accept `check_run` and/or
   `workflow_run` (CI completion) and `pull_request_review`/`issue_comment` (review
   feedback) event deliveries, reusing the existing HMAC-verification and
   delivery-dedup helpers (`readAndDecodeWebhookBody`) rather than duplicating them.
2. On a relevant event for a PR number/branch this instance is actively tracking
   (i.e., has a `pr_pending` backlog item associated with it), invoke the existing
   `PRFixSpawner.AutoReopenForPRFix` (or, for review-verdict-shaped feedback,
   `AutoReopenAfterFailedReview`) path directly — do not reimplement the
   reopen-and-ship logic, the active-session guard, or the rework cap.
3. Keep `PRStatusPoller` running as a lower-frequency reconciliation backstop for
   any webhook delivery that is missed, delayed, or fails signature verification —
   per this repo's stated preference (webhook-triggers requirements doc, and this
   item's own description) for "webhook with polling fallback" over a single point
   of failure. Do not remove or disable the poller.
4. Document (or implement, if in scope for this pass) the public-reachability
   story for this endpoint, following the pattern already established for Slack
   Phase 2 (`.claude/docs/slack-phase2-public-reachability.md`): a path-scoped
   tunnel/reverse-proxy, not a whole-port tunnel.
5. Gate the new event-type handling behind the same (or a sibling) feature flag so
   it ships dark and is opt-in, consistent with `webhook_triggers`.

## Success Metrics

Derived entirely from data this plan already produces — `TriggerFireEvent` rows
with `outcome` in {`rejected`, `no_match`, `fired_success`, `fired_failed`}, and
the first-verified-delivery log line added to plan.md's Risk Control section
(Task 2.3.2b) — no new instrumentation required:

1. **Webhook vs. poller trigger share**: % of PR-fix reconciliations triggered
   by a webhook delivery (`TriggerFireEvent.WorkflowID IS NULL` rows with
   `outcome: "fired_success"`) vs. by `PRStatusPoller`'s tick (visible only in
   `reconcilePRPendingItem`'s existing logs, since the poller path writes no
   `TriggerFireEvent` row). A rising webhook share in the days after rollout is
   the direct signal the feature is doing its job instead of the poller
   silently covering for it.
2. **Time-to-reaction improvement**: for each `fired_success` row, the gap
   between the webhook delivery landing and the `AutoReopenForPRFix` call it
   triggers, compared against the poller's worst-case latency for the same
   reaction today (`PollInterval`, default 60s). Target: webhook-triggered
   reactions land in low single-digit seconds, not up to 60s.
3. **Reachability confirmation**: whether the Task 2.3.2b first-verified-
   delivery log line has ever appeared for each of the 4 new event types, per
   repo with `pr_event_webhooks` enabled. A repo enabled for more than a day
   with zero such log lines signals the tunnel/reverse-proxy setup (Epic 3.2)
   was never completed — not that nothing happened.

## Non-Goals (this pass)

- Rebuilding or redesigning `PRFixSpawner`/`AutoReopenForPRFix`/
  `AutoReopenAfterFailedReview` — this item wires a new *trigger* into them, it does
  not change their reopen/fix-loop behavior.
- General-purpose webhook receiver framework beyond GitHub PR-lifecycle events
  (already scoped as a non-goal in `project_plans/webhook-triggers/requirements.md`
  for the sibling `push`-triggered-session-creation feature).
- Building new outbound-callback or pipeline-chaining machinery (that is the
  `webhook-triggers` project's Goal 2/3, already tracked separately).
- Replacing `PRStatusPoller` outright — it stays as the fallback.
- Lowering `PRStatusPoller`'s `PollInterval` as a substitute for this feature —
  evaluated and rejected as a full replacement in `research/build-vs-buy.md` §5
  (polling faster still costs GitHub API rate-limit quota linearly with poll
  frequency × tracked-item count, with no latency floor near what a webhook
  achieves); a modest interval reduction is recommended there as an
  independent, low-cost complement, not as part of this pass's scope.

## Open Questions

- Does matching an inbound `check_run`/`workflow_run`/`pull_request_review`/
  `issue_comment` payload to a tracked `pr_pending` backlog item require a new
  repo/PR-number lookup (`BacklogItem` doesn't currently index by PR number — needs
  confirming in research), or can it reuse whatever `PRStatusPoller` already uses
  to associate a session with a PR?
- Should `AutoReopenForPRFix` (CI-failure framing) and `AutoReopenAfterFailedReview`
  (review-verdict framing) both be reachable from webhook events, or does a GitHub
  `pull_request_review`/`issue_comment` event map cleanly onto only one of them?
- Is the `webhook_triggers` flag the right gate to reuse, or does this need its own
  flag so it can ship independently of the `push`-triggered-session-creation
  feature that flag currently guards?
- Does the target deployment (per-user local instance, per
  `.claude/docs/state-isolation.md`) already have a tunnel/reverse-proxy set up for
  Slack Phase 2 that could be extended to also forward `/webhooks/github`, or does
  this need its own?
