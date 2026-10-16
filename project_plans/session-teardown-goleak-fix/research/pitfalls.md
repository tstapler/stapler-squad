# Pitfalls: session-teardown-goleak-fix

## 1. Prior art for this exact bug class

No `docs/bugs/fixed/*.md` covers a goleak/goroutine-teardown false positive
directly (`grep -rli goleak docs/bugs/fixed/` — no hits; `grep -rli leak` hits
are unrelated: eventbus nil-context leak, rate-limiter global-state leak,
etc.). The closest true precedent isn't a bug doc — it's already-shipped code
in this package:

- `session/instance.go:456-466` — `driverWG`/`hibernateWG` `sync.WaitGroup`
  fields, added specifically "so tests that don't call Destroy can still wait
  for the goroutine to finish before their tempdir is removed."
- `session/session_driver.go:227` `JoinSessionDriver` and
  `session/instance_hibernate.go:158` `JoinHibernation` — both call
  `internal/syncutil.WaitWithTimeout(&wg, stopJoinTimeout)` and **log.Warn on
  timeout, not `t.Fatal`**. `stopJoinTimeout = 10 * time.Second`
  (`session/pty_discovery.go:28`).

This is the pattern to extend, not invent fresh. But note the acceptance
criteria (`requirements.md` #5: "a clear test failure message on expiry")
wants louder-than-`log.Warn` behavior — the existing `Join*` helpers
deliberately *don't* fail the test, they just warn and let goleak's own
retry/snapshot catch it downstream. Reusing `JoinSessionDriver`'s exact
soft-warn shape for the new mechanism would silently reproduce the same class
of flake it's meant to close, not fix it.

## 2. Timeout value under `-race`

Two existing constants bound this exact family of wait:
- `stopJoinTimeout = 10s` (`session/pty_discovery.go:28`) — doc comment
  reasons from `refreshRate` (5s) × a small multiple, "generous margin
  without risking an indefinite hang."
- `defaultDeleteSessionCleanupTimeout = 5s`
  (`server/services/session_service.go:354`) — explicitly made
  test-overridable (`SetDeleteSessionCleanupTimeout`) "so tests can shrink it
  ... instead of waiting out the real 5 seconds," mirroring
  `BacklogService.triageCleanupTimeout`.

Neither constant's doc comment cites a `-race`-specific tuning history (no
"had to bump this because -race was too slow" note found in either file or in
`server/server_integration_test.go`, which itself uses flat `5*time.Second`/
`30*time.Second`/`60*time.Second` waits with comments only about CI
contention, not `-race` per se, e.g. line ~545: "contended CI runner running
the full `-race` suite in parallel that can take much longer"). Signal: **10s
is the largest existing precedent for "join one background loop," and even
that is described as generous for a 5s-cycle poller** — for 5-6 independent
goroutines (streamLoop, executionLoop, runStatusChangeLoop, pollLoop, mangle
eviction, tmux diagnostic Cmd.Wait), a single shared timeout of 10s is
probably fine if they're waited on concurrently (one `WaitWithTimeout` call
over one aggregate `WaitGroup`), but would be too tight if implemented as 5-6
*sequential* joins at 10s each (worst case ~60s added to a test under `-race`
load). Aggregate into one `WaitGroup`, don't chain sequential per-component
waits.

## 3. Fan-in ordering hazard: `Add()`-after-`Wait()` race

This is the sharpest risk. Confirmed from `session/instance.go:440-449`'s own
doc comment on `driverMu`: `CreateSession`'s **async** initialization goroutine
(`server/services/session_service.go` ~line 2293, `[CreateSession] async
start` — tracked via `trackCleanup`, not a bare `go func()`) calls
`instance.Start(true)` and `StartSessionDriver` **well after the RPC has
already returned**. That existing comment documents a real, previously-fixed
race: "a fast-following `DeleteSession`'s `Destroy()`... can call
`StopSessionDriver` after `StartSessionDriver` has already run... orphaning a
driver goroutine that nothing will ever signal to stop" — fixed by serializing
both under `driverMu`.

`TestSessionService_CreateThenImmediateDelete_NoDataRace` is *literally* this
race by name. Tracing the component-goroutine launch sites:
- `session/claude_controller.go:345,365,368` — `rs.Start(innerCtx)` then `go
  cc.runStatusChangeLoop(innerCtx)`, and `exec.Start(innerCtx)`
- `session/response_stream.go:177,185` — `rs.Start` itself spawns `go
  rs.streamLoop(innerCtx)`
- `session/command_executor.go:148` — `ce.Start` spawns `go
  ce.executionLoop(innerCtx)`
- `session/detection/ratelimit/integration.go:125` — `go pc.pollLoop(ctx)`
- `pkg/analytics/mangle_correlator.go:180` `StartEviction`, called from
  `pkg/analytics/escape_code_parser.go:147`

All of these are reachable only through the same async `instance.Start` chain
launched from `CreateSession`'s post-RPC goroutine. **If the new join
mechanism calls `wg.Add(1)` inside each component's own `Start`/spawn point
rather than synchronously before `CreateSession`'s async goroutine returns to
the caller, a fast-following `DeleteSession` can call the new `Wait`/`Join`
helper before `Add()` has run for one or more components** — `sync.WaitGroup`
requires `Add` to happen-before the corresponding `Wait` returns 0; a `Wait()`
called with counter 0 (because `Add` hasn't executed yet) returns immediately,
masking exactly the goroutine the fix exists to catch. This mirrors the
`driverMu` bug precedent exactly, just one layer of components down. The fix
needs the same fix shape: either (a) make `Add()` happen synchronously as part
of `Instance.Start`'s critical section (guarded by the existing `driverMu` or
an equivalent lock) before it returns, so `DeleteSession`/`Destroy()` can never
observe "not yet added," or (b) have `Destroy()` itself be the single place
that decides whether each component was ever started (checking state under
the same lock) and only wait on the ones that were.

## 4. Test-suite slowdown risk if the pattern spreads

`waitForTmuxTeardown` (`server/server_integration_test.go:628`) is already
called from at least 4 other tests in the same file (`TestSessionService_...`
tests around lines 351, 435, 506) with a flat `5*time.Second` timeout each —
these are the natural adoption sites if this new join helper generalizes
beyond the one flaky test. Each is a **timeout ceiling, not the typical-case
cost** — `WaitWithTimeout`/`wait.WaitForCondition` both poll and return as
soon as the condition is true, so the realistic added latency per test is
however long the 5-6 goroutines actually take to unwind (should be
milliseconds once they're signaled to stop), not the full timeout. The real
slowdown risk is the *failure* path: if this becomes copy-pasted into several
tests and the underlying teardown ever regresses (e.g. one component stops
responding to its cancellation signal), every test using the helper eats the
full timeout serially before failing, multiplying one regression into
several-times-N seconds of CI time. Keep the new helper as one shared,
single-call join point (following `JoinSessionDriver`/`JoinHibernation`'s
existing model of "one function per Instance, called once") rather than
letting each call site re-implement its own per-component wait loop.
