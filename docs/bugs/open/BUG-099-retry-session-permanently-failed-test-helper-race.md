# BUG-099: Data race between MarkPermanentlyFailedForTest and in-flight CreateSession background pipeline [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-05
**Impact**: Intermittent `go test -race` failures in `server/services` for `TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed`. Test-only helper, not a production data race, but it can flake `make ci`/`make ready` and CI's race-enabled test jobs.

## Problem Description

`Instance.MarkPermanentlyFailedForTest()` (test helper) writes/reads `Instance` fields directly rather than routing through the actor's serialized-write channel. When the test calls it shortly after `SessionService.CreateSession()` returns, a background resolution-pipeline goroutine spawned by `CreateSession` (via `trackCleanup`) can still be calling `SetCreationProgress`/`TryForceStatusIfEpoch` on the same `*Instance` concurrently — `go test -race` flags this as two separate data races (one on `buildSnapshot`'s field reads, one on `FailureReason`).

## Reproduction Steps

1. `git worktree add /tmp/x origin/main` (unmodified `main`, confirmed at `54d2a6b85`)
2. `make build`
3. `go test -race -run TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed -count=5 ./server/services/...`
4. Expected: all 5 iterations pass cleanly under `-race`.
5. Actual: intermittent (~1/5 in this run) `WARNING: DATA RACE` failures.

## Root Cause

Not fully root-caused. Working hypothesis: `MarkPermanentlyFailedForTest` (`session/retry_state.go:430`) bypasses the `Instance` actor's normal serialized mutation path and directly triggers `markSessionPermanentlyFailed` → `buildSnapshot` (`session/session_driver.go:934`, `session/instance_snapshot.go:170`), which races against the still-running background pipeline goroutine from the test's earlier `CreateSession` call (`server/services/session_creation_pipeline.go:265` and `:341`, via `trackCleanup` at `server/services/session_service.go:387-389`) writing `Instance` state via `SetCreationProgress`/`TryForceStatusIfEpoch` (`session/instance_actor_setters.go:64`, `:77`, `:604`). A second read/write race on `FailureReason` (`session/instance_state.go:176`) vs. `setFailureReasonLocked` (`instance_actor_setters.go:77`) was observed in the same run, consistent with the same underlying "test didn't wait for the pipeline to quiesce" gap.

## Files Likely Affected

- `session/retry_state.go` — `MarkPermanentlyFailedForTest` (the helper bypassing actor serialization)
- `session/session_driver.go:934` — `markSessionPermanentlyFailed`
- `session/instance_snapshot.go:170` — `buildSnapshot`
- `server/services/session_creation_pipeline.go:265,341` — `runBackgroundResolutionPipeline`
- `server/services/session_service_retry_test.go:101-136` — the test itself; may need to wait for `CreateSession`'s background pipeline to settle before calling the helper

## Fix Approach

Unknown — two candidate directions: (1) route `MarkPermanentlyFailedForTest` through the actor's serialized-write channel (`sendSyncErr` or similar) instead of touching fields directly, or (2) have the test wait for `CreateSession`'s background resolution pipeline to fully complete/quiesce before forcing permanent-failure state via the helper.

## Verification

`go test -race -run TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed -count=20 ./server/services/...` clean.

## Related Tasks

Discovered while verifying `fix/control-mode-race` (a separate, now-fixed `tmux` control-mode data race) — confirmed via a clean `origin/main` worktree that this race pre-exists and is unrelated to that fix.
