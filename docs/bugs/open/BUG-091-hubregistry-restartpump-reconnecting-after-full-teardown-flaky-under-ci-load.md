# BUG-091: `TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` flakes under full-suite CI load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-26
**Impact**: Intermittent CI failure in `server/services`' test suite — no production impact. Not caused by, or related to, this PR's diff (#615, worktree self-heal fix); this test is pre-existing code from `main` (merged into the PR branch via a `main`-sync merge), not something this PR added or touched.

## Problem Description

`TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` (`server/services/connectrpc_websocket_test.go:1263`) failed once in CI (PR #615's `Test` job, run 33023813275, full `go test ./... -race` suite) with:

```
Error:      Condition never satisfied
Messages:   expected live output to resume after reconnecting to a fully-torn-down hub
```

The failing assertion is a `require.Eventually(..., time.Second, 5*time.Millisecond, ...)` waiting for a frame pushed after reconnect to appear in the subscriber's received frames (`connectrpc_websocket_test.go:1262-1263`). This is the same *symptom shape* as BUG-089/BUG-087 (timing-sensitive polling assertion, 1s budget, tight under full-suite `-race` CPU contention) — the test itself was added by the same PR (#595) that fixed the underlying "restart the output pump when a torn-down hub reactivates" bug (`9c604dacc`), and its own doc comment already anticipates scheduler-timing sensitivity around the pump-exit handoff it's testing (see the `require.Eventually` immediately above the failing one, which exists specifically to avoid a race between the test's own reconnect and the original pump's exit).

## Reproduction Steps

Not yet reproduced in isolation. Observed once in CI (PR #615, run 33023813275) under full-suite `-race` load; re-run of the same job triggered to check for flakiness (see PR #615 for outcome).

## Root Cause

Not yet root-caused. Leading hypothesis, per the symptom shape and this test's own doc comments: the 1-second `require.Eventually` budget for observing a frame arrive after reconnect is tight enough that CPU contention from other packages running `-race` in parallel (the same class of CI-load timing pressure documented in `project_plans/worktree-selfheal-test-flake/research/pitfalls.md` for an unrelated flake) can push the actual pump-restart-and-deliver latency past the assertion's timeout even when the underlying fix (#595) is correct.

## Files Likely Affected

- `server/services/connectrpc_websocket_test.go` (`TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown`)
- `server/services/connectrpc_websocket.go` (`pumpControlModeOutputIntoHub`, the pump-restart logic under test)

## Next Steps

- Attempt local reproduction under amplified/contended load (`go-stress` or `go test -race ./server/services/... -count=N` alongside other packages) per `.claude/rules/fix-flaky-tests-dont-defer.md`.
- If confirmed CI-load-timing-sensitive rather than a real logic bug, widen the `require.Eventually` budget (mirrors the timing-assertion widenings already done in PR #595 for other tests in this same area) rather than loosening the assertion's correctness.
