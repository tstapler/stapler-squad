# ADR-002: Extract `ReconcilePRPending`'s Full Per-Item Body (Merge Check Included)

**Status**: Accepted
**Date**: 2026-08-23

## Context

`research/architecture.md` §1 recommends extracting "the per-item logic
inside `ReconcilePRPending`'s loop... everything after `g :=
l.getPRPendingCheckerFactory()(repoPath)`" into a reusable unit — but its own
summary table (and `research/features.md`'s summary table) cite the range as
`session/backlog_lifecycle_pr.go:1355-1541` specifically, which is only the
"PR still open — check CI status and reviews" half of the per-item body
(step 2). The merge-detection half (step 1, roughly lines 1251-1352 —
`g.IsPRMerged`, the `done`-transition, ship-snapshot capture, and worktree
cleanup) is excluded from that citation.

Reading the actual code (`session/backlog_lifecycle_pr.go:1354-1368`)
surfaces why this matters: step 2 begins by fetching `prStatus :=
g.GetPRStatus(...)` and, further down, has a branch keyed on
`prStatus.IsClosed` (`:1368`) that treats a closed PR as **"closed without
merging"** — it builds a fix prompt telling the agent "PR #%d was closed
without merging. Investigate why..." A GitHub PR is `closed` in exactly the
same GitHub API sense whether it was merged or rejected; `prStatus.IsClosed`
alone cannot distinguish the two. The only reason step 2's `IsClosed` branch
is safe today is that **step 1 already routed merged PRs to the `done`
transition and `continue`d before step 2 ever runs** (`:1252-1352`). If a
webhook-triggered on-demand caller ran step 2 alone (per the summary
table's narrower citation) against an item whose PR had, in fact, just been
merged, it would misclassify a successful merge as "closed without
merging" and spawn a fix session with a materially wrong prompt.

This event genuinely can arise for the target event types: `check_run`/
`workflow_run` `completed` deliveries commonly arrive for the merge commit
itself (post-merge CI on the base branch, or a squash-merge triggering a
final check run) while the item's status may still momentarily read
`pr_pending` if the poller hasn't yet run its next tick.

## Decision

Extract the **entire** per-item body — both step 1 (merge detection,
ship-snapshot capture, `done` transition, worktree cleanup) and step 2
(CI/review-check + fix-spawn) — into one new private method, e.g.
`(l *BacklogLifecycleListener) reconcilePRPendingItem(ctx context.Context, er
*EntRepository, item *ent.BacklogItem)`, called identically by
`ReconcilePRPending`'s loop and by the new `TriggerPRFixForEvent`. This
matches the prose framing in `architecture.md` §1 ("everything after `g :=
...`") over the narrower line-range cited in its own summary table, and
treats a webhook-triggered call as a true drop-in "run one item's
reconciliation right now" — not a partial re-implementation that has to
independently re-derive the same merge/closed distinction step 1 already
handles correctly.

## Consequences

- `TriggerPRFixForEvent` inherits step 1's `done`-transition/ship-snapshot/
  worktree-cleanup behavior "for free" on a webhook-triggered call — a PR
  that merges and then produces a trailing `check_run` `completed` event
  will correctly transition the item to `done` via the webhook path (a
  strict improvement over the pre-existing poller-only latency, and
  consistent with the fact this extraction is meant to make *any*
  qualifying event a full stand-in for one reconciliation tick).
- The extraction is a larger, more mechanical diff (~290 lines moved,
  `session/backlog_lifecycle_pr.go:1251-1541`) than the narrower option
  would have been, but it is a pure move (replace the loop body with a call,
  turn every `continue` into a `return`) with no logic changes — the
  existing test suites (`session/backlog_lifecycle_test.go`,
  `session/backlog_lifecycle_stuck_test.go`,
  `session/backlog_lifecycle_superseded_test.go`,
  `session/backlog_lifecycle_pr_branch_guard_test.go`) already exercise
  `ReconcilePRPending` end-to-end and are the regression check for this
  move — see plan.md Task 1.1.1c.
- This resolves an internal tension between two of the research pass's own
  citations (prose vs. summary table) in favor of the interpretation that
  avoids a concrete correctness bug — recorded here rather than left
  implicit, since a future reader diffing against the summary table's
  narrower line range would otherwise reasonably ask why the extraction is
  bigger than cited.
