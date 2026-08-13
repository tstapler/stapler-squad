# Session State Machine Redesign + Hibernation — Validation Plan

## Coverage Summary

| Epic | Acceptance Criteria | Tests Mapped |
|---|---|---|
| Epic 1: State Machine | SM-1 through SM-4, all auto-resume guards | 100% (14/14 criteria) |
| Epic 2: Async Creation | CREATE-1 through CREATE-3 | 100% (6/6 criteria) |
| Epic 3: Sub-Status Visibility | VIS-1 through VIS-3 | 100% (7/7 criteria) |
| Epic 4: Hibernation | FR-1 through FR-8, Technical Constraints | 100% (18/18 criteria) |
| **Total** | **45 acceptance criteria** | **100% (45/45)** |

**Test count by type:**

| Type | Count |
|---|---|
| Unit (Go) | 38 |
| Integration (Go, requires tmux) | 12 |
| Unit (TypeScript/Jest) | 19 |
| E2E (Playwright) | 9 |
| **Total** | **78** |

---

## Requirements Coverage Matrix

| Requirement ID | Acceptance Criterion | Test IDs |
|---|---|---|
| SM-1 | `Running`/`Ready` merged into `Active`; guards gone | SM-1-a through SM-1-e |
| SM-2 | `Loading` merged into `Creating` | SM-2-a, SM-2-b |
| SM-3 | `NeedsApproval` removed as lifecycle state | SM-3-a, SM-3-b |
| SM-4 | Proto enum updated, adapter round-trips lossless | SM-4-a through SM-4-d |
| CREATE-1 | `CreateSession` returns immediately with `Creating` status | CREATE-1-a, CREATE-1-b |
| CREATE-2 | Progress events delivered over `WatchSessions` | CREATE-2-a |
| CREATE-3 | `Creating` sessions disable mutating actions | CREATE-3-a, CREATE-3-b |
| VIS-1 | `sub_status` field in proto; never persisted | VIS-1-a, VIS-1-b |
| VIS-2 | `SessionRow`/`SessionCard` show sub-status chip | VIS-2-a through VIS-2-c |
| VIS-3 | Group-by-sub-status strategy | VIS-3-a |
| FR-1 | Idle auto-hibernate sweeper | FR-1-a through FR-1-c |
| FR-2 | Manual hibernate via context menu | FR-2-a, FR-2-b |
| FR-3 | Resource-pressure hibernate with hysteresis | FR-3-a, FR-3-b |
| FR-4 | Checkpoint written; process killed on hibernate | FR-4-a through FR-4-c |
| FR-5 | Config fields load with defaults | FR-5-a, FR-5-b |
| FR-6 | Resume re-launches process, loads scrollback | FR-6-a through FR-6-c |
| FR-7 | Hibernated badge distinct in row/card views | FR-7-a, FR-7-b |
| FR-8 | Checkpoint deleted when session deleted | FR-8-a |
| TECH-GUARD-1 | `FromInstanceData()` excludes `Hibernated` from auto-start | GUARD-1-a, GUARD-1-b |
| TECH-GUARD-2 | Health checker skips `Hibernated` sessions | GUARD-2-a |
| TECH-GUARD-3 | Stale-resume path skips `Hibernated` sessions | GUARD-3-a |

---

## Epic 1: State Machine Redesign

### State Machine Transition Tests

**File:** `session/state_machine_test.go`

#### SM-TRANS-1: `TestStateMachine_ValidTransitions`

**Type:** Unit
**Test function:** `TestStateMachine_ValidTransitions`

Asserts each allowed transition succeeds (no error from `CanTransition` or `transitionTo`):

```
Creating    → Active
Creating    → Stopped
Active      → Paused
Active      → Stopped
Active      → Hibernated
Paused      → Active
Paused      → Stopped
Stopped     → Active
Hibernated  → Active
Hibernated  → Stopped
```

Each sub-case uses `assert.True(t, CanTransition(from, to), ...)`.

---

#### SM-TRANS-2: `TestStateMachine_InvalidTransitions`

**Type:** Unit
**Test function:** `TestStateMachine_InvalidTransitions`

Asserts rejected transitions return false / error. Critical cases:

| From | To | Why |
|---|---|---|
| `Active` | `Creating` | No back-edge to Creating |
| `Stopped` | `Paused` | Must go through Active |
| `Stopped` | `Hibernated` | Must go through Active |
| `Hibernated` | `Paused` | Must go through Active |
| `Paused` | `Hibernated` | Must go through Active |
| `Creating` | `Paused` | Cannot pause before first start |
| `Creating` | `Hibernated` | Cannot hibernate before first start |

---

#### SM-TRANS-3: `TestStateMachine_RemovedTransitionsReturnFalse`

**Type:** Unit
**Test function:** `TestStateMachine_RemovedTransitionsReturnFalse`

Explicitly verifies removed entries from the old transition table:

- `CanTransition(Running, Ready)` returns `false` (Running and Ready no longer exist as distinct states)
- `CanTransition(Active, NeedsApproval)` returns `false` (`NeedsApproval` removed from lifecycle)
- `CanTransition(Loading, Active)` returns `false` (`Loading` removed)
- `CanTransition(Ready, Running)` returns `false`

These guard against accidental re-addition of removed states.

---

### Status Constant Tests

**File:** `session/instance_test.go` (extend existing) or `session/status_test.go` (create)

#### SM-1-a: `TestStatusConstants_AliasEquality`

**Type:** Unit
**Test function:** `TestStatusConstants_AliasEquality`

Asserts:
- `Running == Active` (alias equality)
- `Ready == Active`
- `Loading == Creating`
- `Active.String() == "Active"`
- `Hibernated.String() == "Hibernated"`
- `Creating.String() == "Creating"`

---

#### SM-1-b: `TestActiveHelper_ReturnsTrueOnlyForActive`

**Type:** Unit
**Test function:** `TestActiveHelper_ReturnsTrueOnlyForActive`

For each lifecycle status (`Active`, `Paused`, `Stopped`, `Creating`, `Hibernated`), creates a minimal `Instance` struct with that status and asserts `Active()` returns the expected boolean.

---

#### SM-1-c: `TestHibernatedHelper_ReturnsTrueOnlyForHibernated`

**Type:** Unit
**Test function:** `TestHibernatedHelper_ReturnsTrueOnlyForHibernated`

Mirror of SM-1-b for `Hibernated()`.

---

#### SM-1-d: `TestStatusIsValid_AllFiveStates`

**Type:** Unit
**Test function:** `TestStatusIsValid_AllFiveStates`

Asserts `IsValid()` returns `true` for all 5 lifecycle states and `false` for removed states (`NeedsApproval` as standalone int, invalid iota values).

---

#### SM-1-e: `TestNoRunningReadyGuards_Compilation`

**Type:** Unit (static, verified via `make lint`)
**Test function:** N/A — enforced by `grep` assertion in CI

Assertion: `grep -rn "== Running\|== Ready\|== NeedsApproval\|case Running\|case Ready\|case NeedsApproval" session/ server/ | grep -v "_test.go" | grep -v "deprecated"` returns zero lines. Document this as a lint rule in the Makefile `ci` target.

---

#### SM-2-a: `TestLoadingAlias_EqualsCreating`

**Type:** Unit
**Test function:** `TestLoadingAlias_EqualsCreating`

Asserts `Loading == Creating` and `Loading.String()` returns `"Creating"` (not `"Loading"`).

---

#### SM-2-b: `TestFromInstanceData_LoadingStatusBecomesCreating`

**Type:** Unit
**Test function:** `TestFromInstanceData_LoadingStatusBecomesCreating`

Constructs an `InstanceData` with a `Status` value that was previously `Loading` (the integer `3` if that was its iota value, or whatever the deprecated integer maps to) and calls `FromInstanceData()`. Verifies the resulting instance has `Status == Creating` (not a garbage/zero value).

---

#### SM-3-a: `TestNeedsApproval_NotALifecycleState`

**Type:** Unit
**Test function:** `TestNeedsApproval_NotALifecycleState`

Asserts that `NeedsApproval` is not declared as a `Status` constant anywhere in the `session` package by verifying the state machine's `allowedTransitions` map has no key or value equal to `NeedsApproval`. Uses the exported `AllowedTransitions()` accessor (or tests via `CanTransition`).

---

#### SM-3-b: `TestApprovalState_NeverSetAsInstanceStatus`

**Type:** Unit
**Test function:** `TestApprovalState_NeverSetAsInstanceStatus`

Calls `status_mapping.go` functions with all `detection.Status` values (including `StatusNeedsApproval`) and asserts none of them produce an `instance.Status` of a removed lifecycle state. Verified via the mapping function's return values.

---

### Proto and Adapter Tests

**File:** `server/adapters/instance_adapter_test.go` (extend or create)

#### SM-4-a: `TestProtoStatusIntegerValues_Stable`

**Type:** Unit
**Test function:** `TestProtoStatusIntegerValues_Stable`

Regression guard — wire values must not shift. Asserts:
- `SESSION_STATUS_ACTIVE == 1`
- `SESSION_STATUS_PAUSED == 4`
- `SESSION_STATUS_STOPPED == 7`
- `SESSION_STATUS_HIBERNATED == 8`
- `SESSION_STATUS_CREATING == 6`

If any of these values shifts, existing sessions in SQLite will silently mis-categorize.

---

#### SM-4-b: `TestAdapterStatusRoundTrip_AllLiveStates`

**Type:** Unit
**Test function:** `TestAdapterStatusRoundTrip_AllLiveStates`

For each live lifecycle state (`Active`, `Paused`, `Stopped`, `Creating`, `Hibernated`): convert Go → proto → Go and assert the round-trip is lossless.

---

#### SM-4-c: `TestAdapterStatusRoundTrip_LegacyProtoValues`

**Type:** Unit
**Test function:** `TestAdapterStatusRoundTrip_LegacyProtoValues`

Verifies that old proto integer values received from a legacy client do not panic and resolve to valid Go statuses:

| Proto integer | Expected Go status |
|---|---|
| 1 (`RUNNING`) | `Active` |
| 2 (`READY`) | `Active` |
| 3 (`LOADING`) | `Creating` |
| 5 (`NEEDS_APPROVAL`) | `Active` |

---

#### SM-4-d: `TestProtoSubStatus_GeneratedTypes`

**Type:** Unit (compilation check)
**Test function:** `TestProtoSubStatus_GeneratedTypes`

Asserts `sessionv1.SubStatus_SUB_STATUS_PROCESSING` and `sessionv1.SubStatus_SUB_STATUS_NEEDS_APPROVAL` exist in the generated Go code (compilation-time check; if this file compiles, the proto was regenerated correctly).

---

### Auto-Resume Guard Tests (CRITICAL)

These three tests are the critical regression guards required by the Technical Constraints section. They must pass before Epic 4 can merge.

**File:** `session/instance_serialization_test.go` (extend existing) or `session/hibernate_guard_test.go` (new)

#### GUARD-1-a: `TestFromInstanceData_HibernatedSessionNotAutoStarted`

**Type:** Integration (requires filesystem; no tmux needed)
**Test function:** `TestFromInstanceData_HibernatedSessionNotAutoStarted`
**Maps to:** Technical Constraint §1, requirements.md acceptance criterion "Hibernated sessions are NOT auto-resumed on server restart"

Setup:
1. Create a minimal `InstanceData` with `Status == Hibernated`.
2. Point `Path` to `t.TempDir()` (exists on disk).
3. Call `FromInstanceData(data, ...)`.

Asserts:
- No error is returned.
- `instance.Status == Hibernated` after deserialization.
- `instance.Started() == true` (tmux object wired, but process not launched).
- The underlying `Start()` method was NOT called — verified via a mock or by asserting `instance.TmuxAlive() == false` (no real tmux was started).

---

#### GUARD-1-b: `TestFromInstanceData_HibernatedSessionWorktreeNotTransitionedToPaused`

**Type:** Unit
**Test function:** `TestFromInstanceData_HibernatedSessionWorktreeNotTransitionedToPaused`
**Maps to:** Technical Constraint §1 (worktree-missing guard update)

Setup:
1. Create `InstanceData` with `Status == Hibernated` and a path that has NO git worktree.
2. Call `FromInstanceData()`.

Asserts:
- `instance.Status` remains `Hibernated` (not silently transitioned to `Paused`).
- No error is returned.

---

#### GUARD-2-a: `TestHealthChecker_SkipsHibernatedSession`

**Type:** Unit
**Test function:** `TestHealthChecker_SkipsHibernatedSession`
**Maps to:** Technical Constraint §2, requirements.md acceptance criterion "Hibernated sessions are NOT auto-resumed on server restart or by the health checker"

Setup:
1. Create a minimal `Instance` with `Status == Hibernated` and `started == true`.
2. Call `CheckHealth(instance)` (or the equivalent health checker function from `session/health.go`).

Asserts:
- Result `IsHealthy == true` (hibernated is expected state, not a failure).
- Result `RecoveryAttempted == false`.
- Result `Actions` contains the string `"Skipped (session is hibernated)"`.
- `instance.Start()` was not called (assert via `instance.TmuxAlive() == false`).

---

#### GUARD-3-a: `TestStaleResumeRecovery_SkipsHibernatedSession`

**Type:** Unit
**Test function:** `TestStaleResumeRecovery_SkipsHibernatedSession`
**Maps to:** Technical Constraint §3

Setup:
1. Create an `Instance` with `Status == Hibernated`.
2. Simulate the stale-resume exit condition (set a conversation UUID whose JSONL file does not exist).
3. Call `recoverFromStaleResume()` directly (or trigger it via the scrollback monitor callback).

Asserts:
- No `Start()` call occurs.
- `instance.Status` remains `Hibernated` after the call.

---

## Epic 2: Async Session Creation

**File:** `server/services/session_service_test.go` (extend) and `tests/e2e/session-creation-async.spec.ts`

#### CREATE-1-a: `TestCreateSession_ReturnsImmediatelyWithCreatingStatus`

**Type:** Integration (Go service-layer)
**Test function:** `TestCreateSession_ReturnsImmediatelyWithCreatingStatus`

Uses the existing service test harness (mock storage). Calls `CreateSession` with a path requiring setup. Records time before and after. Asserts:
- Response received within 200ms.
- Returned session has `status == SESSION_STATUS_CREATING`.
- Background goroutine eventually transitions session to `Active` (poll storage with `require.Eventually`).

---

#### CREATE-1-b: `TestCreateSession_AsyncSetupFailure_TransitionsToStopped`

**Type:** Integration (Go service-layer)
**Test function:** `TestCreateSession_AsyncSetupFailure_TransitionsToStopped`

Injects a setup error (via mock). Asserts:
- Session transitions to `Stopped` with a non-empty `creation_progress` error message.
- No panic in the background goroutine.

---

#### CREATE-2-a: `TestCreateSession_ProgressEventsDelivered`

**Type:** Integration (Go service-layer)
**Test function:** `TestCreateSession_ProgressEventsDelivered`

Uses a mock `WatchSessions` event capture. Asserts:
- At least one `SessionEvent` with `status == Creating` and a non-empty `creation_progress` is broadcast.
- A final event with `status == Active` is broadcast after setup completes.

---

#### CREATE-3-a: `TestHibernateSession_RejectsCreatingSession`

**Type:** Unit (Go service-layer)
**Test function:** `TestHibernateSession_RejectsCreatingSession`

Asserts that calling `HibernateSession` on a session with `Status == Creating` returns `connect.CodeFailedPrecondition`.

---

#### CREATE-3-b: `TestPauseSession_RejectsCreatingSession`

**Type:** Unit (Go service-layer)
**Test function:** `TestPauseSession_RejectsCreatingSession`

Same pattern for `PauseSession`.

---

### TypeScript: Creating UX

**File:** `web-app/src/components/sessions/SessionRow.test.tsx`

#### CREATE-3-c: `SessionRow_DisablesActionsForCreatingSessions`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionRow_DisablesActionsForCreatingSessions`

Renders `<SessionRow>` with a session in `Creating` status. Asserts:
- Hibernate, Pause, and Delete action buttons are either absent or have `aria-disabled="true"`.
- A spinner element (or element with role `status`) is visible.
- `session.creation_progress` text renders when non-empty.

---

## Epic 3: Sub-Status Visibility

### Backend Sub-Status Mapping

**File:** `server/adapters/instance_adapter_test.go`

#### VIS-1-a: `TestToProtoSubStatus_MappingAllDetectedStatuses`

**Type:** Unit
**Test function:** `TestToProtoSubStatus_MappingAllDetectedStatuses`

Table-driven test over all `detection.Status` values. For an `Active` instance:

| `DetectedStatus` | Expected `SubStatus` proto |
|---|---|
| `StatusProcessing` | `SUB_STATUS_PROCESSING` |
| `StatusReady` | `SUB_STATUS_IDLE` |
| `StatusNeedsApproval` | `SUB_STATUS_NEEDS_APPROVAL` |
| `StatusError` | `SUB_STATUS_ERROR` |
| `StatusTestsFailing` | `SUB_STATUS_TESTS_FAILING` |
| Rate limit waiting | `SUB_STATUS_RATE_LIMITED` |

---

#### VIS-1-b: `TestToProtoSubStatus_NonActiveAlwaysUnspecified`

**Type:** Unit
**Test function:** `TestToProtoSubStatus_NonActiveAlwaysUnspecified`

For each non-`Active` lifecycle state (`Paused`, `Stopped`, `Hibernated`, `Creating`), asserts that `toProtoSubStatus()` returns `SUB_STATUS_UNSPECIFIED` regardless of what `GetEffectiveStatus()` would return.

This enforces ADR-B: sub-status is only meaningful for `Active` sessions.

---

#### VIS-1-c: `TestSubStatus_NotPersistedInDatabase`

**Type:** Unit
**Test function:** `TestSubStatus_NotPersistedInDatabase`

Asserts that the ent schema for `Session` does NOT have a `sub_status` column. Verifies the field only appears in the proto-response path, not in `InstanceData` or the ent schema. Implemented as a schema introspection test or by verifying the `InstanceData` struct has no `SubStatus` field via `reflect`.

---

### TypeScript: SubStatusChip Component

**File:** `web-app/src/components/sessions/SubStatusChip.test.tsx`

#### VIS-2-a: `SubStatusChip_RendersNullForIdleAndUnspecified`

**Type:** Unit (Jest/RTL)
**Test function:** `SubStatusChip_RendersNullForIdleAndUnspecified`

Renders `<SubStatusChip subStatus={SubStatus.SUB_STATUS_IDLE} />` and `<SubStatusChip subStatus={SubStatus.SUB_STATUS_UNSPECIFIED} />`. Asserts both return a null/empty render (no DOM element produced).

---

#### VIS-2-b: `SubStatusChip_RendersNeedsApprovalWithWarningStyle`

**Type:** Unit (Jest/RTL)
**Test function:** `SubStatusChip_RendersNeedsApprovalWithWarningStyle`

Renders `<SubStatusChip subStatus={SubStatus.SUB_STATUS_NEEDS_APPROVAL} />`. Asserts:
- An element with text "Needs Approval" is present.
- Element has a CSS class or `data-` attribute indicating warning/orange styling.
- Element has an accessible label (`aria-label` or role).

---

#### VIS-2-c: `SubStatusChip_RendersProcessingWithSpinner`

**Type:** Unit (Jest/RTL)
**Test function:** `SubStatusChip_RendersProcessingWithSpinner`

Renders `<SubStatusChip subStatus={SubStatus.SUB_STATUS_PROCESSING} />`. Asserts:
- A spinner element (or element with `aria-label` containing "Thinking") is present.

---

#### VIS-2-d: `SubStatusChip_RendersAllNonIdleVariants`

**Type:** Unit (Jest/RTL)
**Test function:** `SubStatusChip_RendersAllNonIdleVariants`

Parametrized over `ERROR`, `TESTS_FAILING`, `RATE_LIMITED`. Asserts each renders a non-null element with appropriate text.

---

#### VIS-2-e: `SessionRow_ShowsSubStatusChipForActiveProcessing`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionRow_ShowsSubStatusChipForActiveProcessing`

Renders `<SessionRow>` with a session where `status == Active` and `subStatus == SUB_STATUS_PROCESSING`. Asserts the `SubStatusChip` is present in the rendered output.

---

#### VIS-2-f: `SessionRow_HidesSubStatusChipForPausedSession`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionRow_HidesSubStatusChipForPausedSession`

Renders `<SessionRow>` with `status == Paused`. Asserts no sub-status chip is present.

---

#### VIS-2-g: `SessionRow_HidesSubStatusChipForHibernatedSession`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionRow_HidesSubStatusChipForHibernatedSession`

Renders `<SessionRow>` with `status == Hibernated`. Asserts no sub-status chip is present. This is a focused regression guard to ensure the new `Hibernated` status doesn't accidentally show a stale sub-status.

---

### Grouping by Sub-Status

**File:** `web-app/src/lib/grouping/grouping.test.ts` (extend) or `subStatusGrouping.test.ts`

#### VIS-3-a: `SubStatusGrouping_NeedsApprovalGroupSortedFirst`

**Type:** Unit (Jest)
**Test function:** `SubStatusGrouping_NeedsApprovalGroupSortedFirst`

Creates a list of sessions with mixed sub-statuses (Processing, NeedsApproval, Idle). Applies the SubStatus grouping strategy. Asserts:
- A "Needs Approval" group exists and contains the correct session(s).
- "Needs Approval" group appears first in the output.
- Non-`Active` sessions are grouped under "Inactive" or equivalent.

---

## Epic 4: Session Hibernation

### Config Tests

**File:** `config/config_test.go` (extend)

#### FR-5-a: `TestHibernationConfig_DefaultValues`

**Type:** Unit
**Test function:** `TestHibernationConfig_DefaultValues`

Unmarshals `config.json` with no `hibernation` key set. Asserts:
- `Enabled == true`
- `IdleTimeoutMinutes == 120`
- `MemoryThresholdPct == 85`
- `MemoryHysteresisPct == 75`
- `RetentionDays == 30`

---

#### FR-5-b: `TestHibernationCheckpointDirOrDefault_ExpandsTilde`

**Type:** Unit
**Test function:** `TestHibernationCheckpointDirOrDefault_ExpandsTilde`

Asserts:
- When `CheckpointDir == ""`, returns an absolute path ending in `checkpoints`.
- When `CheckpointDir == "~/custom"`, the `~` is expanded to the home directory.
- Result is always an absolute path (starts with `/`).

---

### Core Hibernate/Resume Unit Tests

**File:** `session/hibernate_test.go` (new file)

#### FR-4-a: `TestHibernate_ActiveSession_WritesCheckpointAndTransitions`

**Type:** Unit (with mock tmux; no real tmux required)
**Test function:** `TestHibernate_ActiveSession_WritesCheckpointAndTransitions`

Setup:
1. Create an `Instance` with `Status == Active`, `started == true`.
2. Use a mock `tmuxManager` that records `KillSession()` calls.
3. Provide `checkpointDir = t.TempDir()`.

Asserts:
- `Hibernate(ctx, checkpointDir)` returns `nil`.
- `instance.Status == Hibernated`.
- `<checkpointDir>/<uuid>/checkpoint.json` exists and is valid JSON.
- `<checkpointDir>/<uuid>/scrollback_ref.txt` exists (or checkpoint.json contains scrollback path field).
- Mock `KillSession()` was called exactly once.

---

#### FR-4-b: `TestHibernate_NonActiveSession_ReturnsError`

**Type:** Unit
**Test function:** `TestHibernate_NonActiveSession_ReturnsError`

Table-driven over `Paused`, `Stopped`, `Creating`, `Hibernated` (already hibernated). Asserts:
- `Hibernate()` returns a non-nil error for each.
- `instance.Status` does not change.
- No checkpoint files are written.

---

#### FR-4-c: `TestHibernate_CheckpointWriteFailure_RollsBackStatus`

**Type:** Unit
**Test function:** `TestHibernate_CheckpointWriteFailure_RollsBackStatus`

Injects a write failure (non-writable checkpoint dir). Asserts:
- `Hibernate()` returns an error.
- `instance.Status` is rolled back to `Active`.
- `KillSession()` was NOT called (process not killed if checkpoint failed).

---

#### FR-6-a: `TestResumeFromHibernation_HibernatedSession_TransitionsToActive`

**Type:** Unit (with mock tmux)
**Test function:** `TestResumeFromHibernation_HibernatedSession_TransitionsToActive`

Setup: `Instance` with `Status == Hibernated`, checkpoint files in `t.TempDir()`.

Asserts:
- `ResumeFromHibernation()` returns `nil`.
- `instance.Status == Active`.
- Checkpoint directory is deleted after successful resume.
- Mock `Start()` was called.

---

#### FR-6-b: `TestResumeFromHibernation_NonHibernatedSession_ReturnsError`

**Type:** Unit
**Test function:** `TestResumeFromHibernation_NonHibernatedSession_ReturnsError`

Asserts `ResumeFromHibernation()` returns an error for `Active`, `Paused`, `Stopped`, `Creating`.

---

#### FR-6-c: `TestResumeFromHibernation_StartFailure_RollsBackToHibernated`

**Type:** Unit
**Test function:** `TestResumeFromHibernation_StartFailure_RollsBackToHibernated`

Mock `Start()` to return an error. Asserts:
- `ResumeFromHibernation()` returns an error.
- `instance.Status` is rolled back to `Hibernated` (not left in `Active`).
- Checkpoint files are NOT deleted (session remains resumable).

---

### Full Hibernation Lifecycle Integration Test

**File:** `session/session_restart_test.go` (extend, following existing integration test patterns)

#### HIBE-INTEG-1: `TestHibernationLifecycle`

**Type:** Integration (requires tmux)
**Test function:** `TestHibernationLifecycle`

This is the primary end-to-end integration test for the hibernation workflow, analogous to `TestColdRestore_WithUUID`.

```
if testing.Short() { t.Skip("skipping integration test that starts real tmux sessions") }
checkTmuxAvailable(t)
```

Steps:
1. Create an instance via `NewInstanceWithCleanup`, using an isolated `coldRestoreSocket`.
2. Start the instance with `StartWithCleanup(true)`.
3. Wait for `TmuxAlive()` to be true (`require.Eventually`, 10s, 50ms).
4. Assert `instance.Status == Active`.
5. Call `instance.Hibernate(ctx, t.TempDir())`.
6. Assert `instance.Status == Hibernated`.
7. Assert `instance.TmuxAlive() == false`.
8. Assert checkpoint files exist on disk.
9. Serialize to `InstanceData` via `ToInstanceData()`.
10. Deserialize via `FromInstanceData()` — simulates server restart.
11. Assert deserialized instance `Status == Hibernated` (NOT auto-started).
12. Assert `deserialized.TmuxAlive() == false`.
13. Call `deserialized.ResumeFromHibernation(ctx, checkpointDir)`.
14. Wait for `deserialized.TmuxAlive()` (`require.Eventually`, 10s, 50ms).
15. Assert `deserialized.Status == Active`.
16. Assert checkpoint directory no longer exists.

---

#### HIBE-INTEG-2: `TestHibernationLifecycle_ServerRestartDoesNotAutoResume`

**Type:** Integration (requires tmux)
**Test function:** `TestHibernationLifecycle_ServerRestartDoesNotAutoResume`

Focused regression guard for TECH-GUARD-1. Verifies that a deserialized hibernated session never transitions to `Active` without an explicit `ResumeFromHibernation()` call, even if deserialization runs the same code path as server startup.

Steps:
1. Create and start a real tmux session.
2. Hibernate it (Status → Hibernated, tmux killed).
3. Serialize to `InstanceData`.
4. Wait 500ms (ensures no background goroutine fires).
5. Deserialize via `FromInstanceData()`.
6. Assert `Status == Hibernated` still.
7. Assert `TmuxAlive() == false`.
8. Assert no tmux sessions matching the instance's prefix were created.

---

### RPC Handler Tests

**File:** `server/services/session_service_test.go` (extend)

#### FR-2-a: `TestHibernateSession_RPC_ActiveSession`

**Type:** Integration (Go service-layer, no tmux)
**Test function:** `TestHibernateSession_RPC_ActiveSession`

Uses mock `Instance` that records `Hibernate()` calls. Asserts:
- RPC returns `HibernateSessionResponse` with `status == Hibernated`.
- Storage `Update()` was called with the hibernated instance.
- `broadcastSessionEvent()` was called.

---

#### FR-2-b: `TestHibernateSession_RPC_PausedSession_ReturnsFailedPrecondition`

**Type:** Unit (Go service-layer)
**Test function:** `TestHibernateSession_RPC_PausedSession_ReturnsFailedPrecondition`

Calls `HibernateSession` with an instance in `Paused` state. Asserts the error code is `connect.CodeFailedPrecondition`.

---

#### FR-6-d: `TestResumeHibernatedSession_RPC_HibernatedSession`

**Type:** Integration (Go service-layer)
**Test function:** `TestResumeHibernatedSession_RPC_HibernatedSession`

Asserts:
- RPC returns `ResumeHibernatedSessionResponse` with `status == Active`.
- Storage `Update()` was called.
- `broadcastSessionEvent()` was called.

---

#### FR-6-e: `TestResumeHibernatedSession_RPC_PausedSession_ReturnsFailedPrecondition`

**Type:** Unit (Go service-layer)
**Test function:** `TestResumeHibernatedSession_RPC_PausedSession_ReturnsFailedPrecondition`

Asserts error code is `connect.CodeFailedPrecondition` when called on a `Paused` session (must not route through this RPC).

---

### Idle Auto-Hibernate Sweeper Tests

**File:** `session/hibernation_sweeper_test.go` (new)

#### FR-1-a: `TestHibernationSweeper_HibernatesIdleActiveSessions`

**Type:** Unit (with mocks)
**Test function:** `TestHibernationSweeper_HibernatesIdleActiveSessions`

Setup:
- Mock storage returning two `Active` instances: one with `LastMeaningfulOutput` 3 hours ago (beyond threshold), one 30 minutes ago (below threshold).
- `IdleTimeoutMinutes = 120`.

Asserts:
- After one `sweep()` call, the 3-hour-idle instance has `Status == Hibernated`.
- The 30-minute-idle instance remains `Active`.
- Storage `Update()` called once (for the idle session only).

---

#### FR-1-b: `TestHibernationSweeper_SkipsNonActiveSessions`

**Type:** Unit (with mocks)
**Test function:** `TestHibernationSweeper_SkipsNonActiveSessions`

Setup: Mock storage returning sessions with statuses `Paused`, `Stopped`, `Creating`, `Hibernated`, all with `LastMeaningfulOutput` well past the idle threshold.

Asserts:
- `sweep()` does not call `Hibernate()` on any of them.
- Storage `Update()` not called.

---

#### FR-1-c: `TestHibernationSweeper_ConfigDisabled_DoesNotStart`

**Type:** Unit
**Test function:** `TestHibernationSweeper_ConfigDisabled_DoesNotStart`

Creates a `HibernationSweeper` with `Enabled = false`. Starts it, waits briefly, asserts no sweep occurred (mock storage `ListAll()` never called).

---

### Resource-Pressure Tests

**File:** `session/hibernation_sweeper_test.go`

#### FR-3-a: `TestResourcePressure_HibernatesOldestIdleFirst`

**Type:** Unit (with mocks)
**Test function:** `TestResourcePressure_HibernatesOldestIdleFirst`

Setup:
- Mock `mem.VirtualMemory()` returning `UsedPercent = 90` (above 85% threshold).
- Three `Active` instances with different idle times: 4h, 2h, 30min.

After `checkResourcePressure()`:
- The 4h-idle session is hibernated first.
- If mocked memory drops to 72% after first hibernation (below 75% hysteresis), stops.
- The 2h-idle and 30min-idle sessions remain `Active`.

---

#### FR-3-b: `TestResourcePressure_Hysteresis_StopsBelow75Pct`

**Type:** Unit (with mocks)
**Test function:** `TestResourcePressure_Hysteresis_StopsBelow75Pct`

Mock memory remains above 85% for all iterations. Asserts all `Active` sessions are eventually hibernated (sweeper doesn't stop prematurely if pressure remains high).

---

### Checkpoint Cleanup Tests

**File:** `server/services/session_service_test.go` (extend) and `session/hibernation_sweeper_test.go`

#### FR-8-a: `TestDeleteSession_RemovesCheckpointFiles`

**Type:** Unit (Go service-layer)
**Test function:** `TestDeleteSession_RemovesCheckpointFiles`

Setup: Create `<tmpDir>/<uuid>/checkpoint.json` on disk. Call `DeleteSession` with a session using that UUID.

Asserts:
- `<tmpDir>/<uuid>/` directory no longer exists after delete.
- Deleting a session that never had a checkpoint does not error.

---

#### FR-8-b: `TestPruneStaleCheckpoints_RemovesOldOrphanedCheckpoints`

**Type:** Unit
**Test function:** `TestPruneStaleCheckpoints_RemovesOldOrphanedCheckpoints`

Setup: Create checkpoint directories with timestamps 45 days old and 10 days old. Storage returns no matching live sessions for either.

Asserts:
- 45-day checkpoint directory is pruned (> `RetentionDays = 30`).
- 10-day checkpoint directory is kept.

---

#### FR-8-c: `TestPruneStaleCheckpoints_SkipsLiveSessions`

**Type:** Unit
**Test function:** `TestPruneStaleCheckpoints_SkipsLiveSessions`

Setup: Create a checkpoint directory for a UUID that IS in storage (live session).

Asserts:
- Even if the checkpoint is old (> `RetentionDays`), it is NOT pruned while the session exists.

---

### Frontend: Hibernate/Resume UI Tests

**File:** `web-app/src/components/sessions/SessionActionsOverflow.test.tsx`

#### FR-2-c: `SessionActionsOverflow_ShowsHibernateForActiveSessions`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionActionsOverflow_ShowsHibernateForActiveSessions`

Renders `<SessionActionsOverflow>` with an `Active` session and `onHibernate` prop. Asserts:
- A menu item with text "Hibernate" is present.
- Clicking it calls `onHibernate()` and closes the menu.

---

#### FR-2-d: `SessionActionsOverflow_HidesHibernateForNonActiveSessions`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionActionsOverflow_HidesHibernateForNonActiveSessions`

Renders with `Paused`, `Stopped`, `Creating`, `Hibernated` sessions. Asserts "Hibernate" is absent in all cases.

---

#### FR-7-a: `SessionActionsOverflow_ShowsResumeForHibernatedSessions`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionActionsOverflow_ShowsResumeForHibernatedSessions`

Renders with `Hibernated` status and `onResumeFromHibernation` prop. Asserts "Resume" menu item is present and calls the callback on click.

---

#### FR-7-b: `SessionRow_ShowsHibernatedBadgeWithAccessibleLabel`

**Type:** Unit (Jest/RTL)
**Test function:** `SessionRow_ShowsHibernatedBadgeWithAccessibleLabel`

Renders `<SessionRow>` with `status == Hibernated`. Asserts:
- An element with `aria-label="Hibernated"` (or equivalent accessible name) is present.
- A `data-status="hibernated"` attribute is present on the status element (for CSS targeting).
- The element is visually distinct from `Active`/`Paused` badges (test via CSS class name or `data-status` value).

---

#### HOOK-1: `useSessionService_HibernateSession_CallsRPCAndUpdatesStore`

**Type:** Unit (Jest, hook testing)
**Test function:** `useSessionService_HibernateSession_CallsRPCAndUpdatesStore`

Mocks the ConnectRPC client. Calls `hibernateSession(id)`. Asserts:
- Client's `hibernateSession` method called with `{ id }`.
- Redux store dispatches `upsertSession` with the returned session.

---

#### HOOK-2: `useSessionService_ResumeHibernatedSession_CallsRPCAndUpdatesStore`

**Type:** Unit (Jest, hook testing)
**Test function:** `useSessionService_ResumeHibernatedSession_CallsRPCAndUpdatesStore`

Mirror of HOOK-1 for `resumeHibernatedSession`.

---

## E2E Tests (Playwright)

**Directory:** `tests/e2e/`

All e2e tests start with `// @feature session:hibernate, session:state-machine` and run against the test server at `http://localhost:8544`.

---

#### E2E-1: `session-hibernation.spec.ts` — `TestManualHibernate_ContextMenu`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-hibernation.spec.ts`
**Test name:** `session-hibernation > manual hibernate via context menu`

Steps:
1. Create a session via the Omnibar; wait for `Active` status.
2. Right-click the session row.
3. Assert "Hibernate" appears in the context menu.
4. Click "Hibernate".
5. Assert the session row's status badge changes to "Hibernated" within 5s.
6. Assert no spinner or "Active" badge is visible.

Uses `data-testid` selectors and ARIA roles only (no CSS class selectors, per conventions).

---

#### E2E-2: `session-hibernation.spec.ts` — `TestHibernatedBadge_Visible`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-hibernation.spec.ts`
**Test name:** `session-hibernation > hibernated badge visible in row view`

After E2E-1:
1. Assert the status element has `data-status="hibernated"`.
2. Assert "Resume" appears in the context menu (right-click).
3. Assert "Hibernate" does NOT appear in the context menu for the hibernated session.

---

#### E2E-3: `session-hibernation.spec.ts` — `TestResume_HibernatedSession`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-hibernation.spec.ts`
**Test name:** `session-hibernation > resume hibernated session`

After hibernating a session:
1. Right-click → "Resume".
2. Assert status badge transitions to "Active" within 10s.
3. Assert the terminal panel becomes interactive (attach button active or terminal visible).

---

#### E2E-4: `session-hibernation.spec.ts` — `TestSubStatus_ProcessingChipVisible`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-hibernation.spec.ts`
**Test name:** `session-hibernation > sub-status processing chip visible for active session`

This test requires backend injection of `StatusProcessing`. If a real AI process is too slow, use a mock/stub session that reports `StatusProcessing` from the test server.

Steps:
1. Assert an `Active` session shows a sub-status chip with "Thinking" or spinner.
2. Assert the chip is absent for a `Paused` session.

---

#### E2E-5: `session-creation-async.spec.ts` — `TestAsyncCreation_ShowsCreatingStatus`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-creation-async.spec.ts`
**Test name:** `async-creation > shows Creating status immediately after submit`

Steps:
1. Open Omnibar, configure a session with a slow-setup path (e.g., path that triggers a known-slow init).
2. Click Create.
3. Assert the session row appears with "Creating" status within 1s of submit.
4. Assert a spinner is visible.
5. Wait for transition to "Active" (up to 30s).

---

#### E2E-6: `session-creation-async.spec.ts` — `TestAsyncCreation_ProgressTextUpdates`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-creation-async.spec.ts`
**Test name:** `async-creation > creation_progress text updates during Creating state`

Asserts that at least one non-empty `creation_progress` string is rendered beneath the session title while in `Creating` state.

---

#### E2E-7: `session-state-machine.spec.ts` — `TestHibernateMenuItem_NotShownForPausedSession`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-state-machine.spec.ts`
**Test name:** `state-machine > hibernate not shown for paused session`

Pause an existing session. Right-click. Assert "Hibernate" is absent from the context menu. Assert "Resume" (for Paused) is present.

---

#### E2E-8: `session-state-machine.spec.ts` — `TestSubStatusNeedsApproval_ShowsInList`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-state-machine.spec.ts`
**Test name:** `state-machine > needs-approval sub-status chip shown in session list`

Requires a session that reaches `NeedsApproval` detection state (or a test double). Asserts:
- Sub-status chip with "Needs Approval" text and warning styling is visible.
- The session lifecycle badge still shows "Active" (not "NeedsApproval").

---

#### E2E-9: `session-state-machine.spec.ts` — `TestGroupBySubStatus_NeedsApprovalFirst`

**Type:** E2E (Playwright)
**File:** `tests/e2e/session-state-machine.spec.ts`
**Test name:** `state-machine > group by sub-status sorts needs-approval first`

Steps:
1. Select "Group by Sub-Status" from grouping strategy dropdown.
2. Assert a "Needs Approval" group heading appears at the top (or before "Idle/Other").

---

## Regression Tests: No-Regression in Existing Flows

**File:** `session/session_restart_test.go` (extend with one assertion per existing flow)

These are guards to run after each Epic lands to confirm no regressions:

#### REG-1: `TestExistingFlow_CreateStopDelete_Unaffected`

**Type:** Integration
**Test function:** `TestExistingFlow_CreateStopDelete_Unaffected`

Runs the basic create → stop → delete lifecycle and asserts status transitions are correct with the new state machine. Confirms `Running` alias still works for existing test code that uses it.

---

#### REG-2: `TestExistingFlow_PauseResume_Unaffected`

**Type:** Integration
**Test function:** `TestExistingFlow_PauseResume_Unaffected`

Runs pause → resume and asserts `Paused → Active` transition succeeds. Confirms no interference from `Hibernated` state additions.

---

#### REG-3: `TestExistingFlow_HealthCheckerAutoRestart_StillWorks`

**Type:** Integration
**Test function:** `TestExistingFlow_HealthCheckerAutoRestart_StillWorks`

Adapts `testHealthCheckerAutoRestart` to use `Active` (instead of `Running`) and confirms health checker still auto-restarts a crashed `Active` session.

---

## Test File Locations Summary

| File | Test Type | Tests Contained |
|---|---|---|
| `session/state_machine_test.go` | Unit | SM-TRANS-1, SM-TRANS-2, SM-TRANS-3 |
| `session/instance_test.go` (extend) | Unit | SM-1-a through SM-1-e, SM-2-a |
| `session/instance_serialization_test.go` (extend) | Integration | SM-2-b, GUARD-1-a, GUARD-1-b |
| `session/hibernate_guard_test.go` (new) | Unit/Integration | GUARD-2-a, GUARD-3-a |
| `session/hibernate_test.go` (new) | Unit | FR-4-a, FR-4-b, FR-4-c, FR-6-a, FR-6-b, FR-6-c |
| `session/hibernation_sweeper_test.go` (new) | Unit | FR-1-a, FR-1-b, FR-1-c, FR-3-a, FR-3-b, FR-8-b, FR-8-c |
| `session/session_restart_test.go` (extend) | Integration | HIBE-INTEG-1, HIBE-INTEG-2, REG-1, REG-2, REG-3 |
| `server/adapters/instance_adapter_test.go` (extend) | Unit | SM-3-a, SM-3-b, SM-4-a, SM-4-b, SM-4-c, SM-4-d, VIS-1-a, VIS-1-b, VIS-1-c |
| `server/services/session_service_test.go` (extend) | Unit/Integration | CREATE-1-a, CREATE-1-b, CREATE-2-a, CREATE-3-a, CREATE-3-b, FR-2-a, FR-2-b, FR-6-d, FR-6-e, FR-8-a |
| `config/config_test.go` (extend) | Unit | FR-5-a, FR-5-b |
| `web-app/src/components/sessions/SubStatusChip.test.tsx` (new) | Unit (Jest) | VIS-2-a, VIS-2-b, VIS-2-c, VIS-2-d |
| `web-app/src/components/sessions/SessionRow.test.tsx` (extend) | Unit (Jest) | CREATE-3-c, VIS-2-e, VIS-2-f, VIS-2-g, FR-7-b |
| `web-app/src/components/sessions/SessionActionsOverflow.test.tsx` (extend) | Unit (Jest) | FR-2-c, FR-2-d, FR-7-a |
| `web-app/src/lib/hooks/useSessionService.test.ts` (extend) | Unit (Jest) | HOOK-1, HOOK-2 |
| `web-app/src/lib/grouping/subStatusGrouping.test.ts` (new) | Unit (Jest) | VIS-3-a |
| `tests/e2e/session-hibernation.spec.ts` (new) | E2E (Playwright) | E2E-1, E2E-2, E2E-3, E2E-4 |
| `tests/e2e/session-creation-async.spec.ts` (new) | E2E (Playwright) | E2E-5, E2E-6 |
| `tests/e2e/session-state-machine.spec.ts` (new) | E2E (Playwright) | E2E-7, E2E-8, E2E-9 |

---

## Critical Path: Tests That Must Pass Before Epic 4 Merges

The following tests must be green before the hibernation Epic 4 PR can merge. They are the minimum safety net against auto-resume regressions:

1. **GUARD-1-a** — `TestFromInstanceData_HibernatedSessionNotAutoStarted`
2. **GUARD-1-b** — `TestFromInstanceData_HibernatedSessionWorktreeNotTransitionedToPaused`
3. **GUARD-2-a** — `TestHealthChecker_SkipsHibernatedSession`
4. **GUARD-3-a** — `TestStaleResumeRecovery_SkipsHibernatedSession`
5. **SM-4-a** — `TestProtoStatusIntegerValues_Stable` (wire value regression)
6. **HIBE-INTEG-2** — `TestHibernationLifecycle_ServerRestartDoesNotAutoResume`

These six tests can be written before the implementation is complete (TDD-style) as they precisely define the contract that the guard code must satisfy.
