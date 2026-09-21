# BUG-100: `TestSlackNotifier_RecoversFromPanic_And_LogsError` captures a concurrent test's log line under full-suite parallel load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-04, running `go test ./server/services/... ./session/...` (full scoped suite,
`-count=1`) while shipping PR #697 (unrelated frontend/Makefile fix).
**Impact**: Test-only, low severity — same class as BUG-098 (cross-test pollution via shared
process-global state under parallel execution), different symptom and different test.

## Problem Description

```
--- FAIL: TestSlackNotifier_RecoversFromPanic_And_LogsError (0.00s)
    slack_notifier_test.go:561:
        Error:  "time=2026-09-04T16:10:41.877-07:00 level=INFO msg=\"[SessionService] AI rule generation enabled\" backend=cli:claude\n" does not contain "recovered from panic"
```

The test asserts its captured log output contains "recovered from panic", but instead captured a log
line (`[SessionService] AI rule generation enabled`) emitted by a **different, concurrently running**
test — the panic-recovery log line the test itself triggers is nowhere in the captured buffer at all.
This points to a shared/global log writer or capture buffer that multiple parallel `t.Parallel()` tests
write into, rather than something scoped per-test.

## Reproduction Steps

Not reproducible in isolation: `go test ./server/services/... -run TestSlackNotifier_RecoversFromPanic_And_LogsError -count=5 -v`
passes 5/5. Only observed as part of the full `./server/services/... ./session/...` suite under
`-count=1` (bypassing test cache) — i.e. only under real parallel load from the rest of the suite,
matching BUG-098's reproduction profile.

## Root Cause Hypothesis

Not yet investigated in depth. Candidates:
- The test's log-capture mechanism (whatever swaps in a buffer to assert against) uses a
  package-level/global writer rather than one scoped to the test's own logger instance, so a
  concurrently-running parallel test's log output lands in the same buffer.
- Related to the same class of process-global state leak BUG-098 documents (that one is
  `STAPLER_SQUAD_TEST_DIR` via an unjoined pipeline goroutine; this one is log output), suggesting a
  broader pattern of "some shared/global sink not properly scoped per-test" worth auditing once,
  rather than treating each instance as a one-off.

## Suggested Fix Direction

1. Find the log-capture setup in `slack_notifier_test.go` (likely swaps `slog`'s default handler or a
   package-level `io.Writer`) and confirm whether it's global vs. per-test-instance.
2. If global: scope it per-test (e.g. inject a logger instance into the code under test rather than
   swapping a shared default), and/or ensure the test doesn't run with `t.Parallel()` alongside tests
   that log through the same shared sink.
3. Cross-check against BUG-098's `trackCleanup`-join precedent — if the actual leak is an unjoined
   goroutine still logging after its owning test returns (not the assertion's capture mechanism
   itself), the fix is routing that goroutine through the same cleanup-join pattern BUG-098 documents.

## Why Filed Instead of Fixed Now

Confirmed unrelated to PR #697 (neither `slack_notifier.go` nor `slack_notifier_test.go` appear in
that PR's diff), and root-causing a shared-log-capture architecture issue is a larger, separate
investigation than the PR's own scope. Filed per this repo's `fix-flaky-tests-dont-defer` convention
(root-cause-and-fix or file-as-tracked-bug-immediately) rather than silently ignored.
