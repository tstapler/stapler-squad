# BUG-085: `TestIdleDetector_PatternMatching` Flakes Under `make quick-check`'s Full-Suite Parallel Load [SEVERITY: Low]

**Status**: 🔓 Open
**Discovered**: 2026-08-20/21, while validating the `session`/`session/mux`/`session/tmux`/`testutil` `-p 1` scoping fix for the flaky-tmux-tests backlog item (see `docs/bugs/fixed/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`).
**Impact**: `session/detection`'s `TestIdleDetector_PatternMatching` intermittently fails under `make quick-check`'s full-suite parallel invocation. Does not affect runtime correctness — only test-suite reliability.

## Problem Description

Surfaced in a `make quick-check` run's `test-coverage`/`test-race` phase alongside the tmux-contention failures BUG-051 documents. `session/detection` is not part of the `-p 1`-scoped group (it doesn't fork tmux subprocesses), so it runs under the default, unscoped `t.Parallel()` fan-out — the same failure class as BUG-051/BUG-083/BUG-084 (a fixed timing/budget assumption getting blown under full-suite scheduler contention), just in a different package.

## Suggested Investigation

- Re-run `go test ./session/detection -run TestIdleDetector_PatternMatching -race -count=20` in isolation to confirm it passes reliably outside full-suite contention (same isolation check BUG-084 used).
- If the test asserts on a fixed real-time window (timer-based pattern match debounce, etc.), check whether it's a candidate for `testing/synctest` conversion (see the `golang-testing` skill and the precedent noted in `session_driver_test.go`) rather than a widened margin.

## Related

- Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` — found during BUG-051 remediation validation but out of scope to fix in that change (different package, would expand that change's blast radius).
- Same failure class as `docs/bugs/fixed/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`, `docs/bugs/open/BUG-083-server-services-flaky-under-full-suite-parallel-load.md`, and `docs/bugs/open/BUG-084-forbidden-deps-test-flaky-under-full-suite-parallel-load.md`.
