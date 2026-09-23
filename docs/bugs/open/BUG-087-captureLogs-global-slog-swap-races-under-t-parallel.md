# BUG-087: `captureLogs()` global `slog.SetDefault` swap races across `t.Parallel()` tests [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-23
**Impact**: Intermittent, non-deterministic failures in `server/services` test suite — no production impact, but erodes trust in CI red as a signal for this package. Currently hit at least `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug`; likely also hits every other test in the file that calls `captureLogs()`.

## Problem Description

`server/services/autonomous_orchestration_service_test.go:418-425`'s `captureLogs(t *testing.T) *bytes.Buffer` helper swaps the process-global default logger via `slog.SetDefault(...)` for the duration of the calling test, restoring the previous default on cleanup. Multiple tests in this file call `t.Parallel()` (10+ call sites, e.g. lines 60, 72, 92, 118, 147, 178, 251, 300, 368) and several of those also call `captureLogs()` (e.g. lines 451, 501). When two such tests run concurrently, both are writing to (and racing to restore) the single process-global `slog` default — a test's captured buffer can pick up log lines emitted by a *different*, concurrently-running test, not just its own code path under test.

The code already has a doc comment (`autonomous_orchestration_service_test.go:409-416`) acknowledging this exact risk: "`-race` never flags concurrent swaps, but two `t.Parallel()` tests both redirecting it..." — i.e., this was a known, accepted risk at write time, not a latent surprise, but the tests were shipped calling `t.Parallel()` anyway.

## Reproduction Steps

1. Run the full package: `go test ./server/services/... -count=1`
2. Occasionally, `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug` fails with an assertion that its captured log buffer contains a `level=WARN` line it didn't expect.
3. Expected: the test's captured buffer contains only log output from its own code path (a DEBUG-level "not-linked" log line).
4. Actual: the buffer additionally contains a `level=WARN msg="failed to resolve origin main tip, falling back to ambient HEAD"` line that is unrelated to this test — traced to a concurrently-running test (`TestSpawnSessionFromItem_WIPLimit_AllowsReopenAtCap...`, inferred from the embedded temp-dir path in the leaked log line) exercising an unrelated go-git fetch-failure path at the same moment.
5. Confirmed non-reproducible in isolation: `go test ./server/services/... -run 'TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug' -count=5 -v` passes 5/5 — the failure only manifests when run alongside the rest of the package's parallel tests.

## Root Cause

Confirmed (not speculative): shared mutable global state (`slog`'s process-wide default logger) mutated by `captureLogs()` without any synchronization, combined with `t.Parallel()` on multiple tests in the same file that use it. `-race` does not catch this because `slog.SetDefault` itself is internally synchronized (no data race on the pointer swap) — the bug is semantic (test isolation), not a memory-safety race.

## Files Likely Affected

- `server/services/autonomous_orchestration_service_test.go` — `captureLogs` (and its siblings `captureInfoLog`/`captureErrorLog` per the doc comment at line 411) plus every test in this file that combines `t.Parallel()` with a log-capture call.

## Fix Approach

Two viable directions, either is a real (not one-line) fix — this is why it wasn't fixed inline in the unrelated PR (#605) that surfaced it:

1. **Remove `t.Parallel()`** from every test in this file that calls a log-capture helper, sacrificing this file's test-parallelism for correctness. Simplest, but slows this file's test run and doesn't fix the underlying global-state pattern if it's reused elsewhere.
2. **Redesign `captureLogs`/`captureInfoLog`/`captureErrorLog`** to avoid mutating the process-global `slog` default at all — e.g. have the code under test accept an explicit `*slog.Logger` (dependency injection) so each test can pass its own private logger/buffer with no shared global, or use a `t.Cleanup`-scoped `context.Context`-carried logger if the call sites support it. More invasive but fixes the pattern at its root and would be the more durable choice if this helper is copy-pasted elsewhere in the codebase (worth a repo-wide grep for `slog.SetDefault` in test files before choosing).

Recommend option 2 if the code under test can be reasonably threaded with an explicit logger; fall back to option 1 as a fast, low-risk stopgap if not.

## Verification

After the fix: run `go test ./server/services/... -count=20` (or under `go test ./server/services/... -run '<affected tests>' -count=50 -parallel N` with N ≥ number of affected parallel tests) with no failures, specifically stress-testing concurrent execution of tests that call the log-capture helpers alongside other `t.Parallel()` tests in the same package that emit WARN-level logs (e.g. anything exercising a go-git fetch-failure fallback path).

## Related Tasks

Discovered while running `sdd:6-verify`/`github:pr-ship`'s local test gate for PR #605 (`stapler-squad-web-transport` branch, project `web-transport-architecture-review`) — confirmed unrelated to that PR's diff (which touches `server/server_integration_test.go`, `server/services/ws_stream_bridge_test.go`, `server/services/watch_sessions_native_streaming_integration_test.go`, `testutil/tls.go`, and `web-app/`, none of which touch `autonomous_orchestration_service_test.go` or `captureLogs`).
