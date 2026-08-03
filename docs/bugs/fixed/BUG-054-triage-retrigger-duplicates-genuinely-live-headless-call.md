# BUG-054: Retriggering Triage Silently Duplicates a Genuinely Still-Running Headless Call [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-08-01)
**Discovered**: 2026-08-01, live — while manually retrying 6 items stuck with `STUCK_REASON_ORPHANED_TRIAGE` (via the "Retry now" UI action / `TriggerRemediationNow` RPC) as part of BUG-053's incident response, at least one retry produced a genuine race.
**Fixed**: 2026-08-01 — `server/services/backlog_service.go`, `server/services/backlog_service_triage.go`.
**Impact**: Any retrigger of triage (the "Retry now" UI button, `TriggerRemediationNow`, or the automatic backoff-gated remediation sweep) for an item whose prior headless triage call is genuinely still running — not actually dead — silently marks that live call's `ItemSession` row "ended" in the DB (without touching the real subprocess) and starts a second, fully redundant LLM triage call. Confirmed live: this produced a real `"concurrent modification detected"` error when the original call finished and tried to also transition the item `idea`→`ready` after the duplicate had already done so.

## Problem Description

While responding to BUG-053's incident (6 items stuck in `STUCK_REASON_ORPHANED_TRIAGE` after a service restart killed their in-flight triage calls), 6 manual retries were issued via `TriggerRemediationNow`. Cross-referencing `~/.stapler-squad/logs/staplersquad.log` timestamps against `ps aux` afterward showed the *original* auto-respawned triage calls (from the restart-recovery sweep, started ~11:28) were never actually dead — they were still genuinely running, taking their full ~28–30 minute budget. The manual retry at 11:53 nonetheless proceeded to start a second call for the same items. For item `9209b4b9`:

```
ERROR: [TriggerTriage] status transition idea→ready item=9209b4b9...: precondition failed: concurrent modification detected: expected status "idea", got "ready"
INFO:  [TriggerTriage] headless triage complete item=9209b4b9... elapsed=3m54s suggestions=5 tasks=12
```

The newer (manually-retried) call finished first and correctly transitioned the item to `ready`. The older, genuinely-still-running call then also finished ~28 minutes after it started, tried to make the same transition, and hit `TransitionBacklogItemStatus`'s optimistic-concurrency precondition check — which correctly refused the double transition (no data corruption), but only after a full, wasted duplicate LLM triage run.

**Root cause**: `tombstoneOrphanTriageSessions` (`server/services/backlog_service_triage.go`) had no way to tell a genuinely-still-running headless triage call apart from a dead one, and resolved that unknown by simply assuming "dead" unconditionally:

```go
isHeadless := strings.HasPrefix(is.SessionUUID, headlessTriageUUIDPrefix)
isStale := time.Since(is.CreatedAt) > maxTriageSessionAge
notLive := isHeadless || isStale || s.sessionStopper == nil || !s.sessionStopper.IsSessionLive(is.SessionUUID)
```

`isHeadless` is `true` for every triage session (all of them use the `headless-triage-` UUID prefix), so `notLive` was unconditionally `true` for the entire triage-session code path — the comment ("Headless triage sessions have no live in-memory instance; treat as orphaned") documents the assumption but the assumption itself is false: a headless call is backed by a real, long-running (`claude -p`) subprocess for up to 30 minutes, same as any other in-flight work. Unlike a work/review session, there was genuinely no liveness signal available for it to check *against* — no tmux session, no tracked PID, nothing — so the code fell back to treating "unknown" as "dead" rather than tracking the one signal this process actually has: whether one of its own goroutines is still driving that call.

## Fix Applied

Added `triageInFlight sync.Map` to `BacklogService` (`server/services/backlog_service.go`), directly mirroring the existing `spawnInFlight sync.Map` pattern already used by `SpawnSessionFromItem` for the identical "prevent two concurrent attempts for one item" shape:

- `TriggerTriage` does `LoadOrStore(itemID, struct{}{})` immediately after `tombstoneOrphanTriageSessions` succeeds (closing the TOCTOU window between that check and this item's new `ItemSession` row being created) — a concurrent second call for the same item is rejected with `CodeAlreadyExists` instead of proceeding.
- The entry is deleted via `defer` inside the async goroutine that actually drives the call, covering every exit path (success, LLM error, parse error, persist failure, and the shutdown-cancellation path) — so it's cleared exactly when the real call ends, not before.
- If `TriggerTriage` returns before ever launching that goroutine (e.g. `CreateItemSession` fails), a synchronous `defer` clears the entry itself, gated by a `triageStarted` flag, so it never leaks past a call that didn't actually start async work.
- `tombstoneOrphanTriageSessions` now checks `s.triageInFlight.Load(itemID)` for a headless session instead of assuming `notLive` unconditionally; a non-headless (tmux-backed) session's behavior is unchanged (still falls back to `sessionStopper.IsSessionLive`).

This is deliberately an in-process, non-persisted signal — after a real restart, every item's `triageInFlight` entry is (correctly) absent in the new process, since no goroutine in that process could possibly still be driving an old call; this is exactly the condition BUG-053's fix already handles via the `end_reason` field. The two fixes are complementary: BUG-053 makes the *system* stop over-penalizing a shutdown-caused orphan; this fix makes a *manual* or automatic retrigger stop assuming every not-yet-ended headless session is already dead.

Per `.claude/rules/interface-pollution-checklist.md`: no new interface or abstraction — `triageInFlight` is the same primitive (`sync.Map`, LoadOrStore-on-entry/Delete-via-defer-on-exit) the file already uses one field above it for the same reason, not a novel pattern.

## Regression Test

`server/services/backlog_service_test.go`:

- `TestTriggerTriage_AlreadyExists_LiveHeadlessSession` — creates an open headless triage `ItemSession`, marks it live via `svc.triageInFlight.Store(item.ID, struct{}{})` (simulating what `TriggerTriage` itself would have set before launching its goroutine), then asserts a second `TriggerTriage` call for the same item returns `CodeAlreadyExists` and the original session is **not** tombstoned (`EndedAt` still `nil`).

**Verified to fail against pre-fix code**: before this fix, `tombstoneOrphanTriageSessions` had no `triageInFlight` field to consult — `isHeadless` alone made `notLive` unconditionally `true`, so this exact scenario (an open headless session, regardless of any liveness signal) would have been silently tombstoned and a second `TriggerTriage` call would have proceeded to `CodeOK` with a new session, not `CodeAlreadyExists`.

Existing tests confirming no regression to the surrounding behavior:
- `TestTriggerTriage_OrphanedHeadlessSession` — a headless session with **no** `triageInFlight` entry (the true "process restarted, call is genuinely dead" case) still gets tombstoned and re-trigger still succeeds.
- `TestTriggerTriage_AlreadyExists_LiveSession` — a live non-headless (tmux-backed) session via `sessionStopper` still blocks re-trigger exactly as before.
- `TestTriggerTriage_DoubleTriggerGuard` — unaffected.

## Verification

```
$ go build ./...
(clean)

$ gofmt -l server/services/backlog_service.go server/services/backlog_service_triage.go server/services/backlog_service_test.go
(clean — no output)

$ go test ./server/services/ -run 'TestTriggerTriage' -v
=== RUN   TestTriggerTriage_should_FallBackToDefaultWithWarnLog_When_ReferencedModeDeletedBeforeTriage
--- PASS
=== RUN   TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext
--- PASS
=== RUN   TestTriggerTriage_DoubleTriggerGuard
--- PASS
=== RUN   TestTriggerTriage_NilPool
--- PASS
=== RUN   TestTriggerTriage_Success
--- PASS
=== RUN   TestTriggerTriage_AutoSpawnSession_SpawnsWorkSessionWithoutManualClick
--- PASS
=== RUN   TestTriggerTriage_AutoSpawnSessionFalse_LeavesItemAtReadyForManualSpawn
--- PASS
=== RUN   TestTriggerTriage_PersistFailurePublishesNotification
--- PASS
=== RUN   TestTriggerTriage_RefineWithFeedback
--- PASS
=== RUN   TestTriggerTriage_RefineWithFeedback_RequiresPriorResult
--- PASS
=== RUN   TestTriggerTriage_HeadlessPoolError
--- PASS
=== RUN   TestTriggerTriage_AlreadyExists_LiveSession
--- PASS
=== RUN   TestTriggerTriage_OrphanedHeadlessSession
--- PASS
=== RUN   TestTriggerTriage_AlreadyExists_LiveHeadlessSession
--- PASS
=== RUN   TestTriggerTriage_NeverPublishesUntaggedNotification_OnHeadlessPoolFailureOrSuccess
--- PASS (2 subtests)
=== RUN   TestTriggerTriage_should_UseModeSpecificTriagePrompt_When_ItemHasNonDefaultPipelineModeAndFirstTriageBranch
--- PASS
=== RUN   TestTriggerTriage_should_UseUnmodifiedRetriagePrompt_When_RetriagingRegardlessOfPipelineMode
--- PASS
PASS
ok  	github.com/tstapler/stapler-squad/server/services	4.618s

$ go test ./server/services/...
ok  	github.com/tstapler/stapler-squad/server/services	72.315s

$ golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./server/services/...
0 issues.
```

## Related

- `docs/bugs/fixed/BUG-053-graceful-shutdown-kills-inflight-triage-treated-as-real-failure.md` — the incident response that surfaced this bug. Complementary, not overlapping: BUG-053 is about the system's *own* automatic recovery over-penalizing a shutdown-caused orphan; this bug is about *any* retrigger (manual or automatic) wrongly assuming a not-yet-ended headless session is already dead. Both fixes together mean: a genuinely dead headless session (post-restart) is recognized as dead and retried without penalty (BUG-053), while a genuinely live one is recognized as live and left alone rather than duplicated (BUG-054).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Type Safety Gap (in the informal sense — a missing invariant, not a Go-type-system gap). The code had no representation at all for "is this headless call actually still running," so the boolean logic silently substituted a *convenient default* (headless implies dead) for a *missing fact*. The bug wasn't a wrong calculation; it was the absence of the one primitive (a liveness record) needed to make the calculation possible at all.

**Earliest achievable enforcement**: The regression test is the earliest practical level. This is inherently a runtime/process-liveness fact — no compile-time type or lint rule can express "a goroutine elsewhere in this process is still driving this session," so a behavioral test asserting the correct `CodeAlreadyExists` outcome is the right ceiling.

**Recurring shape**: Not a repeat of one of `docs/tasks/backlog-feature-improvement.md`'s named shapes (this isn't an event lost across a restart, a self-defeating sweep exclusion, or unresolved notify-once state) — it's closer to the general "assumed-safe default masking a missing liveness check" pattern, and the fix directly reuses `spawnInFlight`'s existing precedent in the same file (added for the identical concurrent-duplicate-spawn shape on `SpawnSessionFromItem`) rather than inventing a new one. Worth naming for a future audit: `SpawnSessionFromItem`'s `spawnInFlight` guard and `TriggerTriage`'s new `triageInFlight` guard are now the two instances of "in-process LoadOrStore/defer-Delete liveness guard for a headless/async backlog action" — if a third async backlog action (e.g. review) is ever found to have the same silent-duplicate risk, this is the established pattern to reach for, not a bespoke one.
