# BUG-087: `TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches` flaky under CI timing [SEVERITY: Low]

**Status**: ✅ FIXED (2026-08-24, commit `39751d4c6`, same fix as BUG-090)
**Resolution**: Root cause was not test timing but a real ordering bug in `ForceTeardown` — `State()` could observe `HubTornDown` before `StopControlMode` had been called. Fixed by only flipping state after `StopControlMode` returns. Verified `go test ./session/streamhub/... -run TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches -race -count=50` all green.
**Discovered**: 2026-08-23, during `github:pr-ship` CI run for PR #609 (`fix(session): durable signal + badge for lost-history cold restore`, session-revive-uuid-loss).
**Impact**: Intermittent CI failure on `go test ./session/streamhub/...` with coverage instrumentation. Not reproducible locally (5/5 passes in isolation, 5/5 passes for the full package with `-count=1`, no `-race` issue observed).

## Problem Description

CI run (https://github.com/tstapler/stapler-squad/actions/runs/32660257837/job/97245558816, `Run tests with coverage (pinned tmux 3.4)` step) failed with:

```
--- FAIL: TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches (0.03s)
    lifecycle_test.go:132: expected StopControlMode called exactly once, got 0 calls
FAIL
coverage: 92.8% of statements
FAIL	github.com/tstapler/stapler-squad/session/streamhub	3.745s
```

Every other package and test in the same CI run passed (27/28 other checks green, including the rest of `go test ./...` with coverage). This was the only failure in the entire run.

## Confirmed unrelated to PR #609

PR #609's diff touches `proto/session/v1/types.proto`, `server/adapters/instance_adapter*.go`, `server/services/session_service*.go`, `session/instance*.go`, `session/storage.go`, and `web-app/src/components/sessions/*` only — no file under `session/streamhub/`. Local verification on the same tree:
- `go test ./session/streamhub/... -run TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches -v -count=5` — 5/5 pass.
- `go test ./session/streamhub/... -count=1` x5 — 5/5 pass (full package, no isolation needed).

## Likely Cause

The test asserts a grace-period-delayed teardown callback (`StopControlMode`) fires exactly once after the last subscriber detaches. `lifecycle_test.go:132`'s assertion firing with "got 0 calls" suggests the test's wait/poll for the grace-period timer to elapse raced ahead of the timer actually firing — consistent with a fixed-duration `time.Sleep`/short timeout in the test being too tight for a loaded CI runner (this run's `Test` job took ~28 minutes total, coverage-instrumented, likely under heavier scheduling contention than a local run). Not yet root-caused; needs someone to read `session/streamhub/lifecycle_test.go` around line 132 and the grace-period scheduling code it exercises.

## Fix Approach

Not yet investigated. Likely candidates:
- Replace a fixed `time.Sleep` wait with a polling `require.Eventually`/condition-based wait for the grace-period callback.
- If the grace period itself is a short fixed duration in the test, make it configurable/injectable and use a duration with more margin under CI load, or a fake clock.

## Related

Possibly related to BUG-086 (`session/tmux/control_mode.go` `StartControlMode`/`StopControlMode` refcounting race) given both involve `StopControlMode` call-count expectations, but the failure signature here is a timing/scheduling issue in `session/streamhub`'s own test, not a data race in `session/tmux`. Not confirmed to share a root cause — flagged for whoever investigates either to check.

Logged per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than re-excused silently. Not fixed as part of PR #609 since it requires reading and understanding `session/streamhub/lifecycle_test.go`'s grace-period test harness, which is unrelated to and outside the scope of the AC3 notification/badge work that PR ships.
