# Stack Research: Review Queue Auto-Advance Bug Fix

## Bug Summary

In `web-app/src/app/review-queue/page.tsx` (lines 236-243), a `useEffect` watches
`reviewQueueItems` (a `Session[]` array derived from filtered, visible queue items) to detect when
the currently selected session has been "deleted externally." The problem: when a session transitions
to `WorkingState.ACTIVE` or `WorkingState.PROCESSING`, `ReviewQueuePanel` filters it out of the
visible queue and calls `onItemsChange` with the smaller list. This updates `reviewQueueItems` in
`page.tsx`, which makes the effect believe the session was deleted, triggering an incorrect
auto-advance.

The fix: check against `allItems` from `useReviewQueueContext().items` (unfiltered Redux store
items, type `ReviewItem[]`) instead of `reviewQueueItems` (filtered `Session[]`).

---

## 1. React Hooks Patterns in Use

### Hooks used in `page.tsx`

| Hook | Purpose |
|---|---|
| `useState` | `selectedSession`, `reviewQueueItems`, `queueItems`, `autoAdvance`, `isSessionFullscreen`, etc. |
| `useRef` | `reviewQueueItemsRef`, `selectedSessionRef`, `autoAdvanceRef`, `modalContentRef` — all used to avoid stale closure issues inside `setTimeout` / `useCallback` |
| `useCallback` | `handleAutoAdvance`, `handleAcknowledged`, `handleDismissFromQueue`, `handleItemsChange`, `handleRunOneShot` |
| `useEffect` | 5+ effects: deep-link URL sync, ref sync, `queueItems`→`reviewQueueItems` derivation, auto-advance on deletion, `autoAdvanceRef` sync |
| `Suspense` | Wraps `ReviewQueueContent` for `useSearchParams()` |

### Key stale-closure pattern

`handleAutoAdvance` reads `reviewQueueItemsRef.current` and `selectedSessionRef.current` rather
than `reviewQueueItems` and `selectedSession` directly. Both refs are kept in sync via their own
`useEffect` hooks (lines 78-79). This means `handleAutoAdvance` always sees the latest values even
though it only captures stable refs in its dependency list.

---

## 2. Redux Store: Selectors and `selectReviewQueueItemsWithLiveStatus`

### Store structure (`reviewQueueSlice.ts`)

The Redux slice (`web-app/src/lib/store/reviewQueueSlice.ts`) stores:
```
state.reviewQueue.reviewQueue: ReviewQueue | null
  └── .items: ReviewItem[]          // unfiltered; source of truth
state.reviewQueue.loading: boolean
state.reviewQueue.error: string | null
state.reviewQueue.stats: ReviewQueueStats
```

### Critical selector: `selectReviewQueueItemsWithLiveStatus`

```ts
export const selectReviewQueueItemsWithLiveStatus = createSelector(
  [
    (state: RootState) => state.reviewQueue.reviewQueue?.items ?? [],
    (state: RootState) => state.sessions.detectedStatusMap,
  ],
  (items, detectedStatusMap) =>
    items.map((item) => {
      const liveStatus = detectedStatusMap[item.sessionId];
      if (!liveStatus) return item;
      const liveWorkingState = deriveWorkingState({
        subStatus: item.subStatus,
        detectedStatus: liveStatus.detectedStatus,
      });
      return { ...item, workingState: liveWorkingState };
    })
);
```

This selector joins raw `ReviewItem[]` from the store with live `detectedStatus` from the
`sessionsSlice`. It **does NOT filter** by `workingState` — it only overrides the `workingState`
field using live terminal-scraped status. All items remain in the returned array regardless of
whether they are ACTIVE, PROCESSING, or WAITING.

### Other relevant selectors

- `selectWaitingItems`: filters out ACTIVE/PROCESSING items — NOT used in the context hook path
- `selectReviewQueue`: returns the raw `ReviewQueue` object
- `removeItem`: reducer that removes an item by `sessionId` (called optimistically on acknowledge)
- `addItem`, `updateItem`: called from WebSocket `ReviewQueueEvent` stream events

---

## 3. How `useReviewQueueContext` Exposes `items`

### Chain of data flow

1. `useReviewQueue()` hook (`web-app/src/lib/hooks/useReviewQueue.ts`):
   - Subscribes to Redux via `useAppSelector(selectReviewQueueItemsWithLiveStatus)` → stored as `liveItems`
   - Returns `{ items: liveItems, ... }` where `items` is the **unfiltered** `ReviewItem[]` with live status overlaid

2. `ReviewQueueProvider` (`web-app/src/lib/contexts/ReviewQueueContext.tsx`):
   - Calls `useReviewQueue({ baseUrl, useWebSocketPush: true, autoRefresh: true })`
   - Exposes the return value as `ReviewQueueContextValue`
   - Type: `ReturnType<typeof useReviewQueue>` — structural, not nominal

3. `useReviewQueueContext()`:
   - Returns the full context value including `items: ReviewItem[]`
   - `items` field type: `ReviewItem[]` (protobuf type from `@/gen/session/v1/types_pb`)
   - This is **unfiltered** — contains all queue items regardless of `workingState`

---

## 4. `reviewQueueItems` vs `allItems` from Context

### `allItems` from context (`useReviewQueueContext().items`)

- Type: `ReviewItem[]`
- Source: `selectReviewQueueItemsWithLiveStatus` Redux selector
- Contains: **all** items in the review queue, with live `workingState` overlaid
- Filtering: **none** — ACTIVE, PROCESSING, and WAITING items all present
- Key field for membership check: `item.sessionId` (string)

### `reviewQueueItems` in `page.tsx`

- Type: `Session[]`
- Source: derived state computed by a `useEffect` (lines 164-169):
  ```ts
  useEffect(() => {
    const queueSessions = queueItems.map(
      (item) => sessions.find((s) => s.id === item.sessionId) ?? sessionFromReviewItem(item)
    );
    setReviewQueueItems(queueSessions);
  }, [queueItems, sessions]);
  ```
- `queueItems` (`ReviewItem[]`) comes from `handleItemsChange`, which is the `onItemsChange`
  callback passed to `ReviewQueuePanel`
- `ReviewQueuePanel` calls `onItemsChange(items)` where `items` is the **already-filtered**
  list (ACTIVE/PROCESSING sessions excluded by `allFilteredItems` in the panel)
- So `reviewQueueItems` only contains sessions that are **visible** in the queue panel — NOT
  sessions that are currently ACTIVE or PROCESSING

### The bug

When a session transitions to ACTIVE:
1. `selectReviewQueueItemsWithLiveStatus` updates `workingState` to ACTIVE for that item
2. `ReviewQueuePanel.allFilteredItems` filters it out (line 200: `ws !== WorkingState.ACTIVE`)
3. `ReviewQueuePanel` calls `onItemsChange(items)` with the session removed
4. `page.tsx` `handleItemsChange` stores the filtered list → `queueItems`
5. The `useEffect` recomputes `reviewQueueItems` without that session
6. The buggy `useEffect` (lines 236-243) fires: `reviewQueueItems.some(s => s.id === selectedSession.id)` returns `false`
7. Auto-advance fires — incorrectly, because the session is still in the queue (just working)

### The fix

Replace `reviewQueueItems.some(...)` with a check against `useReviewQueueContext().items`:
```ts
const { items: allItems } = useReviewQueueContext();
// ...
useEffect(() => {
  if (!selectedSession) return;
  const stillInQueue = allItems.some((item) => item.sessionId === selectedSession.id);
  if (!stillInQueue) {
    handleAutoAdvance(selectedSession.id, true);
  }
}, [allItems]);
```

Note the field name difference: `ReviewItem.sessionId` vs `Session.id` — both are the same
underlying session ID but accessed via different field names on different types.

---

## 5. TypeScript Types

### `ReviewItem` (from proto `@/gen/session/v1/types_pb`)

Relevant fields:
- `sessionId: string` — the session ID (matches `Session.id`)
- `sessionName: string`
- `workingState: WorkingState` — enum (ACTIVE, PROCESSING, WAITING, UNSPECIFIED)
- `subStatus: string`
- `status: SessionStatus`
- `priority: Priority`
- `reason: AttentionReason`
- `path: string`, `workingDir: string`, `branch: string`, `program: string`, `tags: string[]`

### `Session` (from proto `@/gen/session/v1/types_pb`)

Relevant fields:
- `id: string` — the session ID (matches `ReviewItem.sessionId`)
- `title: string`, `status: SessionStatus`, etc.

### Key type difference for the fix

The buggy effect uses `Session.id` (from `reviewQueueItems: Session[]`):
```ts
reviewQueueItems.some((s) => s.id === selectedSession.id)
```

The fix uses `ReviewItem.sessionId` (from `allItems: ReviewItem[]`):
```ts
allItems.some((item) => item.sessionId === selectedSession.id)
```

Both refer to the same underlying session identifier; only the field name differs.

---

## 6. Effect Dependency Considerations

The buggy effect has an eslint-disable comment suppressing `react-hooks/exhaustive-deps`:
```ts
// eslint-disable-next-line react-hooks/exhaustive-deps
}, [reviewQueueItems]);
```

This was likely done to avoid adding `handleAutoAdvance` to deps (which would require `router` and
cause unnecessary re-runs). The fix should maintain the same pattern: watch `allItems` from context
and keep `handleAutoAdvance` out of deps (it reads stable refs internally). The eslint-disable
comment is still appropriate for the same reason.

### Fixed effect signature

```ts
const { items: allItems, acknowledgeSession } = useReviewQueueContext();
// ...
useEffect(() => {
  if (!selectedSession) return;
  const stillInQueue = allItems.some((item) => item.sessionId === selectedSession.id);
  if (!stillInQueue) {
    handleAutoAdvance(selectedSession.id, true);
  }
// eslint-disable-next-line react-hooks/exhaustive-deps
}, [allItems]);
```

`selectedSession` does not need to be in deps because the effect is intended to fire when
`allItems` changes, and at that point `selectedSession` is always the value at render time (the
effect will re-run whenever `allItems` changes, which happens immediately after any queue mutation).
