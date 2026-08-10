# Validation Plan: tmux-ptmx-race-fix

**Date**: 2026-08-06
**Requirements**: `project_plans/tmux-ptmx-race-fix/requirements.md`
**Plan**: `project_plans/tmux-ptmx-race-fix/implementation/plan.md`
**Requirements Coverage**: 6/6

---

## Happy Path Scenario

Given a `TmuxSession` whose async controller-start goroutine is mid-flight inside `GetPTY()` (reading `t.ptmx`) while a concurrent `DeleteSession` cleanup goroutine calls `closePTYAndAttachCmd()` (nil-ing `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce`) on the same session, when both goroutines run under `go test -race`, then `ptmxMu` serializes every access to the PTY triple so no data race is reported and `GetPTY()` observes either a valid, fully-formed `*os.File` from the still-live generation or the well-defined `"PTY not initialized"` error — never a torn read across the three fields.

---

## Summary

| Test Type | Count |
|---|---|
| Unit tests (`go test -race`, `session/tmux/tmux_test.go`) | 11 |
| Integration tests (`server/server_integration_test.go`) | 2 |
| Structural / CI-gate tests | 3 |
| Verification commands (full-suite / `make` gates, not new named tests) | 3 |
| **Total named test cases** | **16** |

Requirements coverage: **6/6** (AC1–AC6 each have ≥1 test case or gate).

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: PTY triple never touched outside `ptmxMu` + 3 helpers | `session/tmux/tmux_test.go` | `TestLockedPTMX_ReturnsNil_BeforeAnyTripleSet` | Unit | Happy path — fresh `TmuxSession`, `lockedPTMX()` returns `nil` without panicking or racing |
| AC1 | `session/tmux/tmux_test.go` | `TestSetPTYTriple_AssignsAllThreeFieldsTogether` | Unit | Happy path — `setPTYTriple(file, cmd, waitOnce)` makes all 3 fields observable as one unit via `lockedPTMX()` + direct same-package field reads taken immediately after |
| AC1 | `session/tmux/tmux_test.go` | `TestClearPTYTriple_CapturesThenNilsAllThreeFields` | Unit | Error/teardown path — `clearPTYTriple()` returns the pre-clear values and leaves `lockedPTMX()` returning `nil` afterward; calling it twice in a row returns `nil, nil, nil` the second time (idempotent) |
| AC1 | `Makefile` (target `ptmx-field-guard`) | `make ptmx-field-guard` (positive case) | Structural | Given the fixed `session/tmux/*.go`, when the guard target runs, then it exits 0 — only `lockedPTMX`/`setPTYTriple`/`clearPTYTriple` touch `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce` directly |
| AC1 | `Makefile` (target `ptmx-field-guard`) | `make ptmx-field-guard` (negative case, one-time implementation-time check per Task 1.3.1b) | Structural | Given a throwaway `t.ptmx = nil` line added outside the 3 helpers, when the guard runs, then it exits 1 naming the violation; line is reverted immediately after confirming — proves the guard actually detects the class of bug it exists to prevent, not just that it passes on already-correct code |
| AC1 (doc requirement, shared with AC4) | `session/tmux/tmux.go` (grep-based check) | `TestPtmxMuDocComment_StatesLeafLockInvariant` | Structural | Given the `ptmxMu` field declaration, when its doc comment is read, then it contains the string "leaf lock" and names `detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, and `recoveryMu` — mirrors the `installOpenCode()` rationale-comment check pattern already used in this repo (`project_plans/antigravity-opencode-parity/implementation/validation.md` ST-03) |
| AC2: `go test -race ./session/... ./server/... -count=10` clean | `session/tmux/tmux_test.go` | `TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized` | Unit (race) | Given a `TmuxSession` with `t.ptmx` set to one end of an `os.Pipe()`, when the test holds `ptmxMu` before launching one goroutine calling `GetPTY()` and another calling `closePTYAndAttachCmd()`, then releases the lock, both complete within a 2s deadlock-guard `select` and `GetPTY()`'s result is exactly the valid `*os.File` or the `"not initialized"` error — never a panic or corrupted pointer. Run standalone with `-race -count=50` to exercise both interleavings (`GetPTY`-wins vs. `closePTYAndAttachCmd`-wins) |
| AC2 | — (whole-suite gate) | `go test -race ./session/... ./server/... -count=10` | Verification command | Given every write/read site converted (Epics 1.1/1.2), when the full command runs from repo root, then it exits 0 with no `"DATA RACE"` string in output — the umbrella gate AC2 names directly |
| AC3: originally-flaky test + new direct repro pass reliably | `server/server_integration_test.go` | `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` (existing test, re-verified) | Integration (race regression) | Given the fix applied, when run via `go test -race ./server/... -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort -count=20`, then all 20 runs pass with no race — confirms the originally-reported flake (filed per `.claude/rules/fix-flaky-tests-dont-defer.md`) is actually gone, not just newly unlucky |
| AC3 | `server/server_integration_test.go` | `TestSessionService_CreateThenImmediateDelete_NoDataRace` | Integration (race) | Given `CreateSession` is called and its response returned without calling `waitForLiveInstance`, when `DeleteSession` is issued immediately after on the same session ID, then `go test -race ./server/... -run TestSessionService_CreateThenImmediateDelete_NoDataRace -count=20` passes cleanly — lands inside the original repro window (async controller-start goroutine's `GetPTY()` vs. delete cleanup's `closePTYAndAttachCmd()`) deterministically rather than incidentally |
| AC4: no new deadlocks; `ptmxMu` lock order documented and correct | `session/tmux/tmux_test.go` | `TestDetachSafely_ConcurrentWithGetPTY_NoDeadlock` | Unit (race + deadlock guard) | Given a `TmuxSession` with a live PTY triple, when one goroutine repeatedly calls `DetachSafely()` (which holds `detachMutex` across a `closePTYAndAttachCmd()`/`Restore()` call that internally acquires `ptmxMu`) concurrently with another goroutine repeatedly calling `GetPTY()`, then all iterations complete within a bounded `select`+`time.After` deadlock guard — proves the documented `detachMutex` (outer) → `ptmxMu` (inner/leaf) ordering never reverses or self-deadlocks |
| AC4 | — (whole-package gate) | `go test -race ./session/tmux/...` | Verification command | Given the fix applied, when the full `session/tmux` suite runs, then it exits 0 with no `deadlock.Mutex` timeout / `"POTENTIAL DEADLOCK"` output and no test failures — the umbrella gate AC4 names directly |
| AC5: `make quick-check` passes with no regressions | — | `make quick-check` | Verification command | Given the fix applied, when `make quick-check` (build + test + lint) runs, then it exits 0 with no new lint findings or test failures attributable to this change |
| AC6: no observable PTY lifecycle behavior change, except the one documented accepted side effect | `session/tmux/tmux_test.go` | `TestClosePTYAndAttachCmd_OnlyFirstConcurrentCallerPerformsCleanup` | Unit | Given two goroutines call `closePTYAndAttachCmd()` concurrently on a session with one live triple, when both complete, then exactly one observes a non-nil captured triple (performs `Close()`/`Kill()`/logs "killing orphaned tmux attach process") and the other is a no-op returning `nil` errors — the single accepted intentional behavior change (serialization narrows, not removes, the existing `"file already closed"` suppression's necessity), asserted directly rather than left implicit |
| AC6 | `session/tmux/tmux_test.go` | `TestAttachToExisting_ReturnsSameWrappedError_When_PtyFactoryStartFails` | Unit | Error path regression — `ptyFactory.Start` returns an error; asserts `AttachToExisting()`'s returned error still matches `"failed to attach PTY to session '%s': %w"` unchanged after the `lockedPTMX()`/`setPTYTriple()` conversion |
| AC6 | `session/tmux/tmux_test.go` | `TestGetPTY_ReturnsNotInitializedError_When_TripleNeverSet` | Unit | Happy/error path regression — confirms `GetPTY()`'s error string is still exactly `"PTY not initialized - session may not be started"` post-conversion (Task 1.2.1a) |
| AC6 | `session/tmux/tmux_test.go` | `TestTapEnter_TapDAndEnter_SendKeys_ReturnSameWrappedErrors_When_PTYNil` | Unit | Error path regression — table test over all 3 converted methods (Task 1.2.1b) confirming each still wraps `GetPTY()`'s error in its own pre-existing `fmt.Errorf` message (e.g. `"error sending enter keystroke to PTY: %w"`) and `SendKeys` still returns `(0, err)` |
| AC6 | `session/tmux/tmux_test.go` | `TestUpdateWindowSize_ReturnsSameErrors_When_PTYNilOrFdInvalid` | Unit | Error path regression — confirms `"PTY is not initialized"`, `"PTY file descriptor is invalid"`, and `"PTY file descriptor is closed or invalid: %v"` are all still returned verbatim after the single-snapshot rewrite (Task 1.2.1c) |
| AC6 | `session/tmux/tmux_test.go` | `TestLockedPTMX_ReflectsNewestGeneration_When_SetPTYTripleSwapsMidLoop` | Unit | Proxy/regression for the `Attach()` stdin-forward goroutine's re-snapshot-every-iteration behavior (Task 1.2.1e) — repeated `lockedPTMX()` calls interleaved with a `setPTYTriple()` swap always observe the newest generation, never a value stale from before the swap, matching the pre-fix closure's implicit re-read semantics without needing a real tmux-backed `Attach()` call |

---

## Test Stack

- **Unit**: Go `testing` + `testify/require` (matches existing `session/tmux/tmux_test.go` usage), run with `-race`. Concurrency tests use the file's existing deadlock-guard idiom (`select` + `time.After(2 * time.Second)`), consistent with `TestDoesSessionExist_LockReleasedBeforeRecovery` and `TestRecoverFromServerFailure_ConcurrentGuard`. `MockPtyFactory`/`MockCmdExec` (already defined in `session/tmux/tmux_test.go` / `test_helpers.go`) supply fake attach commands; `os.Pipe()` supplies a real, closable `*os.File` pair for `t.ptmx` where a genuine file descriptor is needed (matches Task 1.2.2a's precedent).
- **Integration**: Go `testing` against the real `SessionService`/`BuildDependencies()` wiring in `server/server_integration_test.go`, using `installFakeClaudeBinary(t)` (existing helper) so no real Claude Code process is spawned. Run with `-race -count=20` for the two race-sensitive tests.
- **Structural / CI gates**: shell-based (`grep`) via the `Makefile`'s `ptmx-field-guard` target, following the existing `actor-field-guard` precedent; a doc-comment content check on `ptmxMu`'s declaration, following the `installOpenCode()` rationale-comment precedent from `project_plans/antigravity-opencode-parity/implementation/validation.md` (ST-03).
- **E2E / UX**: N/A — pure backend concurrency fix, no UX surface (per task framing; UX Acceptance Tests section omitted).

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/tmux/... ./server/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | No decrease in `session/tmux/tmux.go` line coverage vs. pre-fix baseline; all 3 new helper methods (`lockedPTMX`, `setPTYTriple`, `clearPTYTriple`) at 100% line coverage (every line is exercised by at least one test above) |

- All 6 acceptance criteria have ≥1 test case or verification-command gate (see mapping table).
- All 3 new helper methods: happy path + at least one nil/empty-triple edge case covered (AC1 rows).
- All 3 write sites (`AttachToExisting`, `RestoreWithWorkDir` retry loop, `closePTYAndAttachCmd`) and all 6+ read sites (`GetPTY`, `TapEnter`/`TapDAndEnter`/`SendKeys`, `updateWindowSize`, both `Attach()` goroutines) have regression coverage proving their pre-fix error strings/behavior are unchanged (AC6 rows) — `RestoreWithWorkDir`'s retry loop itself is exercised indirectly by the existing `TestEnsureServerRunningWithRetry`/`TestServerStartAttempt` suite, which continues to pass unmodified as part of AC4's whole-package gate.
- Race-sensitive tests (`TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized`, `TestDetachSafely_ConcurrentWithGetPTY_NoDeadlock`, `TestSessionService_CreateThenImmediateDelete_NoDataRace`) are all run under `-race` with `-count` ≥ 10 before being considered passing — a single green run is not sufficient evidence for a concurrency fix.
