# Research: Feature Landscape — pr-event-webhooks

Scope: edge cases and failure modes for routing GitHub `check_run`/`workflow_run`
(CI completion) and `pull_request_review`/`issue_comment` (review feedback) events
into `PRFixSpawner.AutoReopenForPRFix`/`AutoReopenAfterFailedReview`, **beyond**
what `project_plans/webhook-triggers/research/features.md` already covers for
`push`-triggered session creation (dedup by `X-GitHub-Delivery`, signature
verification, out-of-order events, trigger-match ambiguity, prompt-injection
defense, rate limiting). That doc's coverage is not repeated here except where
this feature's shape changes the answer.

## 1. How a `pr_pending` item associates with a GitHub PR today (Q1)

**VERIFIED**: `BacklogItem` has direct `pr_url string` + `pr_number int` fields
(`session/ent/schema/backlog_item.go:92-96`) — no join table, no dependency on
`Instance`/session GitHub state. `FindPRPendingItems` (`session/storage_backlog.go:980`)
queries `status == "pr_pending" AND pr_number > 0`.

This is a **different association axis than `PRStatusPoller` uses**. `PRStatusPoller`
(`session/pr_status_poller.go`) operates per in-memory `Instance`, reading
`inst.GitHub.GitHubOwner/GitHubRepo/GitHubPRNumber` (actor-owned snapshot fields) and
auto-discovering the PR via `CurrentBranch()` when the number isn't yet known
(`fetchAndUpdatePRStatus`, lines 285-345). The webhook path should **not** try to
reuse the poller's Instance-keyed association — it should match directly against
`BacklogItem.PrNumber` + a repo identity, same as `ReconcilePRPending` does.

**Open sub-problem confirmed real, not just a documentation gap**: `BacklogItem.RepoPath`
is a local filesystem absolute path (verified: every use in
`server/services/backlog_service_triage.go` treats it as a directory — `filepath.IsAbs`,
`os.Stat` checks at lines 2532-2538), not an `owner/repo` string. A webhook payload's
`repository.full_name` (e.g. `"tstapler/stapler-squad"`) cannot be compared to
`RepoPath` directly. The existing, directly-reusable helper for this exact conversion is
`github.GetOwnerRepoFromRemote(repoPath)` (used by `defaultPRByNumberFinder`,
`session/backlog_lifecycle_pr.go:134-143`) — resolves owner/repo from the repo's git
remote. **The webhook handler needs a new lookup**: `FindPRPendingItems()` (or a new,
more targeted `FindPRPendingItemByPRNumber(prNumber)` query, since iterating and
resolving every item's git remote on every webhook delivery is wasteful once there are
many concurrent `pr_pending` items) filtered to items whose `PrNumber` matches the
event's PR number AND whose `RepoPath`'s resolved owner/repo matches
`repository.full_name`. Neither this query nor a PR-number index exists today — confirmed
via `grep -n "PRNumberEQ\|FindByPRNumber" session/*.go server/services/*.go` → no hits.

A second existing verification layer is directly reusable here:
`verifyPRHeadBranchMatchesTracked` / `verifyPRAssociationForFixSpawn`
(`session/backlog_lifecycle_pr.go:1309`, `:1603-1626`) already re-checks, via a live
GitHub call, that a PR's head branch still matches the item's last tracked work-session
branch before trusting the association for a merge-completion or fix-spawn decision, and
prepends `unverifiedPRAssociationDisclaimer` to the fix prompt when it can't verify. The
webhook path should invoke this same guard before calling `AutoReopenForPRFix`, not skip
it just because the trigger was push-based rather than poll-based.

## 2. No matching `pr_pending` item for the event (Q2)

Concrete causes: the PR belongs to a different tool/human entirely; the item already
transitioned to `done`/`archived` (event arrives after merge — race between GitHub's
webhook delivery and this instance's own merge-detection tick); the item's PR was
detached (`BUG-040`'s `pr_number=0` clearing path, `session/backlog_lifecycle_pr.go:1441-1449`,
fires when a closed-PR fix reopen succeeds) so the number briefly matches nothing;
or the repo isn't tracked by any backlog item at all.

**Safe no-op behavior**: exactly mirrors the sibling project's `no_match` outcome for
`push` — return HTTP 200 (never treat "no matching item" as an error GitHub should
retry) and persist a `TriggerFireEvent` with `Outcome: "no_match"` via
`persistTriggerFireEvent` for audit visibility (`server/services/github_webhook_handler.go:81`
is the exact precedent). Do **not** log this above `Debug`/`Info` level in steady state —
a repo with mixed tooling (Dependabot PRs, human PRs, Copilot-authored PRs) will
generate many no-op deliveries for unrelated PR numbers, and this is expected traffic,
not an error condition.

## 3. Multiple rapid events for the same PR (Q3)

**VERIFIED — this is already solved, but at the wrong layer for a webhook to lean on
naively.** `ReconcilePRPending` never calls `AutoReopenForPRFix` directly; every call
site funnels through `remediatePRFixWithBackoffGate`
(`session/backlog_lifecycle_pr.go:1181-1222`), which:
1. Calls `MarkStuck(itemID, StuckReasonPRNeedsFix, ...)` — idempotent open-or-refresh of a
   durable stuck-state row.
2. Notifies once on first sighting (`row.NotifiedAt == nil` guard).
3. Gates the actual `AutoReopenForPRFix` call behind `Storage.RemediationDue` — a
   backoff/attempt-cap mechanism shared with every other auto-respawn path in this file
   (`autoReopenWithBackoffGate`, `remediateStaleWorkWithBackoffGate`, etc.), keyed per
   `(itemID, StuckReason)`.

**Implication for the webhook design**: if the webhook handler calls the *same*
`remediatePRFixWithBackoffGate` entry point (or an equivalent thin wrapper around it)
rather than calling `fixSpawner.AutoReopenForPRFix` directly, five rapid `check_run`
"failed" events for one PR are **already deduped for free** — the second through fifth
calls will all hit `RemediationDue == false` (or find `MarkStuck` reports "already open,
no-op refresh") and do nothing. This is the strongest argument for the webhook handler's
design: **treat each qualifying event as "nudge ReconcilePRPending's per-item logic to
run for this one item now" rather than "independently decide fix-or-not from this event's
payload and call the spawner."**

What is **not** already solved, and the webhook path does need its own guard for: a burst
of 5 events in the same second each triggering a live `GetPRStatus`/`GetOwnerRepoFromRemote`
GitHub API call before the backoff gate is even reached — `remediatePRFixWithBackoffGate`
gates the *spawn*, not the *reconciliation work* (item lookup, association re-verify,
GitHub status fetch) leading up to it. A short in-process debounce per `(repo, PR number)`
(e.g. coalesce events arriving within a few seconds, GitHub's own delivery jitter window)
avoids burning GitHub API rate-limit quota redundantly — the sibling project's ETag-cache
precedent (`p.etagCache`, shared across `PRStatusPoller` and presumably this handler) helps
but doesn't eliminate the redundant round-trip.

## 4. Does "completed" mean success or failure? (Q4)

**VERIFIED — the existing reconciler deliberately does NOT trust a single check's
conclusion in isolation, and the webhook handler should not either.** `git.PRStatus.CIFailing`
(`session/git/worktree_git.go:500-502`, set at lines 752-757) is computed from GitHub's
full `statusCheckRollup` for the PR (fetched via `GetPRStatus`, a live `gh pr view --json
statusCheckRollup` equivalent) — `CIFailing = true` when **any** check in the rollup has a
terminal `FAILURE`/`TIMED_OUT`/`CANCELLED` conclusion, regardless of other checks still
`IN_PROGRESS`/`PENDING`. This is confirmed by
`TestParsePRStatusPayload_CIFailing` (`session/git/worktree_git_test.go:260-294`).

GitHub's `check_run`/`workflow_run` payload shape: only the `action: "completed"` delivery
carries a non-null `conclusion` (`created`/`in_progress`/`requested`/`rerequested`
deliveries have `conclusion: null`). Valid terminal `conclusion` values: `success`,
`failure`, `neutral`, `cancelled`, `timed_out`, `action_required`, `stale`, `skipped`.

**Recommended handling, matching existing semantics rather than inventing new
success/failure logic**: filter to `action == "completed"` (ignore `created`/`in_progress`/
`requested`/`rerequested` — they carry no actionable conclusion), then **do not decide
fix-or-not from the single event's `conclusion` field at all** — use the event purely as a
trigger to re-run the same `GetPRStatus`-based `CIFailing` check `ReconcilePRPending`
already uses for the matched item, and let that aggregate verdict decide. This sidesteps
two failure modes a naive single-event read would hit: (a) a `success` conclusion on
*this* check while an *earlier* check already failed (would wrongly look like "CI passed"
if read in isolation), and (b) treating `neutral`/`skipped`/`stale` as failures when the
existing rollup logic doesn't.

## 5. `pull_request_review` / `issue_comment` — event actions and bot-loop avoidance (Q5)

**`pull_request_review`** actions: `submitted`, `edited`, `dismissed`. Only `submitted`
carries a fresh decision point. `review.state` values: `approved` (no action — merge-ready
detection already happens via the existing poll-based `prReadyToMergeSolo` path, see
`session/backlog_lifecycle_pr.go:1479`), `changes_requested` (maps to the existing
`HasBlockingReviews` signal the reconciler already computes — this maps to
`AutoReopenForPRFix`'s CI/review-issues branch, **not** the separate
`AutoReopenAfterFailedReview`, which per `server/services/backlog_service_triage.go:1667`
is the path for a **failed internal review verdict** on this app's own review pipeline,
a distinct concept from an external GitHub reviewer's verdict on the PR — confirmed by
reading both call sites' doc comments), `commented` (maps to the existing
`hasNewFeedback`/`PrFeedbackAddressedAt` watermark path, same as a plain comment below).

**`issue_comment`** actions: `created`, `edited`, `deleted`. Only relevant when
`issue.pull_request` is present (a comment on an issue, not a PR, is out of scope —
GitHub's API represents PR conversation-tab comments as `issue_comment` events with this
field set). `issue.number` is the PR number.

**Bot-loop risk is real and concrete, not hypothetical** — `GitHubService.PostPRComment`
(`server/services/github_service.go:137`) exists and is called from a forward-sync path
(`server/services/backlog_github_forward_sync.go`, per test
`TestForwardSyncSubscriber_PersistsWatermarkWhenPostCommentFails`,
`server/services/backlog_github_forward_sync_test.go:265-272`) — this instance **does**
post comments onto the live GitHub PR under its own configured GitHub identity/token, not
just internal `notify()` calls. An `issue_comment` webhook fired by that very comment would
otherwise re-trigger `hasNewFeedback`, creating exactly the self-referential loop
`.claude/rules/...` and project memory (`feedback_document_ai_decisions_in_edge_cases.md`)
warn about avoiding. `RequestCopilotReview` (`session/backlog_lifecycle_pr.go:644`) is a
second bot-comment source (Copilot's own review submissions would arrive as
`pull_request_review` events from a `Copilot`/bot actor).

Reuse, don't reinvent: `PrFeedbackAddressedAt`'s existing watermark logic
(`hasNewFeedback`, `session/backlog_lifecycle_pr.go:1452-1457`) is comparing
`prStatus.LatestFeedbackAt` from a **live re-fetch**, not the raw webhook payload — so if
the webhook handler again just nudges reconciliation rather than acting on the payload
directly, whatever filtering `GetPRStatus`'s comment/review fetch already applies (if any)
is inherited for free. What still needs a **new** filter, since it doesn't exist yet
anywhere in this codebase (confirmed: no `sender.type == "Bot"` / login-suffix check
found via `grep -rn "\[bot\]\|SenderType\|sender.*type.*Bot" server/services/*.go`): the
webhook payload's `comment.user.type`/`sender.type` field (GitHub sets this to `"Bot"` for
bot/App accounts) or a login-suffix check against this instance's **own** configured
GitHub identity (the same token identity `PostPRComment`/`RequestCopilotReview` post
under) — self-comments specifically, not all bots, since a legitimate CI bot's failure
summary comment might be exactly the kind of "review feedback" a human would want acted
on. Filtering *all* bots by type would silently drop that case; filtering *self* by login
is the narrower, correct rule.

## 6. Unstated need: visible attribution for why a fix was auto-triggered (Q6)

Project memory (`feedback_document_ai_decisions_in_edge_cases.md`) states self-heal/
auto-close actions should post a visible comment + `notify()`, not act silently. Current
`AutoReopenForPRFix` call sites already do the `notify()` half —
`remediatePRFixWithBackoffGate` posts an in-app notification ("PR needs attention...") on
first sighting (`session/backlog_lifecycle_pr.go:1191-1196`), and `fixCtx` strings passed
to `AutoReopenForPRFix` already carry a human-readable reason (e.g. `"PR #%d (%s) needs
fixes:\n\n%s"`, line 1517) that ends up in the spawned session's prompt. **What's new for
this feature specifically**: the *source* of the trigger (webhook vs. the 60s poller) is
currently invisible in that reason string — an operator watching a PR get auto-reopened
seconds after pushing a fix has no way to tell "the webhook worked" from "the poller just
happened to tick." Recommend threading a trigger-source tag (e.g. `"webhook:check_run"` vs
`"poller"`) into `fixCtx`/the notification body, both for operator trust-building during
initial rollout and for debugging if webhook delivery silently stops working and only the
poller fallback is actually firing (a regression that would otherwise be invisible — see
Goal 3's polling-backstop requirement, whose entire value depends on the two paths being
distinguishable when only one is working).

## Summary of key file references for planning

| Concern | File |
|---|---|
| `BacklogItem` PR association fields (`pr_url`, `pr_number`) | `session/ent/schema/backlog_item.go:92-96` |
| Query for `pr_pending` items with a PR number | `session/storage_backlog.go:980` (`FindPRPendingItems`) — needs a PR-number-indexed variant |
| Local repo path → GitHub owner/repo (for matching `repository.full_name`) | `github.GetOwnerRepoFromRemote`, used by `session/backlog_lifecycle_pr.go:134-143` |
| Existing PR-association re-verification before trusting a fix spawn | `session/backlog_lifecycle_pr.go:1309` (`verifyPRHeadBranchMatchesTracked`), `:1603-1626` (`verifyPRAssociationForFixSpawn`) |
| Existing dedup/backoff for repeated fix-spawn attempts on one item | `session/backlog_lifecycle_pr.go:1181-1222` (`remediatePRFixWithBackoffGate`) — reuse this, don't call `AutoReopenForPRFix` directly |
| Aggregate CI-failure derivation (don't trust one event's `conclusion`) | `session/git/worktree_git.go:500-502,752-757`; test at `session/git/worktree_git_test.go:260-294` |
| Review-verdict vs. CI-fix framing distinction | `server/services/backlog_service_triage.go:1667` (`AutoReopenAfterFailedReview`) vs. `:2018` (`AutoReopenForPRFix`) |
| Review-feedback dedup watermark | `session/backlog_lifecycle_pr.go:1452-1457` (`hasNewFeedback`, `PrFeedbackAddressedAt`) |
| This instance's own PR-comment-posting paths (bot-loop source) | `server/services/github_service.go:137` (`PostPRComment`), `server/services/backlog_github_forward_sync.go`, `session/backlog_lifecycle_pr.go:644` (`RequestCopilotReview`) |
| Existing `push`-event webhook handler to extend (signature verify, dedup, no-op outcome pattern) | `server/services/github_webhook_handler.go` |
| Sibling project's already-covered edge cases (dedup, sig failure, ordering, injection, rate limiting) | `project_plans/webhook-triggers/research/features.md` |
