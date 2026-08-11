# BUG-027: `stop_session` (MCP tool / `StopSession` RPC) Never Updates the Backlog `ItemSession.EndedAt` [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-20
**Fixed**: 2026-07-22 — `session/instance.go`, `session/instance_controller.go`, `session/backlog_lifecycle.go`
**Impact**: A backlog work session stopped via the `stop_session` MCP tool (or the underlying `SessionService.StopSession`/`DeleteSession` RPCs, or `BacklogService.SessionStopper.StopSessionByUUID` used by stale-work remediation) left its `ItemSession.EndedAt` permanently `null`, even though the tmux process and worktree were gone. The item's own bookkeeping believed the session was still running. This bypassed the `onSessionExited` reconciliation path (`session/backlog_lifecycle.go`) entirely — none of the stale-work remediation (PR #196), review-verdict processing, or auto-ship logic ever fired for a session torn down this way, because they all key off `onSessionExited` being called on natural process exit.

## Live Symptoms

- Confirmed live: backlog item `c2ad7bf3-91bf-4d47-8654-0f2f20869080` ("Dedent shortcut broken in edit mode") had a zombie work session (`stelekit-fix-shift-tab-dedent-desktop-r9`) stuck with `endedAt: null` for 6+ hours after the underlying blocker had already been resolved out-of-band. `stop_session` confirmed the session was fully gone (`SESSION_NOT_FOUND` on a follow-up `get_session`), but `GetBacklogItem` still showed `endedAt: null` — no transition, no reconciliation, nothing.
- Manually stopping a stuck backlog session was silently equivalent to abandoning it — worse than a hung session, since there was no longer even a process left for a stale-work detector to eventually notice as "no activity."

## Root Cause

`Instance.Destroy()` (`session/instance.go`) — the method called by every operator-initiated teardown path (`stop_session`'s MCP handler in `server/mcp/tools_lifecycle.go`, `SessionService.DeleteSession`'s async cleanup goroutine, and `BacklogService.SessionStopper.StopSessionByUUID`'s `inst.Kill()` used by stale-work remediation) — never fired any `LifecycleListener` notification. The only lifecycle event the codebase had, `EventExited`, was deliberately scoped by its own doc comment to natural exits only: *"fires when the underlying program exits unexpectedly (not via an operator-initiated Kill/Stop)."* `BacklogLifecycleListener`'s per-instance shim (`instanceBacklogListener.OnLifecycleEvent`, registered via `WireToInstance`) only switched on `EventStarted`/`EventExited` — so a deliberate stop simply produced no signal at all for it to react to, and `onSessionExited` — the function that updates `ItemSession.EndedAt`, transitions the item to `review`/`done`, and spawns the review gate — never ran.

Auditing the doc comment's stated rationale ("Callers may use this to drive auto-restart logic") against the actual wiring: the only three components registered as `LifecycleListener`s today are `BacklogLifecycleListener`, `autoArchiveListener`, and `sessionExitedPublisher` (`server/services/session_service.go`) — none of which is an auto-restart mechanism (`session/session_driver.go` does not register as a `LifecycleListener` at all). The exclusion was therefore stale/aspirational relative to current code, not protecting any live consumer — but it was still correct to keep `EventExited` itself untouched (see Fix Applied) rather than repurposing it, since `sessionExitedPublisher` asynchronously re-saves the instance to storage on `EventExited`, and firing that during `Destroy()` risks racing the synchronous `DeleteInstance()` call that follows it in the `stop_session`/`DeleteSession` handlers — a real regression the analysis surfaced and deliberately avoided by not touching `EventExited`'s existing consumers.

## Fix Applied

1. **`session/instance.go`**: added a new `EventStopped` lifecycle event, fired unconditionally via `defer` at the top of `Destroy()` — including the not-started early-return path, so a `stop_session` call against an already-paused/never-hydrated backlog session still gets its bookkeeping closed out. Kept fully separate from `EventExited` so `sessionExitedPublisher` and `autoArchiveListener` — whose current behavior on `EventExited` was not audited for stop-path safety — are entirely unaffected; only listeners that explicitly opt into `EventStopped` see it.
2. **`session/backlog_lifecycle.go`**: `instanceBacklogListener.OnLifecycleEvent` now handles `EventExited, EventStopped` identically (both call `onSessionExited`) — a deliberate operator stop ends the session exactly as much as an unexpected exit does, so the same `ItemSession.EndedAt` update, `in_progress`→`review`/`done` transition, stale-work resolution, and review-gate spawn now fire for both.
3. **`session/instance_controller.go`**: updated `RegisterLifecycleListener`'s doc comment to mention `EventStopped` alongside `EventStarted`/`EventExited`.

This closes the gap for all three real call sites at once (`stop_session` MCP tool, `DeleteSession` RPC, and `BacklogService.SessionStopper.StopSessionByUUID`) since they all funnel through `Instance.Destroy()`/`Kill()`.

## Files Affected

- `session/instance.go` — new `EventStopped` constant, `Destroy()` fires it via `defer`
- `session/instance_controller.go` — doc comment update on `RegisterLifecycleListener`
- `session/backlog_lifecycle.go` — `instanceBacklogListener.OnLifecycleEvent` handles `EventStopped`
- `session/instance_lifecycle_test.go` — new `TestDestroy_FiresEventStopped_EvenWhenNeverStarted`
- `session/backlog_lifecycle_test.go` — new `TestBacklogLifecycleListener_WireToInstance_EventStopped_TransitionsToReview`

## Verification

- `TestDestroy_FiresEventStopped_EvenWhenNeverStarted` — unit test on a bare `&Instance{}` confirming `Destroy()` fires `EventStopped` with a non-empty reason, even on the not-started early-return path.
- `TestBacklogLifecycleListener_WireToInstance_EventStopped_TransitionsToReview` — end-to-end via the real `WireToInstance` shim: firing `EventStopped` on a wired instance drives the same `in_progress`→`review` transition and `ItemSession.EndedAt` update that `EventExited` already produced (mirrors the pre-existing `TestBacklogLifecycleListener_WireToInstance` pattern for `EventStarted`).
- **Verified to fail against pre-fix code**: `git stash push -- session/instance.go session/backlog_lifecycle.go session/instance_controller.go` then re-running both new tests produces a build failure (`undefined: EventStopped`) in both new test files, confirming the fix is load-bearing.
- All pre-existing `TestBacklogLifecycleListener_OnSessionExited_*` and `TestLifecycleCallbackConcurrency` tests still pass unmodified — `EventExited`'s existing behavior and consumers (`sessionExitedPublisher`, `autoArchiveListener`) are untouched.
- Full `go test ./session/... ./server/mcp/... ./server/services/...` — green (the one `session/vc` failure seen mid-run was the known `TMPDIR`-ancestry false positive from testing under a Jujutsu-polluted job directory, confirmed non-regressing by re-running `go test ./session/vc/...` with default `TMPDIR`).
- `go build ./...`, `golangci-lint run ./session/...` — clean.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — `EventExited`'s doc comment explicitly scoped it away from operator-initiated stops, but nothing filled the resulting gap for listeners (like `BacklogLifecycleListener`) that need to know about *any* form of session termination, not just unexpected ones. The interface (`LifecycleListener`/`OnLifecycleEvent`) was correct; the event vocabulary was incomplete for a real consumer's needs.

**Earliest achievable enforcement**: A regression test is the practical level here — "does every session-teardown path notify listeners that care about session end" is a cross-cutting behavioral property much like BUG-034's watch-list symmetry, not something a type system or lint rule can verify generically without deep semantic modeling of which methods are "read" vs "terminal."

**Recurring shape**: Seventh instance this session of "something is torn down/completed but a corresponding notification or cleanup step is silently skipped" (BUG-029 stale review sessions, BUG-030 stranded items, BUG-032 superseded PRs, BUG-033 escaped WorkingDir, BUG-034 scanner watch-list leak, BUG-035 stale-instance lookup) — this time at the lifecycle-event layer: a teardown path existed, but no notification fired from it at all, rather than a notification firing with wrong/stale data. Strong repeated evidence this shape (add path wired, remove/notify path forgotten or scoped away) is systemic across this codebase's async/event-driven plumbing, not particular to any one subsystem — reinforces the case (already raised in BUG-034's reflection) for a dedicated `quality:architecture-review` pass specifically auditing "for every state-creating operation, does a corresponding state-closing operation exist and actually fire" across `session/` and `server/services/`.

## Related

- PR #196 (`fix(backlog): auto-remediate stale work sessions instead of notify-only`) — the stale-work remediation this gap bypassed entirely, since a session torn down via `stop_session` never accumulated the "no activity for N hours" signal.
- BUG-026 (`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md`) — same general area of the codebase, found during the same original investigation.
- BUG-034, BUG-035 — the two immediately preceding bugs fixed this session, both instances of the same "teardown/notification gap" recurring shape noted above.
