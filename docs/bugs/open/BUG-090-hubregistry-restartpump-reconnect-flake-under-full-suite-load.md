# BUG-090: TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown flakes under full `server/services` suite load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-26 (during Epic 1.1 `async-session-creation` implementation — unrelated to that
diff; surfaced by running the full `server/services` suite as a verification step)
**Impact**: Test-only. `go test ./server/services/... -timeout 20m` intermittently reports one failure
(`Condition never satisfied — expected live output to resume after reconnecting to a fully-torn-down
hub`, `connectrpc_websocket_test.go:1263`) among ~2000 tests. Re-running the same test in isolation
(`-run TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown`) passes reliably.

## Problem Description

`TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` (`server/services/connectrpc_websocket_test.go:1199`)
runs `t.Parallel()` and relies on several `require.Eventually` windows of exactly `1 * time.Second`
(lines ~1218, ~1244, and the one that fails at line 1263) to observe:

1. the first connection's frame delivered,
2. the original pump goroutine having exited after `ForceTeardown` + closing `controller.updates`,
3. a frame delivered on the *second* (post-reconnect) transport.

Under the full package's ~2000-test, heavily-parallel run, scheduler contention appears to push one of
these 1-second windows past its deadline occasionally — the isolated single-test run has no such
contention and passes every time observed.

## Reproduction Steps

1. `go test ./server/services/... -timeout 20m` — occasionally reports this test as `FAIL`.
2. `go test ./server/services/... -run 'TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown' -v` — passes.
3. Not yet reproduced with `-race` or a tight repeat loop (`-count=N`) in isolation; needs the full-suite
   parallel load to reproduce, which makes it awkward to bisect without a dedicated flake-hunting session.

## Root Cause (partial — not yet fully confirmed)

**Hypothesis, not yet verified**: one or more of the three `require.Eventually(..., time.Second, 5*time.Millisecond, ...)`
windows in this test is too tight when the process is running ~2000 other tests' goroutines
concurrently (many of them also spawning real tmux/PTY/streamhub goroutines elsewhere in this large
package). This is consistent with the failure only appearing under full-suite load and never in
isolation, but the actual scheduler-delay mechanism (GC pause, GOMAXPROCS goroutine queueing, or a
specific contended lock inside `streamhub`/`hubRegistry`) has not been isolated with `-count` + a
CPU-loaded machine or `GOMAXPROCS=1` repro.

## Files Affected

- `server/services/connectrpc_websocket_test.go` (test itself, lines ~1199-1270)
- Possibly `session/streamhub/` internals if the real root cause turns out to be a genuine race rather
  than a test-timing budget issue — not yet ruled out.

## Fix Approach

1. Reproduce reliably: run the full suite (or a targeted subset that reproduces scheduler pressure) in a
   loop (`go test ./server/services/... -count=5`) or under artificial load (`GOMAXPROCS=1`, or `stress`
   from golang.org/x/tools) until it flakes again, ideally with `-race` to rule out a genuine data race
   in `hubRegistry`/`streamhub.StreamHub`'s pump-restart path rather than a pure timing budget.
2. If it's a timing budget issue: widen the three `require.Eventually` windows (or replace with an
   explicit signal — e.g. a channel closed when `MarkPumpExited`/pump-restart actually completes —
   instead of polling with a fixed deadline), following this repo's own guidance in
   `.claude/rules/fix-flaky-tests-dont-defer.md` to prefer a real synchronization point over a longer
   timeout.
3. If it's a genuine race in the pump-restart path: root-cause via `-race` output and fix the underlying
   synchronization bug in `hubRegistry`/`StreamHub`, not just the test.

## Verification

After fix: `go test ./server/services/... -race -count=10 -run TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown`
must pass every iteration, and the full-suite run (`go test ./server/services/... -timeout 20m`, several
repeats) must not reproduce the flake.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — this repo's standing rule against re-excusing a known
  flake without root-causing or filing it; this bug is that filing for the instance discovered during
  Epic 1.1 (`async-session-creation`).
