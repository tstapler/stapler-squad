# Architecture Research: Review Queue Auto-Advance False Trigger

## Data Flow Overview

The bug path:
1. `ReviewQueuePanel` consumes `allItems` from `useReviewQueueContext().items` (`selectReviewQueueItemsWithLiveStatus` — raw Redux queue items overlaid with live session working states)
2. `ReviewQueuePanel` filters out ACTIVE/PROCESSING sessions in `allFilteredItems` (line 197-209), then applies snapshot filtering to produce `items` (the visible list)
3. `ReviewQueuePanel` calls `onItemsChange(items)` whenever `items` changes — this is what page.tsx captures via `handleItemsChange` → `setQueueItems`
4. `page.tsx` recomputes `reviewQueueItems` from `queueItems` (line 164-169)
5. The "deleted externally" effect (line 236-243) watches `reviewQueueItems` and fires auto-advance when `selectedSession` is not in that list

**The bug**: When a session transitions to ACTIVE/PROCESSING, `selectReviewQueueItemsWithLiveStatus` reflects the new working state immediately (live status from `sessionsSlice.detectedStatusMap`). `allFilteredItems` in ReviewQueuePanel drops it. The session disappears from the visible `items` list → `onItemsChange` fires → `queueItems`/`reviewQueueItems` update → auto-advance fires. The session is still in `state.reviewQueue.reviewQueue.items` (Redux raw store) — it was never removed.

---

## Option A: Guard the effect using `allItems` from `useReviewQueueContext()`

**What changes**: In `page.tsx`, call `useReviewQueueContext()` and derive a `selectedSessionInAllItems` boolean from its `items` (which is `selectReviewQueueItemsWithLiveStatus` — the raw queue including ACTIVE/PROCESSING sessions). Replace the auto-advance guard from `reviewQueueItems.some(...)` to `allItems.some(...)`.

```tsx
const { items: allQueueItems } = useReviewQueueContext();
// ...
useEffect(() => {
  if (!selectedSession) return;
  const stillInQueue = allQueueItems.some((s) => s.sessionId === selectedSession.id);
  if (!stillInQueue) {
    handleAutoAdvance(selectedSession.id, true);
  }
}, [reviewQueueItems]); // still fire on visible-list changes, but guard against allItems
```

**Pros**:
- Minimal change: single additional selector read, one extra `.some()` check
- Semantically correct: "deleted externally" should only fire when the session is actually removed from the Redux queue (`removeItem` dispatch), not when it's merely filtered out of the visible list
- `allQueueItems` includes ACTIVE/PROCESSING sessions — this is the right ground truth for "is this session still in the queue at all?"
- No dependency on Redux internals or action streams

**Cons**:
- `allQueueItems` from `selectReviewQueueItemsWithLiveStatus` still overlays live working states, so the item returned may have a different `workingState` than what the Redux store's raw `reviewQueue.items` has. For membership checking (`.some(s => s.sessionId === id)`), this is irrelevant — the items array is unchanged.
- Minor: adds a second context consumer in `page.tsx` (but `acknowledgeSession` is already consumed from `useReviewQueueContext()` at line 67, so the hook is already called — just destructure `items` from the existing call)

**Verdict**: This is the cleanest fix. `page.tsx` already calls `useReviewQueueContext()` and the `items` array from that hook is exactly the right membership oracle.

---

## Option B: Subscribe to Redux `removeItem` action directly

**What changes**: Use a Redux middleware, `useSelector` on a "last removed ID" field added to the slice, or listen to Redux store actions via a custom middleware/effect to detect actual `removeItem` dispatches.

**Pros**:
- Most precise: fires only on actual queue removal, not on working-state change

**Cons**:
- Requires adding state to `reviewQueueSlice` (a `lastRemovedId` field) or a Redux middleware/listener — significant complexity
- Creates a temporal coupling problem: the effect would need to react to a Redux action firing in the past tick, requiring careful cleanup to avoid re-triggering on re-renders
- `removeItem` is dispatched by both optimistic acknowledge AND the WebSocket `itemRemoved` event — the effect would need to distinguish between "removed because user dismissed" (already handled by `handleDismissFromQueue`/`handleAcknowledged`) and "removed externally". This requires additional state to track which removes were user-initiated, which is what the bug originally conflated.
- Over-engineered for the actual invariant needed: "is this session still in the queue?"

**Verdict**: Avoid. The complexity doesn't buy correctness advantages over Option A.

---

## Option C: Add `selectedSessionInAllItems` boolean flag

Identical in substance to Option A but extracts the membership check into a named local variable:

```tsx
const { items: allQueueItems, acknowledgeSession } = useReviewQueueContext();
// ...
useEffect(() => {
  if (!selectedSession) return;
  const selectedSessionInAllItems = allQueueItems.some((s) => s.sessionId === selectedSession.id);
  if (!selectedSessionInAllItems) {
    handleAutoAdvance(selectedSession.id, true);
  }
}, [reviewQueueItems]);
```

**Verdict**: Same as Option A — slightly cleaner naming but functionally equivalent. Worth using the named variable for readability.

---

## Recommendation: Option A / C

Use `allQueueItems` from the already-consumed `useReviewQueueContext()` call (merge the `items` destructure into the existing line 67 destructure). Check `allQueueItems.some(s => s.sessionId === selectedSession.id)` as the guard in the auto-advance effect.

Key implementation note: the effect's dependency array should remain `[reviewQueueItems]` (not `[allQueueItems]`). The visible list changing is still the right trigger to re-evaluate — but the guard condition switches from "is session in visible list" to "is session in all-items list". This preserves the existing timing: the effect runs when the panel reports a new visible list, but only advances if the session is genuinely gone from the underlying queue.

**Data flow implication**: `reviewQueueItems` continues to drive the visible navigation list and "queue position" badge. `allQueueItems` serves only as membership oracle for the "externally deleted" guard. These are cleanly separated concerns.
