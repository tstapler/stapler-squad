# BUG-102: `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged` intermittently times out waiting for the abandoned-generation metric [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-08, during `make quick-check` on PR #735 (`fix-control-mode-input-silent-drop`)
— unrelated to that diff (which only touches `session/tmux`, the older backend; this is
`session/tymux`, a different package merged in from `main` via PR #739 in the same session).

## Problem Description

`session/tymux/stream_test.go:916`'s `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`
reopens a session whose old Attach-stream reader is permanently wedged (`Receive()` does a bare
`select {}`, so it can truly never return — see `wedgedAttachStream`'s doc comment), then asserts the
new generation's `session_lifecycle_active_generations{subsystem="tymux_stream"}` OTel gauge settles at
`before+1` within `require.Eventually(t, ..., time.Second, time.Millisecond, ...)` (`stream_test.go:948`).
Intermittently this never becomes true and the test fails with "Condition never satisfied".

```
--- FAIL: TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged (1.10s)
    stream_test.go:948: Error: Condition never satisfied
    Messages: the abandoned generation must stay counted as active — its EndGeneration is never reached
```

## Reproduction

Reproduces in isolation (no other test's state involved — `-run` skips every other test's body
entirely, and nothing in `session/tymux` calls `t.Parallel()`, so no cross-test contamination is
possible):

```
go test ./session/tymux/... -run TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged -race -count=20
```

Observed failure rate is low and inconsistent across runs on this machine (2 failures in ~400 total
iterations across several batches: 1/20, 1/50, then 0/80, 0/150) — genuinely rare, not a "fails every
Nth run" pattern, and the two observed failures both happened during a locally CPU-loaded window (this
session had a concurrent `make quick-check` full-suite `-race` run and several other background test
processes going at the time).

## Root Cause (partial — not confirmed)

**Ruled out**: a too-tight `require.Eventually` bound alone. Widened the bound from `1*time.Second` to
`5*time.Second` (matching `stream.go`'s own `maxTeardownWait = 5 * time.Second` precedent) and reran —
still failed once, at `5.11s`, i.e. it hit the new bound too, not just the old one. This means the
condition isn't merely slow to become true under scheduler contention; something can make it not
become true within a materially longer window too. (Reverted the widened-bound edit — unverified as a
fix, so not worth shipping as a guess.)

**Leading hypothesis, not yet confirmed**: the "before"/"after" gauge reads
(`session/tymux/stream_test.go`'s `collectMetric`/`sumForSubsystem`) go through a package-global
`sdkmetric.NewManualReader()` (`stream_test.go:34`, wired via a package `init()` that calls
`otel.SetMeterProvider` process-wide). The *new* generation's `lifecycle.StartGeneration` call
(`session/tymux/stream.go:121`) happens inside a `go func() { ... }()` spawned by `openStandingStream`
(`stream.go:120-124`) — i.e. asynchronously, after `Start()`/the reopen call has already returned to
the test. `require.Eventually` exists precisely to bridge that async gap, but if `ManualReader.Collect`
(which walks the SDK's internal aggregation state) can race the metric-recording goroutine's write in a
way that isn't simply "not enough elapsed time" — e.g. an OTel SDK internal locking/flush ordering
subtlety, not investigated — that would explain a failure that doesn't resolve just by waiting longer.
Not confirmed; needs either instrumenting `collectMetric` to log the actual observed value on each poll
iteration during a captured failure, or reading through `go.opentelemetry.io/otel/sdk/metric`'s
`ManualReader.Collect` implementation for a known non-monotonic-read caveat with concurrent
`Add`/`Record` calls from another goroutine.

## Files Affected

- `session/tymux/stream_test.go` (test itself, `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`,
  and the shared `collectMetric`/`sumForSubsystem`/`testTymuxMetricReader` helpers other tests in the
  file also depend on)
- `session/tymux/stream.go` (`openStandingStream`'s fire-and-forget generation-tracking goroutine,
  `lifecycle.StartGeneration`/`EndGeneration` call sites)
- Possibly `session/lifecycle` (`StartGeneration`/`EndGeneration`'s OTel instrument recording) or the
  vendored `go.opentelemetry.io/otel/sdk/metric` `ManualReader` itself

## Fix Approach

1. Reproduce reliably first — a tight repeat loop alone hasn't been enough (400 iterations, 2 hits).
   Try artificial CPU load (`stress`/parallel `go build` in the background) or `GOMAXPROCS=1` while
   looping, mirroring BUG-090/BUG-091's own reproduction notes for this "needs genuine scheduler
   contention" class of flake.
2. Once reproduced under a debugger/instrumented run, log the actual gauge value on every
   `require.Eventually` poll iteration (not just the final failure) to see whether it's stuck at
   `before` (metric genuinely never recorded — a real synchronization bug) or oscillating/overshooting
   (an aggregation or attribute-matching bug in `sumForSubsystem`/the OTel SDK path).
3. Do not ship a bare timeout-widening as the fix without first confirming the failure mode via step 2
   — this bug's own investigation already showed a 5x wider bound didn't guarantee success once.

## Verification

After fix: `go test ./session/tymux/... -race -count=200 -run TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`
must pass every iteration, including at least one run performed under deliberate background CPU load to
match the conditions both observed failures occurred under.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — this repo's standing rule against re-excusing a known
  flake without root-causing or filing it; this bug is that filing.
- `docs/bugs/open/BUG-091-claudesettingswatcher-tempdir-cleanup-flake-under-full-suite-load.md` and
  `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md` — sibling
  full-suite/scheduler-contention-only flakes in unrelated packages, same discovery pattern (surfaced
  while verifying an unrelated diff's `make quick-check`/full-suite run).
