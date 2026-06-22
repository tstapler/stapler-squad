# Validation Plan: backlog-triage-autonomous

**Date**: 2026-06-22

---

## Happy Path Scenario

Given a `BacklogItem` in `idea` status in a running `BacklogService` with a healthy headless pool, when the operator clicks "Trigger Triage" in the detail pane, then within 30 minutes the item transitions to `ready` status, plan artifacts are written to `docs/tasks/<slug>/`, and the detail pane shows "Ready" badge, a "Plan Artifacts" path, and an enabled "Approve Plan" button — all without any operator intervention.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: Triage prompt injected within 30s of startup | `session/backlog_triage_test.go` | `BuildHeadlessTriagePrompt_should_ContainItemIDAndArtifactPath_When_ValidInput` | Unit | Happy path — prompt contains required fields |
| REQ-1: Triage prompt injected within 30s of startup | `server/services/backlog_service_test.go` | `TriggerTriage_should_ReturnWithinTwoSeconds_When_HeadlessPoolIsHealthy` | Integration | RPC returns synchronously before LLM finishes |
| REQ-2: Session runs autonomously to completion | `server/services/backlog_service_test.go` | `TriggerTriage_should_TransitionItemToReady_When_FakePoolReturnsValidJSON` | Integration | Happy path — goroutine drives full flow to completion |
| REQ-2: Session runs autonomously to completion | `server/services/backlog_service_test.go` | `TriggerTriage_should_LeaveItemAsIdea_When_HeadlessPoolReturnsError` | Integration | Error path — failure leaves item unchanged |
| REQ-3: Plan artifacts written to docs/tasks/<slug>/ | `session/backlog_triage_test.go` | `BuildHeadlessTriagePrompt_should_ContainArtifactAbsPath_When_SlugAndRepoPathProvided` | Unit | Happy path — prompt embeds artifact dir |
| REQ-3: Plan artifacts written to docs/tasks/<slug>/ | `server/services/backlog_service_test.go` | `TriggerTriage_should_SetPlanArtifactsPathOnItem_When_TriageSucceeds` | Integration | Happy path — artifact path persisted on item after goroutine |
| REQ-4: idea → ready transition on triage completion | `server/services/backlog_service_test.go` | `TriggerTriage_should_TransitionItemToReady_When_FakePoolReturnsValidJSON` | Integration | Happy path — status transitions after goroutine completes |
| REQ-4: idea → ready transition on triage completion | `server/services/backlog_service_test.go` | `TriggerTriage_should_LeaveItemAsIdea_When_HeadlessPoolReturnsError` | Integration | Error path — status NOT transitioned on failure |
| REQ-5: Full stack (real Claude, real pool, real tmux) | `tests/e2e/backlog-triage.spec.ts` | `backlog-triage > should transition item to ready after autonomous triage` | E2E | Happy path — full stack integration against live server |

### Parser and Prompt Unit Tests

| Requirement / Function | Test File | Test Name | Type | Scenario |
|------------------------|-----------|-----------|------|----------|
| `ParseHeadlessTriageResult` — valid JSON | `session/backlog_triage_test.go` | `ParseHeadlessTriageResult_should_ReturnResult_When_ValidJSONProvided` | Unit | Happy path |
| `ParseHeadlessTriageResult` — fenced JSON | `session/backlog_triage_test.go` | `ParseHeadlessTriageResult_should_StripFencesAndParse_When_JSONWrappedInBackticks` | Unit | Happy path variant — LLM wraps output in code fences |
| `ParseHeadlessTriageResult` — invalid JSON | `session/backlog_triage_test.go` | `ParseHeadlessTriageResult_should_ReturnError_When_InputIsNotJSON` | Unit | Error path |
| `ParseHeadlessTriageResult` — task cap | `session/backlog_triage_test.go` | `ParseHeadlessTriageResult_should_CapTasksAtTwelve_When_LLMReturnsMoreThanTwelve` | Unit | Edge case — cap at 12 |
| `BuildHeadlessTriagePrompt` — contains item ID | `session/backlog_triage_test.go` | `BuildHeadlessTriagePrompt_should_ContainItemIDAndArtifactPath_When_ValidInput` | Unit | Happy path |
| `BuildHeadlessTriagePrompt` — does not call MCP tool | `session/backlog_triage_test.go` | `BuildHeadlessTriagePrompt_should_NotReferenceSubmitTriageResult_When_Built` | Unit | Pitfall guard — headless mode must not invoke MCP tool |
| `headlessTriageSystemPrompt` — no MCP reference | `session/headless/features_test.go` | `HeadlessTriageSystemPrompt_should_NotContainSubmitTriageResult_When_Inspected` | Unit | Pitfall guard — system prompt safety check |
| `TriggerTriage` — nil pool guard | `server/services/backlog_service_test.go` | `TriggerTriage_should_ReturnCodeUnimplemented_When_HeadlessPoolIsNil` | Unit | Error path — guard condition |
| `BacklogService.Shutdown` — unblocks semaphore | `server/services/backlog_service_test.go` | `Shutdown_should_UnblockWaitingGoroutine_When_TriageSemIsFull` | Integration | Shutdown cancellation — goroutine unblocks via shutdownCtx |

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-1: "Trigger Triage" disabled while running | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_DisableTriggerTriageButton_When_TriageStatusIsRunning` | Jest/RTL | Render detail with `triageStatus="running"`; assert button has `disabled` attribute and `title` contains "already running" |
| AC-1: Button title describes why disabled | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_SetDisabledTitleOnTriggerTriageButton_When_TriageStatusIsRunning` | Jest/RTL | Assert `title` attribute on disabled button is non-empty |
| AC-2: In-progress indicator within one poll cycle | `tests/e2e/backlog-triage.spec.ts` | `backlog-triage > should show TriageLoadingIndicator within 6s of triggering` | Playwright | Click "Trigger Triage"; wait ≤ 6s; assert triage indicator is visible |
| AC-3: Label changes at 60 seconds | `web-app/src/components/backlog/TriageLoadingIndicator.test.tsx` | `TriageLoadingIndicator_should_ShowLongRunningLabel_When_ElapsedSecondsGte60` | Jest/RTL | Render with `elapsedSeconds=60`; assert label reads "Still thinking" |
| AC-3: Label at < 60 seconds | `web-app/src/components/backlog/TriageLoadingIndicator.test.tsx` | `TriageLoadingIndicator_should_ShowThinkingLabel_When_ElapsedSecondsLt60` | Jest/RTL | Render with `elapsedSeconds=30`; assert label reads "Thinking about acceptance criteria" |
| AC-4: Compact pill in list view during triage | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_ShowCompactTriagePill_When_HasActiveTriageAndStatusIsIdea` | Jest/RTL | Render card with `hasActiveTriage=true` and `status="idea"`; assert `TriageLoadingIndicator` is visible |
| AC-4: "Mark Ready" hidden when pill shown | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_HideMarkReadyButton_When_CompactPillIsVisible` | Jest/RTL | Render card with `hasActiveTriage=true`; assert "Mark Ready" is absent |
| AC-4: Pill not shown when not triaging | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_ShowNoPill_When_HasActiveTriageIsFalse` | Jest/RTL | Render card with `hasActiveTriage=false`; assert no `TriageLoadingIndicator` |
| AC-5: InlineError shown on failure | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowInlineError_When_TriageStatusIsFailed` | Jest/RTL | Render with `triageStatus="failed"`; assert `InlineError` present with `role="alert"` |
| AC-5: Retry button present and clickable | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowRetryButton_When_TriageFailed` | Jest/RTL | Assert "Retry ↺" button rendered; simulate click; assert `triggerTriage` was called |
| AC-6: Timeout failure maps to correct error type | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowTimeoutMessage_When_FailureReasonIsTimeout` | Jest/RTL | Render with `triageFailureReason="timeout"`; assert InlineError headline reads "Triage timed out" |
| AC-6: exit_code_1 maps to permanent error type | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowPermanentError_When_FailureReasonIsExitCode1` | Jest/RTL | Render with `triageFailureReason="exit_code_1"`; assert InlineError type is "permanent" |
| AC-6: Network/undefined maps to transient error | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowTransientError_When_FailureReasonIsNetwork` | Jest/RTL | Render with `triageFailureReason="network"`; assert headline reads "Triage failed" with network body |
| AC-7: View session logs link on permanent failure | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowViewLogsLink_When_PermanentFailureAndLogsSessionIdDefined` | Jest/RTL | Assert link href is `/sessions/{logsSessionId}/logs`, `target="_blank"`, `rel="noopener noreferrer"` |
| AC-7: No logs link when logsSessionId undefined | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_HideViewLogsLink_When_LogsSessionIdIsUndefined` | Jest/RTL | Render with `logsSessionId=undefined`; assert no logs link in DOM |
| AC-8: Plan artifacts section visible when path set | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowPlanArtifactsSection_When_PlanArtifactsPathIsNonEmpty` | Jest/RTL | Render with `planArtifactsPath="docs/tasks/foo/"`; assert section and `<code>` with path are visible |
| AC-9: "Approve Plan" button shown when path set | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowApprovePlanButton_When_StatusReadyAndPlanArtifactsPathSet` | Jest/RTL | Render with `status="ready"` and `planArtifactsPath` set; assert "Approve Plan" visible |
| AC-9: "Approve Plan" hidden when path absent | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_HideApprovePlanButton_When_PlanArtifactsPathIsEmpty` | Jest/RTL | Render with `planArtifactsPath=""`; assert "Approve Plan" absent |
| AC-10: Spawn/Run enabled after approving plan | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_EnableSpawnAndRunButtons_When_PlanIsApproved` | Jest/RTL | Simulate click "Approve Plan"; assert "Spawn Session" and "Run Autonomously" not disabled |
| AC-11: All failure states have exit path | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowRetryButton_InAllFailureStateTypes` | Jest/RTL | Render with each `triageFailureReason`; assert "Retry ↺" present in all cases |
| AC-12: Status badge updates without refresh (detail) | `tests/e2e/backlog-triage.spec.ts` | `backlog-triage > should update status badge to Ready without page refresh` | Playwright | Trigger triage; wait for completion; assert badge reads "Ready" without navigation |
| AC-13: Triage indicator disappears on completion | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_HideTriageIndicator_When_TriageStatusChangesToCompleted` | Jest/RTL | Re-render with `triageStatus="completed"`; assert `TriageLoadingIndicator` absent |
| AC-14: in-progress indicator has role="status" | `web-app/src/components/backlog/TriageLoadingIndicator.test.tsx` | `TriageLoadingIndicator_should_HaveRoleStatus_And_AriaLivePolite_When_Running` | Jest/RTL | Assert `role="status"` and `aria-live="polite"` on container |
| AC-15: InlineError has role="alert" and aria-live="assertive" | `web-app/src/components/common/InlineError.test.tsx` | `InlineError_should_HaveRoleAlertAndAriaLiveAssertive_When_Rendered` | Jest/RTL | Assert `role="alert"` and `aria-live="assertive"` |
| AC-16: Retry button has aria-label="Retry triage" | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_SetAriaLabelOnRetryButton_When_TriageFailed` | Jest/RTL | Assert retry button `aria-label="Retry triage"` |
| AC-17: Disabled button has non-empty title | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_SetDisabledTitleOnTriggerTriageButton_When_TriageStatusIsRunning` | Jest/RTL | Assert `title` attribute non-empty and describes reason |
| AC-18: Retry reachable via keyboard Tab | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ActivateRetryOnEnterKey_When_RetryButtonIsFocused` | Jest/RTL | Simulate `Tab` to reach retry button; simulate `Enter`; assert `triggerTriage` called |
| AC-19: Cancel pill focusable via Tab in list | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_MakeCancelButtonFocusable_When_CompactPillIsVisible` | Jest/RTL | Assert `[×]` button in compact pill is focusable (not `tabIndex=-1`) |
| AC-22: Spinner respects prefers-reduced-motion | Manual checklist | — | Manual | Toggle `prefers-reduced-motion: reduce` in browser DevTools; verify spinner animation pauses |
| AC-24: Cards without active triage show no pill | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_ShowNoPill_When_HasActiveTriageIsFalse` | Jest/RTL | Render card with `hasActiveTriage=false`; assert no pill, normal action button present |
| AC-25: Card click not blocked by pill | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_FireOnClickWhenCardBodyClicked_When_PillIsVisible` | Jest/RTL | Render card with active triage; click card body; assert `onSelect` callback fired |
| AC-27: No gap between indicator and error | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_NeverShowBothIndicatorAndError_When_StatusChanges` | Jest/RTL | Assert mutual exclusion: indicator and InlineError are never simultaneously in DOM |
| AC-28: item transitions to ready without operator action | `server/services/backlog_service_test.go` | `TriggerTriage_should_TransitionItemToReady_When_FakePoolReturnsValidJSON` | Integration | Shared with REQ-4 integration test |

---

## Test Stack

- **Unit (Go)**: `go test` + `testify/assert` + `testify/require`
- **Integration (Go)**: `go test` + `headless.FakeRunner` (or a `HeadlessPoolClient` mock interface) + in-memory `BacklogStorage` backed by SQLite (matching existing test patterns in `session/backlog_integration_test.go`)
- **Unit (TypeScript)**: Jest + React Testing Library (RTL)
- **E2E**: Playwright against `http://localhost:8544` (the test server port)
- **Manual checklist**: for reduced-motion and color contrast checks that require browser DevTools

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥ 80% line coverage on `session/backlog_triage.go` and the new `TriggerTriage` goroutine path in `backlog_service.go` |
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="BacklogItemDetail\|BacklogItemCard\|TriageLoadingIndicator\|InlineError"` | ≥ 80% line coverage on modified components |

---

## Test File Locations Summary

| Test file | Contains |
|-----------|----------|
| `session/backlog_triage_test.go` | Unit: `ParseHeadlessTriageResult` (4 cases), `BuildHeadlessTriagePrompt` (3 cases) |
| `session/headless/features_test.go` | Unit: `HeadlessTriageSystemPrompt` content guard (1 case) |
| `server/services/backlog_service_test.go` | Integration: `TriggerTriage` success, failure, nil-pool guard, shutdown cancellation (5 cases) |
| `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | Jest/RTL: in-progress disable, failure states, error types, logs link, plan artifacts, approve plan, a11y (17 cases) |
| `web-app/src/components/backlog/BacklogItemCard.test.tsx` | Jest/RTL: compact pill, mark-ready hidden, no-pill guard, card click passthrough (5 cases) |
| `web-app/src/components/backlog/TriageLoadingIndicator.test.tsx` | Jest/RTL: label transitions, role/aria-live (3 cases) |
| `web-app/src/components/common/InlineError.test.tsx` | Jest/RTL: role="alert", aria-live (1 case, may already exist) |
| `tests/e2e/backlog-triage.spec.ts` | Playwright: full triage flow, status badge update, in-progress indicator within 6s (3 cases) |
