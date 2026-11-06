# Implementation Plan: Backlog Smart Creation

## Executive Summary

Three compounding issues prevent the "Create Backlog Item" flow from working as expected:
(1) `CreateBacklogItemFromChatRequest` has no `repo_path` field, so `MaybeTriggerTriage` always
receives an empty string and always returns `false`; (2) the client notification ignores the
`triageTriggered` flag and always says "triage starting"; (3) no AI enrichment runs at or after
creation. The fix ships in three sequenced tracks — Track A (critical correctness, 1 day), Track
B (background enrichment, 2 days), Track C (brainstorm UX, 2 days) — with Track A independently
releasable and immediately beneficial.

## Track A — Repo Path & Truthful Notifications (P1)

### A-1: Proto — add `repo_path` to `CreateBacklogItemFromChatRequest`

**File**: `proto/session/v1/backlog.proto`

Add field:
```protobuf
message CreateBacklogItemFromChatRequest {
  string message        = 1;
  string existingItemId = 2;
  string repo_path      = 3;  // NEW: empty string means "no repo context"
}
```

Run `make proto-gen` after the change.

**Estimate**: 30 min

---

### A-2: Backend — forward `RepoPath` to `CreateBacklogItem`

**File**: `server/services/backlog_service_chat.go`

In the `CreateBacklogItemFromChat` handler, pass `req.Msg.RepoPath` to `CreateBacklogItem`:

```go
result, err := s.createBacklogItemCore(ctx, session.CreateBacklogItemInput{
    Title:       firstLine(req.Msg.Message),
    Description: req.Msg.Message,
    RepoPath:    req.Msg.RepoPath,  // NEW: wire through
    // ... existing fields
})
```

No other backend changes needed — `CreateBacklogItem` → `MaybeTriggerTriage` already handles
the empty-string case correctly (skips triage).

**Estimate**: 30 min

---

### A-3: Frontend — add `repoPath` to `createBacklogItemFromChat` hook

**File**: `web-app/src/lib/hooks/useBacklogService.ts`

Update `createBacklogItemFromChat` signature:
```ts
createBacklogItemFromChat: (message: string, options?: { existingItemId?: string; repoPath?: string }) 
  => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>
```

Pass `options.repoPath` to the proto call. Remove the "repo_path intentionally omitted" comment
at line 1019 — this plan supersedes that decision.

**Estimate**: 1 hour

---

### A-4: Compose widget — add repo path UI

**File**: `web-app/src/components/backlog/BacklogComposeWidget.tsx`

Add props:
```ts
interface BacklogComposeWidgetProps {
  initialText: string;
  suggestedRepoPaths?: string[];  // NEW: pre-fill candidates, editable
  onSubmit: (text: string, repoPath: string) => void;  // updated signature
  onCancel: (draft: string) => void;
}
```

Add a minimal repo path field below the textarea:
- Combobox with `suggestedRepoPaths` as options (most-recent first)
- Clearable (empty string allowed — "no repo context")
- Label: "Repository (for triage)" with tooltip: "AI triage needs a repo path to run"
- Keyboard-accessible (Tab + Enter selects)

The `onSubmit` signature change is backward-compatible if `repoPath` is included only when
`BacklogComposeWidget` is the caller (audit: only `Omnibar.tsx` renders it today).

**Estimate**: 2 hours

---

### A-5: Omnibar — wire repo paths and fix notification

**File**: `web-app/src/components/sessions/Omnibar.tsx`

1. Import and call `useSessionRepoPaths()` to get `knownRepoPaths: string[]`.
2. Pass `knownRepoPaths` as `suggestedRepoPaths` to `BacklogComposeWidget`.
3. Update `createBacklogItemInBackground` to accept `(text: string, repoPath: string)` and pass
   `repoPath` through to `createBacklogItemFromChat`.
4. Fix the notification:
   ```ts
   if (result.triageTriggered) {
     addNotification({ title: "Backlog item created — triage underway", ... });
   } else {
     addNotification({
       title: "Backlog item created",
       message: repoPath 
         ? "Triage could not start — check the repo path." 
         : "Add a repo path to enable automatic triage.",
       onView: () => router.push(`/backlog?item=${result.item.id}`),
     });
   }
   ```
5. Update `BacklogComposeWidget`'s `onSubmit` callback to receive `(text, repoPath)`.

**Estimate**: 2 hours

---

### A-6: Tests — Track A

**File**: `web-app/src/components/sessions/__tests__/Omnibar.backlogCompose.test.tsx`

Add/update:
- `"passes repoPath to createBacklogItemFromChat when user selects a suggestion"` — mock `useSessionRepoPaths()` returning `["/home/user/my-repo"]`, verify proto call receives `repo_path: "/home/user/my-repo"`.
- `"shows triage-underway notification when triageTriggered===true"` — mock response `{ triageTriggered: true }`.
- `"shows add-repo-path message when triageTriggered===false and repoPath is empty"` — mock response `{ triageTriggered: false }`.

**Estimate**: 1 hour

---

## Track B — Background AI Enrichment (P2)

Background: enrichment runs only when `triageTriggered === false` (to avoid the race where triage
and enrichment both update title/description/ACs concurrently — see pitfalls.md §5).

### B-1: Verify `UpdateBacklogItem` field coverage

Check whether `UpdateBacklogItem` RPC accepts `title`, `description`, and `acceptance_criteria`.
If yes, skip B-2 and proceed to B-3. If not, add B-2.

**Estimate**: 15 min

---

### B-2 (conditional): Add `EnrichBacklogItem` RPC

Only if `UpdateBacklogItem` doesn't support these fields.

Add to proto:
```protobuf
message EnrichBacklogItemRequest {
  string item_id            = 1;
  string title              = 2;
  string description        = 3;
  repeated AcCriterion acs  = 4;
  string repo_path          = 5;
}
message EnrichBacklogItemResponse { BacklogItem item = 1; }
```

Backend: apply fields only if still at their raw/created values (conditional update guard to
avoid overwriting user edits that happened during the enrichment call's latency).

Run `make registry-generate` and add `tested: true` to the registry JSON.

**Estimate**: 2 hours (if needed)

---

### B-3: Background enrichment hook

**File**: New `web-app/src/lib/hooks/useBacklogEnrichment.ts` or inline in `Omnibar.tsx`.

After `createBacklogItemInBackground` completes with `triageTriggered === false`:
```ts
// fire-and-forget
parseBacklogItemIntent(text, knownRepoPaths).then((draft) => {
  if (!draft) return;
  updateBacklogItem(itemId, {
    title: draft.title,
    description: draft.description,
    acCriteria: draft.acceptanceCriteria.map((text, i) => ({ index: i, text, status: "pending" })),
  });
  addNotification({
    title: "Backlog item enriched — review it now",
    onView: () => router.push(`/backlog?item=${itemId}`),
  });
});
```

**Estimate**: 2 hours

---

### B-4: Tests — Track B

- `"calls parseBacklogItemIntent and enriches item when triageTriggered===false"` — verify the update call fires with enriched fields.
- `"skips enrichment when triageTriggered===true"` — verify no `updateBacklogItem` call.
- `"enrichment failure is silent (no error notification)"` — mock parse failure, verify only the creation notification exists.

**Estimate**: 1 hour

---

## Track C — Brainstorm Mode (P3)

**Architectural note**: The brainstorm panel uses `parseBacklogItemIntent` for structured
feedback (ACs, risks), not for open-ended prose chat. Each turn can ask "what are the ACs for
this idea?" or "what risks exist?" — the response's draft fields are used for display.

### C-1: `BrainstormPanel` component

**File**: `web-app/src/components/backlog/BrainstormPanel.tsx`

A collapsible panel below the `BacklogComposeWidget` textarea:
- Toggle button: "✨ Explore with AI" / "Hide AI explore"
- Input box + "Ask" button (or Enter key)
- Each turn calls `parseBacklogItemIntent(userQuestion + currentDraftText, knownRepoPaths)`
- Shows the draft's acceptance_criteria as suggestions: "Suggested ACs:" + bullet list
- Append-only display (previous turns visible for context)
- Does not block the submit button

### C-2: Wire chat context into submission

When the brainstorm panel has been used, append a summary of the last draft suggestion to
the compose text before calling `parseBacklogItemIntent` in Track B. This passes the
brainstorm context to enrichment automatically.

### C-3: Tests — Track C

- Toggle shows/hides the panel.
- Submitting a question calls `parseBacklogItemIntent` with the textarea content appended.
- Panel is scrollable with many turns.

**Estimate**: 4 hours total for C-1 through C-3.

---

## Implementation Sequence

| Order | Track | Deliverable | Estimate |
|---|---|---|---|
| 1 | A-1 | Proto change + `make proto-gen` | 30m |
| 2 | A-2 | Backend handler | 30m |
| 3 | A-3 | TS hook update | 1h |
| 4 | A-4 | Compose widget UI | 2h |
| 5 | A-5 | Omnibar wiring + notification fix | 2h |
| 6 | A-6 | Track A tests | 1h |
| **A total** | | | **~7h** |
| 7 | B-1 | Verify UpdateBacklogItem | 15m |
| 8 | B-2 | (conditional) EnrichBacklogItem RPC | 2h |
| 9 | B-3 | Enrichment hook | 2h |
| 10 | B-4 | Track B tests | 1h |
| **B total** | | | **~5h** |
| 11 | C-1,2,3 | Brainstorm panel | 4h |
| **C total** | | | **4h** |
| **Grand total** | | | **~16h** |

Tracks A, B, and C can be shipped as separate PRs. Track A is independently releasable and
fixes the P1 correctness gap. Track B adds value only after Track A ships. Track C is
independently mergeable (behind a feature flag is optional but advisable for initial rollout).

## Adversarial Review

**Q: Does adding `repo_path` to `CreateBacklogItemFromChatRequest` break existing callers?**
A: No — proto3 defaults new string fields to `""`, which is the existing behavior. All existing
callers that don't send `repo_path` continue to get the same empty-repoPath triage-skip path.

**Q: What if `useSessionRepoPaths()` returns an empty list?**
A: The repo path UI shows an empty/clearable combobox. `repoPath` is sent as `""`, triage is
skipped, and the notification says "Add a repo path to enable triage." No regression from
current behavior.

**Q: Could background enrichment overwrite user edits?**
A: Enrichment fires immediately after creation (~2–90 second latency for the LLM call).
The conditional update guard in B-2 (write only if fields still match created values) prevents
overwriting edits the user makes after opening the item.

**Q: Does the brainstorm panel cost too much per turn?**
A: `parseBacklogItemIntent` has a 90s budget and is the existing AI call. Each brainstorm
turn is one additional call; the user must explicitly click "Ask" (no auto-trigger).
Acceptable cost for a voluntary interaction.

**Verdict: PASS — no plan changes required.**
