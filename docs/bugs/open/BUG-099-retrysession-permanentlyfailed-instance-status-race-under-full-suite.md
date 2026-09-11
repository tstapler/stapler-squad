# BUG-099: `TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed` races on `Instance` status under full `server/services -race` load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-04, running `make quick-check`'s `test-race` target (`go test -race -short -timeout=20m ./...`) while
verifying an unrelated `log`/`config` workspace-path fix (backlog item `ebfd919f-e97a-404b-a23b-fd626bfbc929`).
**Impact**: Test-only, but fails `-race` for the whole `server/services` package (cascades into ~35 unrelated
`--- FAIL` lines once the race detector aborts the binary). `go test -race -run
TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed -count=20
./server/services` passes 20/20 in isolation — this only reproduces under the full package's parallel load.

## Problem Description

```
WARNING: DATA RACE
Read at 0x00c014669668 by goroutine 48184:
  session.(*Instance).TryForceStatusIfEpoch.func1()
      session/instance_actor_setters.go:601
  session.(*Instance).sendSyncErr()
      session/actor.go:37
  session.(*Instance).TryForceStatusIfEpoch()
      session/instance_actor_setters.go:597
  server/services.commitTerminalStatus()
      server/services/session_creation_terminal_write.go:49
  server/services.(*SessionService).runBackgroundResolutionPipeline.func1()
      server/services/session_creation_pipeline.go:123
  ...
  server/services.(*SessionService).CreateSession.func1()
      server/services/session_service.go:2637
  server/services.(*SessionService).trackCleanup.func1()
      server/services/session_service.go:372

Previous write at 0x00c014669668 by goroutine 8159:
  session.markSessionPermanentlyFailed()
      session/session_driver.go:924
  session.(*Instance).MarkPermanentlyFailedForTest()
      session/retry_state.go:430
  server/services.TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed()
      server/services/session_service_retry_test.go:134
```

A second, related race in the same run additionally shows `session.(*Instance).loadStatus()`
(`session/instance_state.go:22`, called from `RecoverFromStopped` → `restartForRetry` → `RetryNow`)
writing the same field concurrently with the `TryForceStatusIfEpoch.func1` read above.

The test calls `CreateSession` (line 101), which spawns a background `runBackgroundResolutionPipeline`
goroutine (via `trackCleanup`) that is still running — writing/reading the instance's status through
`commitTerminalStatus`/`TryForceStatusIfEpoch` — when the test's own goroutine concurrently calls
`MarkPermanentlyFailedForTest` (line 134) and then `RetrySession` → `RetryNow` → `restartForRetry` →
`RecoverFromStopped` → `loadStatus` (line 136), all racing on the same status field without the test
waiting for the earlier background pipeline goroutine to finish first.

## Confirmed unrelated to the workspace-path fix

The diff under review (`config/workspacepath`, `log/log.go`, `config/config.go`, docs) touches none of
`session/instance_actor_setters.go`, `session/instance_state.go`, `session/retry_state.go`,
`session/session_driver.go`, `server/services/session_creation_terminal_write.go`, or
`server/services/session_creation_pipeline.go`. A `make quick-check` run on this same branch *before* an
unrelated lint-only fix landed already showed `test-race` passing; the very next run (identical
session/server code, differing only in an unrelated `session/worktree_pr_poller_discovery_test.go` lint
fix) hit this race — confirming it is non-deterministic, load-dependent flakiness, not something either
change introduced.

## Likely Cause (not yet fully confirmed)

**Hypothesis**: `CreateSession`'s background resolution pipeline goroutine (`runBackgroundResolutionPipeline`,
launched via `trackCleanup` at `server/services/session_service.go:370-372`) is not guaranteed to have
finished touching the instance's status before the test proceeds to call `MarkPermanentlyFailedForTest`
and `RetrySession` on the same instance. In isolation the pipeline goroutine finishes fast enough that the
test's subsequent calls never overlap it; under full-suite parallel load, scheduling delay lets the test's
main goroutine race ahead while the pipeline goroutine is still mid-flight.

## Files Likely Affected

- `server/services/session_service_retry_test.go:101-136` — the test itself; may need to wait for
  `CreateSession`'s background pipeline to settle (e.g. via a test hook/channel) before proceeding.
- `server/services/session_service.go` (`trackCleanup`, `CreateSession`) — where the background pipeline
  goroutine is launched with no completion signal exposed to callers/tests.
- `session/instance_actor_setters.go` (`TryForceStatusIfEpoch`) / `session/instance_state.go` (`loadStatus`)
  — confirm whether the underlying status field genuinely needs additional synchronization beyond what's
  already race-flagged here, or whether this is purely a test-sequencing gap.

## Fix Approach

1. Reproduce under artificial load (`go test -race ./server/services/... -count=5`, or `GOMAXPROCS` tuning)
   to confirm it's schedule-dependent rather than count-dependent.
2. If it's a test-sequencing gap: give the test a way to wait for `CreateSession`'s background pipeline to
   complete before calling `MarkPermanentlyFailedForTest`/`RetrySession` (a completion channel/hook), per
   this repo's `fix-flaky-tests-dont-defer` skill's preference for a real synchronization point over a
   fixed sleep.
3. If the underlying status field truly lacks proper synchronization outside of this specific interleaving,
   root-cause via `-race` and fix in `session/instance_actor_setters.go`/`instance_state.go` directly rather
   than only in the test.

## Verification

After the fix: `go test -race ./server/services/... -run
TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed -count=20` must
keep passing, and a full `go test -race -short -timeout=20m ./...` (matching `make test-race`) must not
reproduce this race across several repeated runs.

## Related

- `fix-flaky-tests-dont-defer` skill — this repo's standing rule against re-excusing a known flake without
  root-causing or filing it; this bug is that filing for the instance discovered while verifying backlog
  item `ebfd919f-e97a-404b-a23b-fd626bfbc929`.
- Same family as BUG-083/084/085/090/092 (full-suite-load-only flakes in `server/services`).
