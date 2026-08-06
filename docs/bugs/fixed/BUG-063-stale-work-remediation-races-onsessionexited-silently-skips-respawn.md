# BUG-063: Stale-Work Remediation Races `onSessionExited`, Silently Discarding the Intended Fresh Work-Session Respawn [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-08-06, live in this repo's own deployed instance — backlog item `2d7fac56-4b86-46d6-9d4b-b318a595d372` bounced back to `in_progress` after a `PARTIAL` review verdict, sat with no live tmux session for ~1h42m, then — instead of getting a fresh work session to fix the reviewer's noted gaps — was silently pushed straight into a *third* review cycle against the exact same stale, already-twice-rejected diff.
**Impact**: High. Any item whose stale work-session-remediation path (`RemediateStaleWorkSession`) fires loses its fresh work-session turn essentially every time, defeating the entire purpose of that remediation (see BUG-041's precedent: "a live item ... had bounced through this exact stale-agent-idle shape 14 times with nothing ever unsticking it" before that remediation existed). The failure is completely silent — no error, no log line — because it's a race won by the *faster* of two competing code paths, not an exception.

## Root Cause

`RemediateStaleWorkSession` (`server/services/backlog_service_triage.go:1662`) is supposed to close out a stale-but-alive work session and hand off to `AutoRespawnAutonomousWork`, which gives the item a fresh work-session turn while leaving status at `in_progress`:

```go
if s.sessionStopper != nil {
    if killErr := s.sessionStopper.KillTmuxPaneOnly(ctx, active.SessionUUID); killErr != nil { ... }
}
now := time.Now()
if endErr := s.storage.UpdateItemSessionEnded(ctx, active.ID, now); endErr != nil { ... }
return s.AutoRespawnAutonomousWork(ctx, itemID)
```

`KillTmuxPaneOnly` → `Instance.KillSession()` → `pm().Close()` tears down the tmux pane, which triggers the *same* generic instance-exit machinery used for every session end: `instanceBacklogListener.OnLifecycleEvent` fires `go il.parent.onSessionExited(instanceUUID)` **in its own goroutine** (`session/backlog_lifecycle.go:816-821`), for both `EventExited` and `EventStopped` — deliberately unified since BUG-027, "a deliberate operator stop ... ends the session exactly as much as an unexpected exit does."

`onSessionExited` (`session/backlog_lifecycle.go:850`) does more than bookkeeping, though: for any `role=work` `ItemSession` ending while the item is `in_progress`, it *unconditionally* transitions the item straight to `review` (or `done` for `SkipReviewGate`) — logic written for the normal case ("the agent finished naturally, send it to review"), with no way to tell that apart from "the system just killed this pane on purpose because it's about to spawn a replacement."

This produces a race between two things reacting to the same tmux-pane kill:
- `RemediateStaleWorkSession`'s synchronous path: end session → `AutoRespawnAutonomousWork` (re-fetches item, checks `status == in_progress`, then spawns a new work session).
- `onSessionExited`'s asynchronous goroutine: end session (again) → unconditionally flip `in_progress` → `review`.

`onSessionExited`'s path is pure DB bookkeeping; `AutoRespawnAutonomousWork`'s path additionally calls `GetBacklogItem`, `ListItemSessions`, tombstones dead sessions, and only then spawns — strictly more work. `onSessionExited` therefore wins essentially every time. Once it does, `AutoRespawnAutonomousWork`'s own guard (`if status != in_progress { return nil }`, "already moved on ... nothing to do") makes the intended respawn a **completely silent no-op** — no error, no log line at all.

Confirmed live via `~/.stapler-squad/logs/staplersquad.log` for item `2d7fac56` at `2026-08-06T00:44:12` (timestamps to the millisecond):

```
00:44:12.163238552  "successfully killed tmux session" ...verify-tmux-flake-fix-and-close-r1
backlog_service_triage.go:1713  [RemediateStaleWorkSession] item=2d7fac56... ended stale work session=16dc6abd... (session_uuid=3278f577...), respawning
backlog_lifecycle.go:920        [BacklogLifecycle] item 2d7fac56... transitioned to review (session 3278f577... exited)
review_gate.go:94               [PipelineEngine] item=2d7fac56... stage=review mode="default"
```

No `AutoRespawnAutonomousWork` log line (success or error) ever appears for this item — a full-log `grep` for it across the incident returns nothing, confirming the silent no-op.

## Fix

Two complementary changes close the race — one converting it from probabilistic ("usually wins") to a guaranteed happens-before, the other making the receiving side actually respect that guarantee:

**1. `server/services/backlog_service_triage.go`'s `RemediateStaleWorkSession`**: reordered to call `UpdateItemSessionEnded` (mark the stale session ended) **before** `KillTmuxPaneOnly` (kill the pane), not after. This guarantees — by program order, not timing luck — that `EndedAt` is already non-nil by the time the pane kill's asynchronous exit event reaches `onSessionExited`.

**2. `session/backlog_lifecycle.go`'s `onSessionExited`**: capture whether the `ItemSession` was **already ended** (`is.EndedAt != nil`, read at fetch time, before this function's own `UpdateItemSessionEnded` overwrites it) as `alreadyEndedByOtherPath`. If true, some other code path (stale-work remediation, or any future pre-ending caller) already deliberately closed this session out and owns whatever comes next — skip the `in_progress`→`review`/`done` transition entirely instead of racing it:

```go
alreadyEndedByOtherPath := is.EndedAt != nil
// ... existing UpdateItemSessionEnded bookkeeping unchanged ...
switch is.Role {
case SessionRoleReview:
    l.handleReviewSessionExited(ctx, is, false)
    return
case SessionRoleWork:
    // fall through
default:
    return
}
if alreadyEndedByOtherPath {
    log.DebugLog.Printf(...)
    return
}
// existing in_progress -> review/done transition, unchanged
```

Either change alone is incomplete: the reordering without the guard still lets `onSessionExited` unconditionally flip status regardless of `EndedAt`; the guard without the reordering still depends on timing luck to have `EndedAt` set before the kill's exit event is read. Together they make the fix deterministic instead of merely making the race less likely to lose.

The `onSessionExited` guard is scoped to the `role=work` branch only (the evidenced bug) — the `SessionRoleReview` branch's verdict-processing dispatch already has its own idempotency handling and is out of scope here. The guard generalizes safely beyond just `RemediateStaleWorkSession`: `tombstoneOrphanWorkSessions` and similar call sites already pre-end confirmed-dead sessions elsewhere in this codebase, and any of those racing a belated real exit event now gets the same protection.

## Regression Tests

`session/backlog_lifecycle_test.go`:
- `TestBacklogLifecycleListener_OnSessionExited_WorkSession_SkipsTransition_WhenAlreadyEndedByOtherPath` — pre-ends a work `ItemSession` (mirroring `RemediateStaleWorkSession`'s own `UpdateItemSessionEnded` call) before invoking `onSessionExited`, then asserts the item stays `in_progress` instead of being flipped to `review`. Verified this test fails (item incorrectly ends up at `review`) against the pre-fix code, confirming it reproduces the exact live bug.

`server/services/backlog_service_test.go`:
- `TestRemediateStaleWorkSession_should_EndSessionBeforeKillingPane_When_ActiveWorkSessionIsStale` — a hook on `mockSessionStopper.KillTmuxPaneOnly` inspects storage at the exact moment the (first) pane kill occurs and asserts `EndedAt` is already set by then. Verified this test fails against the pre-fix (kill-then-end) ordering — the hook observes `EndedAt == nil` on the first kill call — confirming it catches a regression to the wrong ordering. (The hook ignores a second, later kill call made by `killEndedWorkSessionPanes` right before the replacement session spawns — a legitimate, separate re-invocation on an already-ended session that isn't the one racing `onSessionExited`.)

All pre-existing `onSessionExited` and `RemediateStaleWorkSession` tests (`TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToReview`, `..._TransitionsToDone_WhenSkipReviewGate`, `..._ReviewSession_NoTransition`, `..._ReviewSession_RoutesToHandleReviewSessionExited`, `TestRemediateStaleWorkSession_should_killTombstoneAndRespawn_When_ActiveWorkSessionIsStale`, `..._should_noop_When_ItemNoLongerInProgress`, etc.) continue to pass unchanged.

`go test ./session ./server/services` and `make lint` both pass; `go build ./...` and `make build` are clean.

## Phase D — Classification (per `quality:reflect-and-fix`)

**Classification**: Concurrency/Race — two independent code paths (a synchronous remediation flow and an asynchronous, goroutine-dispatched lifecycle event) both react to the same underlying state change (a tmux pane closing) and both believe they own the resulting status transition, with no coordination between them.

**Earliest enforcement point**: A pure type-system fix isn't available here — the race is between a synchronous call chain and a `go`-dispatched event handler triggered by the same OS-level action (closing a tmux pane), and Go's type system has no way to statically forbid two goroutines racing to write the same row. The regression test above, at the exact `onSessionExited` entry point, is the earliest achievable enforcement level; it directly exercises the same "was this session already closed out by someone else" signal the runtime fix relies on, so a future regression to this guard is caught by intent, not incidentally.

**Recurring shape**: Any code path that (a) deliberately ends a session/resource via a direct storage write, and (b) also triggers that resource's normal teardown machinery (killing a tmux pane, closing a process, etc.), must consider whether the normal teardown path's own event handlers will *also* react and potentially race the deliberate caller's own follow-up logic. This is the same shape as BUG-027 (which unified `EventExited`/`EventStopped` for bookkeeping purposes) one layer up: unifying the *bookkeeping* handling of deliberate vs. accidental exits was correct, but the *side effects* riding along with that bookkeeping (here, the status transition) needed their own "was this already handled elsewhere" guard, which BUG-027 didn't add. Future additions to `onSessionExited` (or any other lifecycle-listener side effect) should ask the same question before assuming an exit event always means "this ended on its own."

## Related

- BUG-027 — introduced the `EventExited`/`EventStopped` unification in `instanceBacklogListener.OnLifecycleEvent` that this bug's race flows through.
- BUG-041 (`docs/bugs/fixed/BUG-041-backlog-nudge-retry-never-backs-off.md`) and the `StaleWorkRemediator` doc comment (`session/backlog_lifecycle.go`) — cite the original "14 rework rounds, nothing ever unsticking it" motivation for `RemediateStaleWorkSession`'s existence, which this bug was silently defeating again via a different mechanism.
