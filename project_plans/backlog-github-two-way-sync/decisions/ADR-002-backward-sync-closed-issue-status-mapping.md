# ADR-002: Backward-Sync Closed-Issue Status-Mapping Policy (archived, never done)

**Status**: Accepted
**Date**: 2026-08-03
**Related**: `project_plans/backlog-github-two-way-sync/requirements.md` AC4, AC7; `research/architecture.md` §5.1; `research/pitfalls.md` §2

## Context

Backward sync must decide what local status a `BacklogItem` should move to when
its linked GitHub issue is observed closed (AC4). This app has 9 statuses
(`idea, refining, ready, queued, in_progress, review, pr_pending, done,
archived` — `session/domain/backlog.go:15-25`); GitHub issues have exactly 2
(`open`/`closed`). There is no canonical mapping from a 2-state model onto a
9-state one — even GitHub's own Projects v2 treats this as a local product
decision, not something to infer generically (research features.md §2).

The naive answer — "closed → done" — collides directly with
`TransitionGuard`'s `to == BacklogStatusDone` branch
(`session/domain/backlog.go:473-486`):

```go
case to == BacklogStatusDone:
    if item.OverrideReason != "" {
        return nil
    }
    if item.OverallOutcome != ReviewOutcomePass {
        return ErrVerdictRequired
    }
    if item.HasUnshippedCode {
        return ErrCodeNotOnMain
    }
    return nil
```

Only `review` and `pr_pending` have a `validTransitions` edge to `done` at
all (`session/domain/backlog.go:363,370`) — `idea`/`refining`/`ready`/`queued`/
`in_progress` cannot reach `done` structurally, regardless of the guard.
And even for `review`/`pr_pending`, `OverallOutcome`/`HasUnshippedCode` are
**not present on `BacklogItemData`** — computing them requires either a
separate `Storage.GetMostRecentReviewVerdictForItem` call (available,
same-package) or porting `isCodeShippedToMain`'s private
worktree/git-ancestry logic (`server/services/backlog_service_lifecycle.go`,
using `session/git.IsCommitOnMain`) into package `session`, since `SyncOne`
cannot import `server/services` (would create an import cycle: `server/services`
already imports `session`).

Using `OverrideReason` to force the guard defeats its purpose: someone
closing a GitHub issue as "won't fix"/"duplicate"/"stale" is not the same
signal as "the code shipped and passed review" — treating every closed issue
as an implicit override would let a maintainer's routine issue-triage action
silently mark unfinished backlog work as done.

## Decision

Backward sync's closed-issue policy targets **`BacklogStatusArchived`**, and
only for items in `idea`, `refining`, `ready`, or `queued` — the exact set of
statuses `validTransitions` already allows a direct edge to `archived` from
(`session/domain/backlog.go:332-354`). `TransitionGuard`'s `switch` has no
`case` for `to == BacklogStatusArchived` at all (only `to == BacklogStatusDone`
gets special-cased; everything else — including `archived` — falls to
`default: return nil`), so this policy needs **zero** `OverallOutcome`/
`HasUnshippedCode` computation, sidestepping the cross-package porting problem
entirely. A new pure function, `determineBackwardSyncTarget(current
BacklogStatus) (target BacklogStatus, ok bool)`, encodes this table
explicitly (see `implementation/plan.md`, Phase 2, Epic 2.1).

For items in `in_progress`, `review`, or `pr_pending` when their issue closes
externally: **no transition is attempted**. `archived` is not reachable from
any of these three statuses per `validTransitions`, so there is no safe
target under this policy — this is logged (`skipped`, not `errored`) and
surfaced via the item's `BacklogStatusEvent`-adjacent audit trail, not
silently dropped. A maintainer closing the GitHub issue while an agent
session is actively working the item (or awaiting review/PR merge) is a
genuinely ambiguous signal this plan deliberately does not resolve
automatically.

For items already `done`: **no transition is attempted**, even though
`archived` is technically reachable from `done` per `validTransitions`
(`session/domain/backlog.go:377-384`). Auto-archiving a `done` item the
instant its issue is observed closed would fire on every normal case (forward
sync itself closes the issue when the item reaches `done` — this is the
expected, common path, not an anomaly) and would add a redundant, noisy
`archived` transition for the overwhelmingly common case where forward sync
already did its job correctly. `done` is left alone by this policy; a user
who wants to archive a done item does so explicitly.

**Reopened issues** (`state: "open"`) on an already `archived` or `done` item
are a **documented no-op**: `archived`'s only outgoing edge is to `idea`
(`session/domain/backlog.go:385-387`) — forcing a full re-triage cycle
automatically, just because someone reopened an issue on GitHub, is a far
more drastic and surprising automatic action than the closed-issue case, and
the research explicitly recommends against it (architecture.md §5.1). This
policy logs the reopen (`"GitHub issue reopened; backlog item is X — reopen
manually to re-triage"`) and takes no further action. The user reopens the
GitHub issue and, if they also want backlog-side action, does so manually.

## Consequences

- A future maintainer must not "simplify" `determineBackwardSyncTarget` to
  fall through to `done` for `review`/`pr_pending` without first solving the
  `HasUnshippedCode`/`OverallOutcome` cross-package problem this ADR
  deliberately avoided — doing so naively (e.g. via `OverrideReason`) recreates
  the exact "closing an issue ≠ shipping code" conflation this ADR rejects.
- `in_progress`/`review`/`pr_pending` items with an externally-closed issue
  are a known, accepted gap — visible only via the sync log/audit trail, not
  auto-resolved. If this proves too silent in practice, a future iteration
  could surface it more prominently in the UI (e.g. a card-level "issue closed
  externally, item still active" indicator) without changing the underlying
  no-transition decision.
- Reopening a GitHub issue never automatically revives an `archived`/`done`
  backlog item. This is a deliberate, conservative choice — a false negative
  (user has to manually reopen) is preferred over a false positive (an
  archived item's whole triage cycle silently restarting from an external
  signal the user may not have intended to have that effect).
