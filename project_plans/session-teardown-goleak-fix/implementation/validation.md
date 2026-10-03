# Validation Plan: session-teardown-goleak-fix

**Date**: 2026-09-14

## Happy Path Scenario

Given a live `ClaudeController`/`PTYConsumer` session, when `Stop()`/`DeleteSession` is called, then `Stop()` blocks until `runStatusChangeLoop` and `pollLoop` have both exited, so no goroutine survives into the next test's goleak snapshot.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: `ClaudeController.Stop()`/`PTYConsumer.Stop()` block until `runStatusChangeLoop`/`pollLoop` have actually exited (not just tmux-level teardown) | `session/claude_controller_test.go` | `ClaudeController_Stop_should_BlockUntilRunStatusChangeLoopExits_When_Called` | Unit | Happy path: start controller, call `Stop()`, assert `cc.wg.Wait()` (or a goroutine-count probe via `runtime.NumGoroutine()`/goleak) shows the loop goroutine gone before `Stop()` returns |
| REQ-1 (error path) | `session/claude_controller_test.go` | `ClaudeController_Stop_should_ReturnErrorWithoutHanging_When_ControllerNotStarted` | Unit | Error path: call `Stop()` on a never-`Start()`-ed (or already-stopped) `ClaudeController`; assert it returns the existing `"controller not started"` error immediately and does not block on `cc.wg.Wait()` (regresses the double-`Add`/zero-`Add` deadlock risk) |
| REQ-1: `PTYConsumer.Stop()` blocks until `pollLoop` exits | `session/detection/ratelimit/integration_test.go` | `PTYConsumer_Stop_should_BlockUntilPollLoopExits_When_Called` | Unit | Happy path: `Start()` a `PTYConsumer`, call `Stop()`, assert `pc.wg.Wait()` returns and the goroutine is gone (e.g. via a `sync/atomic` flag set at the top of `pollLoop` and cleared/observed after `Stop()`) |
| REQ-1 (error path) | `session/detection/ratelimit/integration_test.go` | `PTYConsumer_Stop_should_BeNoOpWithoutHanging_When_NotRunning` | Unit | Error/edge path: call `Stop()` on a `PTYConsumer` that was never `Start()`-ed; assert it returns immediately (the existing `if !pc.running { return }` guard) and does not call `pc.wg.Wait()` on a zero-`Add` WaitGroup in a way that would panic or hang |
| REQ-1 (integration) | `server/server_integration_test.go` | `TestSessionService_CreateThenImmediateDelete_NoDataRace` (existing, unmodified per plan.md) | Integration | `waitForTmuxTeardown` after `DeleteSession` now implicitly also waits for `cc.wg`/`pc.wg` joins, since `StopController()` runs synchronously before `KillSession()` — no new assertions added, existing test becomes a stronger proof once the fix lands |
| REQ-2: `go test ./server/... -race -count=10` no longer reproduces the `TestServer_Shutdown_JoinsBackgroundTickers` goleak failure | N/A (repro command, not a new test) | `go test ./server/... -race -count=10 -run 'TestServer_Shutdown_JoinsBackgroundTickers\|TestSessionService_CreateThenImmediateDelete_NoDataRace\|TestServer'` | Integration (repro) | Plan.md Task 1.1.3a — run 10x, all iterations must pass with zero `found unexpected goroutines` failures at `server/server_test.go:257` |
| REQ-3: `TestServer_Shutdown_JoinsBackgroundTickers` itself is unmodified in behavior | `server/server_test.go` | `TestServer_Shutdown_JoinsBackgroundTickers` (existing, unmodified) | Unit/Integration (existing) | Happy path: re-run the existing test unchanged; `git diff server/server_test.go` shows no changes — this is a negative-change assertion, not new test code |
| REQ-3 (verification) | N/A | `git diff --stat server/server_test.go` | Integration (repro) | Confirms zero lines changed in this file as part of the fix, per plan.md Story 1.1.1's third acceptance criterion |
| REQ-4: three superseded bug docs moved to `docs/bugs/fixed/` and cross-referenced | N/A (doc-only Phase 2 task, not a test) | N/A | N/A | Not test-coverable — verified by file existence/content check (`git mv` + grep for the backlog ID string in each moved doc), not automated test; performed as Task 2.1.1a after Story 1.1.3 verification passes |
| REQ-5: no new goroutine leak or deadlock risk in `DeleteSession`'s teardown path; any new wait has a bounded timeout with a clear failure message on expiry | `session/claude_controller_test.go` | `ClaudeController_Stop_should_NotDeadlock_When_StatusChangeListenerDoesNotReenterInstanceLock` | Unit | Happy path: register a `StatusChangeListener` that performs a benign, non-reentrant callback; call `Stop()` under `-race` and assert it returns within a bounded test timeout (e.g. `require.Eventually`/a goroutine + `select` with `t.Fatal` on timeout channel) — guards the `i.mu` re-entrancy risk flagged in plan.md's Task 1.1.1b |
| REQ-5 (error path / stress) | `server/server_test.go` or `server/server_integration_test.go` | `TestServer_Shutdown_JoinsBackgroundTickers` under `go test -race -count=10` (same repro as REQ-2) | Integration (repro) | Stress path: repeated `-count=10` run is itself the evidence that the new unbounded `cc.wg.Wait()`/`pc.wg.Wait()` calls never hang in practice (plan.md explicitly rejects a test-side timeout wrapper here, matching the existing unbounded `exec.Stop()`/`rs.Stop()` sibling calls) — no new source-level timeout is introduced, so this run is the substitute verification |

## UX Acceptance Tests

N/A — pure backend/test-infrastructure fix, no user-facing surface, no design/ux.md exists for this project.

## Test Stack

- **Unit**: Go testing package + testify (this repo's convention)
- **Integration**: go test with -race, goleak (go.uber.org/goleak, already used by server_test.go)
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked + at least one integration test
- UX acceptance criteria: N/A
