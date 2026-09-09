# BUG-103: Two `TestTriggerTriage_*` tests intermittently fail their `require.Eventually` bound under full-package parallel load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-09, running `go test ./server/services/...` full-package (not `-run`-scoped)
while verifying an unrelated `/quality:reflect-and-fix` fix (BUG-free ambient-env test isolation,
commits `f26d5bd04`/`91842ea95`) — unrelated to that diff, which only touches `TestMain`/env handling.

## Problem Description

Two tests in `server/services/backlog_service_triage_test.go` each poll for an async condition via
`require.Eventually` with a fixed timeout, and intermittently fail that bound only when run as part
of the full package's parallel test suite on a heavily loaded machine (this session's machine was
concurrently running the live production `stapler-squad` service at 300%+ CPU plus several other
Claude sessions):

```
--- FAIL: TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown (6.42s)
    backlog_service_triage_test.go:4947: Error: Condition never satisfied
    Messages: all 8 occupiers must have actually entered CallBlocking before the 9th is triggered

--- FAIL: TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors (5.60s)
    backlog_service_triage_test.go:3524 (via waitForTriageFailureCaptured)
             backlog_service_triage_test.go:3603
    Messages: expected the triage ItemSession to record a non-empty FailureCapturePath
```

`TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown`
(`backlog_service_triage_test.go:4947`) waits up to **2s** for 8 goroutines it just spawned via
`TriggerTriage` to each reach `pool.CallBlocking` (`pool.callCount() == 8`). Under scheduler
contention, 8 freshly-spawned goroutines racing to get CPU time within 2s is a tight bound.

`TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors` (`:3603`) calls the
shared `waitForTriageFailureCaptured` helper (`:3521`), which waits up to **5s** for a single async
triage call (with a simulated LLM error) to fully complete, log the failure capture file to disk, and
persist it via storage — also a fixed wall-clock bound competing with every other parallel test in the
package for goroutine scheduling and disk I/O.

## Reproduction

Both pass reliably in isolation, confirming this is scheduler-contention-only, not a logic bug:

```
go test ./server/services/... -run 'TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown|TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors' -v -count=1
--- PASS: TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors (3.17s)
--- PASS: TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown (5.06s)
```

Only observed once each, during one full-package `-v` run (`go test ./server/services/... -v`,
~350s wall clock, hundreds of parallel tests) on a machine with heavy concurrent CPU load from
an unrelated live process. Not yet reproduced in a tight repeat loop — occurrence rate under full
package load is unknown (n=1 for each).

## Root Cause (not confirmed)

Not yet root-caused to a specific scheduling mechanism — no instrumentation added to log actual
elapsed time or intermediate poll values during a captured failure. The working hypothesis is plain
CPU starvation: both bounds (2s, 5s) assume the goroutines under test get scheduled promptly, which
doesn't hold when hundreds of other parallel subtests and an unrelated 300%+ CPU external process are
competing for the same cores. This matches the same "full-suite/scheduler-contention-only" class
already tracked for other packages (see Related, below) rather than a package-specific defect in
`backlog_service_triage_test.go` or the triage pipeline itself.

## Files Affected

- `server/services/backlog_service_triage_test.go`:
  - `TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown` (`:4927`, bound at `:4947`)
  - `waitForTriageFailureCaptured` (`:3521`, bound at `:3536`) and its caller
    `TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors` (`:3583`)

## Fix Approach

1. Reproduce reliably first, ideally under deliberate background CPU load (mirrors BUG-090/091/102's
   own reproduction notes for this flake class) rather than widening bounds on a single observed
   failure — one occurrence each isn't enough to confirm a wider bound actually fixes it (BUG-102's
   own investigation found a 5x-wider bound still failed once).
2. If confirmed as pure scheduling contention (not a real synchronization gap), the fix is most likely
   widening both `require.Eventually` timeouts to a value proportional to the operation's real cost
   under load (e.g. 5s→15s, 2s→10s) rather than a structural change — but confirm via step 1 first.
3. Consider whether `TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown`'s
   first wait (8 goroutines reaching `CallBlocking`) could instead synchronize on a channel/WaitGroup
   the fake pool signals directly, removing the wall-clock guess entirely per the
   `deterministic-fast-tests` skill — likely the more robust fix if practical without a larger
   `fakeHeadlessPool` API change.

## Verification

After fix: both tests must pass reliably under `-count=50`+ while a background CPU load generator
(e.g. `stress` or several parallel `go build` invocations) runs concurrently, matching the load
conditions the original failure occurred under.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — this repo's standing rule against re-excusing a known
  flake without root-causing or filing it; this bug is that filing.
- `docs/bugs/open/BUG-102-tymux-openstandingstream-abandoned-generation-metric-flake.md`,
  `docs/bugs/open/BUG-091-claudesettingswatcher-tempdir-cleanup-flake-under-full-suite-load.md`,
  `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md`,
  `docs/bugs/open/BUG-099-server-services-package-slow-enough-to-blow-ci-job-budgets.md` — sibling
  full-suite/scheduler-contention-only flakes, same discovery pattern (surfaced while verifying an
  unrelated diff's full-package test run).
