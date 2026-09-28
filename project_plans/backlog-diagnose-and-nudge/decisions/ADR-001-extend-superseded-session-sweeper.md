# ADR-001: Extend `SupersededSessionSweeper`, Don't Build a Parallel Stale-Session Mechanism

**Status**: Accepted
**Date**: 2026-09-27

## Context

`backlog-diagnose-and-nudge` needs a "stale retry-session cleanup" mechanism: after a
handoff summary is generated for a stuck session, the old session's tmux pane/worktree
must be torn down. `research/architecture.md` §1.1 and `research/pitfalls.md` §3 both
confirm:

- `server/services/superseded_session_sweeper.go` (PR #808) already provides a
  ticker-driven, convergent, no-special-case archival mechanism —
  `findSupersededSessions` (latest-`CreatedAt`-wins tie-break,
  `server/services/backlog_service_triage.go:1320`) + `archiveIfNotLive` (skips a
  confirmed-live session, never hard-kills) + `ArchiveSessionByUUID`/`KillTmuxPaneOnly`.
- The 8 named spawn-site bypasses from `superseded-rework-session-retirement` are
  **already covered** by this sweeper as a defense-in-depth backstop.
- This feature's cleanup predicate ("idle and stalled, no newer round exists") is
  **not the same question** the sweeper currently answers ("a newer round exists and
  supersedes this one") — the sweeper is scope-limited to `in_progress`/`review` items
  comparing rounds within one item's history.
- `follow-ups.md` for that prior feature records an explicitly open gap: no lint
  ratchet enforces that a future automated-lifecycle path calls `IsArchived()`. This
  feature's cleanup path is exactly the "next" such path.

Building a second, independent "which session is current / should this be archived"
predicate would reproduce the mass-session-resurrection failure mode
(`superseded-rework-session-retirement` requirements.md) this codebase already paid
to fix once.

## Decision

Extend `SupersededSessionSweeper` with a second, distinct sweep predicate
(`findIdleStaleSessions`, new function, same file or an adjacent
`diagnose_stale_session_cleanup.go` in `server/services/`) that:

1. Reuses the **same** `supersededSessionStore` interface and the **same** archival
   primitives (`ArchiveSessionByUUID`, `KillTmuxPaneOnly`) — never `StopSessionByUUID`.
2. Inserts a handoff-then-cleanup step (`HandoffSummaryGenerator.BeginGeneration` →
   `GenerateAndPersist` → poll `FindRowBySessionID` until terminal) **before** calling
   `ArchiveSessionByUUID`, which the existing sweeper never does today.
3. Calls `Instance.IsArchived()` itself before acting (closing follow-up #1's gap for
   this one new path, per Mandatory Design Decision #11b — see the narrow lint
   analyzer in Phase 4, Epic 4.2).
4. Never redefines "is this session current" — it only decides "is this specific,
   already-non-current-or-orphaned session idle-and-stale," a strictly narrower
   question layered on top of, not competing with, `findSupersededSessions`.

## Consequences

- One new predicate function + one new poll-loop helper, not a new service, not a new
  ticker (reuses `SupersededSessionSweeper`'s existing 60s ticker or, if scheduling
  needs diverge, a second ticker instance constructed from the same store interface —
  decided in Phase 6, Story 6.1.1).
- The handoff-then-cleanup step is the one genuinely new piece of control flow; it must
  never fire while `IsSessionLive`/`IsArchived()` says otherwise.
- Future maintainers extending stale-session logic have exactly one place
  (`superseded_session_sweeper.go` and its narrow sibling) to look, not two competing
  mental models of "which session survives."

## Alternatives Considered

- **Parallel bespoke reconciler** with its own "is this session current" logic —
  rejected: this is structurally the exact "ninth bypass" shape `pitfalls.md` §3 warns
  against, and duplicates `findSupersededSessions`' tie-break for no benefit.
- **Fold cleanup entirely into `BacklogService`** (no sweeper involvement, cleanup runs
  synchronously inside the diagnose dispatch flow only) — rejected: loses the
  self-healing backstop property (a diagnose dispatch that crashes mid-cleanup would
  leave the stale session orphaned with nothing to retry it, the same gap PR #808
  itself was created to close for the original 8 bypasses).
