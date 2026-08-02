# Requirements: pr-review-followup

**Date**: 2026-08-02
**Type**: feature addition (extension of existing autonomous reconciliation loop)
**Complexity**: 2 — targeted extension of an existing, well-tested mechanism (not new infrastructure)

## Problem Statement

The backlog item's premise, as written, is that PR review follow-up is a **one-shot
check** performed only by the interactive `/backlog:ship` → `github:pr-ship` slash-command
pipeline (`~/dotfiles/.claude/commands/github/pr-ship.md`, `~/dotfiles/.claude/skills/github/skills/pr-ship`)
right after PR creation, and that nothing revisits the PR afterward.

Investigation of the actual Go backend shows this premise is **partially stale**: a
separate, always-on autonomous mechanism already exists and already does most of what
the item asks for.

- `ReconcilePRPending` (`session/backlog_lifecycle.go:3850`) polls every `pr_pending`
  item on a **60-second tick**, forever, as one of the `ReconcileStuck` sweep's
  registered detectors (`server/dependencies.go:918`, `l.runStuckDetector("pr_ready+merge_detection", ...)`
  at `session/backlog_lifecycle.go:1630`) — not a one-shot check tied to the ship session.
- `GetPRStatus` (`session/git/worktree_git.go:526`) calls
  `gh pr view --json statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft`
  and `parsePRStatusPayload` (`worktree_git.go:549`) evaluates three boolean signals —
  `CIFailing`, `HasBlockingReviews`, `HasConflicts` — onto a `PRStatus` struct.
- When any of the three is true, `ReconcilePRPending` calls
  `remediatePRFixWithBackoffGate` → `AutoReopenForPRFix` (`backlog_lifecycle.go:3844`),
  which spawns a fix session, bounded by `maxAutoReworkIterations = 3`
  (`StuckReasonReworkCap`, `session/domain/backlog.go:44`) — the same auto-fix loop
  the item's Proposed Change §3 asks to reuse.
- `StuckReasonPRNeedsFix` (`domain/backlog.go:132`) and the `/unfinished` stuck-item
  visibility UI already surface items where this loop can't self-resolve (item's
  Proposed Change §4) — for whichever conditions actually trip `HasBlockingReviews`/
  `CIFailing`/`HasConflicts` in the first place.

**The real, narrower gap**, found by reading `parsePRStatusPayload`
(`worktree_git.go:620-636`):

```go
for _, r := range payload.Reviews {
    switch strings.ToUpper(r.State) {
    case "CHANGES_REQUESTED":
        status.HasBlockingReviews = true
        ...
    case "APPROVED":
        status.ApprovedCount++
    }
}
for _, c := range payload.Comments {
    status.generalComments = append(status.generalComments, "@"+c.Author.Login+": "+c.Body)
}
```

`HasBlockingReviews` fires **only** on a review with GitHub review-state
`CHANGES_REQUESTED`. A review left in `COMMENTED` state — which is how GitHub Copilot's
automated code review typically posts (a review object plus inline comments, without
formally requesting changes) — and plain issue/PR comments from a human are folded into
`generalComments`/`FeedbackText` purely as **context text attached to an
already-triggered fix session**. Neither ever independently sets a boolean that would
cause `ReconcilePRPending` to call `AutoReopenForPRFix` on their own. Concretely: if a
PR has green CI, no merge conflicts, and a Copilot review with several inline
suggestions but state `COMMENTED` (not `CHANGES_REQUESTED`), `!CIFailing &&
!HasBlockingReviews && !HasConflicts` is true, the reconciler treats the PR as healthy
and waits for merge — the comments are never looked at, and the item never leaves
`pr_pending` until a human merges it manually or happens to notice.

Separately, nothing in the ship pipeline (`pushAndCreatePR` /
`~/dotfiles/.claude/commands/github/pr-ship.md`) requests a GitHub Copilot review on PRs
the automation opens — Copilot review only happens if configured as a required reviewer
at the repo/org level (outside this codebase) or requested manually.

## Baseline

Today:
- A PR with `CHANGES_REQUESTED` reviews, failing CI, or `mergeStateStatus: DIRTY` is
  already detected within 60s and already gets an autonomous fix session, capped at 3
  attempts, surfaced via `StuckReasonPRNeedsFix`/`StuckReasonReworkCap` if it can't
  self-resolve.
- A PR with only `COMMENTED`-state reviews (Copilot's typical posture) or plain human
  comments and otherwise-green status is **never** flagged, never triggers a fix
  session, and sits in `pr_pending` indefinitely with the feedback unaddressed.
- No step in the ship flow requests a Copilot review on newly opened PRs.

## Users / Consumers

The backlog automation system (`BacklogLifecycleListener.ReconcilePRPending`) is the
primary consumer of the extended detection signal. Indirect beneficiary: the repo
owner, who today has to notice comment-only feedback manually since nothing else will.

## Success Metrics

- A PR with substantive `COMMENTED`-state review feedback (Copilot or human) and
  otherwise-healthy CI/mergeability is detected by `ReconcilePRPending` within one
  60s polling tick and triggers the same `AutoReopenForPRFix` path used for
  `CHANGES_REQUESTED` reviews today.
- PRs opened by the ship pipeline get a Copilot review requested automatically (where
  not already enforced by repo/org policy), so Copilot feedback has a chance to exist
  before the item leaves the ship session's immediate attention window.
- A `pr_pending` item whose comment-driven fix attempts exhaust
  `maxAutoReworkIterations` is surfaced exactly like existing `StuckReasonReworkCap`/
  `StuckReasonPRNeedsFix` cases today — no new, parallel visibility mechanism.
- Zero regressions to existing `CIFailing`/`HasBlockingReviews`/`HasConflicts`
  detection and their tests (`session/git/worktree_git_test.go`,
  `session/backlog_lifecycle_test.go`).

## Appetite

Small (1-3 days) — this is a signal-detection extension on an existing, already-tested
polling/spawn mechanism, not new infrastructure.
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must reuse `AutoReopenForPRFix`/`remediatePRFixWithBackoffGate` and
  `maxAutoReworkIterations` — no parallel fix-spawn mechanism (mirrors the explicit
  decision already made for conflict detection in `backlog-pr-conflict-detection`).
- `gh` CLI remains the exclusive PR-status data source; no new GitHub App/webhook
  integration for this scope.
- Must not regress `CHANGES_REQUESTED` review detection or the existing
  `FeedbackText`/`render()` assembly order (conflict first, per
  `session/git/worktree_git.go:469`).
- Must distinguish "new/unaddressed" comment-only feedback from feedback already
  factored into a prior fix attempt — otherwise every 60s tick would re-spawn a fix
  session for a `COMMENTED` review that was already addressed and never dismissed
  (GitHub does not let PR authors dismiss `COMMENTED` reviews the way they can
  dismiss `CHANGES_REQUESTED` ones), which would blow through
  `maxAutoReworkIterations` on stale feedback instead of ever reaching a healthy
  steady state.

## Non-functional Requirements

- **Performance SLO**: none beyond staying inside the existing 60s
  `ReconcileStuck` tick budget; this adds parsing of already-fetched `gh pr view`
  JSON, not a new API call.
- **Scalability**: same order of magnitude as existing `pr_pending` item count
  (single digits today).
- **Security classification**: internal — uses the same authenticated `gh` CLI
  session already used for all other PR operations.
- **Data residency**: no special requirements.

## Scope

### In Scope
- Extend `parsePRStatusPayload`/`PRStatus` to surface unaddressed `COMMENTED`-state
  review feedback and/or new plain PR comments as a signal that can drive
  `AutoReopenForPRFix`, without regressing `HasBlockingReviews`'s existing
  `CHANGES_REQUESTED`-only meaning (new field, not a redefinition).
- A staleness/dedup mechanism so already-addressed comment feedback doesn't
  re-trigger a fix session every tick (see Constraints) — mechanism to be decided in
  Phase 3 planning (e.g. tracking last-seen comment IDs/timestamps vs. last fix-spawn
  time on the backlog item).
- Wire a Copilot review request into the ship flow (`gh pr edit --add-reviewer` or
  the Copilot-specific mechanism) if not already reliably triggered.
- Regression tests for the new signal, plus confirmation existing
  `CHANGES_REQUESTED`/CI/conflict tests still pass unchanged.

### Out of Scope
- Any change to `PRStatusPoller`/`WorktreePRPoller`'s UI-facing badge computation
  (display-only, separate from `ReconcilePRPending`'s action-driving path — same
  boundary already drawn in `backlog-pr-conflict-detection`).
- Webhook/real-time notification for review comments — stays poll-based, consistent
  with the rest of the pipeline.
- A dedicated "comment triage" flow distinct from the existing generic fix-session
  path (same reuse decision already made for conflicts).
- Building new stuck-item visibility infrastructure — reuse
  `StuckReasonPRNeedsFix`/`StuckReasonReworkCap` and `/unfinished`.

## Rabbit Holes

- **Comment/review dedup granularity**: does dedup need to be per-comment-ID, or is a
  simple "any new review/comment since the last fix-spawn timestamp on this item" good
  enough? Under-engineering here directly causes the rework-cap-exhaustion failure
  mode called out in Constraints. Flag for Phase 3.
- **What counts as "substantive"**: a bare "LGTM" `COMMENTED` review or a bot
  status-only comment shouldn't spawn a fix session. Needs a defined filter (e.g.
  ignore reviews with empty body and zero inline comments) so noise doesn't burn
  `maxAutoReworkIterations`.
- **Copilot review request idempotency**: `gh pr edit --add-reviewer` behavior when
  Copilot is already a reviewer or has already completed a review — must not error the
  ship flow or request duplicate reviews.

## Alternatives Considered

- **New parallel comment-triage mechanism** (separate from `AutoReopenForPRFix`):
  rejected for the same reason the conflict-detection project rejected a dedicated
  rebase-only flow — more code, inconsistent with the one pattern already proven to
  work for CI/review/conflict triggers.
- **Webhook-based comment notification**: rejected — entire pipeline
  (`ReconcilePRPending`, `PRStatusPoller`, `WorktreePRPoller`) is poll-based; a
  webhook for just this one signal is a disproportionate new integration class.

## Feasibility Risks

- Without a correct dedup/staleness mechanism, this feature could spawn a fix session
  on every 60s tick for feedback that was already addressed but is structurally
  impossible to "resolve" via GitHub's API the way `CHANGES_REQUESTED` reviews are
  dismissed — the single highest-risk item in this scope (see Constraints and Rabbit
  Holes).
- `gh pr view --json comments` returns issue-level comments; Copilot's inline
  review-thread comments may require a different `gh api`/`--json` shape to detect
  reliably — needs verification in Phase 2 research, not assumed.

## Observability Requirements

Log when comment-driven feedback is detected and a fix session is spawned (matching
the existing `log.InfoLog`/`log.WarningLog` pattern in `ReconcilePRPending`), including
which signal triggered the spawn (CI / review / conflict / comment-feedback) — mirrors
the existing Observability Requirement from `backlog-pr-conflict-detection`, extended
with the new trigger type.

## Risk Control

- Bounded by the existing `maxAutoReworkIterations = 3` cap — shared with the other
  triggers unless Phase 3 planning finds a specific reason to separate them.
- No feature flag — extension of an already-live, unconditional reconciliation loop,
  consistent with how CI/review/conflict triggers already ship.
- Rollback: revert the `PRStatus`/`parsePRStatusPayload`/`ReconcilePRPending`/ship-flow
  changes; no schema/data migration involved.

## Open Questions

- ~~Dedup mechanism for comment feedback~~ — **resolved by Phase 2 research**: dedup on
  GitHub review/comment **IDs** (not timestamps, to avoid clock skew and correctly
  re-trigger on a genuinely new `COMMENTED` review), recording the marker atomically
  with the `remediatePRFixWithBackoffGate` spawn decision (`build-vs-buy.md`,
  `architecture.md`, `pitfalls.md` all converge here). `parsePRStatusPayload`
  currently discards `id`/timestamp fields entirely (`pitfalls.md`) — decoding them is
  a prerequisite, not optional.
- ~~Exact `gh` invocation for Copilot's inline review-thread comments~~ — **resolved**:
  the existing `gh pr view --json reviews,comments` call already returns per-item
  `id`/`createdAt`/`submittedAt` with no `--json` flag change (`architecture.md`,
  verified live). Full thread-resolution state (`isResolved`) requires a *separate*
  `gh api graphql` call against `reviewThreads` (`stack.md`, `features.md`, verified
  live but empty on sampled PRs) — needed only if Phase 3 decides to honor manual
  "resolve conversation" as addressing feedback; the base comment/review-ID dedup
  above does not require it.
- Definition of "substantive" comment/review worth acting on vs. noise to ignore
  (e.g. bare "LGTM", empty body) — **unresolved after Phase 2 research**; carry into
  Phase 3 planning.
- Whether Copilot review requests should be gated behind a repo-level Settings toggle
  or always attempted — **unresolved after Phase 2 research**; carry into Phase 3
  planning.
- **New, found during Phase 2 research** (`ux.md`): `domain.StuckReasonPRNeedsFix` has
  no corresponding proto `StuckReason` enum value — `toProtoStuckReason`
  (`server/services/backlog_service_stuck.go:28-57`) silently falls through to
  `STUCK_REASON_UNSPECIFIED`, so items stuck on this reason today render as "Unknown
  reason" on `/unfinished` and its "Retry now" button is broken for them. This
  project's reuse-first plan depends on that surfacing path actually working, so
  fixing the enum gap (proto + Go switches + TS `Record` maps) must be in scope for
  Phase 3, not treated as pre-existing/out-of-scope.
- **New, found during Phase 2 research** (`architecture.md`): `remediatePRFixWithBackoffGate`
  (added after `backlog-pr-conflict-detection` shipped, not reflected in this
  document's original Problem Statement) already provides a time-based backoff gate
  (`Storage.RemediationDue`, 5 attempts) shared by all triggers — this reduces but
  does not eliminate the dedup risk (time-based backoff still eventually re-fires on
  content that never self-clears), so the content-based ID dedup above remains
  required, layered on top of, not instead of, the existing gate.
- **New, found during Phase 2 research** (`architecture.md`, `pitfalls.md`,
  `build-vs-buy.md`): the installed `gh` CLI in this environment is v2.86; `gh pr edit
  --add-reviewer @copilot` requires v2.88+ (shipped 2026-03-11). Phase 3 planning must
  pick between upgrading `gh` in the deployment environment or falling back to the
  legacy `copilot-pull-request-reviewer[bot]` login / raw `gh api` reviewer-request
  endpoint.
