# Validation Plan: triage-autonomous-migration

**Date**: 2026-06-15
**Derived from**: plan.md + requirements.md (AC-1 through AC-5)

---

## Requirements-to-Test Traceability

| Requirement | Test ID | Test Type | Description |
|---|---|---|---|
| AC-1.a: initial snapshot hides hidden sessions | T-GO-01 | Unit | WatchSessions initial snapshot omits `Hidden=true` sessions |
| AC-1.b: live events suppress hidden sessions | T-GO-02 | Unit | WatchSessions live event loop does not forward events for `Hidden=true` |
| AC-1.c: EventsSince replay suppresses hidden sessions | T-GO-03 | Unit | EventsSince replay skips `Hidden=true` events |
| AC-1: non-hidden sessions unaffected | T-GO-04 | Unit | WatchSessions still delivers non-hidden sessions in all three paths |
| AC-2: CreateDirectorySession starts controller | T-GO-05 | Unit | `GetController()` is non-nil after `CreateDirectorySession` with non-nil statusManager |
| AC-2: autonomous driver start (triage) | T-GO-06 | Unit | `TriggerTriage` calls `StartAutonomousDriverWithTimeout` when `autonomousStarter != nil` |
| AC-2: graceful degradation (no headless pool) | T-GO-07 | Unit | `TriggerTriage` creates `oneShot=true` session when `autonomousStarter == nil` |
| AC-2: autonomous driver start (re-review) | T-GO-08 | Unit | `TriggerReReview` calls `StartAutonomousDriverWithTimeout` when `autonomousStarter != nil` |
| AC-3: triage completion via LLM DONE (not Stop) | T-GO-09 | Unit | `submit_triage_result` does NOT call `StopDriverForSession`; driver receives no Stop signal |
| AC-3: re-review explicit stop after verdict | T-GO-10 | Unit | `submit_review_verdict` calls `StopDriverForSession` with the correct session title |
| AC-5.a: stuck-triage emits notification | T-GO-11 | Unit | `onAutonomousDriverComplete(Stuck=true, RoleTriage)` publishes NotificationEvent |
| AC-5.b: successful triage emits no stuck notification | T-GO-12 | Unit | `onAutonomousDriverComplete(Done=true, RoleTriage)` does NOT publish stuck NotificationEvent |
| AC-5.c: work-stuck does not emit triage notification | T-GO-13 | Unit | `onAutonomousDriverComplete(Stuck=true, RoleWork)` does NOT publish stuck-triage NotificationEvent |
| AC-4.a: triage → ready on Done | T-GO-14 | Unit | `onAutonomousDriverComplete(Done=true, RoleTriage)` transitions item to `BacklogStatusReady` |
| AC-4.b: triage stuck → no transition | T-GO-15 | Unit | `onAutonomousDriverComplete(Stuck=true, RoleTriage)` does NOT call `TransitionBacklogItemStatus` |
| AC-4.c: work → review transition | T-GO-16 | Unit | `onAutonomousDriverComplete(Done=true, RoleWork)` transitions item to `BacklogStatusReview` |
| AC-4.d: review → no transition | T-GO-17 | Unit | `onAutonomousDriverComplete(*, RoleReview)` does NOT call `TransitionBacklogItemStatus` |
| Epic 6: configurable startup timeout | T-GO-18 | Unit | `NewAutonomousDriver(WithStartupTimeout(5*time.Minute))` sets `startupTimeout=5min`; default is 60s |
| E-B2: compile-time assertion | T-GO-19 | Compile | `var _ AutonomousDriverStarter = (*SessionService)(nil)` compiles without error |
| Epic 7: full build + lint | T-CI-01 | CI | `make quick-check` passes with zero errors |
| Epic 7: targeted package tests | T-CI-02 | CI | `go test ./server/services/... ./server/mcp/... ./session/...` all pass |

---

## Test Case Definitions

### T-GO-01: WatchSessions initial snapshot hides hidden sessions

**File**: `server/services/session_service_test.go`
**Task**: 1.1d
**Setup**: Seed store with one `Hidden=true` session, one `Hidden=false` session.
**Action**: Call `WatchSessions` and collect the initial snapshot events.
**Assert**: Only the non-hidden session appears in the snapshot. The hidden session is absent.

---

### T-GO-02: WatchSessions live event loop suppresses hidden session events

**File**: `server/services/session_service_test.go`
**Task**: 1.1d
**Setup**: Subscribe to `WatchSessions`. Publish a `SessionCreated` event for a `Hidden=true` session.
**Assert**: The stream receives no event for the hidden session.

---

### T-GO-03: EventsSince replay skips hidden sessions

**File**: `server/services/session_service_test.go`
**Task**: 1.1d
**Setup**: Publish a `SessionCreated` event for a `Hidden=true` session. Then connect a client with `AfterSeq` pointing before that event.
**Assert**: The replay does not deliver the hidden session event to the new subscriber.

---

### T-GO-04: Non-hidden sessions are unaffected

**File**: `server/services/session_service_test.go`
**Task**: 1.1d
**Setup**: Seed one `Hidden=false` session.
**Assert**: It appears in the initial snapshot, live events, and EventsSince replay.

---

### T-GO-05: CreateDirectorySession starts the controller

**File**: `server/services/session_service_test.go`
**Task**: 0.1b
**Setup**: Build a `SessionService` with a non-nil mock `statusManager`. Call `CreateDirectorySession`.
**Assert**: The returned instance's `GetController()` is non-nil.
**Note**: The test MUST wire a non-nil `statusManager` — `StartController()` is gated on that condition. A nil `statusManager` makes this test vacuous.

---

### T-GO-06: TriggerTriage calls StartAutonomousDriverWithTimeout

**File**: `server/services/backlog_service_test.go`
**Task**: 2.1a, 2.2b
**Setup**: Wire `BacklogService` with a mock `AutonomousDriverStarter`. Configure `autonomousStarter != nil`.
**Action**: Call `TriggerTriage` on a backlog item.
**Assert**: Mock's `StartAutonomousDriverWithTimeout` was called with the newly-created instance and `5*time.Minute`.
**Assert**: `CreateDirectorySession` was called with `oneShot=false`.

---

### T-GO-07: TriggerTriage falls back to oneShot when autonomousStarter is nil

**File**: `server/services/backlog_service_test.go`
**Task**: 2.1a
**Setup**: Wire `BacklogService` with `autonomousStarter=nil`.
**Action**: Call `TriggerTriage`.
**Assert**: `CreateDirectorySession` was called with `oneShot=true`.
**Assert**: `StartAutonomousDriverWithTimeout` was NOT called.

---

### T-GO-08: TriggerReReview calls StartAutonomousDriverWithTimeout

**File**: `server/services/backlog_service_test.go`
**Task**: 3.1a
**Setup**: Mirror T-GO-06 but call `TriggerReReview`.
**Assert**: Mock's `StartAutonomousDriverWithTimeout` called with `5*time.Minute`.

---

### T-GO-09: submit_triage_result does not stop the AutonomousDriver

**File**: `server/mcp/tools_backlog_test.go`
**Task**: Epic 5 design — absence of stop for triage
**Setup**: Register a mock `ReviewCompletionSignaler`. Invoke `submitTriageResult` (the MCP handler for `submit_triage_result`).
**Assert**: Mock `StopDriverForSession` was NOT called.

---

### T-GO-10: submit_review_verdict stops the re-review AutonomousDriver

**File**: `server/mcp/tools_backlog_test.go`
**Task**: 5.1e
**Setup**: Register a mock `ReviewCompletionSignaler` as `reviewStopper`. Invoke `submitReviewVerdict` with a known session UUID.
**Assert**: Mock `StopDriverForSession` was called with the title matching the UUID.

---

### T-GO-11: Stuck triage emits NotificationEvent

**File**: `server/services/session_service_test.go`
**Task**: 8.1b
**Setup**: Configure a mock `eventBus`. Call `onAutonomousDriverComplete` with `Stuck=true` and an `ItemSession` with `SessionRoleTriage`.
**Assert**: `eventBus.Publish` was called with a `NotificationEvent` whose title contains "stuck" (case-insensitive) or matches "Triage stuck".

---

### T-GO-12: Successful triage does not emit stuck notification

**File**: `server/services/session_service_test.go`
**Task**: 8.1b
**Setup**: Same as T-GO-11 but with `Done=true`.
**Assert**: `eventBus.Publish` was NOT called with a "Triage stuck" NotificationEvent.

---

### T-GO-13: Work-stuck does not emit triage notification

**File**: `server/services/session_service_test.go`
**Task**: 8.1b
**Setup**: Call `onAutonomousDriverComplete` with `Stuck=true` and `SessionRoleWork`.
**Assert**: No "Triage stuck" NotificationEvent published.

---

### T-GO-14: Triage Done → BacklogStatusReady

**File**: `server/services/session_service_test.go`
**Task**: 4.1b
**Setup**: Mock `TransitionBacklogItemStatus`. Call `onAutonomousDriverComplete(Done=true, RoleTriage)`.
**Assert**: `TransitionBacklogItemStatus` called with `BacklogStatusReady` and `ExpectedStatus=idea`.

---

### T-GO-15: Triage Stuck → no transition

**File**: `server/services/session_service_test.go`
**Task**: 4.1b
**Setup**: Mock `TransitionBacklogItemStatus`. Call `onAutonomousDriverComplete(Stuck=true, RoleTriage)`.
**Assert**: `TransitionBacklogItemStatus` NOT called.

---

### T-GO-16: Work Done → BacklogStatusReview

**File**: `server/services/session_service_test.go`
**Task**: 4.1b
**Assert**: `TransitionBacklogItemStatus` called with `BacklogStatusReview` and `ExpectedStatus=in_progress`.

---

### T-GO-17: Review role → no transition

**File**: `server/services/session_service_test.go`
**Task**: 4.1b
**Assert**: `TransitionBacklogItemStatus` NOT called regardless of Done/Stuck.

---

### T-GO-18: Configurable startup timeout

**File**: `session/autonomous_driver_test.go`
**Task**: 6.1d
**Assert (custom timeout)**: `NewAutonomousDriver(inst, pool, "goal", 0, WithStartupTimeout(5*time.Minute))` → `driver.startupTimeout == 5*time.Minute`.
**Assert (default)**: `NewAutonomousDriver(inst, pool, "goal", 0)` → `driver.startupTimeout == 60*time.Second`.

---

### T-GO-19: Compile-time interface assertion

**File**: `server/services/session_service.go`
**Task**: 2.2c
**Assert**: `var _ AutonomousDriverStarter = (*SessionService)(nil)` present and compiles. Verified by `go build ./server/services/...` succeeding.

---

### T-CI-01: make quick-check passes

```bash
make quick-check
```
Expected: zero errors, zero lint warnings that block CI.

---

### T-CI-02: Targeted package tests pass

```bash
go test ./server/services/... ./server/mcp/... ./session/...
```
Expected: all tests pass, no race conditions (`-race` flag if available).

---

## Coverage Summary

| Package | Tests | ACs Covered |
|---|---|---|
| `server/services` | T-GO-01–05, T-GO-11–17 | AC-1, AC-2 (partial), AC-4, AC-5 |
| `server/services` (backlog) | T-GO-06–08 | AC-2 |
| `server/mcp` | T-GO-09–10 | AC-3 |
| `session` | T-GO-18 | Epic 6 |
| build | T-GO-19, T-CI-01, T-CI-02 | E-B2, Epic 7 |

All 5 ACs from requirements.md have at least one test case. AC-4 (architecture clarity) is research-only and has no code test.
