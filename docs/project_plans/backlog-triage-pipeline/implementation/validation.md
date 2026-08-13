# Backlog Triage Pipeline — Validation Plan

**Project**: backlog-triage-pipeline  
**Phase**: 4 — Validation  
**Date**: 2026-05-18  
**Status**: Ready for implementation session

---

## Summary

| Metric | Value |
|---|---|
| Total test cases | 42 |
| Unit (Go) | 14 |
| Unit (TypeScript/Jest) | 16 |
| Integration (Go) | 6 |
| E2E (Playwright) | 6 |
| Requirements covered | 7 / 7 (R1–R7-lite: all covered) |

---

## Requirement Coverage Matrix

| Requirement | Test IDs |
|---|---|
| R1 — Auto-trigger triage on item creation | U-BE-01, U-BE-02, U-BE-03, U-BE-04, U-BE-05, I-01, I-02 |
| R2 — Vagueness detection before triage | U-FE-01, U-FE-02, U-FE-03, U-FE-04, U-FE-05, E2E-01, E2E-02 |
| R3 — Expose triage result via API | U-BE-06, U-BE-07, I-03 |
| R4 — Triage output diff/preview in detail pane | U-FE-06, U-FE-07, U-FE-08, U-FE-09, U-FE-10, U-FE-11, U-FE-12, U-FE-13 |
| R5 — Apply triage suggestions to item | U-FE-14, U-FE-15, U-FE-16, I-04 |
| R6 — Triage complete notification | U-BE-08, U-BE-09, I-05, E2E-05, E2E-06 |
| R7-lite — Clarifying questions via suggestions array | U-BE-10, U-BE-11, U-FE-17, I-06 |

---

## Section 1 — Go Unit Tests

**File**: `server/services/backlog_service_test.go`  
**Run**: `go test ./server/services/... -run TestCreateBacklogItem_|TestTriggerTriage_|TestItemSessionToProto_|TestBuildTriagePrompt_`

---

### U-BE-01 — Auto-trigger fires when repoPath is set

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet` |
| **Verifies** | `TriggerTriage` is called once and `TriageTriggered=true` is returned in the response when `skip_triage=false` and `repo_path` is non-empty |
| **Must fail pre-fix** | Yes — `TriggerTriage` call does not exist in `CreateBacklogItem` before T-02 |

**Scenario**: Mock `sessionCreator` that records calls. Create item with `RepoPath="/some/repo"` and `SkipTriage=false`. Assert `resp.Msg.TriageTriggered == true` and `sessionCreator.TriggerTriageCalls == 1`.

---

### U-BE-02 — Auto-trigger skipped when `skip_triage=true`

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue` |
| **Verifies** | `TriggerTriage` is NOT called and `TriageTriggered=false` when `skip_triage=true` |
| **Must fail pre-fix** | Yes |

**Scenario**: Same mock; `SkipTriage=true`. Assert `TriageTriggered == false` and mock call count is 0.

---

### U-BE-03 — Auto-trigger skipped when `repo_path` is empty

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestCreateBacklogItem_SkipsTriageWhenRepoPathEmpty` |
| **Verifies** | `TriggerTriage` is NOT called when `repo_path=""` regardless of `skip_triage` value |
| **Must fail pre-fix** | Yes |

**Scenario**: `RepoPath=""`, `SkipTriage=false`. Assert `TriageTriggered == false`.

---

### U-BE-04 — Item creation succeeds even when auto-triage fails

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestCreateBacklogItem_ItemCreatedEvenWhenAutoTriggerFails` |
| **Verifies** | `CreateBacklogItem` returns success (no error) and a valid item even when the internal `TriggerTriage` call errors |
| **Must fail pre-fix** | Yes |

**Scenario**: Mock `sessionCreator` that returns an error from `TriggerTriage`. Assert `err == nil`, `resp.Msg.Item != nil`, `resp.Msg.TriageTriggered == false`.

---

### U-BE-05 — Auto-trigger timeout guard (30s context)

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestCreateBacklogItem_AutoTriggerRespectsTimeout` |
| **Verifies** | A `TriggerTriage` call that blocks past the 30s deadline is cancelled; `CreateBacklogItem` still returns in under 35s with `TriageTriggered=false` |
| **Must fail pre-fix** | Yes |

**Scenario**: Mock `sessionCreator` that blocks until its context is cancelled. Assert `err == nil` within 35s and `TriageTriggered == false`.

---

### U-BE-06 — Double-trigger guard returns `CodeAlreadyExists`

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestTriggerTriage_DoubleTriggerGuard` |
| **Verifies** | If an in-flight triage `ItemSession` (no `ended_at`) already exists for the item, `TriggerTriage` returns `connect.CodeAlreadyExists` and spawns no new session |
| **Must fail pre-fix** | Yes — guard does not exist before T-02 |

**Scenario**: Seed a triage `ItemSession` with `EndedAt=nil`. Call `TriggerTriage` for the same item. Assert `connect.CodeOf(err) == connect.CodeAlreadyExists`.

---

### U-BE-07 — Double-trigger guard allows re-trigger after session ends

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestTriggerTriage_AllowsRetriggerAfterSessionEnded` |
| **Verifies** | If the existing triage session has `ended_at` set (completed/failed), `TriggerTriage` succeeds |
| **Must fail pre-fix** | No (guard does not exist, so call succeeds) — this is a regression guard |

**Scenario**: Seed a triage `ItemSession` with `EndedAt=time.Now()`. Call `TriggerTriage`. Assert no error.

---

### U-BE-08 — `itemSessionToProto` maps triage result fields correctly

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestItemSessionToProto_MapsTriageResult` |
| **Verifies** | Valid `triage_result` JSON in `ItemSession.TriageResult` maps to `proto.TriageResult` with correct `summary`, `suggestions[].text`, `suggestions[].rationale`, and `clarifying_questions` |
| **Must fail pre-fix** | Yes — `itemSessionToProto` does not populate `TriageResult` before T-03 |

**Scenario**: Construct `ItemSession` with `TriageResult = '{"summary":"s","suggestions":[{"text":"t","rationale":"r"}],"clarifying_questions":["q1"]}'`. Call `itemSessionToProto`. Assert all fields.

---

### U-BE-09 — `itemSessionToProto` handles malformed JSON gracefully

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestItemSessionToProto_HandlesInvalidTriageResultJSON` |
| **Verifies** | Malformed JSON in `TriageResult` does not panic; `proto.TriageResult` is nil (warn-and-continue behavior) |
| **Must fail pre-fix** | No — this is a defensive safety test |

**Scenario**: `TriageResult = "not-json{{"`. Assert no panic and `proto.ItemSession.TriageResult == nil`.

---

### U-BE-10 — `itemSessionToProto` with empty `TriageResult` string produces nil proto field

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestItemSessionToProto_EmptyTriageResultProducesNilField` |
| **Verifies** | When `ItemSession.TriageResult == ""`, the proto `triage_result` field is nil (not a zero-value struct) |
| **Must fail pre-fix** | No — regression guard |

---

### U-BE-11 — `buildTriagePrompt` includes clarifying-questions instruction

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestBuildTriagePrompt_IncludesClarifyingQuestionsInstruction` |
| **Verifies** | The prompt returned by `buildTriagePrompt` contains the instruction to include up to 3 clarifying questions as `suggestions` entries with `rationale: "question"` |
| **Must fail pre-fix** | Yes — instruction does not exist before T-03 |

**Scenario**: Call `buildTriagePrompt` with a sample item. Assert `strings.Contains(prompt, "rationale")` and `strings.Contains(prompt, "question")`.

---

### U-BE-12 — `submitTriageResult` publishes notification when EventBus is set

**File**: `server/mcp/tools_backlog_test.go`

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/mcp/tools_backlog_test.go` |
| **Test name** | `TestSubmitTriageResult_PublishesNotificationOnSuccess` |
| **Verifies** | After a successful `submitTriageResult`, `eventBus.Publish` is called once with an event containing `item_id` in metadata, `"Triage complete"` as title, and `NOTIFICATION_PRIORITY_NORMAL` |
| **Must fail pre-fix** | Yes — EventBus integration does not exist before T-04 |

**Scenario**: Construct `backlogHandlers` with a mock `EventBus`. Call `submitTriageResult` with valid args. Assert `mockBus.PublishCalls == 1` and inspect event payload.

---

### U-BE-13 — `submitTriageResult` is safe when EventBus is nil

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/mcp/tools_backlog_test.go` |
| **Test name** | `TestSubmitTriageResult_NoNotificationWhenEventBusNil` |
| **Verifies** | `submitTriageResult` does not panic and still stores the result when `eventBus == nil` |
| **Must fail pre-fix** | No — nil guard is defensive |

---

### U-BE-14 — Notification message format matches spec

| Field | Value |
|---|---|
| **Type** | Unit (Go) |
| **File** | `server/mcp/tools_backlog_test.go` |
| **Test name** | `TestSubmitTriageResult_NotificationMessageFormat` |
| **Verifies** | Notification body is `"<item title> — <N> suggestion(s). Click to review."` with correct count |
| **Must fail pre-fix** | Yes |

**Scenario**: Seed item with title `"Add login"`. Call `submitTriageResult` with 2 suggestions. Assert event body is `"Add login — 2 suggestion(s). Click to review."`.

---

## Section 2 — TypeScript / Jest Unit Tests

**Run**: `cd web-app && npx jest --no-coverage --testPathPatterns="VaguenessPromptModal|TriageReviewPanel|useBacklogService"`

---

### U-FE-01 — Vagueness modal renders when description is short and no AC

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_renders_when_description_short_and_no_ac` |
| **Verifies** | Modal is visible when `description.length < 80` AND `acCriteria.length === 0` |
| **Must fail pre-fix** | Yes — component does not exist before T-06 |

---

### U-FE-02 — Vagueness modal does not render when item has AC

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_does_not_render_when_ac_present` |
| **Verifies** | Modal is not shown when `acCriteria.length >= 1`, regardless of description length |
| **Must fail pre-fix** | Yes |

---

### U-FE-03 — Vagueness modal does not render when description is long

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_does_not_render_when_description_at_threshold` |
| **Verifies** | Modal is not shown when `description.length >= 80` and `acCriteria.length === 0` |
| **Must fail pre-fix** | Yes |

---

### U-FE-04 — Vagueness modal calls `onRefine` when "Add more detail" clicked

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_calls_onRefine_when_refine_clicked` |
| **Verifies** | `onRefine` callback fires when user clicks the "Add more detail" / refine button |
| **Must fail pre-fix** | Yes |

---

### U-FE-05 — Vagueness modal calls `onProceed` when "Run triage anyway" clicked

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_calls_onProceed_when_proceed_clicked` |
| **Verifies** | `onProceed` callback fires when user clicks "Run triage anyway" |
| **Must fail pre-fix** | Yes |

---

### U-FE-06 — Vagueness modal has no escape-key dismissal

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` |
| **Test name** | `VaguenessPromptModal_has_no_escape_dismiss` |
| **Verifies** | Pressing Escape does not close the modal (user must choose one of the two explicit options) |
| **Must fail pre-fix** | Yes |

---

### U-FE-07 — `TriageReviewPanel` renders diff when suggestions are present

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_renders_diff_when_suggestions_present` |
| **Verifies** | When `triageResult.suggestions.length > 0` (with `rationale !== "question"`), the two-column diff section is rendered and Apply + Skip buttons are visible |
| **Must fail pre-fix** | Yes — component does not exist before T-08 |

---

### U-FE-08 — `TriageReviewPanel` renders summary-only when no suggestions

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_renders_summary_only_when_no_suggestions` |
| **Verifies** | When `triageResult.suggestions.length === 0`, the panel shows summary text and a "Mark ready" shortcut, but no diff columns |
| **Must fail pre-fix** | Yes |

---

### U-FE-09 — `TriageReviewPanel` does not render when dismissed in localStorage

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_does_not_render_when_dismissed_in_localStorage` |
| **Verifies** | If `localStorage.getItem("triage-panel-dismissed-<id>")` is set, the panel is not rendered |
| **Must fail pre-fix** | Yes |

---

### U-FE-10 — `TriageReviewPanel` does not render when `item.status !== "idea"`

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_does_not_render_when_status_not_idea` |
| **Verifies** | Panel is hidden when item is in `"ready"`, `"in_progress"`, `"review"`, or `"done"` status |
| **Must fail pre-fix** | Yes |

---

### U-FE-11 — Apply calls `updateBacklogItem` then `transitionStatus`

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_apply_calls_updateBacklogItem_then_transitionStatus` |
| **Verifies** | Clicking "Apply suggestions" calls `updateBacklogItem` first with suggested AC, then calls `transitionStatus` to `"ready"` only after update succeeds; both called exactly once |
| **Must fail pre-fix** | Yes |

---

### U-FE-12 — Apply shows error banner on failure

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_shows_error_banner_on_apply_failure` |
| **Verifies** | When `updateBacklogItem` rejects (optimistic concurrency / `CodeAborted`), `TriageErrorBanner` is rendered with a relevant message and the Apply button is re-enabled |
| **Must fail pre-fix** | Yes |

---

### U-FE-13 — Apply shows undo toast on success

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_shows_undo_toast_on_apply_success` |
| **Verifies** | After a successful apply, an undo toast is shown with an "Undo" button; clicking "Undo" calls `updateBacklogItem` with the pre-apply AC and `transitionStatus` back to `"idea"` |
| **Must fail pre-fix** | Yes |

---

### U-FE-14 — Questions section renders for `rationale === "question"` suggestions

| Field | Value |
|---|---|
| **Type** | Unit (Jest/RTL) |
| **File** | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` |
| **Test name** | `TriageReviewPanel_renders_questions_section_for_question_rationale` |
| **Verifies** | Suggestions with `rationale === "question"` are excluded from the diff section and rendered in a separate "Triage Questions" sub-section |
| **Must fail pre-fix** | Yes |

---

### U-FE-15 — `mapBacklogItem` triageStatus is `"failed"` when session ended with no triageResult

| Field | Value |
|---|---|
| **Type** | Unit (Jest) |
| **File** | `web-app/src/lib/hooks/useBacklogService.test.ts` |
| **Test name** | `mapBacklogItem_triageStatus_is_failed_when_session_ended_but_no_triageResult` |
| **Verifies** | P12 fix: if the triage `ItemSession` has `endedAt` set but `triageResult` is undefined/null, `triageStatus` is `"failed"` not `"completed"` |
| **Must fail pre-fix** | Yes — P12 fix is in T-05 |

---

### U-FE-16 — `mapBacklogItem` triageStatus is `"completed"` when session ended with triageResult

| Field | Value |
|---|---|
| **Type** | Unit (Jest) |
| **File** | `web-app/src/lib/hooks/useBacklogService.test.ts` |
| **Test name** | `mapBacklogItem_triageStatus_is_completed_when_session_ended_and_triageResult_present` |
| **Verifies** | When `triageResult.summary` is non-empty and `endedAt` is set, `triageStatus` is `"completed"` |
| **Must fail pre-fix** | No — regression guard for the P12 fix |

---

### U-FE-17 — `mapItemSession` maps `triageResult` fields from proto

| Field | Value |
|---|---|
| **Type** | Unit (Jest) |
| **File** | `web-app/src/lib/hooks/useBacklogService.test.ts` |
| **Test name** | `mapItemSession_maps_triageResult_from_proto` |
| **Verifies** | A proto `ItemSession` with populated `triageResult` (summary, suggestions, clarifyingQuestions) maps to the domain `LinkedSession` with all fields intact |
| **Must fail pre-fix** | Yes — mapping does not exist before T-05 |

---

## Section 3 — Go Integration Tests

**File**: `server/services/backlog_service_test.go` (integration suite using `createTestStorage`)  
**Run**: `go test ./server/services/... -run TestIntegration_`

---

### I-01 — `CreateBacklogItem` → triage auto-triggered → `TriggerTriage` result is visible

| Field | Value |
|---|---|
| **Type** | Integration (Go, real storage) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestIntegration_CreateBacklogItem_AutoTrigger_SessionCreated` |
| **Verifies** | After `CreateBacklogItem` with `repo_path` set, an `ItemSession` with `SessionRole == "triage"` and `EndedAt == nil` exists in storage for the new item |
| **Must fail pre-fix** | Yes |

---

### I-02 — `CreateBacklogItem` with `skip_triage=true` → no triage session in storage

| Field | Value |
|---|---|
| **Type** | Integration (Go, real storage) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestIntegration_CreateBacklogItem_SkipTriage_NoSessionCreated` |
| **Verifies** | When `skip_triage=true`, no `ItemSession` with role `"triage"` is created for the item |
| **Must fail pre-fix** | Yes |

---

### I-03 — `submit_triage_result` → `GetBacklogItem` returns `triageResult`

| Field | Value |
|---|---|
| **Type** | Integration (Go, real storage) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestIntegration_SubmitTriageResult_SurfacedInGetBacklogItem` |
| **Verifies** | After `submitTriageResult` stores a result JSON, `GetBacklogItem` returns the item with `itemSession.triage_result` fully populated (summary, suggestions, clarifying_questions) |
| **Must fail pre-fix** | Yes — `GetBacklogItem` does not include `triage_result` before T-03 |

---

### I-04 — Apply suggestions: `UpdateBacklogItem` → `TransitionBacklogItemStatus` → `GetBacklogItem` reflects changes

| Field | Value |
|---|---|
| **Type** | Integration (Go, real storage) |
| **File** | `server/services/backlog_service_test.go` |
| **Test name** | `TestIntegration_ApplySuggestions_UpdatesACAndTransitionsToReady` |
| **Verifies** | Calling `UpdateBacklogItem` with new AC, then `TransitionBacklogItemStatus` to `"ready"`, results in `GetBacklogItem` returning the updated AC and `status == "ready"` |
| **Must fail pre-fix** | No — uses existing RPCs; verifies the expected two-step sequence works end-to-end |

---

### I-05 — `submitTriageResult` publishes notification with correct `item_id`

| Field | Value |
|---|---|
| **Type** | Integration (Go, mock EventBus + real storage) |
| **File** | `server/mcp/tools_backlog_test.go` |
| **Test name** | `TestIntegration_SubmitTriageResult_NotificationItemIdMatchesCreatedItem` |
| **Verifies** | The `item_id` in the published notification's metadata matches the ID of the backlog item that was triaged; item title from storage is used in the message body |
| **Must fail pre-fix** | Yes |

---

### I-06 — Triage prompt clarifying questions appear in stored `triageResult`

| Field | Value |
|---|---|
| **Type** | Integration (Go, real storage) |
| **File** | `server/mcp/tools_backlog_test.go` |
| **Test name** | `TestIntegration_SubmitTriageResult_QuestionsStoredAsSuggestions` |
| **Verifies** | When `submitTriageResult` is called with suggestions containing `rationale: "question"`, those entries are preserved in the JSON stored in `ItemSession.TriageResult` and surfaced back via `GetBacklogItem.itemSession.triage_result.suggestions` |
| **Must fail pre-fix** | No — storage is format-agnostic; this is a round-trip regression guard for R7-lite |

---

## Section 4 — Playwright E2E Tests

**File**: `tests/e2e/backlog-triage-pipeline.spec.ts` (new file)  
**Feature annotation**: `// @feature backlog:triage-pipeline`  
**Server**: `http://localhost:8544`  
**Run**: `cd tests/e2e && npx playwright test backlog-triage-pipeline.spec.ts`

---

### E2E-01 — Creating a clear item auto-triggers triage (no button click)

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_autoTriggerTriage_when_itemCreatedWithRepoPath` |
| **Verifies** | After creating a backlog item with a description ≥ 80 chars, the item detail shows the triage loading indicator without any manual "Trigger Triage" click |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Navigate to `/backlog`.
2. Open create form; fill title + description (≥ 80 chars); no AC.
3. Submit.
4. Open item detail.
5. Assert `[data-testid="triage-loading-indicator"]` is visible within 5s.

---

### E2E-02 — Creating a vague item shows the vagueness prompt modal

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_showVaguenessPrompt_when_itemIsVague` |
| **Verifies** | Creating an item with a short description (< 80 chars) and no AC shows `VaguenessPromptModal` before triage starts |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Create item with `description = "short"` and no AC.
2. Submit.
3. Assert `[data-testid="vagueness-prompt-modal"]` is visible.
4. Assert no triage loading indicator is shown yet.

---

### E2E-03 — Vagueness prompt "Run triage anyway" starts triage

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_startTriage_when_userClicksRunAnywayFromVaguenessPrompt` |
| **Verifies** | Clicking "Run triage anyway" in the vagueness modal dismisses the modal and shows the triage loading indicator |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Create vague item (step 1–2 of E2E-02).
2. Click `[data-testid="vagueness-proceed-button"]`.
3. Assert modal is gone.
4. Assert `[data-testid="triage-loading-indicator"]` appears within 5s.

---

### E2E-04 — Triage review panel appears after triage completes

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_showTriageReviewPanel_when_triageSessionCompletes` |
| **Verifies** | After triage session ends (simulated via `submit_triage_result` MCP call in fixture), opening item detail shows `TriageReviewPanel` with summary and Apply/Skip buttons |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Create item + seed a completed triage `ItemSession` with fixture data (summary + 2 suggestions).
2. Navigate to `/backlog?item=<id>`.
3. Assert `[data-testid="triage-review-panel"]` is visible.
4. Assert Apply and Skip buttons are present.

---

### E2E-05 — Triage complete notification appears in the notification panel

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_showNotification_when_triageCompletes` |
| **Verifies** | After `submit_triage_result` fires, a notification with title "Triage complete" appears in the notification panel |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Create item with `repo_path` set; wait for triage session to exist.
2. Trigger `submit_triage_result` via fixture helper.
3. Open notification panel.
4. Assert notification with title "Triage complete" is visible.

---

### E2E-06 — Notification click navigates to item detail

| Field | Value |
|---|---|
| **Type** | E2E (Playwright) |
| **File** | `tests/e2e/backlog-triage-pipeline.spec.ts` |
| **Test name** | `backlog-triage-pipeline_should_navigateToItem_when_triageNotificationClicked` |
| **Verifies** | Clicking the triage-complete notification navigates to `/backlog?item=<id>` and opens the item detail panel |
| **Must fail pre-fix** | Yes |

**Steps**:
1. Perform E2E-05 steps 1–4.
2. Click the notification item.
3. Assert URL contains `item=<item_id>`.
4. Assert `[data-testid="backlog-item-detail"]` is visible and shows the correct item title.

---

## Section 5 — Edge Cases and Race Conditions

These cases are included in the test suites above but called out explicitly here for traceability.

| Scenario | Test ID | Category |
|---|---|---|
| Double-trigger: concurrent `TriggerTriage` calls for same item | U-BE-06 | Race condition — guard must be synchronous check before session spawn |
| Context cancel on auto-trigger (30s timeout) | U-BE-05 | Race condition — deadline fires before session spawn completes |
| `skip_triage=true` with a non-empty `repo_path` | U-BE-02 | Edge case — skip flag overrides path presence |
| `repo_path=""` with `skip_triage=false` | U-BE-03 | Edge case — guard prevents trigger without target repo |
| Auto-trigger fails silently; item creation succeeds | U-BE-04 | Error path — error must not propagate to `CreateBacklogItem` response |
| Optimistic concurrency error on apply | U-FE-12 | Error path — `CodeAborted` shows error banner, not silent failure |
| `TriageReviewPanel` hidden when `status !== "idea"` | U-FE-10 | Edge case — panel must not show for in-progress/done items |
| `TriageReviewPanel` hidden when dismissed | U-FE-09 | Edge case — localStorage flag respected on re-mount |
| Malformed JSON in `triage_result` column | U-BE-09 | Error path — warn-and-continue, no panic |
| P12: crashed triage session shows `"failed"` not `"completed"` | U-FE-15 | Edge case — session ended without storing result |
| Suggestions array empty → summary-only panel | U-FE-08 | Edge case — no diff rendered without suggestions |
| `rationale === "question"` excluded from diff, shown in questions section | U-FE-14 | Edge case — R7-lite questions must not pollute AC diff |
| EventBus nil → no panic, notification not sent | U-BE-13 | Error path — nil guard required for stdio MCP server path |

---

## Section 6 — Pre-Fix Failure Confirmation

Before implementation, the following tests MUST fail against the current codebase (confirming the test targets new behavior rather than existing behavior):

| Test ID | Expected failure reason |
|---|---|
| U-BE-01 | `TriggerTriage` not called from `CreateBacklogItem` |
| U-BE-02 | `SkipTriage` field not accepted in request (proto field missing) |
| U-BE-03 | Same as U-BE-01 |
| U-BE-04 | Same as U-BE-01 (error-path branch of non-existent feature) |
| U-BE-05 | Same as U-BE-01 |
| U-BE-06 | Double-trigger guard not present in `TriggerTriage` |
| U-BE-08 | `itemSessionToProto` does not populate `TriageResult` |
| U-BE-11 | `buildTriagePrompt` does not contain clarifying-questions instruction |
| U-BE-12 | EventBus field not present on `backlogHandlers` |
| U-BE-14 | Same as U-BE-12 |
| U-FE-01–06 | `VaguenessPromptModal` component does not exist |
| U-FE-07–14 | `TriageReviewPanel` component does not exist |
| U-FE-15 | P12 fix not applied; crashed triage session incorrectly shows `"completed"` |
| U-FE-17 | `mapItemSession` does not map `triageResult` field |
| I-01, I-02 | Same as U-BE-01 |
| I-03 | `GetBacklogItem` does not include `triage_result` in response |
| I-05 | EventBus not threaded into MCP handlers |
| E2E-01–06 | All new UI/backend features absent |

---

## Section 7 — Test Execution Order

Run after each implementation epic:

| After Epic | Tests to run | Command |
|---|---|---|
| T-01 (proto regen) | Compile check only | `make build` |
| T-02 (auto-trigger) | U-BE-01 through U-BE-07 | `go test ./server/services/... -run TestCreateBacklogItem_\|TestTriggerTriage_` |
| T-03 (triage result mapping) | U-BE-08, U-BE-09, U-BE-10, U-BE-11 | `go test ./server/services/... -run TestItemSessionToProto_\|TestBuildTriagePrompt_` |
| T-04 (EventBus) | U-BE-12, U-BE-13, U-BE-14 | `go test ./server/mcp/... -run TestSubmitTriageResult_` |
| T-05 (frontend hook) | U-FE-15, U-FE-16, U-FE-17 | `cd web-app && npx jest --no-coverage --testPathPatterns="useBacklogService"` |
| T-06 (vagueness modal) | U-FE-01 through U-FE-06 | `cd web-app && npx jest --no-coverage --testPathPatterns="VaguenessPromptModal"` |
| T-08 (triage panel) | U-FE-07 through U-FE-14 | `cd web-app && npx jest --no-coverage --testPathPatterns="TriageReviewPanel"` |
| T-10 (notification deep-link) | E2E-05, E2E-06 | `cd tests/e2e && npx playwright test backlog-triage-pipeline.spec.ts --grep notification` |
| All epics complete | Full suite | `make ci && cd tests/e2e && npx playwright test backlog-triage-pipeline.spec.ts` |

---

## Section 8 — Definition of Done (Test Perspective)

- [ ] All 42 test cases exist in the correct files
- [ ] All "must fail pre-fix" tests were verified to fail before implementation began
- [ ] All 42 tests pass after implementation
- [ ] `make ci` passes (build + lint + all Go tests)
- [ ] `cd web-app && npx jest --no-coverage` passes
- [ ] `cd tests/e2e && npx playwright test backlog-triage-pipeline.spec.ts` passes
- [ ] No existing backlog tests regress (`backlog.spec.ts`, existing `backlog_service_test.go` tests)
- [ ] Coverage gaps in `docs/registry/coverage-gaps.json` do not increase
