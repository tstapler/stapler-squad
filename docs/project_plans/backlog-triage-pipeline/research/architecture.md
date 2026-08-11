# Architecture Research — backlog-triage-pipeline

**Date**: 2026-05-18  
**Scope**: How each requirement fits into the existing system layers.

---

## System Layers (Existing)

```
proto/session/v1/          ← wire contract (backlog.proto, types.proto)
session/ent/schema/        ← DB schema (ent ORM)
session/storage_backlog.go ← persistence layer
server/services/           ← RPC handlers (BacklogService, ApprovalHandler)
server/mcp/tools_backlog.go← MCP tool handlers (submit_triage_result etc.)
server/events/             ← EventBus (pub/sub for notifications)
web-app/src/gen/           ← generated proto bindings (TypeScript)
web-app/src/lib/hooks/     ← useBacklogService (domain mapping)
web-app/src/components/backlog/ ← React UI (BacklogItemDetail, etc.)
```

---

## R1 — Auto-trigger triage

**Touch points:**

1. **Proto** (`backlog.proto`): Add `bool skip_triage = 9` to `CreateBacklogItemRequest`; add `bool triage_triggered = 2` to `CreateBacklogItemResponse`.

2. **Service** (`backlog_service.go` `CreateBacklogItem`): After `storage.CreateBacklogItem` succeeds, if `!req.Msg.SkipTriage && item.RepoPath != "" && s.sessionCreator != nil`, call `s.TriggerTriage(ctx, connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: created.ID}))` in a goroutine (fire-and-forget to not block the create response). Set `triage_triggered = true` in response.

   **Goroutine approach vs. synchronous**: Spawning a session (CreateDirectorySession) involves subprocess execution and may take hundreds of milliseconds. Making it async with `go func()` prevents `CreateBacklogItem` from feeling slow. The `ItemSession` record creation inside `TriggerTriage` is the only DB write that races — this is acceptable since `CreateBacklogItem` has already committed before the goroutine fires.

   **Alternative (synchronous)**: Call `TriggerTriage` synchronously and return its `ItemSession` in the response. Simpler, no goroutine, but slows the create response by ~200-500ms. Recommended if the UX team wants `triage_triggered=true` to be reliable in the response.

3. **Frontend hook** (`useBacklogService.ts`): Update `createBacklogItem` return type to include `triageTriggered?: boolean`; map from `resp.triageTriggered`.

4. **Frontend component** (`BacklogItemDetail.tsx` or `BacklogList`): After `createBacklogItem` resolves, check vagueness (R2) before auto-polling for triage status.

---

## R2 — Vagueness detection

**Architecture: pure client-side, no new RPC**

After `createBacklogItem` call resolves (item returned in response), the client checks:
```
isVague = (item.description?.length ?? 0) < 80 && item.acCriteria.length === 0
```
If `isVague && resp.triageTriggered`, render a modal/banner:
- "Refine before triage": navigate to edit form (or inline-expand description input), user edits, saves, then triage runs on the updated item
- "Run triage anyway": dismiss the prompt; triage already fired (or fires now)

**Component placement**: A `VaguenessPrompt` modal component rendered from the backlog creation form's parent component (`BacklogList.tsx` or the page), passing item data and `onDismiss`/`onRefine` callbacks.

---

## R3 — Expose triage result via API

**Data flow:**
```
ItemSession.triage_result (JSON string in DB)
  → itemSessionToProto() (backlog_service.go)  [NEW: unmarshal → TriageResult proto]
  → ItemSession proto message                   [NEW: triage_result field]
  → GetBacklogItem / ListBacklogItems responses
  → mapItemSession() (useBacklogService.ts)     [NEW: map to LinkedSession.triageResult]
  → BacklogItem domain type                     [triageResult on matching triage session]
```

**Proto changes:**
```protobuf
message TriageResult {
  string summary = 1;
  repeated TriageSuggestion suggestions = 2;
  repeated string clarifying_questions = 3;
}

message TriageSuggestion {
  string text = 1;
  string rationale = 2;
}
```
Add to `ItemSession` message: `TriageResult triage_result = 12;`

**Backend change** (`itemSessionToProto`): Unmarshal `is.TriageResult` JSON into `TriageResult` proto only if non-empty. JSON struct must match what `submitTriageResult` writes (currently `{"summary":"...","suggestions":[{"text":"...","rationale":"..."}]}`).

**Frontend type addition** (`useBacklogService.ts`):
```typescript
export interface TriageResult {
  summary: string;
  suggestions: Array<{ text: string; rationale: string }>;
  clarifyingQuestions: string[];
}
```
Add `triageResult?: TriageResult` to `LinkedSession`. Map in `mapItemSession()` from `s.triageResult` if present. Expose convenience getter on `BacklogItem`: `get triageResult()` — finds last triage session with a triageResult.

---

## R4 — Triage diff/preview panel

**Component architecture:**

```
BacklogItemDetail.tsx
  ├── [existing] TriageLoadingIndicator  (triageStatus === "running")
  ├── [NEW]      TriageReviewPanel       (triageStatus === "completed" && !dismissed)
  └── [existing] GateVerdictBox          (status === "review")
```

`TriageReviewPanel` props:
```typescript
interface TriageReviewPanelProps {
  item: BacklogItem;            // for current description + acCriteria
  triageResult: TriageResult;   // suggestions + summary
  planArtifactsPath?: string;
  onApply: () => Promise<void>; // calls applyTriageSuggestions
  onSkip: () => void;           // sets localStorage dismissed flag + hides
}
```

**Dismissed state** (`localStorage`):
- Read on mount: `const dismissed = localStorage.getItem('triage-panel-dismissed-${item.id}')`
- Set on skip: `localStorage.setItem('triage-panel-dismissed-${item.id}', '1')`
- Component renders `null` if dismissed === '1'

**Diff display**: No third-party diff library needed. The suggestions array replaces AC wholesale. Simple side-by-side rendering: current AC list (left column) vs. suggested AC list (right column) with visual highlights (added = green, same = neutral, removed = gray strikethrough). Implemented with vanilla-extract `.css.ts` styles — no `react-diff-viewer` or similar dependency needed for the v1.

---

## R5 — Apply suggestions

**Action sequence in `TriageReviewPanel.onApply`:**
1. Call `updateBacklogItem(item.id, { acCriteria: triageResult.suggestions.map((s, i) => ({ index: i, text: s.text, status: 'pending' })) })`
2. If success, call `transitionStatus(item.id, 'ready', 'idea')`
3. If `CodeAborted` from step 2 (optimistic concurrency mismatch), show error + allow retry
4. If both succeed: dismiss triage panel (set localStorage) + show success toast ("Triage applied — item is now ready") + reload item

**Toast infrastructure**: Check if a toast/notification component already exists in the codebase before building a new one.

---

## R6 — Triage complete notification

**Architecture: publish from MCP tool handler (`submitTriageResult`)**

Rationale: `submit_triage_result` is the point where the triage AI signals completion. The MCP handler is the right place because it already has the `itemID`, `summary`, and `suggestions` count. The `BacklogService` layer does not need to know about notifications.

**Change to `backlogHandlers` struct:**
```go
type backlogHandlers struct {
    storage  *session.Storage
    store    session.InstanceStore
    eventBus *events.EventBus  // NEW
}
```

**Change to `mcp/server.go`** `NewCore()` signature: add `eventBus *events.EventBus` parameter; thread from `dependencies.go` where `mcp.NewCore()` is called.

**Publication in `submitTriageResult`** (after successful save):
```go
if h.eventBus != nil {
    event := events.NewNotificationEvent(
        callerUUID,                       // sessionID
        "",                               // sessionName (triage session; no display name needed)
        uuid.New().String(),              // notificationID
        int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
        int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL),
        "Triage complete",
        fmt.Sprintf("%s — %d suggestion(s). Click to review.", itemTitle, len(suggestions)),
        map[string]string{"item_id": itemID},
    )
    h.eventBus.Publish(event)
}
```

**Item title lookup**: `submitTriageResult` currently only has `itemID`. Load the backlog item title: `item, _ := h.storage.GetBacklogItem(ctx, itemID)` before building the notification. This is a cheap read.

**Frontend deep-link**: notification metadata `item_id` → click handler navigates to `/backlog?item=<item_id>`. Requires updating the notification click handler in `useSessionNotifications.ts` or the notification panel component to check `metadata.item_id`.

---

## R7 — Interactive clarification

**Architecture: prompt-only change + optional JSON field**

The entire implementation is in `buildTriagePrompt()` (`backlog_service.go:1033`). Add a "clarifying questions" section to the Step 4 instructions:

```
### Step 4 — Clarification (if needed)
If the item description is ambiguous and you need specific information to produce
a quality plan, ask ≤3 clarifying questions using AskUserQuestion before proceeding.
If no questions are needed, skip this step. If the user does not respond within
30 minutes, proceed with the available context.
```

The AI's use of `AskUserQuestion` automatically routes through the existing `broadcastQuestionNotification` flow in `ApprovalHandler` — no new server code needed.

**Optional: persist clarifying questions in triage result JSON**
Add `clarifying_questions []string` to `triageSuggestion` Go struct / JSON payload in `submitTriageResult`. Map to `TriageResult.clarifying_questions` proto field. This allows the UI to display questions in the triage review panel (R4) for context.

---

## Data Flow Summary

```
User creates item
  → CreateBacklogItem (backlog_service.go)
      → storage.CreateBacklogItem
      → [R1] auto-trigger: go TriggerTriage(ctx, item.ID)
      → response: { item, triage_triggered: true }
  → [R2] client-side: if vague, show VaguenessPrompt

AI triage session runs
  → submit_triage_result MCP tool (tools_backlog.go)
      → storage.UpdateItemSessionTriageResult (JSON payload)
      → storage.UpdateBacklogItem (plan_artifacts_path)
      → [R6] eventBus.Publish(INPUT_REQUIRED notification)

User opens item detail
  → GetBacklogItem → itemSessionToProto → [R3] TriageResult mapped
  → BacklogItemDetail renders [R4] TriageReviewPanel

User clicks "Apply suggestions"
  → [R5] updateBacklogItem (new AC) → transitionStatus(ready)
  → dismiss panel, show toast
```
