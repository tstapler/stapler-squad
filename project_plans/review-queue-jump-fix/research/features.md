# Review Queue Auto-Advance: Feature Research

## All Auto-Advance Trigger Paths

There are four distinct paths that invoke `handleAutoAdvance` in `web-app/src/app/review-queue/page.tsx`:

### 1. `handleDismissFromQueue` (line 226–231)
User clicks "Dismiss" button inside the session detail modal.
- Calls `acknowledgeSession(current.id)` (optimistic Redux `removeItem` dispatch + RPC)
- Then calls `handleAutoAdvance(current.id, force=true)`
- The `force=true` bypasses the auto-advance user preference toggle

### 2. `onApprovalResolved` callback (line 301)
Rendered as `onApprovalResolved={() => handleAutoAdvance(selectedSession.id)}` on `SessionDetail`.
- Fires when an approval request is resolved from inside the modal (approve/deny)
- Calls `handleAutoAdvance(selectedSession.id, force=false)` — respects user preference

### 3. `handleAcknowledged` (line 218–222)
Called by `ReviewQueuePanel` via `onAcknowledged` prop when the user acknowledges a session
from the queue list (not the modal).
- Only triggers advance if the acknowledged session IS the currently selected session
- Calls `handleAutoAdvance(sessionId, force=false)` — respects user preference

### 4. "Deleted externally" effect (lines 236–243) — THE BUGGY PATH
```tsx
useEffect(() => {
  if (!selectedSession) return;
  const stillInQueue = reviewQueueItems.some((s) => s.id === selectedSession.id);
  if (!stillInQueue) {
    handleAutoAdvance(selectedSession.id, true);
  }
}, [reviewQueueItems]);
```
- Watches `reviewQueueItems`, which is derived from the FILTERED visible list
- `reviewQueueItems` is populated via `handleItemsChange` → `setQueueItems`, which receives
  only items passing the `WorkingState` filter from `ReviewQueuePanel` (excludes ACTIVE/PROCESSING)
- **Bug**: When a session transitions to ACTIVE/PROCESSING, it disappears from the filtered list,
  but the session is NOT gone from the underlying Redux store (`allItems`). The effect sees
  `stillInQueue = false` and fires `handleAutoAdvance(sessionId, true)` incorrectly.
- Uses `force=true`, so it always advances regardless of user preference

---

## handleAutoAdvance Logic

```
handleAutoAdvance(resolvedSessionId?, force=false)
  ↓ runs in setTimeout(300ms)
  ↓ if !force && !autoAdvanceRef.current → return (user disabled)
  ↓ reads reviewQueueItemsRef.current (snapshot of last-known filtered visible items)
  ↓ filters out resolvedSessionId from remainingItems
  ↓ if remainingItems.length === 0:
      → router.push("/review-queue") + setSelectedSession(null)  [empty queue close]
  ↓ if currentSelected not in remainingItems:
      → navigate to item at same position as resolvedSessionId (clamped to list end)
  ↓ if currentSelected in remainingItems:
      → advance to next item (circular)
```

Key detail: `handleAutoAdvance` reads `reviewQueueItemsRef.current` — a ref that mirrors
`reviewQueueItems` (the filtered list). So the bug cascades: both the trigger check and
the advance logic operate on the same filtered list, which excludes ACTIVE/PROCESSING sessions.

---

## Edge Cases

### Empty queue after advance
When `remainingItems.length === 0`, the modal closes and the completion/empty state is shown.
This is correct behavior for all four trigger paths.

### Single-item queue
If there is only one item and it gets resolved, `remainingItems` becomes empty.
The modal closes (same empty-queue path). Correct.

### All sessions become ACTIVE simultaneously
With the bug: all sessions transition to ACTIVE, `reviewQueueItems` becomes `[]`, and for
each render where `selectedSession` is set, `stillInQueue = false` fires `handleAutoAdvance`
with `remainingItems = []`, closing the modal to the empty state. This is the false positive.
With the fix (`allItems` check): `allItems` still contains the sessions (they are in Redux
but filtered out of the visible panel), so `stillInQueue = true` and the effect does NOT fire.

### Session acknowledged while modal is open
`handleAcknowledged` is the correct path for this. The "deleted externally" effect would also
fire, but `handleAutoAdvance` calls inside `setTimeout(300ms)` so there is a race. The `force`
param doesn't change correctness here since both paths call with `force=true` or the
acknowledged path calls with `force=false` (respecting preference).

### Session deleted from another tab
The Redux `removeItem` dispatch fires from the `ReviewQueueEvent.itemRemoved` WebSocket event.
This removes the item from `allItems` (Redux store). After the fix, `allItems` is used for
the existence check, so the "deleted externally" effect correctly fires when the session is
truly gone from the store (not just filtered out of the visible list).

---

## Impact of the Fix

The fix changes the "deleted externally" effect's existence check from `reviewQueueItems` to
`allItems` from `useReviewQueueContext().items` (the unfiltered Redux store items via
`selectReviewQueueItemsWithLiveStatus`).

**`allItems` data path**:
`state.reviewQueue.reviewQueue.items` (Redux) joined with `state.sessions.detectedStatusMap`
→ `selectReviewQueueItemsWithLiveStatus` → `useReviewQueueContext().items` → `allItems` in
`ReviewQueuePanel` → exposed via `useReviewQueueContext()` in the page.

**The fix does NOT affect:**
- `handleDismissFromQueue` — uses `force=true`, bypasses preference; unrelated to existence check
- `handleAcknowledged` — operates on the session being acknowledged, not on queue item existence
- `onApprovalResolved` — fires explicitly, not based on queue membership
- The `handleAutoAdvance` advance logic itself — still uses `reviewQueueItemsRef.current`
  (the visible filtered list) to find the next item to navigate to. This is correct: when
  advancing, you want to navigate to the next VISIBLE (non-ACTIVE) item in the panel.

**Subtle correctness note**: After the fix, if a session is "deleted externally" (truly removed
from Redis/backend), `allItems` shrinks via `removeItem` Redux dispatch, and the effect correctly
fires. If a session merely becomes ACTIVE (filtered out of panel), `allItems` still contains it,
and the effect correctly does NOT fire.

**The dismissed flow is unaffected**: `handleDismissFromQueue` calls `acknowledgeSession` which
dispatches `removeItem` to Redux BEFORE calling `handleAutoAdvance`. So by the time any
reactive effect runs, `allItems` already lacks the dismissed session. The 300ms setTimeout
in `handleAutoAdvance` further ensures this ordering.
