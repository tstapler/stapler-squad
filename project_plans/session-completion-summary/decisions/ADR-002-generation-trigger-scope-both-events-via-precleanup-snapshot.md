# ADR-002: Generate on Both EventExited and EventStopped, via a Pre-Cleanup Synchronous Diff Snapshot

**Status**: Accepted
**Date**: 2026-08-03
**Supersedes**: [ADR-001](./ADR-001-completion-summary-generation-scope-and-storage.md) Decision §1 and §3

## Context

ADR-001 (2026-08-02) concluded that automatic, full-fidelity (LLM-narrated,
retrievable) generation should be scoped to `EventExited` only, because
`DeleteSession` (`server/services/session_service.go:1998-2003` dispatches
`inst.Destroy()` in a `go func()`, and `s.storage.DeleteInstance(sessionTitle)`
at line 2024 runs synchronously afterward in the same RPC handler — the
`Session` row is very likely deleted from storage before the async `Destroy()`
goroutine even reaches `CleanupWorktree()` (session/instance.go:1271) and its
deferred `fireLifecycleEvent(EventStopped, "operator-destroy")` (line 1247).
ADR-001 treated this as disqualifying: no `Session` row survives, so no
summary generated on this path can be "retrievable from the session's
Summary tab."

The current `requirements.md` (backlog item `59bbff11-ee8b-418c-8484-64307cb14244`,
this SDD pass) makes both halves of that constraint explicit requirements
instead of consequences to route around:

- **FR-1/AC-1/AC-7**: both `EventExited` (any reason except
  `reconcile-session-missing`) and `EventStopped` (the `stop_session` MCP
  tool, `DeleteSession` RPC, backlog stale-work remediation) MUST trigger
  automatic generation. AC-7 calls the explicit stop/delete path "the common
  real-world session-termination path" and requires the summary be
  retrievable, "not an orphaned write-only row."
- **FR-3/AC-3**: the document must be retrievable after both a server restart
  *and* deletion of the originating `Session` row — i.e. retrievability is
  required to be **independent of the `Session` row's survival**, not
  contingent on it.

Given both are now hard requirements, ADR-001's exit strategy (skip the
`Destroy()` path's full pipeline because the `Session` row won't survive) is
no longer available — retrievability must instead be engineered to not depend
on the `Session` row at all, on *either* path, which is exactly what FR-3
already independently demands.

## Decision

1. **Both `EventExited` (reason ≠ `reconcile-session-missing`) and
   `EventStopped` trigger the identical, full-fidelity generation pipeline**
   (deterministic snapshot → LLM narrative with fallback → persist → READY),
   via a single `sessionSummaryListener` registered for both events, mirroring
   `instanceBacklogListener`'s `case EventExited, EventStopped:`
   (`session/backlog_lifecycle.go:797`). No asymmetry between the two
   triggers — see plan.md Story 1.1.1 for the exact listener.
2. **`SessionSummary` is persisted with a plain `session_id` string field, no
   edge to `Session`** (per ADR-001 Decision §2, unchanged — this part was
   already correct and independent of the trigger-scope question).
3. **The diff-stat race ADR-001 identified is real but solvable without
   giving up on the `Destroy()` path**: capture the diff snapshot
   *synchronously, before the data it depends on can disappear*, at the two
   exact points where each event fires, using the **already-existing**
   `Instance.UpdateDiffStats()` (session/instance_worktree.go:216, performs
   `git diff` I/O, updates the in-memory cache under `i.mu`) and
   `Instance.GetDiffStats()` (line 284, in-memory read only, no I/O):
   - `session/instance.go:1271` (`Destroy()`, immediately before the existing
     `i.CleanupWorktree()` call) — call `_ = i.UpdateDiffStats()` first, while
     the worktree directory still exists on disk.
   - `session/instance.go:811` (`instanceOnExitCallback`, immediately before
     the existing `i.fireLifecycleEvent(EventExited, reason)` call) — same
     call, for symmetry and to lock in a value newer than whatever the last
     UI-triggered poll happened to compute.
   - `sessionSummaryListener.OnLifecycleEvent` then calls the pre-existing
     `instance.GetDiffStats()` (no I/O, near-instant) as the *only* synchronous
     work it does before dispatching `go` for everything else — satisfying
     `LifecycleListener`'s documented contract ("Implementations must be
     non-blocking", `session/instance.go:92`) and FR-5's "no teardown latency
     impact", since a cached-field read adds negligible latency versus the
     `git diff` subprocess call ADR-001 was implicitly worried about avoiding
     inline.
4. **Approval-decision counts, timeline, and token/cost data are captured as
   the first steps inside the dispatched goroutine, not inside the
   synchronous `OnLifecycleEvent` call.** This is deliberately *not*
   synchronous, unlike the diff snapshot: `NotificationHistoryStore` reads and
   `TokenStore.GetByUUID` JSONL parses are local-disk I/O whose cost is not
   bounded the way a cached in-memory field read is, and `LifecycleListener`
   implementations must not block the caller (`Destroy()`/the exit callback).
   This is safe because `NotificationHistoryStore`'s orphan-pruning
   (`server/notifications/store.go:368-410`) only removes records once
   `SessionID` disappears from `existingSessionIDs()` *and* a 5-minute
   uptime safety window has elapsed (`server/server.go:220`,
   `buildSessionExistenceLookup`) — a `go`-dispatched goroutine that starts
   within microseconds of the synchronous callback returning is far inside
   that window. Timeline data (`Instance.CreatedAt`, `Instance.UUID`,
   `Instance.Title`) comes from the live `*Instance` pointer captured in the
   listener shim's closure at `WireToInstance` time — safe indefinitely, since
   Go does not free an object with a live reference regardless of whether the
   `Session` storage row referencing the same UUID has been deleted.

## Consequences

- The two trigger paths are now **symmetric**: same pipeline, same LLM
  narrative attempt, same persistence, same `Summary` tab retrieval —
  reversing ADR-001 Decision §3's asymmetry (`Destroy()`-path got a
  synchronous, narrative-less, unregisterable snapshot). See plan.md Story
  1.1.1/1.1.2 for the unified listener and Story 1.2.x for the retrieval
  route that works regardless of `Session`-row survival.
- The `Destroy()` and `instanceOnExitCallback` changes are 1-line additions
  each (`_ = i.UpdateDiffStats()`) ahead of existing calls — no behavior
  change to either function's existing return values or error handling; the
  new call's error is intentionally discarded (diff capture is best-effort,
  matching `computeDirDiffStats`'s existing "returns nil on any error, diff
  is optional/cosmetic" convention, `session/instance_worktree.go:299`).
- `SessionSummary` rows become orphaned (pointing at a `session_id` with no
  surviving `Session` row) as soon as `DeleteSession` completes — this is
  intentional per FR-3, not a bug; no eviction/pruning policy is introduced
  for `SessionSummary` in this pass (unlike `NotificationHistoryStore`'s
  `PruneOrphaned`, which must NOT be replicated here — see
  `research/pitfalls.md` §4 and `research/features.md` §3).
- ADR-001's diagnosis of the storage-deletion race (Context section, and its
  reproduced `DeleteSession` excerpt) remains the correct evidence trail for
  *why* a naive "just subscribe to EventStopped too" change would have been
  wrong without the pre-cleanup snapshot points in Decision §3 above — kept
  as historical record, not deleted.
