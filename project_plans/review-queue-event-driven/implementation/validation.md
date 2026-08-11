# Validation Plan: review-queue-event-driven

**Date**: 2026-05-09

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R1.1 — listener fires on status change | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt` | Unit | Happy path: onOutput triggers listener when mock PTY transitions to NeedsApproval |
| R1.1 — listener not registered | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_NilListenerDoesNotPanic` | Unit | Error path: nil listener — onOutput send must not panic |
| R1.1 + wiring E2E | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_StatusChangeCallback_TriggersQueueUpdate` | Integration | Fake controller fires listener → rqm.OnControllerStatusChange → session appears in queue |
| R1.2 — hash cache used | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_UsesHashCache` | Unit | Happy path: two onOutput calls with identical PTY content → GetCurrentStatus called once |
| R1.2 — cache miss | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_RunsDetectionOnCacheMiss` | Unit | Error path: PTY content changes hash → detection runs again |
| R1.3 — dedup identical status | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_SuppressedOnNoChange` | Unit | Happy path: same status emitted twice → listener called exactly once |
| R1.3 — distinct statuses fire twice | `session/claude_controller_test.go` | `TestClaudeController_StatusChangeCallback_FiresEachDistinctStatus` | Unit | Error path: Active→NeedsApproval→Active → listener called twice |
| R1.4 — no server import in session/ | (static analysis) | `TestSessionPackage_NoServerImport` (build-tag lint / depguard) | Unit | Happy path: `go list -deps ./session/... \| grep server` returns empty |
| R1.5 — wiring at session creation | `server/services/session_service_test.go` | `TestSessionService_WireStatusChangeCallback_CalledAtCreate` | Integration | CreateSession wires callback → mock rqm.OnControllerStatusChange receives call |
| R1.5 — wiring for loaded sessions | `server/services/session_service_test.go` | `TestSessionService_WireStatusChangeCallback_CalledAtLoad` | Integration | Sessions loaded from storage also get callback wired |
| R2.1 — idle timeout triggers queue check | `session/review_queue_poller_test.go` | `TestAdaptivePoller_IdleTimerFiresWithoutPoll` | Unit | Happy path: simulate idle threshold expiry → CheckSession called without poll cycle |
| R2.1 — idle event integration | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_IdleTimeout_TriggersQueueCheck` | Integration | IdleDetector timeout callback reaches rqm → CheckSession dispatched |
| R2.2 — timer reset on activity | `session/detection/idle_test.go` | `TestIdleDetector_ResetOnRecordActivity` | Unit | Happy path: RecordActivity within threshold resets timer; no timeout fires |
| R2.2 — timer fires without activity | `session/detection/idle_test.go` | `TestIdleDetector_FiresAfterIdleThreshold` | Unit | Error path: no RecordActivity → timer fires → onTimeout called |
| R2.3 — reuses IdleDetector | `session/claude_controller_test.go` | `TestClaudeController_IdleTimeout_UsesExistingIdleDetector` | Unit | Happy path: idle timer set via SetOnTimeout on the same idleDetector instance |
| R2.4 — debounce prevents thrashing | `session/detection/idle_test.go` | `TestIdleDetector_MinActivityIntervalDebounce` | Unit | Rapid RecordActivity calls within 500ms window → timer reset at most once per window |
| R3.1 — poller retained | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_RetainedForAllSessions` | Unit | Happy path: poller still ticks; reconciliation runs every 30s regardless of controller presence |
| R3.2 — fast-path skipped for controller sessions | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ControllerSessions_SkipFastPath` | Unit | Happy path: session with active controller → fast-path poll skips status detection |
| R3.2 — reconciliation still runs | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ControllerSessions_ReconciliationRuns` | Unit | Error path: even with controller, 30s reconciliation loop executes |
| R3.3 — non-controller sessions polled | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_NonControllerSessions_FullScanContinues` | Unit | Happy path: session without controller → full 2s fast-path scan executes |
| R3.4 — SlowPollInterval preserved | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_SlowPollInterval_BackoffPreserved` | Unit | Error path: empty queue → poller backs off to SlowPollInterval correctly |
| R4.1 — no circular imports | (CI build gate) | `TestBuild_SessionPackage_NoCycleToServer` | Unit | `go build ./session/...` succeeds with no import of server/ |
| R4.2 — callback pattern at boundary | `session/instance_test.go` | `TestInstance_SetStatusChangeCallback_AcceptsPlainFunc` | Unit | Happy path: plain func registered and invoked through wiring |
| R4.3 — EventBus publish in server/ only | (code review / grep) | `TestNoEventBusCallsInSessionPackage` (grep-based CI check) | Unit | `grep -r "EventBus\|pkg/events" session/` returns no matches |
| R5.1 — IsAcknowledgedAfterOutput preserved | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_AcknowledgedSnooze_Preserved` | Integration | Acknowledged session with recent output is still snoozed after status-change event |
| R5.2 — LastOutputSignature dedup | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_LastOutputSignature_PreventsReSqueuing` | Integration | Identical signature → CheckSession does not re-add session to queue |
| R5.3 — approval within 1 second | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_ApprovalSurfaces_Within1Second` | Integration | PTY writes approval content → session appears in queue within wait.FastWaitConfig deadline (≤1s) |
| R5.3 — approval timing unit | `session/claude_controller_test.go` | `TestClaudeController_ApprovalPrompt_ListenerFiredUnder1Second` | Unit | Happy path: measure elapsed time from onOutput to listener call; assert < 1000ms |
| R5.4 — existing TestReviewQueue* pass | (regression suite) | `TestReviewQueue_ExistingTests_AllPass` (run: `go test ./... -run TestReviewQueue`) | Regression | All pre-existing tests continue to pass unchanged |
| R5.4 — existing TestReviewQueuePoller* pass | (regression suite) | `TestReviewQueuePoller_ExistingTests_AllPass` (run: `go test ./... -run TestReviewQueuePoller`) | Regression | All pre-existing poller tests continue to pass unchanged |
| R5.5 — EventApprovalResponse path unchanged | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_EventApprovalResponse_HandleEventUnchanged` | Integration | EventApprovalResponse still routed through handleEvent; event-driven path does not intercept |
| R5.5 — EventUserInteraction path unchanged | `server/review_queue_manager_test.go` | `TestReactiveQueueManager_EventUserInteraction_HandleEventUnchanged` | Integration | EventUserInteraction still routed through handleEvent; no regression |

---

## Epic-to-Test Coverage Map

### Epic 1 — ClaudeController Status-Change Event Emission

| Task | Tests Covering It |
|------|-------------------|
| 1.1.0 — cacheMu data race fix | `TestClaudeController_StatusChangeCallback_UsesHashCache` (run with -race) |
| 1.1.1 — new fields + type | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt`, `TestClaudeController_StatusChangeCallback_NilListenerDoesNotPanic` |
| 1.1.2 — SetStatusChangeListener setter | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt`, `TestInstance_SetStatusChangeCallback_AcceptsPlainFunc` |
| 1.1.3 — onOutput closure extension | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt`, `TestClaudeController_StatusChangeCallback_NilListenerDoesNotPanic` |
| 1.1.4 — runStatusChangeLoop goroutine | `TestClaudeController_StatusChangeCallback_SuppressedOnNoChange`, `TestClaudeController_StatusChangeCallback_NotCalledAfterStop` |
| 1.1.5 — statusCheckCh init | `TestClaudeController_StatusChangeCallback_NilListenerDoesNotPanic` |
| 1.2.1 — Instance.SetStatusChangeCallback | `TestInstance_SetStatusChangeCallback_AcceptsPlainFunc` |
| 1.2.2 — wireStatusChangeCallback | `TestSessionService_WireStatusChangeCallback_CalledAtCreate`, `TestSessionService_WireStatusChangeCallback_CalledAtLoad` |
| 1.2.3 — listener before Start() | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt` (listener registered pre-Start) |

### Epic 2 — Server-Layer Wiring

| Task | Tests Covering It |
|------|-------------------|
| 2.1.1 — OnControllerStatusChange method | `TestReactiveQueueManager_StatusChangeCallback_TriggersQueueUpdate` |
| 2.1.2 — detection import | (compile-time: `go build ./server/...`) |
| 2.2.1 — wireStatusChangeCallback in session_service | `TestSessionService_WireStatusChangeCallback_CalledAtCreate` |
| 2.2.2 — call sites at creation | `TestSessionService_WireStatusChangeCallback_CalledAtCreate`, `TestSessionService_WireStatusChangeCallback_CalledAtLoad` |
| 2.2.3 — interface extension (optional) | `TestSessionService_WireStatusChangeCallback_CalledAtCreate` (mock implements updated interface) |
| 2.3.1 — fast-path skip for controller sessions | `TestReviewQueuePoller_ControllerSessions_SkipFastPath`, `TestReviewQueuePoller_ControllerSessions_ReconciliationRuns` |

### Epic 3 — Idle Timeout Event Emission

| Task | Tests Covering It |
|------|-------------------|
| 3.1.1 — SetOnTimeout / StartIdleTimer / StopIdleTimer | `TestIdleDetector_FiresAfterIdleThreshold`, `TestIdleDetector_ResetOnRecordActivity` |
| 3.1.2 — RecordActivity timer reset | `TestIdleDetector_ResetOnRecordActivity`, `TestIdleDetector_MinActivityIntervalDebounce` |
| 3.1.3 — Start/Stop from ClaudeController | `TestClaudeController_IdleTimeout_UsesExistingIdleDetector`, leak check via `TestClaudeController_StatusChangeCallback_NotCalledAfterStop` |
| 3.2.1 — Instance.SetIdleTimeoutCallback | `TestReactiveQueueManager_IdleTimeout_TriggersQueueCheck` |
| 3.2.2 — wireIdleTimeoutCallback in session_service | `TestReactiveQueueManager_IdleTimeout_TriggersQueueCheck` |

---

## Pitfall Guard Tests

These tests exist specifically to catch the mitigations identified in the plan:

| Pitfall | Guard Test | File |
|---------|-----------|------|
| P1 — callback fires after Stop() | `TestClaudeController_StatusChangeCallback_NotCalledAfterStop` | `session/claude_controller_test.go` |
| P2 — GetCurrentStatus() in onOutput (subprocess-per-write) | `TestClaudeController_StatusChangeCallback_UsesHashCache` (assert GetCurrentStatus not called from onOutput directly) | `session/claude_controller_test.go` |
| P3 — listener registered after Start() → first event lost | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt` (listener always set pre-Start in setup) | `session/claude_controller_test.go` |
| P4 — per-session idle timer goroutine leak | `TestIdleDetector_StopIdleTimer_NoPanic`, goroutine count assertion in controller stop test | `session/detection/idle_test.go` |
| P5 — statusCheckCh collapses bursts | `TestClaudeController_StatusChangeCallback_SuppressedOnNoChange` (rapid sends → one check) | `session/claude_controller_test.go` |
| P6 — listener called while mu held (re-entrancy deadlock) | `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt` (run with -race + -timeout 5s) | `session/claude_controller_test.go` |

---

## Test Stack

- **Unit (Go)**: `testing` stdlib + `github.com/stretchr/testify/assert` + `github.com/stretchr/testify/require`. Use `testutil.WaitForCondition` / `wait.FastWaitConfig()` for async assertion. Run with `-race` flag.
- **Integration (Go)**: Same framework. Uses real `IdleDetector`, `ClaudeController` with mock PTY (`newControllerWithMock`), real `ReactiveQueueManager` with fake `CheckSession` double. No external processes.
- **Regression (Go)**: Existing test suite run unchanged via `go test ./... -run TestReviewQueue -race`.
- **Static / Import guard**: `go list -deps` shell assertion in `Makefile` target or CI step.
- **E2E (Playwright)**: Not required for this architectural refactor. Existing Playwright suite provides smoke coverage.

---

## Coverage Targets

- **Unit test coverage**: ≥ 80% line coverage for `session/claude_controller.go`, `session/detection/idle.go`, `session/instance_controller.go`
- **All public service methods with new behavior**: happy path + at least one error / edge path
- **All external integrations (goroutines, timers)**: unit test with mock double + at least one integration test asserting the full path to `ReactiveQueueManager`
- **Race-free**: all new tests MUST pass under `go test -race`

---

## Test Execution Order

Tests must be executable in isolation. The following order is recommended for CI:

1. `go build ./session/...` — import guard (catches circular imports before tests run)
2. `go test -race ./session/detection/... -run TestIdleDetector` — Epic 3 unit foundation
3. `go test -race ./session/... -run TestClaudeController` — Epic 1 unit tests
4. `go test -race ./session/... -run TestReviewQueuePoller` — Epic 2 poller unit tests
5. `go test -race ./server/... -run TestReactiveQueueManager` — Epic 2 integration tests
6. `go test -race ./server/services/... -run TestSessionService` — Epic 2 wiring integration
7. `go test -race ./...` — full regression suite (R5.4)

---

## New Test Files Summary

| File | New Tests | Covers |
|------|-----------|--------|
| `session/claude_controller_test.go` | 8 new tests | R1.1, R1.2, R1.3, R2.3, P1–P6 |
| `session/detection/idle_test.go` | 4 new tests (+ 1 existing) | R2.1, R2.2, R2.4, P4 |
| `session/review_queue_poller_test.go` | 5 new tests | R2.1, R3.1, R3.2, R3.3, R3.4 |
| `session/instance_test.go` | 1 new test | R4.2 |
| `server/review_queue_manager_test.go` | 6 new tests | R1.1, R1.5, R2.1, R5.1–R5.5 |
| `server/services/session_service_test.go` | 2 new tests | R1.5 |

**Total new test functions: 26**
**Regression tests reused unchanged: 2 suites (TestReviewQueue*, TestReviewQueuePoller*)**

---

## Requirements Coverage Summary

| Requirement | # Unit Tests | # Integration Tests | Covered? |
|-------------|-------------|-------------------|---------|
| R1.1 | 2 | 1 | Yes |
| R1.2 | 2 | 0 | Yes |
| R1.3 | 2 | 0 | Yes |
| R1.4 | 1 (static) | 0 | Yes |
| R1.5 | 1 | 2 | Yes |
| R2.1 | 2 | 1 | Yes |
| R2.2 | 2 | 0 | Yes |
| R2.3 | 1 | 0 | Yes |
| R2.4 | 1 | 0 | Yes |
| R3.1 | 1 | 0 | Yes |
| R3.2 | 2 | 0 | Yes |
| R3.3 | 1 | 0 | Yes |
| R3.4 | 1 | 0 | Yes |
| R4.1 | 1 (CI build) | 0 | Yes |
| R4.2 | 1 | 0 | Yes |
| R4.3 | 1 (grep/CI) | 0 | Yes |
| R5.1 | 0 | 1 | Yes |
| R5.2 | 0 | 1 | Yes |
| R5.3 | 1 | 1 | Yes |
| R5.4 | 0 | 2 (regression) | Yes |
| R5.5 | 0 | 2 | Yes |
| **Totals** | **22 unit** | **11 integration** | **21/21 (100%)** |
