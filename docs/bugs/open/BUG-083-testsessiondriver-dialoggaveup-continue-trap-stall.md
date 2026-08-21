# BUG-083: TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation stalls in continue trap [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-21
**Impact**: One `session` package test fails reproducibly in isolation, not just under load — signals a real logic issue in `session_driver.go`'s dialog-gave-up fall-through path, not just test flakiness. No confirmed production impact yet (unverified whether the underlying `dialogGaveUp`/inactivity-escalation behavior is actually broken for live sessions, or only the test's simulation of it).

## Problem Description

`TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation` (`session/session_driver_test.go:1230`) fails with:

```
SendKeys count never exceeded 3 — the dialogGaveUp fall-through never reached the initial-prompt-send step (stuck in the continue trap)
```

This reproduced in an isolated single-test run (`go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -race -v -count=1`, ~68s), not just inside a large concurrent full-suite `-race` run — ruling out simple resource-contention flakiness as the sole cause.

## Reproduction Steps

1. `go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -race -v -count=1`
2. Expected: test passes — the simulated dialog-gave-up condition falls through to the inactivity-escalation initial-prompt-send step, and `SendKeys` is called more than 3 times.
3. Actual: `SendKeys` count never exceeds 3; the driver logic appears stuck in what the test's own failure message calls "the continue trap" — i.e., `dialogGaveUp`'s fall-through condition is not being reached.

## Root Cause

Unknown — needs investigation. Not yet determined whether this is:
- A genuine regression in `session_driver.go`'s dialog/nudge/inactivity-escalation state machine, or
- A test-only issue (e.g. the test's simulated turn/nudge sequence no longer matches the driver's current expectations after some unrelated prior change), or
- A timing-sensitive test that is flaky even in isolation under certain system load, though the isolated reproduction above argues against pure flakiness.

**Ruled out**: this is unrelated to the concurrent `terminal-multi-connection-streaming` implementation effort (`session/streamhub/*`, `server/services/connectrpc_websocket.go`, `session/external_streamer_transport.go`, `session/external_tmux_streamer_transport.go`) — none of that work touches `session_driver.go`/`session_driver_test.go`. An attempt to bisect against the pre-streamhub-work baseline (checking out `session/session_driver.go`/`session_driver_test.go` at commit `3818c35e6^`) was started but not completed before the investigating session's environment began killing long-running background test runs; the working tree was restored to `HEAD` without a conclusive bisect result.

## Files Likely Affected

- `session/session_driver.go` — the `dialogGaveUp`/continue-trap/inactivity-escalation state machine logic itself.
- `session/session_driver_test.go:1230` — the failing test and its `SendKeys`-count assertion.

## Fix Approach

Unknown. Next step: re-run the bisect against `3818c35e6^` (or further back) in an environment that isn't killing long-running test processes, to determine whether this predates all recent work or was introduced by some other, unrelated recent change. If it predates everything recent, root-cause the actual `dialogGaveUp` fall-through condition directly.

## Verification

`go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -race -v -count=5` passes consistently (5/5), both in isolation and as part of a full `./session/...` run.

## Related Tasks

Discovered during `sdd:5-implement`'s final full-repo verification pass for `project_plans/terminal-multi-connection-streaming/` (see that project's `implementation/plan.md`) — this bug is unrelated to that project's own scope and does not block it.
