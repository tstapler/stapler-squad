# Validation Plan: review-queue-state-detection

**Date**: 2026-05-02

---

## Requirement → Test Mapping

### AC-1: Active session does NOT appear as idle in the review queue

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectProcessing_EbbingPattern` | Unit | P0 | "Ebbing..." in tail → `StatusProcessing` |
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectActive_ToolCallOutput` | Unit | P0 | "Bash(" / "Read(" in tail → `StatusActive` |
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectActive_EscToInterrupt` | Unit | P0 | "esc to interrupt" in tail → `StatusActive` |
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestDetectRecent_DoesNotMatchStaleContent` | Unit | P0 | Stale "esc to interrupt" in first 5000 bytes, idle prompt in last 4096 → `StatusIdle`, not `StatusActive` |
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestDetectRecent_TailWindowBoundary` | Unit | P1 | Buffer exactly `statusDetectionTailBytes` long — full buffer scanned |
| AC-1, FR-1 | `session/detection/detector_test.go` | `TestDetectRecent_ShorterThanWindow` | Unit | P1 | Buffer shorter than tail window — full buffer scanned, no panic |
| AC-1, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_WorkingSession_NotAddedToQueue` | Unit | P0 | Session with `StatusActive` is never added to review queue |
| AC-1, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ProcessingSession_NotAddedToQueue` | Unit | P0 | Session with `StatusProcessing` is not added to review queue |
| AC-1, FR-1 | `session/detection/snapshot_test.go` | `TestSnapshotDetection_ClaudeEbbing` | Unit | P0 | Snapshot fixture `claude_ebbing.txt` → `StatusProcessing` |
| AC-1, FR-1 | `session/detection/snapshot_test.go` | `TestSnapshotDetection_ClaudeActiveToolUse` | Unit | P0 | Snapshot fixture `claude_active_tool_use.txt` → `StatusActive` |

### AC-2: Session re-enters queue within 5 seconds after finishing turn

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-2, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectSuccess_CostSummaryLine` | Unit | P0 | `$0.42 • 3 tool uses` in tail → `StatusSuccess` |
| AC-2, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectIdle_ReadlinePrompt` | Unit | P0 | `^> $` (readline prompt) in tail → `StatusIdle` |
| AC-2, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_DetectSuccess_CostSummaryVariants` | Unit | P1 | Multiple cost summary formats ($1.23, $10.05) all match |
| AC-2, FR-1 | `session/detection/detector_test.go` | `TestStatusDetector_ReadlinePrompt_NotMatchedMidLine` | Unit | P1 | `"> some text"` mid-line does not trigger idle (anchored pattern) |
| AC-2, FR-2 | `session/review_queue_poller_test.go` | `TestPushBasedRemoval_SessionRemovedOnPTYOutput` | Unit | P0 | PTY output with active signal arrives → `queue.Remove()` called before next poll tick |
| AC-2, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ActiveToIdle_ReEntersQueue` | Unit | P0 | Session transitions from `StatusActive` to `StatusIdle` → re-added to queue |
| AC-2, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ActiveToSuccess_ReEntersQueue` | Unit | P0 | Session transitions from `StatusActive` to `StatusSuccess` → re-added within poll interval |
| AC-2, FR-1 | `session/detection/snapshot_test.go` | `TestSnapshotDetection_ClaudeCostSummary` | Unit | P0 | Snapshot fixture `claude_cost_summary.txt` → `StatusSuccess` |
| AC-2, FR-1 | `session/detection/snapshot_test.go` | `TestSnapshotDetection_ClaudeReadlinePrompt` | Unit | P0 | Snapshot fixture `claude_readline_prompt.txt` → `StatusIdle` |

### AC-3: Review queue updates in real time without page refresh

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-3, FR-2 | `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_filterOutWorkingItems_When_workingStateIsActive` | Unit | P0 | Stream update with `WORKING_STATE_ACTIVE` → item absent from returned list |
| AC-3, FR-2 | `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_filterOutWorkingItems_When_workingStateIsProcessing` | Unit | P0 | Stream update with `WORKING_STATE_PROCESSING` → item absent from returned list |
| AC-3, FR-2 | `web-app/src/lib/store/__tests__/reviewQueueSlice.test.ts` | `selectWaitingItems_should_excludeActiveAndProcessing_When_mixedStatesPresent` | Unit | P0 | Redux selector `selectWaitingItems` excludes `ACTIVE`/`PROCESSING` items |
| AC-3, FR-2 | `web-app/src/lib/store/__tests__/reviewQueueSlice.test.ts` | `selectQueueStats_should_countByState_When_queueHasMixedItems` | Unit | P0 | `selectQueueStats` returns `{ waiting, working, stuck }` counts |
| AC-3, FR-2 | `web-app/src/lib/store/__tests__/reviewQueueSlice.test.ts` | `selectWaitingItems_should_returnAll_When_allItemsAreIdle` | Unit | P1 | All items `IDLE` → `selectWaitingItems` returns full list unchanged |
| AC-3, FR-3 | `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_mapWorkingStateFromProto_When_streamUpdateArrives` | Unit | P0 | Proto `WorkingState` enum mapped correctly to Redux `workingState` field |
| AC-3, FR-3 | `session/review_queue_poller_test.go` | `TestWorkingState_PropagatedThrough_ReviewItem` | Unit | P0 | `IdleState` → `WorkingState` adapter maps all enum values correctly |
| AC-3, FR-3 | `server/adapters/instance_adapter_test.go` | `mapIdleStateToWorkingState_should_returnActive_When_IdleStateActive` | Unit | P0 | `IdleStateActive` → `WORKING_STATE_ACTIVE` |
| AC-3, FR-3 | `server/adapters/instance_adapter_test.go` | `mapIdleStateToWorkingState_should_returnIdle_When_IdleStateWaiting` | Unit | P0 | `IdleStateWaiting` → `WORKING_STATE_IDLE` |
| AC-3, FR-3 | `server/adapters/instance_adapter_test.go` | `mapIdleStateToWorkingState_should_returnWaiting_When_IdleStateTimeout` | Unit | P0 | `IdleStateTimeout` → `WORKING_STATE_WAITING` |
| AC-3, FR-3 | `server/adapters/instance_adapter_test.go` | `mapIdleStateToWorkingState_should_returnUnspecified_When_IdleStateUnknown` | Unit | P0 | `IdleStateUnknown` → `WORKING_STATE_UNSPECIFIED` |
| AC-3, FR-2 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > running session absent from queue while working` | E2E | P0 | Session running Claude is absent from review queue list |
| AC-3, FR-2 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > queue updates without page refresh after state change` | E2E | P0 | State change observed in stream → queue list updates without `page.reload()` |

### AC-4: State corpus tool records labeled snapshot from terminal view

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_WritesJSONFileAtomically` | Unit | P0 | Valid session + label → JSON file created in corpus dir via tmp rename |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_FileContainsRequiredFields` | Unit | P0 | Written JSON contains `timestamp`, `session_id`, `label`, `scrollback_tail` |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_ReturnsFilePath` | Unit | P1 | Response `file_path` matches actually written file path |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_InvalidSessionID_ReturnsError` | Unit | P0 | Unknown session ID → `connect.CodeNotFound` error |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_InvalidLabel_ReturnsError` | Unit | P1 | Label not in `{working, idle, stuck}` → `connect.CodeInvalidArgument` |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestCaptureStateSnapshot_CorpusDir_CreatedIfMissing` | Unit | P1 | Corpus dir does not exist → created automatically, file written |
| AC-4, FR-4 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > capture state button visible in debug mode` | E2E | P1 | `?debug=1` query param → "Capture State" button present in terminal view |
| AC-4, FR-4 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > capture state records snapshot file` | E2E | P2 | Click "Capture State", select label → corpus dir contains new JSON file |

### AC-5: Corpus validator against ≥10 labeled examples outputs a report

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_OutputsReportForCorpus` | Unit | P0 | Directory with 10+ labeled JSON fixtures → stdout contains TP/FP/FN table |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_ComputesAccurateTPRate` | Unit | P0 | All fixtures labeled `working` with active content → 100% TP rate reported |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_ComputesFPRate` | Unit | P1 | Mix of correctly and incorrectly labeled fixtures → FP rate reflects mismatches |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_EmptyCorpusDir_ReturnsError` | Unit | P0 | Empty directory → error: "corpus must have at least 10 examples" |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_InsufficientCorpus_ReturnsError` | Unit | P0 | Fewer than 10 files → error with count |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_MalformedJSON_Skipped` | Unit | P1 | Corrupt JSON file in corpus → skipped with warning, rest processed |
| AC-5, FR-4 | `cmd/corpus-validate/corpus_validate_test.go` | `TestRunValidator_ReportContainsAllLabelCategories` | Unit | P1 | Corpus has working/idle/stuck examples → report shows all three sections |

### AC-6: Existing idle/stuck session behavior is unchanged

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-6, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_IdleSession_AddedToQueue` | Unit | P0 | Session with `StatusIdle` is still added to queue (regression guard) |
| AC-6, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_StuckSession_AddedToQueue` | Unit | P0 | Session silent beyond stuck threshold is added to queue (regression guard) |
| AC-6, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_AcknowledgedSession_BehaviorUnchanged` | Unit | P0 | Acknowledged session re-surfaces on new output (existing behavior preserved) |
| AC-6, FR-2 | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_SnoozeConditionLogic_Unchanged` | Unit | P1 | Snooze/acknowledgment logic unchanged after `detectProcessing` refactor |
| AC-6, FR-2 | `session/detection/detector_test.go` | `TestStatusDetector_ExistingPatterns_NotBrokenByNewPatterns` | Unit | P0 | All patterns from before this feature still produce same status |
| AC-6, FR-1 | `session/detection/snapshot_test.go` | `TestSnapshotDetection_AllExistingFixtures` | Unit | P0 | All 14 pre-existing testdata fixtures still return expected statuses |
| AC-6, FR-2 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > idle session still appears in queue` | E2E | P0 | Genuinely idle session visible in queue (regression guard) |
| AC-6, FR-2 | `tests/e2e/review-queue-working-state.spec.ts` | `review-queue-working-state > stuck session still appears in queue` | E2E | P0 | Session silent >5 min visible in queue (regression guard) |

---

## Additional Unit Tests (cross-cutting)

### CR Collapsing (Epic 1.2)

| Test File | Test Name | Type | Priority | Scenario |
|-----------|-----------|------|----------|---------|
| `session/detection/detector_test.go` | `TestCollapseCarriageReturns_SpinnerSequence` | Unit | P0 | `"⠋ Thinking\r⠙ Thinking\r"` → `"⠙ Thinking"` |
| `session/detection/detector_test.go` | `TestCollapseCarriageReturns_CRLFPreserved` | Unit | P0 | `"line1\r\nline2"` unchanged (CR+LF not collapsed) |
| `session/detection/detector_test.go` | `TestCollapseCarriageReturns_EmptyInput` | Unit | P1 | Empty string → empty string (no panic) |
| `session/detection/detector_test.go` | `TestCollapseCarriageReturns_NoCR` | Unit | P1 | String with no `\r` → unchanged |
| `session/detection/detector_test.go` | `TestStripANSI_WithCarriageReturn_FullPipeline` | Unit | P0 | `collapseCarriageReturns` then `stripANSI` — spinner ANSI sequence produces clean final text |

### detectProcessing() Refactor (Epic 1.4)

| Test File | Test Name | Type | Priority | Scenario |
|-----------|-----------|------|----------|---------|
| `session/review_queue_poller_test.go` | `TestDetectProcessing_UsesANSIStrippedPath` | Unit | P0 | Content with ANSI escape codes around processing keyword → correctly detected |
| `session/review_queue_poller_test.go` | `TestDetectProcessing_StaleContentIgnored` | Unit | P0 | Processing keyword only in bytes before tail window → returns false |
| `session/review_queue_poller_test.go` | `TestDetectProcessing_PreviousHardcodedPatterns_AllMatch` | Unit | P1 | Each of the 8 previously hardcoded patterns still triggers `detectProcessing = true` |

### Config Loading (Epic 4.1)

| Test File | Test Name | Type | Priority | Scenario |
|-----------|-----------|------|----------|---------|
| `config/config_test.go` | `TestReviewQueueConfig_LoadedFromJSON` | Unit | P0 | `review_queue` key in config.json → struct populated with custom values |
| `config/config_test.go` | `TestReviewQueueConfig_DefaultsWhenAbsent` | Unit | P0 | No `review_queue` key → zero-value struct, poller uses Go defaults |
| `config/config_test.go` | `TestReviewQueueConfig_PartialOverride` | Unit | P1 | Only `poll_interval_ms` set → only that field overridden, others use defaults |

### Manual Override / ForceQueue (Epic 3.2)

| Test File | Test Name | Type | Priority | Scenario |
|-----------|-----------|------|----------|---------|
| `session/queue/queue_test.go` | `TestReviewQueue_ForceAdd_AppearsInList` | Unit | P0 | `ForceAdd` item with active state → appears in `List()` output |
| `session/queue/queue_test.go` | `TestReviewQueue_ForceAdd_NotRemovedByPoller` | Unit | P0 | Force-added item with `StatusActive` → poller does not remove it |
| `session/queue/queue_test.go` | `TestReviewQueue_ForceAdd_ClearedWhenIdle` | Unit | P1 | Force-added item transitions to `StatusIdle` → force flag cleared, normal removal applies |
| `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_callForceQueueSession_When_markAsWaitingClicked` | Unit | P0 | `forceQueueSession(id)` dispatches RPC call and optimistically adds item to waiting list |

### Transition Notification (Epic 3.2)

| Test File | Test Name | Type | Priority | Scenario |
|-----------|-----------|------|----------|---------|
| `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_fireNotification_When_workingTransitionsToIdle` | Unit | P0 | Stream update: item changes from `WORKING_STATE_ACTIVE` to `WORKING_STATE_IDLE` → notification fired |
| `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_notFireNotification_When_idleStaysIdle` | Unit | P1 | Item remains `WORKING_STATE_IDLE` across updates → no notification |
| `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` | `useReviewQueue_should_notFireNotification_When_activeStaysActive` | Unit | P1 | Item remains `WORKING_STATE_ACTIVE` → no notification |

---

## Integration Tests

| Req | Test File | Test Name | Type | Priority | Scenario |
|-----|-----------|-----------|------|----------|---------|
| AC-1, AC-2, FR-1, FR-2 | `session/review_queue_integration_test.go` | `TestIntegration_PTYOutput_ToQueueStateChange` | Integration | P0 | Simulate PTY output with spinner → verify `StatusActive` → queue removal; then idle prompt → re-addition |
| AC-2, FR-3 | `session/review_queue_integration_test.go` | `TestIntegration_WorkingState_PropagatedToReviewItem` | Integration | P0 | Session `IdleState` changes → `ReviewItem.working_state` in proto response matches expected `WorkingState` enum |
| AC-1, FR-2 | `session/review_queue_integration_test.go` | `TestIntegration_PushBasedRemoval_UnderLoad` | Integration | P1 | Rapid PTY output bursts → no duplicate `queue.Remove()` calls; queue consistent |
| AC-3, FR-3 | `server/services/review_queue_service_test.go` | `TestIntegration_WatchReviewQueue_StreamsWorkingStateChanges` | Integration | P0 | `WatchReviewQueue` stream emits items with updated `working_state` when session state changes |
| AC-4, FR-4 | `server/services/corpus_service_test.go` | `TestIntegration_CaptureStateSnapshot_RoundTrip` | Integration | P0 | Full RPC call → file written → file readable with correct fields |
| AC-3, FR-3 | `server/adapters/instance_adapter_test.go` | `TestIntegration_SessionAdapter_WorkingStateRoundTrip` | Integration | P1 | Session with `IdleStateActive` serialized to proto and back → `WorkingState` preserved |

---

## E2E Tests (Playwright)

All in `tests/e2e/review-queue-working-state.spec.ts`

| ID | Test Name | Req | Priority | Assertion |
|----|-----------|-----|----------|-----------|
| T-E2E-001 | `running session absent from queue while working` | AC-1, AC-3 | P0 | Session with active Claude output: review queue list does not contain it; badge shows "working" count > 0 |
| T-E2E-002 | `session reappears in queue after turn completes` | AC-2, AC-3 | P0 | After idle prompt detected: session appears in queue within 5s; no `page.reload()` called |
| T-E2E-003 | `queue updates without page refresh after state change` | AC-3 | P0 | Mock stream emits state-change event → queue list updates in < 2s; no navigation or reload |
| T-E2E-004 | `queue badge counts split by state` | AC-3, UXR-1 | P1 | With mix of idle/working/stuck sessions: header shows "N waiting · M working · K stuck" |
| T-E2E-005 | `mark as waiting overrides working state detection` | UXR-2, FR-2 | P1 | Click "Mark as waiting" on a working session → appears in main queue list |
| T-E2E-006 | `idle session still appears in queue (regression)` | AC-6 | P0 | Idle session visible in queue list with correct "idle" label |
| T-E2E-007 | `stuck session still appears in queue (regression)` | AC-6 | P0 | Stuck session (silent >5 min) visible in queue list |
| T-E2E-008 | `capture state button visible in debug mode` | AC-4 | P1 | `?debug=1` → terminal view has `data-testid="capture-state-btn"` visible |
| T-E2E-009 | `capture state records snapshot with selected label` | AC-4 | P2 | Click capture, pick "working" label → backend corpus dir has new JSON file with correct label field |

---

## Test Stack

- **Go unit tests**: `testing` stdlib + `testify/assert` + `testify/require` + `testify/mock`
- **Go integration tests**: `testing` stdlib + real in-process wiring (no external services); uses `t.TempDir()` for file I/O
- **Frontend unit tests (Jest)**: Vitest/Jest + React Testing Library + `@testing-library/user-event`; Redux store via `configureStore` with real reducers; hooks tested via `renderHook`
- **E2E**: Playwright + `@playwright/test`; locators via `data-testid` and ARIA roles only; server at `http://localhost:8544`

---

## Coverage Targets

- **Go unit test coverage**: ≥80% line coverage for `session/detection/`, `session/queue/`, `server/services/corpus_service.go`, `server/adapters/instance_adapter.go`
- **All public service methods**: happy path + at least one error path each
- **All external integrations** (file I/O in corpus service, proto serialization): mocked in unit tests + covered by at least one integration test
- **All acceptance criteria**: 6/6 ACs covered (see matrix below)

---

## Acceptance Criteria Coverage Matrix

| AC | Description | Unit Go | Unit TS | Integration | E2E | Covered? |
|----|-------------|---------|---------|-------------|-----|---------|
| AC-1 | Active session NOT in review queue | 10 tests | 2 tests | 3 tests | 2 tests | YES |
| AC-2 | Re-enters queue within 5s after turn | 9 tests | 0 tests | 2 tests | 1 test | YES |
| AC-3 | Real-time updates, no page refresh | 11 tests | 8 tests | 2 tests | 3 tests | YES |
| AC-4 | Corpus snapshot capture from UI | 6 tests | 0 tests | 1 test | 2 tests | YES |
| AC-5 | Corpus validator ≥10 examples → report | 7 tests | 0 tests | 0 tests | 0 tests | YES |
| AC-6 | Existing idle/stuck behavior unchanged | 8 tests | 0 tests | 0 tests | 2 tests | YES |

---

## Test Case Count Summary

| Type | Count |
|------|-------|
| Go unit tests (detection package) | 28 |
| Go unit tests (poller / queue) | 18 |
| Go unit tests (adapter / service) | 9 |
| Go unit tests (config) | 3 |
| Go unit tests (corpus-validate cmd) | 7 |
| **Go unit subtotal** | **65** |
| Go integration tests | 6 |
| Frontend Jest/RTL unit tests | 15 |
| E2E Playwright tests | 9 |
| **Grand total** | **95** |

**Requirements covered: 6/6 AC (100%), 4/4 FR (100%), 3/3 UXR partially covered (UXR-3 via useReviewQueue transition notification tests)**
