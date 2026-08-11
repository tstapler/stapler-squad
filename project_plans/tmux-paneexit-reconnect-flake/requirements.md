# Requirements: Fix flaky TestTmuxServerRegistry_PaneExitChannel

Source: backlog item `1732151a-a9c9-4cdb-9c33-b31fffa9bfa7`. Derived directly from
the item description and acceptance criteria (no interactive interview — this is an
unattended backlog work session).

## Problem

`TestTmuxServerRegistry_PaneExitChannel` (`session/tmux/server_registry_integration_test.go`)
intermittently fails with:

```
server_registry_integration_test.go:185: SubscribePaneExit channel not closed within 3s after kill-session
```

Reproduced on unmodified `origin/main` (`aa29bccd2`), including in isolation
(`-count=3`), so it is a genuine pre-existing flake, not induced by unrelated work.

### Root cause

`TmuxServerRegistry` (`session/tmux/server_registry.go`) detects a session's
disappearance two ways:
1. Live control-mode events (`%session-closed`, `%pane-exited`) while the
   control-mode connection is up.
2. `syncSessions()` diffing, which currently only runs (a) once at `Start()`,
   (b) once per `reconnectLoop` iteration right after a (re)connect succeeds, and
   (c) 50ms after a `%sessions-changed` event *while connected*.

When the control-mode connection is down and `reconnectLoop` is in its exponential
backoff wait (`100ms → 200ms → 400ms → 800ms → 1.6s → ...`, `reconnectLoop`,
`server_registry.go:307-400`), no `syncSessions()` call happens at all until the
*next* reconnect attempt completes. If `kill-session` happens while backoff has
already grown past ~1-1.5s (e.g. because the isolated test socket wasn't
immediately reachable when the registry first connected), the remaining budget in
the test's fixed 3s deadline is not enough for backoff-wait + reconnect-attempt +
`syncSessions()` to complete. Detection latency is thus *bound by backoff*, which
is unbounded-ish (caps at 30s) and uncorrelated with how quickly a test (or a real
caller) needs to observe the exit.

## Goals

1. Decouple pane-exit *detection* latency from the control-mode reconnect backoff
   schedule, so a bounded number of independent, fast re-sync attempts run even
   while `reconnectLoop` is sleeping out a long backoff.
2. Give that bound a concrete, documented ceiling (per AC 5:
   `fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`),
   marked with a `ponytail:`-style comment naming the ceiling and why it must stay
   decoupled from backoff.
3. Fix the actual race, not the test's deadline — do not simply raise the 3s
   timeout.
4. Leave the rest of `session/tmux` behavior unchanged: no regression to
   `TestEnsureServerRunning_NoOp`, `TestKillOrphanedControlModeClients`, or any
   other test in the package.
5. Add a regression test,
   `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`, that
   specifically exercises detection while backoff is artificially elevated, so the
   fix is verified structurally and not just by re-running the flaky test until it
   happens to pass.

## Acceptance Criteria (verbatim from backlog item)

1. `TestTmuxServerRegistry_PaneExitChannel` passes reliably: 20/20 consecutive runs
   of `go test -race -tags integration ./session/tmux -run TestTmuxServerRegistry_PaneExitChannel -count=20`
   succeed locally.
2. The fix addresses the reconnect race itself (a readiness/synchronization point
   decoupled from backoff) rather than only enlarging the test's fixed deadline.
3. No regression to the rest of `session/tmux`:
   `go test -race -tags integration ./session/tmux/...` passes, including the two
   named sibling flakes (`TestEnsureServerRunning_NoOp`,
   `TestKillOrphanedControlModeClients`) and the new
   `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` regression test.
4. `make ci` continues to pass (build, unit tests, integration tests, lint).
5. Because the root cause is structural (backoff-bound detection latency), the fix
   is documented with a `ponytail:`-style comment naming the concrete latency
   ceiling (`fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`)
   and why it must stay decoupled from backoff.
6. No changes to unrelated files/behavior — diff confined to
   `session/tmux/server_registry.go` and
   `session/tmux/server_registry_integration_test.go`, verified mechanically
   (`git diff --name-only`), not just narratively.

## Non-goals

- Reworking the reconnect/backoff algorithm itself (it stays as-is for connection
  retry purposes — only detection gets a decoupled fast path).
- Touching `TestEnsureServerRunning_NoOp` / `TestKillOrphanedControlModeClients`
  beyond confirming they still pass (they are out of scope per the backlog item's
  file-confinement constraint, AC 6).
- Any change outside `session/tmux/server_registry.go` and
  `session/tmux/server_registry_integration_test.go`.

## Constraints

- Must not introduce a new dependency.
- Must respect the existing `subsMu`/close-outside-lock discipline documented at
  `server_registry.go:48-50` (never `close(ch)` while holding `subsMu`).
- Must not change the exported API surface used elsewhere in the codebase
  (`SubscribePaneExit`, `SessionExists`, `ListSessions`, `IsHealthy`, `Start`,
  `Stop`, `GetServerRegistry`, `StopServerRegistry`, `RemoveServerRegistry`) —
  those are consumed by non-tmux packages (`session/`, `server/services/`).

## Post-implementation finding (superseded below): a second, independent race

A prior pass through this item found the backoff-decoupling fix above did not reach a
clean 20/20 on its own and documented two suspected causes: candidate #2 (backoff-bound
detection latency, fixed) and candidate #1 ("the isolated test server itself dying",
undiagnosed). A review pass adversarially re-ran `-count=20` and confirmed the residual
~10-15% failure rate, correctly rejecting "cannot be honestly satisfied" as a stopping
point per `.claude/rules/fix-flaky-tests-dont-defer.md`.

Further root-causing (this pass) found candidate #1 was itself two things conflated:

**2a. Test config pollution (real, fixed, but not the dominant cause).** Every "isolated"
test server was spawned without `-f`, so it silently loaded this developer's real
`~/.tmux.conf` — including `run '~/.tmux/plugins/tpm/tpm'`, which forks several `tmux
<subcommand>` calls against the brand-new server as part of config load. Fixed by passing
`-f /dev/null` on the command that starts each isolated server
(`newSessionWithRetry`). This is legitimate test-hygiene regardless of its impact on the
flake rate — an "isolated" test server should not depend on the operator's interactive
config — but measured alone it left a comparable residual failure rate, so it was not the
dominant contributor.

**2b. A missed-session-creation race in the test scaffolding (the actual dominant cause).**
Root-caused with tmux's own server-side `-v`/`-vv` protocol log (`tmux -L <socket> -v
new-session ...` writes `tmux-server-<pid>.log` with a timestamped trace of every command
and notification): `startIsolatedRegistry` returned as soon as `registry.Start(ctx)` was
called, *without* waiting for the control-mode client to actually finish attaching. Tests
then immediately created their target session (e.g. `testpaneexit`) via a second,
independent tmux client racing the registry's own control-mode `attach-session` command.
When the test's session-create won that race, it landed in a gap tmux's control-mode
protocol cannot close after the fact: the session was created *before* the control client
subscribed, so tmux never emitted `%session-created`/`%sessions-changed` for it to that
client (event delivery is not retroactive), and — since the control-mode connection then
stayed healthy with no further drops — nothing ever triggered another `syncSessions()`
pass before the test's poll timeout. The session was not slow to appear; it was
permanently invisible for the rest of that test run. Confirmed directly in the captured
log: `"session not visible in registry before subscribing"` failing at the full
`registryPollTimeout` with *zero* reconnect/backoff log lines in between — i.e. the
connection never dropped, so `reconnectLoop`'s own periodic re-sync (the mechanism
candidate #2's fix improved) never got a chance to run at all. This explains why the
residual failures didn't consistently match either previously-suspected signature: some
runs surfaced as `"SubscribePaneExit channel not closed"` (the session was visible, but a
*subsequent* sync after `kill-session` lost the same kind of race on the deletion side)
and others as `"session not visible ... before subscribing"` (this creation-side race),
depending on which side of `startIsolatedRegistry`'s missing readiness wait a given run's
timing happened to land on.

**Fix**: `startIsolatedRegistry` now blocks on `registry.IsHealthy()` (via the existing
`pollUntil` helper, `registryPollTimeout` budget) before returning. `IsHealthy()` is only
set `true` after the post-connect `syncSessions()` round trip succeeds, which requires a
live response from a server that has therefore already processed the earlier-submitted
`attach-session` command too (tmux processes commands in receipt order via a single event
loop) — so any session a test creates after `startIsolatedRegistry` returns is
guaranteed to be created *after* the control client has subscribed, closing the gap
structurally rather than by widening a timeout.

**Verification**: with both 2a and 2b applied, `go test -race -tags integration
./session/tmux -run TestTmuxServerRegistry_PaneExitChannel`:
- `-count=40`: 40/40
- `-count=20` × 3 consecutive runs: 20/20, 20/20, 20/20 (60/60)
- `-count=100`: 100/100

260/260 across five independent invocations, zero failures — AC1 as literally written is
satisfied. `go test -race -tags integration ./session/tmux/...` (full package, including
`TestEnsureServerRunning_NoOp`, `TestKillOrphanedControlModeClients`, and the
`TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` regression test) passes
cleanly across multiple runs.
