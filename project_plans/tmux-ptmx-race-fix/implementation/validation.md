# Validation Plan: tmux-ptmx-race-fix

**Date**: 2026-08-06

## Happy Path Scenario
Given a `TmuxSession` whose `CreateSession` async controller-start goroutine is still wiring up the PTY (writing the PTY triple via `setPTYTriple`) while a concurrent `DeleteSession` cleanup goroutine calls `closePTYAndAttachCmd` (clearing the PTY triple via `clearPTYTriple`), when both goroutines access `ptmx`/`attachCmd`/`attachCmdWaitOnce` exclusively through the `ptmxMu`-guarded helpers (`lockedPTMX`/`setPTYTriple`/`clearPTYTriple`), then `go test -race` reports no data race and `GetPTY()` returns either a valid, non-torn `*os.File` from the still-live generation or the clean `"PTY not initialized"` error — never a torn read or a panic.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: PTY triple never read/written outside `ptmxMu` | `session/tmux/tmux_test.go` | `TestSetPTYTriple_UpdatesAllThreeFieldsAtomically_When_Called` | Unit | Happy path — `setPTYTriple` assigns `ptmx`/`attachCmd`/`attachCmdWaitOnce` together under lock; all 3 observable via `lockedPTMX()`/direct field read after unlock |
| AC1: PTY triple never read/written outside `ptmxMu` | `session/tmux/tmux_test.go` | `TestGetPTY_ReturnsNotInitializedError_When_PtmxIsNil` | Unit | Error path — `GetPTY()` on a freshly-constructed session (no PTY installed) returns `"PTY not initialized - session may not be started"`, not a nil-pointer panic |
| AC1: PTY triple never read/written outside `ptmxMu` | `session/tmux/tmux_test.go` | `TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized` (plan Task 1.2.2a) | Integration | `testing/synctest` bubble forces `GetPTY()` and `closePTYAndAttachCmd()` to contend on `ptmxMu` simultaneously (real `os.Pipe()` fd); `synctest.Wait()` proves both are durably blocked before release — no torn read, `-race` clean |
| AC2: `go test -race ./session/... ./server/... -count=10` passes | `session/tmux/tmux_test.go` | `TestClosePTYAndAttachCmd_ClearsTripleAndClosesFile_When_PtmxSet` | Unit | Happy path — `closePTYAndAttachCmd()` on a session with a live PTY triple closes the file, kills/waits the process, and leaves all 3 fields nil |
| AC2: `go test -race ./session/... ./server/... -count=10` passes | `session/tmux/tmux_test.go` | `TestUpdateWindowSize_ReturnsError_When_PtmxIsNil` | Unit | Error path — `updateWindowSize()` on a session with no PTY returns `"PTY is not initialized"` via the `lockedPTMX()` snapshot, not a nil-pointer dereference on `.Fd()` |
| AC2: `go test -race ./session/... ./server/... -count=10` passes | `session/tmux/tmux_test.go` | `TestClosePTYAndAttachCmd_ConcurrentCallersDoNotPanic` (plan Task 1.2.2b) | Integration | `testing/synctest` bubble forces two concurrent `closePTYAndAttachCmd()` callers to contend on `ptmxMu`; asserts no panic (regression guard for the old unsynchronized `waitOnce.Do()` nil-pointer hazard), only pre-existing suppressed error strings, and a fully-cleared end state |
| AC3: originally-flaky test + concurrent load matching original repro | `session/tmux/tmux_test.go` | `TestAttachToExisting_InstallsPTYTriple_When_PtmxNil` | Unit | Happy path — `AttachToExisting()` on a session with no PTY installs a new triple via `setPTYTriple`, all 3 fields set together |
| AC3: originally-flaky test + concurrent load matching original repro | `session/tmux/tmux_test.go` | `TestSendKeys_ReturnsNotInitializedError_When_PtmxIsNil` | Unit | Error path — `SendKeys()` on a session with no PTY returns `(0, err)` with the not-initialized error, mirroring `TapEnter`/`TapDAndEnter`'s converted error path |
| AC3: originally-flaky test + concurrent load matching original repro | `server/server_integration_test.go` | `TestSessionService_CreateThenImmediateDelete_NoDataRace` (plan Task 1.2.2c) | Integration | Real `SessionService.CreateSession` → immediate `DeleteSession` (no `waitForLiveInstance`), looped `iterations=20`, lands inside the actual original repro window (`CreateSession`'s async controller-start goroutine racing `DeleteSession`'s cleanup goroutine); run under `-race -count=10` per Task 1.2.2d, plus the pre-existing `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` rerun at `-count=20` |
| AC4: no new deadlocks; `session/tmux` suite passes; lock order documented | `session/tmux/tmux_test.go` | `TestDetachThenClosePTYAndAttachCmd_DoesNotDeadlock_When_NestedLockOrderFollowed` | Unit | Happy path — exercises the documented `detachMutex`-outer/`ptmxMu`-inner nesting (`Detach()`/`DetachSafely()` calling into `closePTYAndAttachCmd()`) and completes without a `deadlock.Mutex` timeout |
| AC4: no new deadlocks; `session/tmux` suite passes; lock order documented | `session/tmux/tmux_test.go` | `TestClosePTYAndAttachCmd_ReturnsEmptyErrors_When_TripleAlreadyCleared` | Unit | Error/edge path — calling `closePTYAndAttachCmd()` twice in a row (triple already nil on the second call) returns an empty `[]error`, not a nil-pointer panic on the losing caller |
| AC4: no new deadlocks; `session/tmux` suite passes; lock order documented | `session/tmux/...` (whole package) | `go test -race ./session/tmux/...` (plan Task 1.2.2d item 5) | Integration | Full existing `session/tmux` regression suite run under `-race`; zero `"POTENTIAL DEADLOCK"` output, zero test failures |
| AC5: `make quick-check` passes | `Makefile` | `make ptmx-field-guard` (plan Task 1.1.1c) | Static analysis / CI guard | Fails the build if `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce` is accessed anywhere in `session/tmux/tmux.go` outside the bodies of `lockedPTMX`/`setPTYTriple`/`clearPTYTriple`; wired into both `ci` and `quick-check` Makefile targets |
| AC5: `make quick-check` passes | N/A (build pipeline) | `make quick-check` (build + `test-coverage` + `test-race` + `lint` + `lint-css-tokens` + `registry-diff` + `ptmx-field-guard`) | Integration | Full local pre-push validation pipeline; no regressions attributable to this change |
| AC6: no observable PTY lifecycle behavior change | `session/tmux/tmux_test.go` | `TestUpdateWindowSize_SetsSizeOnPTY_When_PtmxValid` | Unit | Happy path — `updateWindowSize()` with a valid PTY snapshot still calls `pty.Setsize` with the same `Winsize` values as before the refactor to a single `lockedPTMX()` snapshot |
| AC6: no observable PTY lifecycle behavior change | `session/tmux/tmux_test.go` | `TestClosePTYAndAttachCmd_SuppressesAlreadyClosedError_When_FileAlreadyClosed` | Unit | Error path — the preserved `"file already closed"` string-match suppression still swallows that specific error and does not append it to the returned `errs` slice |
| AC6: no observable PTY lifecycle behavior change | `session/tmux/tmux_test.go` | `TestAttach_StdinForwardGoroutine_PicksUpNewGeneration_When_RestoreSwapsPTY` | Integration | Verifies the `Attach()` stdin-forward goroutine's per-iteration `lockedPTMX()` re-snapshot (not hoisted) still picks up a new PTY generation installed mid-session by `Restore()`/`setPTYTriple()`, exactly matching pre-fix behavior |

## UX Acceptance Tests
N/A — no user-facing surface, this is an internal concurrency-correctness fix.

## Test Stack
- **Unit**: Go testing + `testing/synctest` (Go 1.26.3, stdlib, no new dependency) + testify/require
- **Integration**: Go testing, real `SessionService`/`tmux.TmuxSession`, ConnectRPC in-process (`server/server_integration_test.go`'s existing `BuildDependencies()`/`installFakeClaudeBinary(t)`/`findFreePort(t)` fixtures — no new test harness needed)
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods (`GetPTY`, `SendKeys`, `TapEnter`, `TapDAndEnter`, `AttachToExisting`, `Attach`, `Close`): happy path + error paths covered
- All external integrations (real tmux attach process spawn/kill/wait, real `os.Pipe()` fds, real `SessionService.CreateSession`/`DeleteSession`): unit mocked (`MockPtyFactory`/`MockCmdExec`) + at least one integration test (`TestSessionService_CreateThenImmediateDelete_NoDataRace`)
- Final verification gate (plan Task 1.2.2d): all 7 acceptance-criteria commands run and shown green before the PR is considered complete — `go test -race ./session/... ./server/... -count=10`; `go test -race ./server/... -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort -count=20`; `go test -race ./server/... -run TestSessionService_CreateThenImmediateDelete_NoDataRace -count=10`; `go test -race ./session/tmux/... -run 'TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized|TestClosePTYAndAttachCmd_ConcurrentCallersDoNotPanic' -count=50`; `go test -race ./session/tmux/...`; `make ptmx-field-guard`; `make quick-check`
