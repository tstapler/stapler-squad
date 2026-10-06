# Validation Plan: tmux-paneexit-reconnect-flake

**Date**: 2026-08-06

## Happy Path Scenario

Given a `TmuxServerRegistry` on an isolated tmux socket whose control-mode connection is down and whose `reconnectLoop` is currently sleeping out an elevated `backoff` wait (≥1600ms, past several failed reconnect attempts), when a tracked session is killed via `kill-session`, then `SubscribePaneExit`'s returned channel closes within `fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval)` = 700ms via an independent `syncSessionsFastRecheck` call — not after the full `backoff` wait elapses.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: `TestTmuxServerRegistry_PaneExitChannel` passes reliably (20/20) | `session/tmux/server_registry_integration_test.go` | `TestTmuxServerRegistry_PaneExitChannel` (existing, unmodified) | Integration, reliability run | `go test -race -tags integration ./session/tmux -run TestTmuxServerRegistry_PaneExitChannel -count=20` exits 0, 20/20 `PASS` (Task 2.1.1a) |
| AC2a: fix addresses the reconnect race itself, structurally | `session/tmux/server_registry_integration_test.go` | `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` (new — Tasks 1.2.1a/1.2.1b) | Integration, regression | Elevates real `backoff` to a verified 1600ms via `waitForReconnectCycles` (condition-driven, ADR-003-compliant), then asserts pane-exit is still detected within 1.5s — proves detection is decoupled from backoff's magnitude, not just "would pass eventually" |
| AC2b: fix does not just enlarge the test's fixed deadline | `session/tmux/server_registry_integration_test.go` | N/A — static diff check (Task 2.1.1a, 2nd half) | Integration, static/process-level | `git diff -- session/tmux/server_registry_integration_test.go \| grep -n '3 \* time.Second'` shows `TestTmuxServerRegistry_PaneExitChannel`'s deadline line absent from the diff (unchanged) |
| AC3: no regression to rest of `session/tmux`, incl. 2 named sibling flakes + new regression test | `session/tmux/*_test.go` (whole package) | `TestEnsureServerRunning_NoOp`, `TestKillOrphanedControlModeClients`, `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`, plus every other existing test in the package | Integration + unit, full-package run | `go test -race -tags integration ./session/tmux/...` exits 0; output greped for `--- PASS` on all three named tests (Task 2.1.2a) |
| AC4: `make ci` continues to pass | N/A (repo-wide) | N/A | Process-level (build + unit + integration + lint) | `make ci` exits 0 (Task 2.1.3a) |
| AC5: `ponytail:`-style ceiling comment present and accurate | `session/tmux/server_registry.go` | N/A — static grep check (Task 2.1.4a) | Static/process-level | `git diff -- session/tmux/server_registry.go \| grep -n "ponytail:"` matches, and the matched context contains `fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms` with real constant values |
| AC6: diff confined to the 2 allowed files | N/A (repo-wide) | N/A — static diff check (Task 2.1.5a) | Static/process-level | `git diff --name-only main...HEAD` outputs exactly `session/tmux/server_registry.go` and `session/tmux/server_registry_integration_test.go`, no other paths |
| Story 1.1.1 (syncMu serialization correctness) — indirect coverage | `session/tmux/server_registry_integration_test.go` | `TestTmuxServerRegistry_StartsHealthy`, `TestTmuxServerRegistry_SessionCreated`, `TestTmuxServerRegistry_ListSessions`, `TestTmuxServerRegistry_ConcurrentSubscriptions` (existing, unmodified) | Integration, regression-of-behavior | These already exercise `syncSessionsLocked` via the blocking `syncSessions` path on every `reconnectLoop` cycle; passing under `-race` after the split confirms the refactor didn't change observable behavior for the 3 pre-existing blocking callers |
| Story 1.1.2 (`syncSessionsFastRecheck` TryLock-succeeds path, uncontended) | `session/tmux/server_registry_integration_test.go` | `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` | Integration, regression | Same test as AC2a — during the elevated-backoff wait, `syncMu` is uncontended (no debounce/Start/reconnect-post-connect caller is mid-sync), so this test's detection assertion specifically exercises `TryLock()` succeeding and running `syncSessionsLocked` |

### Explicitly out of scope / documented gap (not a missing task — see plan.md Unresolved Questions)

`syncSessionsFastRecheck`'s **`TryLock()`-fails (skip-if-busy)** branch, and the **lost-update/ghost-session race** `syncMu` exists to prevent, have **no dedicated test in this change**. `session/tmux/server_registry_test.go` — the existing unit test file that drives `readLines` off a fake pipe instead of a real tmux process (`newTestRegistry`/`newPipe` in that file) and, being internal `package tmux`, is the only test file in the package with access to unexported state like `syncMu` — would be the natural place to unit-test this contention path directly (e.g., hold `syncMu` manually, call `syncSessionsFastRecheck`, assert it returns `nil` immediately without mutating `r.sessions`). It is **not** one of the two files this fix's AC6 confines the diff to (`server_registry.go`, `server_registry_integration_test.go`), so adding a test there is out of scope for this change. This matches plan.md's own "Accepted gap, not blocking" entry in Unresolved Questions and its Story 1.2.1 AC's "Known, documented gap" note — recorded here rather than invented as a phantom task that would violate AC6.

## UX Acceptance Tests

N/A — no user-facing surface (pure backend infrastructure fix to `TmuxServerRegistry`'s internal detection latency; no `design/ux.md` exists for this project).

## Test Stack

- **Unit**: Go stdlib `testing`, `go test -race`. No new unit-level (internal `package tmux`) tests are added by this fix — see the documented gap above; existing unit tests in `server_registry_test.go` are unaffected (that file is untouched, verified by AC6).
- **Integration**: Go stdlib `testing` with build tag `integration` (`//go:build integration`), real isolated tmux servers via unique `-L <socket>` per test (`newIsolatedSocket`). New: `waitForReconnectCycles` test helper (condition-driven polling of `IsHealthy()` pulse transitions, ADR-003-compliant — no static sleeps) and `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test -race -tags integration ./session/tmux/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | No regression vs. pre-fix baseline for `session/tmux`; new code (`syncSessionsLocked`, `syncSessionsFastRecheck`, `waitBackoffWithFastRecheck`, `waitForReconnectCycles`) exercised at least once by the integration suite (see mapping above) |

- All public service methods touched by this fix (`Start`, `Stop`, `SubscribePaneExit`, `SessionExists`, `ListSessions`, `IsHealthy`) are unchanged in signature/behavior and remain covered by the 5 pre-existing integration tests, all of which must continue passing (AC3).
- New unexported methods (`syncSessionsLocked`, `syncSessionsFastRecheck`, `waitBackoffWithFastRecheck`): happy path (uncontended `TryLock` succeeds, bounded attempts complete) is covered via `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`; the contended/skip-if-busy branch is a documented, out-of-scope gap (see above) rather than silently uncovered.
- No external integrations beyond the existing `tmux` subprocess boundary, which is already exercised end-to-end by every integration test in this file (no separate mock needed — this package's convention, per `server_registry_test.go`'s fake-pipe unit tests, is to unit-test event parsing with a fake pipe and integration-test the real subprocess lifecycle; this fix only touches the subprocess-lifecycle side).
- UX acceptance criteria: N/A.
