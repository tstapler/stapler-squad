# Implementation Plan: Backlog Compose — Background Creation + Image Fix

## Overview

Two parallel tracks:

**Track A — Image fix (bugfix, self-contained)**: Fix the LLM system prompt and
add a client-side fallback in `BacklogItemIntentReview` so image markdown
references survive the parse step.

**Track B — Background creation (UX change)**: Replace the blocking
intent-review step in the compose widget with immediate creation via
`CreateBacklogItemFromChat`. Add a notification hook that watches triage
completion and pending guidance requests for compose-created items.

The tracks share one proto/backend change (Track B needs `has_pending_guidance_requests`
on `BacklogItem`). Otherwise they are independent and can be reviewed separately.

---

## Track A — Image Fix

### A-1: Update LLM system prompt

**File**: `session/backlog_intent.go`
**Change**: Add a rule to `backlogIntentSystemPrompt`:

```
7. If the input contains any Markdown image references of the form
   `![alt-text](url)`, include ALL of them verbatim, in the same form,
   somewhere in the description field. Never rewrite, shorten, or omit them.
```

This is a stable prompt change (prefix-caching friendly — appended at the end
of the numbered rules list, same style as existing rules).

**Test**: Add a test case in `session/backlog_intent_test.go` that verifies
the image rule is present in the system prompt (string match). No LLM call
needed — the prompt is a pure function.

### A-2: Client-side fallback in draftToInitialValues

**File**: `web-app/src/components/backlog/BacklogItemIntentReview.tsx`
**Change**: In `draftToInitialValues`, after populating `description` from
`draft.description`, extract all `![…](…)` patterns from `rawText` and append
any that are not already substring-present in the draft description.

```ts
function extractImageMarkdown(text: string): string[] {
  return Array.from(text.matchAll(/!\[([^\]]*)\]\(([^)]+)\)/g), (m) => m[0]);
}

function draftToInitialValues(
  draft: ParsedBacklogItemDraft | null,
  rawText: string,
  suggestedRepoPath: string
): Partial<BacklogItem> {
  if (!draft) {
    return { title: fallbackTitle(rawText), description: rawText };
  }
  let description = draft.description;
  const images = extractImageMarkdown(rawText);
  const missing = images.filter((img) => !description.includes(img));
  if (missing.length > 0) {
    description = description.trimEnd() + "\n\n" + missing.join("\n");
  }
  return {
    title: draft.title,
    description,
    ...(suggestedRepoPath ? { repoPath: suggestedRepoPath } : {}),
    acCriteria: draft.acceptanceCriteria.map((text, index) => ({
      index,
      text,
      status: "pending",
    })),
  };
}
```

**Test**: Unit test in `BacklogItemIntentReview.test.tsx` (or new
`BacklogItemIntentReview.imagePreservation.test.tsx`) covering:
- Image in rawText, absent from draft description → appended
- Image in rawText, already in draft description → not duplicated
- No images → no change
- `draft = null` → rawText used verbatim (images present)

---

## Track B — Background Creation

### B-1: Proto — add `has_pending_guidance_requests` to BacklogItem

**File**: `proto/session/v1/backlog.proto`
**Change**: Add to the `BacklogItem` message:
```protobuf
// True when at least one GuidanceRequest for this item has status PENDING.
// Set server-side by backlogItemToProto; the WatchBacklogItems stream carries
// updates whenever this transitions, enabling the compose-creation notifier.
bool has_pending_guidance_requests = <next_field_number>;
```

Find the current max field number via `grep "= [0-9]" proto/session/v1/backlog.proto`.

Run `make build` (triggers `ent-gen` and `proto-gen`).

### B-2: Backend — populate `has_pending_guidance_requests`

**File**: `server/services/backlog_service.go` (locate `backlogItemToProto`)
**Change**: Call `s.storage.CountPendingGuidanceRequests(ctx, item.ID)` (or
the existing guidance-request query) and set the new field if count > 0.

If the count query doesn't exist, add it:
**File**: `session/storage_guidance.go` (or where guidance queries live)
```go
func (s *Storage) HasPendingGuidanceRequests(ctx context.Context, itemID string) (bool, error) {
    count, err := s.db.GuidanceRequest.Query().
        Where(guidancerequest.ItemID(itemID), guidancerequest.StatusEQ(guidancerequest.StatusPending)).
        Count(ctx)
    return count > 0, err
}
```

**Test**: Add a unit test to `server/services/backlog_service_test.go` or the
guidance service test file verifying the field is set when guidance requests
exist for the item.

### B-3: Frontend type — map `has_pending_guidance_requests`

**File**: `web-app/src/lib/hooks/useBacklogService.ts`
**Change**: Add `hasPendingGuidanceRequests?: boolean` to `BacklogItem`
interface; set it from the proto field in `mapBacklogItem`.

### B-4: Notification hook — `useBacklogCreationNotifier`

**File**: `web-app/src/lib/hooks/useBacklogCreationNotifier.ts` (new file)

```ts
// Tracks items created via the compose widget in this session and fires
// in-app notifications when triage completes or guidance is needed.
export function useBacklogCreationNotifier(): {
  trackItem: (id: string, title: string) => void;
} { ... }
```

Implementation:
- Maintain a `Map<string, { title: string; notifiedGuidance: boolean; notifiedComplete: boolean }>`
  in a `useRef` (survives re-renders, no prop drilling needed)
- Subscribe to `useWatchBacklogItems`'s `items` array
- On every items update, for each tracked item:
  - If `item.triageStatus === "completed"` and `!notifiedComplete`:
    - `addNotification({ title: "Triage complete", body: `"${title}" is ready for review`, action: { label: "View", onClick: () => router.push(`/backlog?item=${id}`) } })`
    - Mark `notifiedComplete = true`
  - If `item.triageStatus === "failed"` and `!notifiedComplete`:
    - `addNotification({ title: "Triage failed", body: `"${title}" — open item for details`, ... })`
    - Mark `notifiedComplete = true`
  - If `item.hasPendingGuidanceRequests && !notifiedGuidance`:
    - `addNotification({ title: "AI needs your input", body: `"${title}" has a question for you`, action: { label: "Answer", onClick: () => router.push(`/backlog?item=${id}`) } })`
    - Mark `notifiedGuidance = true`; set to false when guidance clears so
      repeat requests re-notify

**Test**: Unit test in `useBacklogCreationNotifier.test.ts` covering the three
notification triggers.

### B-5: Omnibar — wire compose widget to background creation

**File**: `web-app/src/components/sessions/Omnibar.tsx`
**Change**: Replace the compose-widget `onSubmit` handler:

Before:
```tsx
onSubmit={(text) => {
    setBacklogComposeText(null);
    setBacklogReviewText(text);
}}
```

After:
```tsx
onSubmit={async (text) => {
    setBacklogComposeText(null);
    onClose();
    const result = await createBacklogItemFromChat(text);
    if (result?.item) {
        trackItem(result.item.id, result.item.title);
        addNotification({
            title: "Backlog item created",
            body: `"${result.item.title}" — triage starting`,
        });
    }
}}
```

Where `trackItem` comes from `useBacklogCreationNotifier`, destructured at
component top; `createBacklogItemFromChat` is already in the
`useBacklogService` destructure via `BacklogItemIntentReview`'s dependency.

Also add `const { createBacklogItemFromChat } = useBacklogService()` and
`const { trackItem } = useBacklogCreationNotifier()` to the Omnibar component.

**Test**: Add a test in `Omnibar.test.tsx` (or
`Omnibar.backlogCompose.test.tsx`) verifying:
- Compose submit calls `createBacklogItemFromChat` with the typed text
- Dialog closes immediately (does not transition to intent review)
- `addNotification` is called once creation resolves
- `trackItem` is called with the new item's id and title

---

## Sequencing

1. **A-1 + A-2** (image fix): standalone, no proto change; reviewable independently.
2. **B-1** (proto): prerequisite for B-2, B-3, B-4; needs `make build`.
3. **B-2 + B-3** (backend + type mapping): backend-only, testable in Go.
4. **B-4** (notification hook): pure frontend, depends on B-3's type.
5. **B-5** (Omnibar wiring): depends on B-4.

A-1 and A-2 can ship before Track B; they fix a bug independently.

---

## Adversarial Review

### Concern 1: `createBacklogItemFromChat` is async — what if it fails?

The compose widget's `onSubmit` handler must be async. If creation fails, the
dialog is already closed. We need to show an error notification. Plan: if
`result === null` (already the contract for failures in the hook), call
`addNotification({ title: "Failed to create backlog item", body: "Try again from the backlog page", type: "error" })`.

### Concern 2: Double-notification for guidance (guidance answered → re-triaged → guidance again)

When guidance is answered, triage re-runs and may generate a new guidance
request. The `notifiedGuidance` flag is reset when `hasPendingGuidanceRequests`
goes `false` (guidance answered), so the next cycle produces a new notification.
This is the correct behavior.

### Concern 3: Notification fires for items created in other tabs/browser windows

The tracking set is in-memory, local to the component instance. Items created
in another tab will NOT trigger notifications in this tab. This matches F-7's
scope constraint.

### Concern 4: WatchBacklogItems items map vs. tracked IDs

`useWatchBacklogItems` returns ALL items matching its filter. The notifier must
iterate only its tracked set (not all items). The implementation stores the
tracked set in a `Map` keyed by item id and does a targeted lookup per tracked
id rather than scanning all items.

### Concern 5: `has_pending_guidance_requests` freshness

`backlogItemToProto` is called on every `GetBacklogItem`, `ListBacklogItems`,
and (via `WatchBacklogItems`) on every item-update event. The field is
recomputed from the live DB at each call — no stale cache. The only latency is
the event bus → stream delivery delay, which is already present for all other
fields on `BacklogItem`.
