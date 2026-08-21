# Backlog Triage Pipeline — Requirements

**Project**: backlog-triage-pipeline  
**Date**: 2026-05-18  
**Status**: Ideation complete

---

## Context

The backlog feature currently has a full lifecycle (idea → ready → in_progress → review → done) with AI-assisted triage, execution, and review gates. Triage (`TriggerTriage`) already exists — it spawns a Claude session that runs 4 research subagents, writes a plan, and calls `submit_triage_result` to store a summary + AC suggestions in the `ItemSession.triage_result` JSON field.

**Gaps being addressed:**
- Triage must be manually triggered; no auto-trigger on creation
- Triage output (suggestions + summary) is stored in the DB but never surfaced to the UI
- No vagueness detection — if the item description is too brief, triage just runs on thin context
- No interactive clarification flow when the AI needs more information
- No AC diff/preview before applying triage suggestions

---

## Goals

1. Auto-trigger triage when a new backlog item is created
2. Surface triage output as a diff/preview for user review before committing changes
3. Apply approved suggestions directly to the item's AC list
4. Notify the user when triage completes
5. Allow the triage AI to ask clarifying questions when the item is too vague

---

## Out of Scope

- Triage → Execution automation (spawn session after plan approval) — separate project
- Changes to the review gate or done/override flows
- Changes to `SuggestNextItem` or prioritization

---

## Requirements

### R1 — Auto-trigger triage on item creation

**Given** a user creates a new backlog item via `CreateBacklogItem`  
**When** the item is saved  
**Then** `TriggerTriage` fires automatically with no additional user action

- The auto-trigger must be a server-side behavior (not client-driven polling)
- `CreateBacklogItem` response should indicate whether triage was auto-triggered
- Items created via external sync sources (plugin ItemSource) should also auto-trigger triage
- The auto-trigger should be a configurable default (opt-out via `skip_triage: true` on creation request)

### R2 — Vagueness detection before triage

**Given** a new item has a description shorter than 80 characters AND no acceptance criteria  
**When** triage is about to trigger  
**Then** the UI prompts the user to optionally refine before triage runs

- "Refine before triage" option opens the item edit form pre-focused on description
- "Run triage anyway" option proceeds immediately
- The vagueness check is purely client-side (description length + AC count)
- Threshold: description < 80 chars AND 0 AC criteria = vague
- Items with 1+ AC criteria skip the vagueness prompt regardless of description length

### R3 — Expose triage result via API

**Given** a triage session has submitted its result via `submit_triage_result`  
**When** the frontend fetches the backlog item  
**Then** the triage suggestions and summary are included in the response

- Add `TriageResult` message to `backlog.proto` with `summary`, `suggestions[]`, and `clarifying_questions[]`
- Surface `triage_result` JSON from `ItemSession` in `GetBacklogItem` and `ListBacklogItems` responses
- Map the stored JSON into the new proto field via `mapItemSession` in `useBacklogService.ts`

### R4 — Triage output diff/preview in the detail pane

**Given** an item's triage session has completed (`triageStatus === "completed"`)  
**When** the user opens the item detail  
**Then** a "Triage Ready" panel is shown with:
  - Current description (left) vs triage summary (right)
  - Current AC list vs suggested AC list, with added/changed items highlighted
  - "Apply suggestions" button (applies suggestions to item, transitions to ready)
  - "Skip" button (dismisses the panel without applying)
  - "View plan" link if `planArtifactsPath` is set

- The panel replaces the existing `TriageLoadingIndicator` for `triageStatus === "completed"`
- If suggestions array is empty, the panel shows summary only (no diff)
- The panel is dismissible per-item (dismissed state stored in localStorage by item ID)

### R5 — Apply triage suggestions to item

**Given** the user clicks "Apply suggestions" in the triage review panel  
**When** the `UpdateBacklogItem` RPC completes  
**Then**:
  - The item's AC list is replaced with the suggested AC
  - The item transitions from "idea" to "ready" via `TransitionBacklogItemStatus`
  - The triage panel is dismissed
  - A success toast is shown: "Triage applied — item is now ready"

- "Apply" is a two-step operation: update AC, then transition status
- If the transition fails (optimistic concurrency), show an error and allow retry
- The applied AC text comes from `suggestions[].text` as the criterion text

### R6 — Triage complete notification

**Given** a triage session completes  
**When** `submit_triage_result` is called  
**Then** an `INPUT_REQUIRED` notification is published on the event bus:
  - Title: "Triage complete"
  - Message: `"<item title> — <suggestion count> suggestion(s). Click to review."`
  - Links to the item detail in the backlog

- Notification should use `NOTIFICATION_PRIORITY_NORMAL` (not HIGH)
- The notification must reference `item_id` so the frontend can deep-link
- Frontend notification click navigates to `/backlog?item=<item_id>`

### R7 — Interactive clarification via approval flow

**Given** the triage AI determines the item description is insufficient  
**When** the AI needs a specific answer before proceeding  
**Then** it can use the existing `INPUT_REQUIRED` notification path to surface clarifying questions to the user

- The triage prompt must explicitly instruct the AI to ask ≤3 clarifying questions if description is ambiguous
- Questions surface as approval-style prompts in the notification panel
- User answers are fed back into the triage session as additional context
- If the user doesn't answer within 30 minutes, triage continues with available context

> **Backend note**: The existing `approval_handler.go` `NotificationType_INPUT_REQUIRED` flow already supports this. Triage prompt updates are the primary work for R7.

---

## Manual Gates (Preserved)

The following steps remain explicitly manual (no automation):

| Gate | Reason |
|------|--------|
| Plan approval (`ApprovePlan`) | Human reviews AI-generated plan before execution starts |
| Spawn implementation (`SpawnSessionFromItem`) | Human decides when to start burning compute on implementation |
| Review override (`OverrideVerdict`) | Human judgment required to bypass a failing review gate |

---

## Acceptance Criteria

- [ ] Creating an item in the UI triggers triage automatically (no button click)
- [ ] Items with short description + 0 AC show the vagueness prompt before triage fires
- [ ] Triage suggestions are visible in the item detail pane after triage completes
- [ ] "Apply suggestions" updates AC and transitions item to "ready" in one action
- [ ] A notification appears when triage completes
- [ ] Notification click navigates directly to the item detail
- [ ] Triage prompt instructs AI to ask clarifying questions for vague items
- [ ] All existing triage tests pass; `make test` passes

---

## Key Files

| Layer | Path | Role |
|-------|------|------|
| Proto | `proto/session/v1/backlog.proto` | Add `TriageResult` message, update `ItemSession` |
| Backend handler | `server/services/backlog_service.go` | Auto-trigger in `CreateBacklogItem`, notification in `submitTriageResult` |
| MCP tool | `server/mcp/tools_backlog.go` | Publish notification on `submit_triage_result` completion |
| Frontend hook | `web-app/src/lib/hooks/useBacklogService.ts` | Map new `triageResult` field |
| UI | `web-app/src/components/backlog/BacklogItemDetail.tsx` | Triage review panel |
| UI | `web-app/src/components/backlog/TriageLoadingIndicator.tsx` | Extend for completed state |
| Types | `web-app/src/lib/hooks/useBacklogService.ts` | `TriageResult` domain type |
