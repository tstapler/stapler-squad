# Features Research — backlog-triage-pipeline

**Date**: 2026-05-18  
**Scope**: Existing patterns to reuse, analogous features in the codebase.

---

## R1 — Auto-trigger triage: Reusable Patterns

### Pattern: Post-create side-effect in service handler
`CreateBacklogItem` in `server/services/backlog_service.go` (lines ~269–314) currently creates the item and returns immediately. The pattern for adding a non-blocking post-creation side effect already exists in `SpawnSessionFromItem` (lines ~742–858): load item → validate → call `sessionCreator.CreateDirectorySession` → create `ItemSession` record. Auto-trigger can reuse this exact call sequence inline in `CreateBacklogItem`, called after the item is persisted.

**Degradation contract (already established)**: When `sessionCreator == nil`, handlers return `CodeUnimplemented`. The same nil-guard must be applied for the auto-triage path: if `sessionCreator == nil` or `item.RepoPath == ""`, skip auto-triage silently (do not fail the create).

### Pattern: `skip_planning` / `skip_review_gate` opt-out flags
`CreateBacklogItemRequest` already carries `bool skip_planning` and `bool skip_review_gate` as first-class request fields. `skip_triage bool` follows this exact same pattern — add to `CreateBacklogItemRequest`, default false, check at service layer before calling `TriggerTriage`.

### Pattern: `triage_triggered` in response
`SpawnSessionFromItemResponse` returns `session_uuid` and `item_session`. `TriggerTriageResponse` returns `item_session`. `CreateBacklogItemResponse` can add `bool triage_triggered` (field 2) to indicate whether auto-triage was kicked off — consumers (UI) use this to decide whether to show the vagueness prompt.

---

## R2 — Vagueness detection: Reusable Patterns

### Pattern: Client-side derived state from backlog item
`triageStatus` in `useBacklogService.ts` (lines ~160–165) is already computed client-side from `linkedSessions` without any new RPC — purely derived from item data returned by `GetBacklogItem`/`ListBacklogItems`. The vagueness check (`desc.length < 80 && acCriteria.length === 0`) follows the same pattern: computed from `item.description` and `item.acCriteria` returned by `createBacklogItem` response.

### Pattern: Post-action prompt in BacklogItemDetail
`BacklogItemDetail.tsx` already shows contextual UI based on `item.triageStatus === "running"` (renders `TriageLoadingIndicator`). The vagueness prompt is the same gate pattern: render a modal/banner conditionally after `createBacklogItem` resolves, before triage fires.

---

## R3 — Expose triage result via API: Reusable Patterns

### Pattern: `itemSessionToProto` field mapping
`backlog_service.go` `itemSessionToProto()` (lines ~123–176) already maps `ent.ItemSession` → `sessionv1.ItemSession`. Adding `triage_result` JSON → `TriageResult` proto message follows the exact pattern used for `review_verdict` / `PerCriterion` deserialization (lines ~161–174): unmarshal JSON from string field, populate proto repeated field.

### Pattern: `mapItemSession` in `useBacklogService.ts`
`mapItemSession()` (lines ~104–133) already maps `ItemSessionProto` → `LinkedSession`, including `reviewVerdict`. Adding `triageResult` to `LinkedSession` type and `mapItemSession` follows the same pattern.

---

## R4 — Triage diff/preview panel: Reusable Patterns

### Pattern: `GateVerdictBox` conditional section in `BacklogItemDetail`
`BacklogItemDetail.tsx` renders `<GateVerdictBox>` conditionally on `item.status === "review"` (line ~336). The triage review panel follows the same conditional rendering pattern, triggered by `item.triageStatus === "completed"`.

### Pattern: `TriageLoadingIndicator` dual-context (running state)
`TriageLoadingIndicator.tsx` is already rendered for `triageStatus === "running"` in `BacklogItemDetail`. The completed state panel replaces this — the component currently returns `null` after `TRIAGE_TIMEOUT_SECONDS`. Extending it for a `completed` context (with diff/preview content) is one approach; alternatively, a new `TriageReviewPanel` sibling component is cleaner given the panel has distinct layout requirements (diff left/right columns).

### Pattern: `localStorage` dismissed state
No existing localStorage usage in the backlog components, but the pattern is standard React. Key: `triage-panel-dismissed-${item.id}` — checked on mount, set on "Skip" click.

---

## R5 — Apply suggestions: Reusable Patterns

### Pattern: Two-step `updateBacklogItem` + `transitionStatus`
`BacklogItemDetail.tsx` already chains multiple service calls in `handleAction` (e.g., override_done: `overrideVerdict` → `load()`). The apply-suggestions path: `updateBacklogItem(item.id, { acCriteria: suggested })` → `transitionStatus(item.id, "ready", "idea")` → `load()`.

### Pattern: Optimistic concurrency via `expected_status`
`UpdateBacklogItemRequest` has `expected_status` and `expected_updated_at` fields; `TransitionBacklogItemStatusRequest` also has `expected_status`. On `CodeAborted` from the server, the `lastError` state already propagates the error message — same `setLastError` pattern used in `createBacklogItem`/`updateBacklogItem`.

---

## R6 — Triage complete notification: Reusable Patterns

### Pattern: `broadcastQuestionNotification` in `approval_handler.go`
`ApprovalHandler.broadcastQuestionNotification()` (lines ~377–394) is the canonical example for publishing `NOTIFICATION_TYPE_INPUT_REQUIRED` via `h.eventBus.Publish(events.NewNotificationEvent(...))`. The triage-complete notification in `submitTriageResult` follows the same call exactly, with `NOTIFICATION_TYPE_TASK_COMPLETE` or `NOTIFICATION_TYPE_INPUT_REQUIRED` and `NOTIFICATION_PRIORITY_NORMAL`.

### Key gap: `backlogHandlers` has no `eventBus`
Currently `backlogHandlers` struct (`tools_backlog.go:60`) only holds `storage` and `store`. The `eventBus` must be threaded in from `mcp/server.go` → `dependencies.go`. `BacklogService` in `backlog_service.go` also lacks an eventBus — if notification is triggered from `submitTriageResult` (MCP side), the eventBus is added to `backlogHandlers`; if from the service layer, it goes in `BacklogService`.

### Pattern: notification metadata for deep-linking
Existing notifications use `metadata map[string]string` for structured data (e.g., approval ID as `"context"`). The triage notification metadata should include `"item_id": itemID` for frontend deep-link routing to `/backlog?item=<item_id>`.

---

## R7 — Clarifying questions: Reusable Patterns

### Pattern: `buildTriagePrompt` in `backlog_service.go`
`buildTriagePrompt()` (lines ~1033–1093) builds the triage agent system prompt. R7 only requires adding a "clarifying questions" instruction block to this prompt — the AI output path (`submit_triage_result`) already accepts the result. Adding a `clarifying_questions []string` field to the `triageSuggestion` struct and the stored JSON payload enables persisting questions.

### Pattern: approval flow for questions
`broadcastQuestionNotification` (approval_handler.go) already handles the `AskUserQuestion` → `INPUT_REQUIRED` → user-responds-in-terminal flow. R7 reuses this path by instructing the triage AI to use `AskUserQuestion` (already a Claude tool) when it needs clarification. No new server code is required for question dispatch — only prompt changes and the 30-minute timeout logic.
