# Backlog Triage Pipeline — Implementation Plan

**Project**: backlog-triage-pipeline  
**Phase**: 3 — Planning  
**Date**: 2026-05-18  
**Status**: Ready for implementation session

---

## Archetype

Feature addition across existing service, proto, and UI layers. No new services. No ent schema migration required (`triage_result` column already exists). All new components follow established patterns (`GateVerdictBox`, `TriageLoadingIndicator`, `broadcastQuestionNotification`).

---

## Scope Summary

| Requirement | Status | Notes |
|---|---|---|
| R1 — Auto-trigger triage on creation | In scope | Synchronous (not goroutine) per confirmed architecture decision |
| R2 — Vagueness detection | In scope | Client-side; `skipTriage` passed pre-submit |
| R3 — Expose triage result via API | In scope | New `TriageResult` proto message + `itemSessionToProto` + `mapItemSession` |
| R4 — Triage review panel | In scope | New `TriageReviewPanel` + `TriageDiffSection` + `TriageErrorBanner` components |
| R5 — Apply suggestions | In scope | Bulk apply; undo toast; `updateBacklogItem` → `transitionStatus` |
| R6 — Triage complete notification | In scope | EventBus threaded into `backlogHandlers`; publish from `submitTriageResult` |
| R7 — Interactive clarification | **Descoped** | One-shot sessions are incompatible with pause-and-wait; prompt instructs AI to include questions in `suggestions` array surfaced as "Questions" section in panel |

**Descoped detail (R7)**: `buildTriagePrompt` will instruct the AI to include clarifying questions as entries in the `suggestions` array (with `rationale: "question"` marker) when the description is ambiguous. The `TriageReviewPanel` surfaces these as a separate "Questions" section. No new RPC, no event bus changes, no session lifecycle changes required.

---

## Dependency Chain

```
Epic 1: Proto + Regen (T-01)
    ↓
Epic 2: Backend (T-02, T-03, T-04) — can run in parallel after T-01
    ↓
Epic 3: Frontend (T-05 → T-06 → T-07 → T-08 → T-09) — sequential
    ↓
Epic 4: Notification deep-link (T-10) — depends on T-04 (R6 backend) + T-08 (panel)
    ↓
Epic 5: Tests + hardening (T-11, T-12) — depends on T-02 through T-10
```

---

## Epic 1 — Proto Contract

**Stories**: 1  
**Tasks**: 1

### Story 1.1 — Extend proto messages for triage pipeline

#### T-01: Add `TriageResult` message + update `ItemSession`, `CreateBacklogItemRequest/Response`

**Estimated time**: 1–2h  
**Prerequisite for**: T-02, T-03, T-04, T-05  
**Files to modify**:
- `proto/session/v1/backlog.proto`

**Changes**:

1. Add `TriageResult` and `TriageSuggestion` messages (standalone, after `ReviewVerdict`):

```protobuf
message TriageSuggestion {
  string text = 1;
  string rationale = 2;  // "question" marker for R7-lite questions
}

message TriageResult {
  string summary = 1;
  repeated TriageSuggestion suggestions = 2;
  repeated string clarifying_questions = 3;
}
```

2. Add `triage_result` field to `ItemSession` at field 12:
```protobuf
TriageResult triage_result = 12;
```

3. Add `skip_triage` to `CreateBacklogItemRequest` at field 9 (next available after field 8 `notes`):
```protobuf
bool skip_triage = 9;
```

4. Add `triage_triggered` to `CreateBacklogItemResponse` at field 2:
```protobuf
bool triage_triggered = 2;
```

5. Run `make generate-proto` to regenerate:
   - `session/gen/proto/go/session/v1/backlog.pb.go`
   - `web-app/src/gen/session/v1/backlog_pb.ts`

**Risks**:
- Proto field numbering: verify field 12 on `ItemSession` (fields 1–11 occupied by `review_verdict = 11`); verify field 9 on `CreateBacklogItemRequest` (fields 1–8 occupied — research doc confirms field 8 = `notes`, field 9 is clear). Run `make generate-proto` immediately and verify compile.
- Generated TypeScript bindings: verify `backlog_pb.ts` exports `TriageResult`, `TriageSuggestion`, `skipTriage`, `triageTriggered` before proceeding to T-05.

---

## Epic 2 — Backend

**Stories**: 3  
**Tasks**: 3 (can run in parallel after T-01)

### Story 2.1 — Auto-trigger triage + double-trigger guard (R1)

#### T-02: Synchronous auto-trigger in `CreateBacklogItem`; add double-trigger guard to `TriggerTriage`

**Estimated time**: 2–3h  
**Prerequisite for**: T-05 (frontend uses `triageTriggered` from response)  
**Prerequisite**: T-01 complete (generated proto bindings required)  
**Files to modify**:
- `server/services/backlog_service.go`

**Changes in `CreateBacklogItem`** (after `storage.CreateBacklogItem` succeeds, before return):

```go
triageTriggered := false
if !req.Msg.SkipTriage && created.RepoPath != "" && s.sessionCreator != nil {
    triageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    _, triageErr := s.TriggerTriage(triageCtx,
        connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: created.ID.String()}))
    if triageErr != nil {
        log.WarnLog.Printf("[CreateBacklogItem] auto-triage failed for item %s: %v", created.ID, triageErr)
        // Do not fail the create; log and continue
    } else {
        triageTriggered = true
    }
}
return connect.NewResponse(&sessionv1.CreateBacklogItemResponse{
    Item:            backlogItemToProto(created),
    TriageTriggered: triageTriggered,
}), nil
```

**Note on synchronous vs goroutine**: Calling `TriggerTriage` synchronously (with a 30s timeout) is the confirmed architecture decision. It avoids the context-cancel race (P1), makes `triage_triggered` reliable in the response, and ensures the vagueness prompt (R2) fires before the session is active. The 30s timeout is a safety net — normal session spawn is ~200-500ms.

**Changes in `TriggerTriage`** (double-trigger guard — add after item status check, before artifact dir creation):

```go
// Double-trigger guard: check for an existing running triage session.
existingSessions, _ := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
for _, is := range existingSessions {
    if is.SessionRole == string(session.SessionRoleTriage) && is.EndedAt == nil {
        return nil, connect.NewError(connect.CodeAlreadyExists,
            fmt.Errorf("triage session already running for item %s", req.Msg.ItemId))
    }
}
```

**Risks**:
- `ListItemSessions` method name: verify the exact storage method name for listing item sessions by item ID. Check `session/storage_backlog.go` for the correct signature.
- `connect.CodeAlreadyExists` is the appropriate gRPC code; frontend should treat this as a non-error (no-op) when triggered from the UI "Trigger Triage" button while auto-trigger is running.

---

### Story 2.2 — Expose triage result in API responses (R3)

#### T-03: Map `triage_result` JSON in `itemSessionToProto`; add canonical Go struct

**Estimated time**: 2h  
**Prerequisite**: T-01 complete  
**Files to modify**:
- `server/services/backlog_service.go`
- `server/mcp/tools_backlog.go`

**Changes in `backlog_service.go`**:

1. Add canonical Go struct for triage result JSON (unexported, collocated with `itemSessionToProto`):

```go
// triageResultJSON is the canonical shape written by submitTriageResult and
// read back by itemSessionToProto. Must stay in sync — do not use map[string]interface{}.
type triageResultJSON struct {
    Summary            string             `json:"summary"`
    Suggestions        []triageSuggestion `json:"suggestions"`
    ClarifyingQuestions []string          `json:"clarifying_questions,omitempty"`
}
```

Note: `triageSuggestion` is defined in `tools_backlog.go`. Move the struct to a shared location (e.g., `session/backlog_triage.go`) or redeclare it in the service package as an unexported type. Preferred: define `triageResultJSON` in `backlog_service.go` as a self-contained unexported struct with inline `text`/`rationale` fields to avoid cross-package coupling.

2. In `itemSessionToProto`, after the `ReviewVerdict` mapping block, add:

```go
if is.TriageResult != "" {
    var tr triageResultJSON
    if err := json.Unmarshal([]byte(is.TriageResult), &tr); err != nil {
        log.WarnLog.Printf("[itemSessionToProto] invalid triage_result JSON for session %s: %v", is.ID, err)
    } else {
        suggs := make([]*sessionv1.TriageSuggestion, len(tr.Suggestions))
        for i, s := range tr.Suggestions {
            suggs[i] = &sessionv1.TriageSuggestion{Text: s.Text, Rationale: s.Rationale}
        }
        p.TriageResult = &sessionv1.TriageResult{
            Summary:             tr.Summary,
            Suggestions:         suggs,
            ClarifyingQuestions: tr.ClarifyingQuestions,
        }
    }
}
```

**Changes in `tools_backlog.go`**:

Update `submitTriageResult` to use `triageResultJSON` struct for serialization (currently uses `map[string]interface{}`). This ensures read and write use the same typed struct and prevents P4 (schema drift):

```go
// Replace: triagePayload := map[string]interface{}{...}
triagePayload := triageResultJSON{
    Summary:     summary,
    Suggestions: suggestions,
}
payloadJSON, jsonErr := json.Marshal(triagePayload)
```

If `triageSuggestion` struct stays in `tools_backlog.go`, import or alias it in the service layer. Preferred approach: keep `triageSuggestion` in `tools_backlog.go`, define `triageResultJSON` in `backlog_service.go` with its own inline suggestion type. Both types serialize identically.

**R7-lite prompt change** (also in T-03 scope since it modifies `buildTriagePrompt`):

In `buildTriagePrompt` (`backlog_service.go:1033`), add a clarifying questions instruction block to the Step instructions:

```
### Step 4 — Clarifying Questions (optional)
If the item description is ambiguous and you need specific information to
produce quality acceptance criteria, include up to 3 clarifying questions
in the suggestions array with rationale set to "question":
  { "text": "What is the expected timeout behavior?", "rationale": "question" }
If you have no questions, omit this step. Do not pause or wait for user input.
```

**Risks**:
- P4 (schema drift): mitigated by canonical struct. Verify JSON field names match exactly between `triageSuggestion` (write) and the struct used in `itemSessionToProto` (read).
- `is.TriageResult` field access: confirm the ent-generated `ItemSession` struct exposes `TriageResult` as a `string` field. Check `session/ent/item_session.go` if needed.

---

### Story 2.3 — Thread EventBus into MCP; publish triage-complete notification (R6)

#### T-04: Add `eventBus` to `backlogHandlers`; publish notification in `submitTriageResult`

**Estimated time**: 2–3h  
**Prerequisite**: T-01 complete  
**Files to modify**:
- `server/mcp/tools_backlog.go`
- `server/mcp/server.go`
- `server/server.go` (update `NewHTTPHandler` call)

**Changes in `tools_backlog.go`**:

1. Add `eventBus` field to `backlogHandlers`:

```go
type backlogHandlers struct {
    storage  *session.Storage
    store    session.InstanceStore
    eventBus *events.EventBus  // optional; nil means notifications are disabled
}
```

2. After successful `UpdateItemSessionTriageResult` call in `submitTriageResult`, add notification publish:

```go
if h.eventBus != nil {
    // Load item title for notification message (cheap PK lookup; fallback on error).
    itemTitle := "Item " + itemID
    if backlogItem, loadErr := h.storage.GetBacklogItem(ctx, itemID); loadErr == nil {
        itemTitle = backlogItem.Title
    }

    notifMsg := fmt.Sprintf("%s — %d suggestion(s). Click to review.", itemTitle, len(suggestions))
    event := events.NewNotificationEvent(
        callerUUID,
        "",
        uuid.New().String(),
        int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
        int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL),
        "Triage complete",
        notifMsg,
        map[string]string{"item_id": itemID},
    )
    h.eventBus.Publish(event)
}
```

**Changes in `server.go`** (update `NewCore` to accept and thread `eventBus`):

```go
// Option A: Add eventBus as a parameter (preferred — explicit)
func NewCore(store session.InstanceStore, svc *services.SessionService,
    sbMgr *scrollback.ScrollbackManager, storage *session.Storage,
    eventBus *events.EventBus) *mcpserver.MCPServer {
    // ...
    if storage != nil {
        registerBacklogTools(s, &backlogHandlers{
            storage:  storage,
            store:    store,
            eventBus: eventBus,
        })
    }
}
```

Update `NewHTTPHandler` and `RunServer` signatures correspondingly.

**Changes in `server/server.go`**:

Locate the call to `servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, ...)` and pass `deps.EventBus` as the new final argument. Confirm `deps.EventBus` is available — check `dependencies.go` for the field name.

**Risks**:
- P9 (EventBus threading): `RunServer` (stdio path) also calls `NewCore`. Audit `server/mcp/server.go` to update both callers. If the stdio server has no event bus available, pass `nil` — the guard in `submitTriageResult` handles `nil` safely.
- `uuid.New()` import: ensure `github.com/google/uuid` is already imported in `tools_backlog.go` (check existing imports before adding).
- P8 (item title lookup): the `GetBacklogItem` call must not fail the notification. The fallback string `"Item " + itemID` ensures notification is always sent.

---

## Epic 3 — Frontend

**Stories**: 4  
**Tasks**: 5 (mostly sequential)

### Story 3.1 — Domain types + hook update (R3 frontend)

#### T-05: Add `TriageResult` type; update `LinkedSession`, `mapItemSession`, `mapBacklogItem`, `createBacklogItem`

**Estimated time**: 2h  
**Prerequisite**: T-01 complete (generated TypeScript bindings available)  
**Prerequisite for**: T-06, T-07, T-08  
**Files to modify**:
- `web-app/src/lib/hooks/useBacklogService.ts`

**Changes**:

1. Add `TriageResult` interface after `LinkedSession`:

```typescript
export interface TriageSuggestion {
  text: string;
  rationale: string;  // "question" for R7-lite questions
}

export interface TriageResult {
  summary: string;
  suggestions: TriageSuggestion[];
  clarifyingQuestions: string[];
}
```

2. Add `triageResult?: TriageResult` to `LinkedSession`:

```typescript
export interface LinkedSession {
  // ... existing fields ...
  triageResult?: TriageResult;
}
```

3. In `mapItemSession`, after `reviewVerdict` mapping block, add `triageResult` mapping:

```typescript
if (s.triageResult) {
  const tr = s.triageResult;
  session.triageResult = {
    summary: tr.summary,
    suggestions: (tr.suggestions ?? []).map((sg) => ({
      text: sg.text,
      rationale: sg.rationale,
    })),
    clarifyingQuestions: tr.clarifyingQuestions ?? [],
  };
}
```

4. Update `mapBacklogItem` triage status derivation (P12 fix — check for non-empty `triageResult.summary` before marking `"completed"`):

```typescript
let triageStatus: BacklogItem["triageStatus"];
const triageSession = linkedSessions.filter((s) => s.role === "triage").at(-1);
if (triageSession) {
  if (triageSession.endedAt) {
    // Only mark completed if triage result has content (not a crashed session)
    triageStatus = triageSession.triageResult?.summary ? "completed" : "failed";
  } else {
    triageStatus = "running";
  }
}
```

5. Add `triageResult` convenience getter on `BacklogItem` (expose the most recent triage session's result):

```typescript
export interface BacklogItem {
  // ... existing fields ...
  triageResult?: TriageResult;  // from the most recent triage session
}
```

In `mapBacklogItem`, compute:

```typescript
const triageResult = triageSession?.triageResult;
// ... include in return object: triageResult
```

6. Update `BacklogItemInput` to add `skipTriage` flag:

```typescript
export interface BacklogItemInput {
  // ... existing fields ...
  skipTriage?: boolean;
}
```

7. Update `createBacklogItem` in hook to pass `skipTriage` and return `triageTriggered`:

Update the `UseBacklogServiceReturn` interface:
```typescript
createBacklogItem: (data: BacklogItemInput) => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>;
```

Update the hook implementation:
```typescript
const resp = await clientRef.current.createBacklogItem({
  // ... existing fields ...
  skipTriage: data.skipTriage ?? false,
});
return resp.item
  ? { item: mapBacklogItem(resp.item), triageTriggered: resp.triageTriggered }
  : null;
```

**Risks**:
- Return type change for `createBacklogItem` is a breaking change for existing callers. Audit all call sites in `BacklogItemForm.tsx` and `BacklogPage` before committing — update each to destructure `{ item, triageTriggered }`.
- Generated proto type for `skipTriage` (camelCase) — verify the generated TypeScript field name from `backlog_pb.ts` after T-01.

---

### Story 3.2 — Vagueness prompt modal (R2)

#### T-06: Implement `VaguenessPromptModal` component; hook into `BacklogPage` post-submit

**Estimated time**: 2–3h  
**Prerequisite**: T-05  
**Prerequisite for**: T-08 (panel integration context; can be run in parallel)  
**Files to create**:
- `web-app/src/components/backlog/VaguenessPromptModal.tsx`
- `web-app/src/components/backlog/VaguenessPromptModal.css.ts`

**Files to modify**:
- `web-app/src/app/backlog/page.tsx`
- `web-app/src/components/backlog/BacklogItemForm.tsx` (pass `skipTriage` pre-submit)

**VaguenessPromptModal component**:

Props:
```typescript
interface VaguenessPromptModalProps {
  itemTitle: string;
  onRefine: () => void;   // dismiss modal, re-open form focused on description
  onProceed: () => void;  // dismiss modal, triage proceeds
}
```

Renders as a modal overlay (`createPortal` into `document.body` — per ADR-009 rule against `position: fixed` without portal). Uses `role="dialog"` `aria-modal="true"` `aria-labelledby`. Focus traps between the two buttons. No escape key dismissal (user must choose). See UX mockup Section 4.1.

Styling via `VaguenessPromptModal.css.ts` using `vars` from theme contract — no hardcoded values.

**BacklogItemForm changes**:

Vagueness is evaluated synchronously before `onSubmit` is called. Add `isVague` utility function:

```typescript
function isVague(description?: string, acCriteria?: AcCriterion[]): boolean {
  return (description?.trim().length ?? 0) < 80 && (acCriteria?.length ?? 0) === 0;
}
```

The form evaluates `isVague(data.description, data.acCriteria)` before calling `onSubmit`, and passes `skipTriage: isVague(...)` as part of submitted data. This ensures that vague items are created with `skip_triage: true`, preventing auto-triage on the thin description.

**BacklogPage changes**:

After `createBacklogItem` resolves:
1. If `triageTriggered && isVague(item)`: show `VaguenessPromptModal`
   - "Add more detail" → close modal, re-open form with item ID for editing, show description field focused
   - "Run triage anyway" → close modal (triage already running)
2. If `!triageTriggered && !skipTriage && item.repoPath`: offer "Start triage" button in item detail

Note: Since `skipTriage` is evaluated pre-submit and vague items are created with `skip_triage: true`, `triageTriggered` will be `false` for vague items. The modal fires based on client-side vagueness check of the newly created item, then "Run triage anyway" calls `triggerTriage(item.id)` explicitly.

Updated flow:
```
submit form
  → skipTriage = isVague(description, acCriteria)
  → createBacklogItem({ ...data, skipTriage })
  → if isVague: show VaguenessPromptModal
      → "Add more detail": setShowForm(true) with item.id for UpdateBacklogItem
      → "Run triage anyway": triggerTriage(item.id) explicitly
  → if !isVague: triageTriggered=true (triage already running), proceed normally
```

**Risks**:
- `createPortal` requires `document` to be available (not SSR). Use a `mounted` state flag or `useEffect` pattern to defer portal creation, consistent with Next.js SSR constraints.
- Focus trap implementation: use a simple keyboard event handler on the modal container — no third-party focus-trap library unless one already exists in the codebase. Check existing modal components for the established pattern.
- The "Add more detail" re-open flow: `BacklogItemForm` currently handles `create` only. If re-opened for editing the newly created item, it must call `updateBacklogItem` rather than `createBacklogItem`. Confirm how the form handles edit vs. create mode before implementing.

---

### Story 3.3 — Triage review panel (R4 + R5)

#### T-07: Implement `TriageDiffSection` and `TriageErrorBanner` sub-components

**Estimated time**: 2h  
**Prerequisite**: T-05  
**Prerequisite for**: T-08  
**Files to create**:
- `web-app/src/components/backlog/TriageDiffSection.tsx`
- `web-app/src/components/backlog/TriageDiffSection.css.ts`
- `web-app/src/components/backlog/TriageErrorBanner.tsx`
- `web-app/src/components/backlog/TriageErrorBanner.css.ts`

**TriageDiffSection component**:

```typescript
interface TriageDiffSectionProps {
  currentCriteria: AcCriterion[];
  suggestedSuggestions: TriageSuggestion[];
}
```

Renders two-column diff: current AC (left) vs. suggested AC (right). Uses `+` prefix and `--success-bg` token for added items. Uses `-` prefix and `--error-bg` token for removed items. `aria-label="Added:"` / `aria-label="Removed:"` on each delta item (color not sole indicator). Semantic `<dl>` or `<table>` structure per UX spec Section 7.2.

No third-party diff library — simple set comparison sufficient for v1. Added items = in suggested but not in current (by text). Removed items = in current but not in suggested.

Filters out suggestions with `rationale === "question"` — those render in a separate "Questions" section (R7-lite).

**TriageErrorBanner component**:

```typescript
interface TriageErrorBannerProps {
  message: string;
  onReload: () => void;
  onSkip: () => void;
}
```

`role="alert"` container. Shows error message, "Reload item" button, "Skip without applying" button. Renders inside `TriageReviewPanel` — above the diff, not page-level. See UX mockup Section 4.4.

---

#### T-08: Implement `TriageReviewPanel`; integrate into `BacklogItemDetail`

**Estimated time**: 3–4h  
**Prerequisite**: T-05, T-07  
**Prerequisite for**: T-09 (undo toast), T-10 (notification deep-link)  
**Files to create**:
- `web-app/src/components/backlog/TriageReviewPanel.tsx`
- `web-app/src/components/backlog/TriageReviewPanel.css.ts`

**Files to modify**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx`

**TriageReviewPanel component**:

```typescript
interface TriageReviewPanelProps {
  item: BacklogItem;
  triageResult: TriageResult;
  onApply: () => Promise<void>;
  onSkip: () => void;
}
```

Internal state: `applyState: "idle" | "applying" | "error"`, `applyError?: string`, `preApplyCriteria?: AcCriterion[]` (cached for undo).

**Dismissed state via localStorage**:
```typescript
const DISMISSED_KEY = (id: string) => `triage-panel-dismissed-${id}`;
```
Read on mount; set on skip or successful apply.

**Visibility condition** (in `BacklogItemDetail`):
```typescript
triageStatus === "completed" &&
item.status === "idea" &&
item.triageResult != null &&
!isDismissed(item.id)
```

**Apply flow** (`onApply` prop implementation in `BacklogItemDetail`):
1. Cache `preApplyCriteria = item.acCriteria` in local state
2. Call `updateBacklogItem(item.id, { acCriteria: suggestedCriteria })`
3. On success: call `transitionStatus(item.id, "ready", "idea")`
4. On `CodeAborted` from either call: set `applyState = "error"` with "Item was modified — please reload and try again."
5. On full success: set localStorage dismiss flag + call `onApplySuccess(preApplyCriteria)` (bubble up for undo toast)
6. Call `load()` to refresh item state

**Panel states** (per UX spec Section 3.1):
- With suggestions: full diff + Apply/Skip buttons
- No suggestions: summary-only + "Mark ready" shortcut + Skip
- Applying: Apply button shows spinner + `aria-busy="true"`, all buttons disabled
- Error: `TriageErrorBanner` renders above diff
- Questions section: if `triageResult.suggestions.filter(s => s.rationale === "question").length > 0`, render a "Triage Questions" sub-section below the diff

**Accessibility** (per UX spec Section 7.2):
- `aria-live="polite"` on panel container wrapper in `BacklogItemDetail` (wrapper present before panel mounts)
- `<h3>` for "Triage Ready" heading
- Apply button: `aria-label="Apply triage suggestions — replaces acceptance criteria and marks item ready"`
- Dismiss button: `aria-label="Dismiss triage review"`
- Apply in-progress: `aria-busy="true"`
- Error banner: `role="alert"`

**BacklogItemDetail integration**:

Add above the existing `GateVerdictBox` conditional block:

```typescript
{item.triageStatus === "completed" &&
  item.status === "idea" &&
  item.triageResult &&
  !isDismissed(item.id) && (
    <TriageReviewPanel
      item={item}
      triageResult={item.triageResult}
      onApply={handleApplyTriageSuggestions}
      onSkip={() => { setLocalDismissed(item.id); }}
    />
)}
```

Implement `handleApplyTriageSuggestions` as described in apply flow above.

**Risks**:
- P7 (optimistic concurrency): `updateBacklogItem` with `expectedUpdatedAt` is the safe path. Check if `UpdateBacklogItemRequest` in the current hook already passes `expected_updated_at` — if not, add it using the item's `updatedAt` timestamp.
- `triageStatus === "failed"` state (P12): ensure `BacklogItemDetail` shows an appropriate message for `triageStatus === "failed"` (e.g., extend `TriageLoadingIndicator` or add a sibling banner: "Triage encountered an error. Trigger triage manually to retry.").

---

### Story 3.4 — Undo toast for apply action (R5 finish)

#### T-09: Implement undo toast; integrate post-apply

**Estimated time**: 1–2h  
**Prerequisite**: T-08  
**Files to check first**: Search for existing toast/notification component in `web-app/src/components/` before creating a new one.

**If a toast component exists**: Wire it to the apply success path in `BacklogItemDetail`. Ensure 6–8s duration (longer than standard — per UX spec Section 2.4).

**If no toast component exists**: Create `web-app/src/components/ui/Toast.tsx` + `Toast.css.ts`. Simple implementation: renders via `createPortal`, auto-dismisses after 7s, exposes "Undo" action button.

**Undo action logic** (in `BacklogItemDetail`):
```typescript
const handleUndo = useCallback(async () => {
  if (!preApplyCriteria) return;
  await updateBacklogItem(item.id, { acCriteria: preApplyCriteria });
  await transitionStatus(item.id, "idea", "ready");
  await load();
}, [preApplyCriteria, item.id, updateBacklogItem, transitionStatus, load]);
```

`preApplyCriteria` must be cached in component state before `onApply` runs (set in step 1 of the apply flow in T-08).

---

## Epic 4 — Notification Deep-link (R6 frontend)

**Stories**: 1  
**Tasks**: 1

### Story 4.1 — Handle `item_id` metadata in notification click

#### T-10: Update notification handler to navigate to `/backlog?item=<id>` on triage-complete notification

**Estimated time**: 1–2h  
**Prerequisite**: T-04 (backend publishes notification with `item_id` metadata), T-08 (panel renders on that URL)  
**Files to modify**:
- `web-app/src/lib/hooks/useSessionNotifications.ts` (or the notification panel component where click handlers are defined)

**Change**: In the notification click handler, check for `metadata["item_id"]`:

```typescript
// In the notification onClick/action handler
const itemId = notification.metadata?.["item_id"];
if (itemId) {
  router.push(`/backlog?item=${itemId}`);
  return;
}
```

The backlog page (`/backlog/page.tsx`) already reads `?item=<id>` from `useSearchParams()` and opens the item detail panel (confirmed from existing code at `web-app/src/app/backlog/board/page.tsx` lines 45, 57 and `web-app/src/app/backlog/page.tsx` line 158). No page-level changes required beyond the click handler.

**Risks**:
- `router` availability: `useSessionNotifications.ts` is a hook — it may not have a router. If the notification click handler is defined in a component rather than the hook, apply the change there. Audit before editing.
- The existing `useSearchParams()` handling on the backlog page: confirm it opens the correct item when `?item=<id>` is set. If there is a filter that would exclude the item from the list view, per UX spec Section 6, the item should be shown regardless of active filters. This is a UX enhancement — if it requires significant page changes, defer to a follow-up task.

---

## Epic 5 — Tests and Hardening

**Stories**: 2  
**Tasks**: 2

### Story 5.1 — Backend tests

#### T-11: Go unit tests for R1, R3, R6

**Estimated time**: 3h  
**Prerequisite**: T-02, T-03, T-04 complete  
**Files to modify/create**:
- `server/services/backlog_service_test.go` (existing)
- `server/mcp/tools_backlog_test.go` (existing)

**Test cases to add**:

1. `TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet`: mock `sessionCreator`; verify `TriggerTriage` called, `triage_triggered=true` in response.
2. `TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue`: `skip_triage=true` → `triage_triggered=false`, no `TriggerTriage` call.
3. `TestCreateBacklogItem_SkipsTriageWhenRepoPathEmpty`: no `repo_path` → `triage_triggered=false`.
4. `TestTriggerTriage_DoubleTriggerGuard`: create item + triage session with no `ended_at` → `TriggerTriage` returns `CodeAlreadyExists`.
5. `TestItemSessionToProto_MapsTriageResult`: provide `triage_result` JSON → verify `TriageResult` proto fields populated correctly.
6. `TestItemSessionToProto_HandlesInvalidTriageResultJSON`: malformed JSON → warn + continue, no panic, `triage_result` is nil in proto.
7. `TestSubmitTriageResult_PublishesNotificationOnSuccess`: construct `backlogHandlers` with mock `eventBus`; call `submitTriageResult` with valid args → verify `Publish` called with `item_id` in metadata.
8. `TestSubmitTriageResult_NoNotificationWhenEventBusNil`: `eventBus=nil` → no panic, result still returned.

---

### Story 5.2 — Frontend unit tests

#### T-12: Jest/RTL tests for T-05, T-06, T-08

**Estimated time**: 3h  
**Prerequisite**: T-05 through T-09 complete  
**Files to create**:
- `web-app/src/components/backlog/VaguenessPromptModal.test.tsx`
- `web-app/src/components/backlog/TriageReviewPanel.test.tsx`

**Test cases to add**:

1. `VaguenessPromptModal_renders_when_description_short_and_no_ac`: renders modal after submit when `description.length < 80 && acCriteria.length === 0`.
2. `VaguenessPromptModal_does_not_render_when_ac_present`: `acCriteria.length >= 1` → no modal regardless of description length.
3. `VaguenessPromptModal_calls_onRefine_when_refine_clicked`.
4. `VaguenessPromptModal_calls_onProceed_when_proceed_clicked`.
5. `VaguenessPromptModal_has_no_escape_dismiss`.
6. `TriageReviewPanel_renders_diff_when_suggestions_present`.
7. `TriageReviewPanel_renders_summary_only_when_no_suggestions`.
8. `TriageReviewPanel_does_not_render_when_dismissed_in_localStorage`.
9. `TriageReviewPanel_apply_calls_updateBacklogItem_then_transitionStatus`.
10. `TriageReviewPanel_shows_error_banner_on_apply_failure`.
11. `TriageReviewPanel_shows_undo_toast_on_apply_success`.
12. `mapBacklogItem_triageStatus_is_failed_when_session_ended_but_no_triageResult`.
13. `mapBacklogItem_triageStatus_is_completed_when_session_ended_and_triageResult_present`.

Run:
```bash
cd web-app && npx jest --no-coverage --testPathPatterns="VaguenessPromptModal|TriageReviewPanel|useBacklogService"
```

---

## File Manifest

### New files

| File | Epic | Task |
|---|---|---|
| `web-app/src/components/backlog/VaguenessPromptModal.tsx` | 3 | T-06 |
| `web-app/src/components/backlog/VaguenessPromptModal.css.ts` | 3 | T-06 |
| `web-app/src/components/backlog/TriageDiffSection.tsx` | 3 | T-07 |
| `web-app/src/components/backlog/TriageDiffSection.css.ts` | 3 | T-07 |
| `web-app/src/components/backlog/TriageErrorBanner.tsx` | 3 | T-07 |
| `web-app/src/components/backlog/TriageErrorBanner.css.ts` | T-07 |
| `web-app/src/components/backlog/TriageReviewPanel.tsx` | 3 | T-08 |
| `web-app/src/components/backlog/TriageReviewPanel.css.ts` | 3 | T-08 |
| `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` | 5 | T-12 |
| `web-app/src/components/backlog/TriageReviewPanel.test.tsx` | 5 | T-12 |

### Modified files

| File | Epic | Task | Change summary |
|---|---|---|---|
| `proto/session/v1/backlog.proto` | 1 | T-01 | Add `TriageResult`, `TriageSuggestion`, field 12 on `ItemSession`, field 9 on `CreateBacklogItemRequest`, field 2 on `CreateBacklogItemResponse` |
| `server/services/backlog_service.go` | 2 | T-02, T-03 | Auto-trigger in `CreateBacklogItem`; double-trigger guard in `TriggerTriage`; `triageResultJSON` struct; `itemSessionToProto` mapping; `buildTriagePrompt` R7-lite questions |
| `server/mcp/tools_backlog.go` | 2 | T-03, T-04 | `eventBus` field on `backlogHandlers`; notification publish in `submitTriageResult`; use canonical struct for JSON serialization |
| `server/mcp/server.go` | 2 | T-04 | Add `eventBus` parameter to `NewCore`, `NewHTTPHandler`, `RunServer` |
| `server/server.go` | 2 | T-04 | Pass `deps.EventBus` to `servermcp.NewHTTPHandler` |
| `web-app/src/lib/hooks/useBacklogService.ts` | 3 | T-05 | Add `TriageSuggestion`, `TriageResult` interfaces; `triageResult` on `LinkedSession`; `triageResult` + `skipTriage` on `BacklogItem`/`BacklogItemInput`; update `mapItemSession`, `mapBacklogItem`, `createBacklogItem` |
| `web-app/src/app/backlog/page.tsx` | 3 | T-06 | Post-submit vagueness check; `VaguenessPromptModal` integration; `triggerTriage` explicit call |
| `web-app/src/components/backlog/BacklogItemForm.tsx` | 3 | T-06 | `skipTriage` passed with form data based on pre-submit vagueness check |
| `web-app/src/components/backlog/BacklogItemDetail.tsx` | 3 | T-08 | `TriageReviewPanel` integration; apply flow; undo toast; `triageStatus === "failed"` banner |
| `web-app/src/lib/hooks/useSessionNotifications.ts` | 4 | T-10 | Handle `metadata["item_id"]` → navigate to `/backlog?item=<id>` |
| `server/services/backlog_service_test.go` | 5 | T-11 | New test cases for auto-trigger, skip-triage, triage result mapping |
| `server/mcp/tools_backlog_test.go` | 5 | T-11 | New test cases for notification publish, `eventBus` nil safety |

---

## Risks and Mitigations Summary

| Risk | Likelihood | Impact | Mitigation | Task |
|---|---|---|---|---|
| P1: goroutine context cancel | **Mitigated** | High | Synchronous call with 30s timeout | T-02 |
| P2: double-trigger concurrent sessions | Medium | Medium | `ListItemSessions` guard in `TriggerTriage` | T-02 |
| P4: triage JSON schema drift | Medium | Medium | Canonical `triageResultJSON` struct for both read and write | T-03 |
| P5: unmarshal errors on old records | Low | Low | Wrap in `if is.TriageResult != ""` + warn+continue | T-03 |
| P7: optimistic concurrency on apply | Medium | Medium | `expectedUpdatedAt` in `updateBacklogItem`; error banner + retry | T-08 |
| P8: item title lookup failure | Low | Low | Fallback string `"Item " + itemID` | T-04 |
| P9: `NewCore` signature breakage | Low | High | Audit all 3 callers before changing signature | T-04 |
| P11: vagueness prompt vs. auto-triage timing | **Mitigated** | Medium | Vague items use `skip_triage=true`; explicit `triggerTriage` on "proceed" | T-06 |
| P12: failed triage shows completed panel | Medium | Medium | `triageResult?.summary` check before `"completed"` status | T-05 |
| P14: proto field number collision | Low | High | Verified: field 9 free on `CreateBacklogItemRequest`, field 12 free on `ItemSession` | T-01 |

---

## Execution Order (recommended)

Phase A (can start immediately):
1. **T-01** — Proto + regen. Blocks everything else. Must be committed and verified before proceeding.

Phase B (after T-01; run in parallel across sessions):
2. **T-02** — Auto-trigger backend
3. **T-03** — Triage result mapping + R7-lite prompt
4. **T-04** — EventBus threading + notification
5. **T-05** — Frontend hook + types (requires generated `.ts` bindings from T-01)

Phase C (sequential; start after T-05):
6. **T-06** — Vagueness modal
7. **T-07** — `TriageDiffSection` + `TriageErrorBanner` sub-components
8. **T-08** — `TriageReviewPanel` + `BacklogItemDetail` integration
9. **T-09** — Undo toast

Phase D (after T-04 + T-08):
10. **T-10** — Notification deep-link click handler

Phase E (after all feature tasks):
11. **T-11** — Backend tests
12. **T-12** — Frontend tests

Run `make quick-check` after each phase. Run `make ci` before opening PR.

---

## Definition of Done

- [ ] `make ci` passes (build + test + lint)
- [ ] `cd web-app && npx jest --no-coverage` passes
- [ ] Creating an item with `repoPath` set triggers triage without clicking a button
- [ ] Creating an item with short description and 0 AC shows `VaguenessPromptModal`
- [ ] Triage suggestions visible in `BacklogItemDetail` after triage session completes
- [ ] "Apply suggestions" replaces AC and transitions item to `ready` in one action
- [ ] Undo toast appears after apply; clicking "Undo" reverts AC + status
- [ ] Notification appears in notification panel when triage completes
- [ ] Notification click navigates to `/backlog?item=<id>`
- [ ] `TriageReviewPanel` does not show when `item.status !== "idea"`
- [ ] `TriageReviewPanel` does not show when dismissed (localStorage flag set)
- [ ] All existing triage tests pass unchanged
