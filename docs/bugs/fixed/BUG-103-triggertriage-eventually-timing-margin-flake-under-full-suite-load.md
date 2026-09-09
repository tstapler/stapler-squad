# BUG-103: Two `TestTriggerTriage_*` tests intermittently fail their `require.Eventually` bound under full-package parallel load [SEVERITY: Low]

**Status**: ✅ Fixed — both items resolved.
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

## Root Cause

Item 1 (semaphore occupancy) was pure `require.Eventually` scheduling-latency underspecification,
fixed below with no further investigation needed.

Item 2 (`waitForTriageFailureCaptured`) is now root-caused, and it is **not** primarily scheduler
contention: `BacklogService.captureHeadlessFailure` (`backlog_service_triage.go:2503`) writes the
failure-capture file via `s.cfg.HeadlessFailureCaptureDirOrDefault()`, which — before this fix —
always resolved to the real `os.UserHomeDir()/.stapler-squad/headless-failures`, completely
bypassing `config.GetConfigDir()`'s test/instance/workspace isolation. Every test in this repo that
ever exercised this path (not just `server/services`) wrote real files into that one real,
shared directory — confirmed **661 accumulated files** on this maintainer's machine — which the
*live production `stapler-squad` service* also writes to concurrently. `TriageArtifactDirOrDefault`
had the identical bug (confirmed **27,578 accumulated files**), as did `BacklogAttachmentDirOrDefault`
and `PromptCacheDirOrDefault` (0 files each, but same code shape, same exposure). Under full-package
parallel load, dozens of test processes and the live service were all doing real disk I/O
(open/write/fsync/readdir) against the same real, ever-growing directory — genuine I/O contention,
not just CPU scheduling, and a real production-adjacent bug independent of the flake (test runs were
silently polluting the developer's actual `~/.stapler-squad` state the entire time).

Fixed in `config/config.go`: `HibernationCheckpointDirOrDefault`, `TriageArtifactDirOrDefault`,
`HeadlessFailureCaptureDirOrDefault`, `BacklogAttachmentDirOrDefault`, and `PromptCacheDirOrDefault`
now all resolve their default (no-override) path as `filepath.Join(GetConfigDir(), <name>)` instead
of a hardcoded `os.UserHomeDir()/.stapler-squad/<name>` — inheriting the same
`STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE`/per-PID/workspace-mode isolation `config.json` and
`sessions.json` already had. Production behavior is unchanged in the default (non-workspace-mode)
case: `GetConfigDir()` resolves to exactly `~/.stapler-squad` there, identical to the old hardcoded
path. See `docs/explanation/test-io-storage-isolation.md` for the general strategy this bug motivated.

## Files Affected

- `server/services/backlog_service_triage_test.go`:
  - `TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown` (`:4927`, bound at `:4947`)
  - `waitForTriageFailureCaptured` (`:3521`, bound at `:3536`) and its caller
    `TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors` (`:3583`)

## Fix Approach

1. ~~Reproduce reliably first...~~ — superseded for item 1 by the deterministic fix below, which
   removes the wall-clock guess entirely rather than needing to first reproduce it under load.
2. ~~If confirmed as pure scheduling contention, widen both `require.Eventually` timeouts...~~ — not
   taken for item 1 (see below); still the fallback approach for item 2 if reproduction confirms
   pure scheduling contention there too.
3. **Done for item 1** (`TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown`):
   added `fakeHeadlessPool.onEnter func()`, invoked synchronously the instant `CallBlocking` is
   entered (call recorded, before the delay/`ctx.Done()` block). The test now sets `onEnter` to send
   on a `chan struct{}` buffered to 8, and waits by receiving 8 times (each receive = one occupier
   definitely inside `CallBlocking`) instead of polling `pool.callCount() == 8` against a fixed 2s
   wall-clock bound.
4. **Done for item 2**: fixed at the source — `config.HeadlessFailureCaptureDirOrDefault` (and its 4
   siblings) now route through `GetConfigDir()` instead of a hardcoded real-home path, so the write
   `waitForTriageFailureCaptured` waits on lands in the test's own isolated per-run directory instead
   of one real, ever-growing (661-file) directory shared with every other concurrent test process and
   the live production service. No timeout widening needed — the 5s bound was never really the
   problem, contention on the shared directory was.

## Verification

Item 1: `go test ./server/services/... -run
TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown -race
-count=20` — 20/20 pass (5–17s each, wide variance confirming the old fixed 2s bound really was
just a guess about scheduling latency, not a real invariant).

Item 2: `go test ./server/services/... -run
TestTriggerTriage_should_PersistFailureCapture_When_HeadlessCallItselfErrors -race -count=30` —
30/30 pass (1.4s–8.1s each).

Neither re-run under deliberate background CPU load to reproduce the exact original single-occurrence
conditions — both fixes address the actual mechanism (a wall-clock guess, and shared-directory I/O
contention, respectively), not just the symptom, so this is lower priority than it would be for a
widened-timeout-only fix.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — this repo's standing rule against re-excusing a known
  flake without root-causing or filing it; this bug is that filing.
- `docs/bugs/open/BUG-102-tymux-openstandingstream-abandoned-generation-metric-flake.md`,
  `docs/bugs/open/BUG-091-claudesettingswatcher-tempdir-cleanup-flake-under-full-suite-load.md`,
  `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md`,
  `docs/bugs/open/BUG-099-server-services-package-slow-enough-to-blow-ci-job-budgets.md` — sibling
  full-suite/scheduler-contention-only flakes, same discovery pattern (surfaced while verifying an
  unrelated diff's full-package test run).
