# Pitfalls: Review Queue Auto-Advance False-Trigger Fix

## The Bug (Recap)

`ReviewQueuePanel` calls `onItemsChange(items)` where `items` is the **filtered, snapshot-stabilized** visible list — it excludes sessions in `WorkingState.ACTIVE` or `WorkingState.PROCESSING`. The page's auto-advance effect watches `reviewQueueItems` (set from `onItemsChange`), so when a session transitions to ACTIVE/PROCESSING it drops out of `items`, causing the page to think it was "deleted externally" and fire `handleAutoAdvance`.

---

## Pitfall 1 — Using `allItems` for the "deleted externally" guard DOES preserve the intended behavior

The intended trigger is: session removed from the Redux store entirely (e.g., `removeItem` dispatch from `acknowledgeSession` or a `WatchReviewQueue itemRemoved` event). In both cases, the session disappears from `allItems` (the raw Redux slice items via `selectReviewQueueItemsWithLiveStatus`) because `removeItem` operates on `state.reviewQueue.items`. A status transition to ACTIVE/PROCESSING does NOT remove the item from the Redux store — it stays in `allItems` with an updated `workingState`. Therefore: if we check `allItems.some(i => i.sessionId === selectedSession.id)` instead of `reviewQueueItems.some(...)`, a genuine external delete (removeItem dispatch) still triggers auto-advance correctly, while a status change to ACTIVE/PROCESSING does not.

**However**, note a secondary path: `acknowledgeSession` calls `dispatch(removeItem(sessionId))` optimistically BEFORE the network call completes. This means the optimistic removal from the Redux store causes `allItems` to lose the session, which would trigger the "deleted externally" effect at the same time as the explicit `handleDismissFromQueue` → `handleAutoAdvance(current.id, true)` path, resulting in two `handleAutoAdvance` calls 300ms apart. This double-advance is already a latent bug even with `reviewQueueItems`, and switching to `allItems` does not change it — the explicit `handleDismissFromQueue` path removes the session from `allItems` first via the optimistic dispatch, so the effect would fire too. The guard in `handleAutoAdvance` (`if (currentIdx !== -1)`) handles graceful landing, but the second call could navigate away from wherever the first advance landed.

---

## Pitfall 2 — Ordering divergence between `allItems` and `reviewQueueItems`

`allItems` comes from `selectReviewQueueItemsWithLiveStatus`, which returns items in the Redux store insertion order (server order from WatchReviewQueue). `reviewQueueItems` is derived from `queueItems` (which comes from `onItemsChange(items)`) — and `items` in `ReviewQueuePanel` is further stabilized by the **snapshot-on-enter pattern**: new items arriving after the snapshot are excluded from `items` until the user clicks "Refresh". This means `allItems` can contain sessions not in `reviewQueueItems` (new arrivals in the snapshot window) and can have a different ordering (filter + snapshot exclusions change the relative positions).

The `handleNextSession` / `handlePreviousSession` / `handleAutoAdvance` all use `reviewQueueItems` for index-based navigation. If the fix uses `allItems` only for the "still in queue?" guard but continues using `reviewQueueItems` for finding the next item, ordering is consistent. If the fix naively replaces all uses of `reviewQueueItems` with `allItems`, navigation would jump to server-ordered positions and bypass the snapshot stabilization, causing the queue to jump when new items arrived during triage.

---

## Pitfall 3 — No existing tests cover the "deleted externally" auto-advance behavior

Searching `web-app/src/components/sessions/__tests__/` and `web-app/src/app/review-queue/` reveals zero page-level tests for `ReviewQueueContent` (Next.js `"use client"` pages are not unit-tested here). `ReviewQueuePanel.test.tsx` covers panel rendering, PR/Rule modal behavior, and empty-state — but has no tests for the `onItemsChange` callback triggering auto-advance, nor for the status-transition filtering behavior. The auto-advance effect (lines 236–243 in `page.tsx`) is completely untested. Any fix should add a new test file (e.g., `web-app/src/app/review-queue/ReviewQueueContent.auto-advance.test.tsx`) covering:
  - Status transition to ACTIVE does NOT trigger auto-advance.
  - `removeItem` dispatch (genuine delete) DOES trigger auto-advance.
  - The ordering of the next-session selection is unaffected by snapshot filtering.

---

## Additional Risk: `selectReviewQueueItemsWithLiveStatus` overrides `workingState` via `detectedStatusMap`

The selector used by `allItems` joins items with live `detectedStatus` from the sessions WebSocket stream and calls `deriveWorkingState` to compute `workingState`. This means a session that transitions to ACTIVE via the live stream updates its `workingState` in `allItems` (not just in `items`). So checking `allItems.some(i => i.sessionId === ...)` correctly says "session is still in the queue" even when ACTIVE — the item object is still there, just with a different `workingState`. This confirms Pitfall 1's conclusion that `allItems` is the right source for the "deleted externally" guard.
