# BUG-092: TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown times out under scheduler contention [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-27
**Impact**: Occasional false-red CI (`Test` job in the `Build` workflow); no production impact.

## Problem Description

`TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` in
`server/services/connectrpc_websocket_test.go:1199` uses `require.Eventually` with fixed
1-second wall-clock timeouts (e.g. line 1263: "expected live output to resume after
reconnecting to a fully-torn-down hub") to wait on goroutine scheduling events. It passes
reliably in quick isolation (15/15 in a `-count=15` run) but failed once in a heavier
`-race -count=20` run, and failed in CI's `Test` job for PR #641 (unrelated web-app-only diff).

## Reproduction Steps

1. `go test ./server/services -run TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown -count=15` — passes reliably.
2. `go test ./server/services -run TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown -count=20 -race` — intermittently fails with "Condition never satisfied" at connectrpc_websocket_test.go:1263.
3. Expected: passes consistently regardless of scheduler load.
4. Actual: fails under heavy contention (full test suite in CI, or locally under `-race`, which adds significant scheduling overhead).

## Root Cause

Same class as BUG-071/BUG-091: the test's `require.Eventually` calls use fixed 1-second
wall-clock budgets to wait on goroutine teardown/restart signaling (`TryStartPump`/
`MarkPumpExited`, subscriber attach/detach). Under CI's full parallel test-package run (or
locally under `-race`, which slows the scheduler substantially), that fixed budget is
occasionally too tight even though the same sequence of goroutine transitions completes well
under 1s in isolation.

## Files Likely Affected

- `server/services/connectrpc_websocket_test.go:1199-1266` — the flaky test and its fixed 1s `require.Eventually` timeouts

## Fix Approach

Raise the `require.Eventually` timeouts to give headroom for full-suite/`-race` scheduler
contention, or replace the wall-clock waits with a synchronization primitive (e.g. a channel
signaled directly by `MarkPumpExited`/pump-restart) that isn't sensitive to scheduler load.

## Verification

Run `go test ./server/services -run TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown -count=50 -race` in a loop; the test should pass consistently without needing isolation from other load.

## Related Tasks

Found while shipping PR #641 for backlog item `0ddc4edb-ae2e-4d85-b9cf-067af72be323`
(useFocusTrap trigger-focus-return) — unrelated to that change, which touches only
`web-app/src/**` test files.
