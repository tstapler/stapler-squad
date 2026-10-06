# Backlog Completion Gate and Terminal-Transition Cleanup

How a backlog item's self-reported "I'm done" gets checked before it's
believed, and how its worktree/tmux resources get reclaimed once it
actually is done.

## The AC-completeness gate

`request_review` (`server/mcp/tools_backlog.go`) no longer trusts a work
session's say-so. Before it transitions the item at all, it calls
`unmetAcCriteriaError(item)`, which walks `item.AcceptanceCriteria` and
fails closed unless every criterion has been marked `pass` via
`report_progress` — `pending`, `in_progress`, and `fail` are all treated as
"not done yet," and an item is never rejected forever for having zero
criteria (nothing to verify passes trivially). This applies identically to
`SkipReviewGate=true` items: that flag only skips the independent
`review`-role LLM re-check afterward, not this deterministic check — before
this change it skipped both.

A rejection:
- Leaves the item's status unchanged (no transition to `review`/`done`).
- Persists a `[request_review:rejected]`-marked line on `item.Notes` via
  `recordRejectedRequestReview`, so the rejection is visible to a human or a
  later session even though nothing else about the item's state changed.
- Counts toward the same escalation counter as `report_blocked`
  (`countBlockedCycles` sums `[request_review:rejected]` and
  `[report_blocked]` notes together) — a session that alternates between the
  two doesn't get two independent budgets. Past `blockedCycleThreshold`
  (currently 3), the *next* `report_blocked` call escalates straight to
  `review` for a human instead of bouncing back to `ready`.

## Synchronous cleanup on every terminal transition

`CleanupTerminalItem` (`server/services/backlog_service.go`) runs
`cleanupItemWorktrees` + `archiveItemWorkSessions` for a done/archived item.
Previously only `TransitionBacklogItemStatus`'s RPC handler called it
inline — the internal transition paths in `session/` (`transitionBouncingItemToDone`,
PR-merge-detected `done`) called the lower-level storage method directly and
relied entirely on the 60s `reconcileTerminalItemSessions` sweep to notice
and clean up later.

`session/` cannot import `server/services/` (package-cycle constraint), so
the fix is the `WorktreeCleaner` interface (`session/backlog_lifecycle_archive.go`),
implemented by `BacklogService` and wired in via
`BacklogLifecycleListener.SetWorktreeCleaner` (`server/dependencies.go`).
Every internal terminal-transition path now calls
`BacklogLifecycleListener.cleanupTerminalItemSync`, which invokes the
injected cleaner synchronously (a no-op if unwired) — the 60s sweep is now a
pure safety net for anything that slips through (e.g. a crash mid-transition),
not the only path that ever runs cleanup.

**Unchanged by this**: actual worktree *deletion* still goes through
`SessionRetentionSweeper`'s staged safety checks (retention window,
PR-terminal check, dirty-worktree check, sibling-rework-session check) —
this work only made the existing cleanup call *reachable* from every
terminal transition, not more aggressive.

## Known gap: per-session review diffing

The review gate (`session/review_gate.go`) computes the diff to hand a
reviewer against the *specific work session's* recorded `base_commit_sha`,
not against the item's overall base. A work session that calls
`request_review` without having committed anything itself — e.g. a fresh
session picking up an item whose real work already landed in earlier
sessions' commits — sees an empty diff and gets hard-blocked with "no
committed changes were found for this session," even though the branch's
cumulative diff against `main` is not empty. This is a pre-existing,
separately-tracked infrastructure gap (see `docs/tasks/backlog-feature-improvement.md`),
not something this gate/cleanup work changed or fixed.
