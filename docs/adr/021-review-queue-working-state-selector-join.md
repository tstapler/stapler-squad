# ADR-021: Review Queue Working-State Derivation — Selector Join vs. Dual-Source

**Status**: Accepted
**Date**: 2026-06-20

## Context

`useReviewQueue.ts` subscribes to the `WatchReviewQueue` ConnectRPC server stream. When a `ReviewQueueItemAddedEvent`, `ReviewQueueItemRemovedEvent`, or `ReviewQueueItemUpdatedEvent` arrives, the current implementation discards the delta payload and instead calls `refreshRef.current()` — a full `GetReviewQueue` REST re-fetch. This "Phase 1 shortcut" was introduced as a temporary workaround to avoid delta application complexity, but it has two consequences:

1. **Latency:** Every queue change incurs a REST round-trip (100–300ms) on top of the streaming event. Under batch operations or rapid approval cycles, the queue panel visibly lags the session card.
2. **Dual-source divergence (gap G5):** `reviewQueueSlice` items carry their own `workingState` / `subStatus` copy, populated from the last `GetReviewQueue` REST snapshot. The live `detectedStatusMap` in `sessionsSlice` is updated continuously by `WatchSessions` `SessionUpdatedEvent`s. After an approval fires, `sessionsSlice` clears the `detectedStatus` within one event cycle, but `reviewQueueSlice` retains the stale "Needs Approval" value until the next REST re-fetch completes. This means the session card and the review queue panel show different states for the same session for up to one full poll cycle (up to 500ms in practice, potentially longer under load).

Two approaches were evaluated:

**Option A — Selector join (derive working state from `sessionsSlice`):**
Remove the Phase 1 REST re-fetch shortcut. Apply `itemAdded`, `itemRemoved`, and `itemUpdated` delta events directly to `reviewQueueSlice` via new `addItem`, `updateItem`, and `removeItem` (already exists) reducers. Add a memoized `createSelector` (`selectReviewQueueItemsWithLiveStatus`) that joins `reviewQueueSlice.items` with `sessionsSlice.entities` on `sessionId`, overriding the queue item's stale `workingState` with the live `detectedStatus` from the session entity. `ReviewQueuePanel.tsx` switches from reading `reviewQueueSlice.items` directly to reading the joined selector output.

**Option B — Backend sends `ItemUpdatedEvent` when `DetectedStatus` changes:**
Wire `ReactiveQueueManager.OnControllerStatusChange` to emit a `ReviewQueueItemUpdatedEvent` whenever a session's detected status changes. The frontend applies the delta event directly without needing a cross-slice join. This keeps slices fully independent but requires backend wiring that is more complex and has higher risk of introducing new ordering or race conditions in `ReactiveQueueManager`.

## Decision

Remove the Phase 1 REST re-fetch shortcut from `useReviewQueue` and derive working state via a memoized selector join (Option A, Epic 4 Stories 4.1, 4.2, 4.4).

Specific changes:

1. **`reviewQueueSlice.ts`** — add `addItem` and `updateItem` reducers. Fix `removeItem` to derive `totalItems` from `items.length` after the filter rather than decrementing a separate counter (idempotent double-remove safety, Story 2.2).

2. **`useReviewQueue.ts`** — replace `refreshRef.current()` inside stream event handlers with direct Redux dispatches (`addItem`, `removeItem`, `updateItem`). Retain the 30-second `GetReviewQueue` polling fallback as a recovery mechanism (full state replacement, not per-item delta), and retain the `initialSnapshot: true` flag on (re)connect so the snapshot path re-hydrates fresh state after a disconnect.

3. **`reviewQueueSlice.ts` (or `reviewQueueSelectors.ts`)** — add `selectReviewQueueItemsWithLiveStatus`:
   ```ts
   export const selectReviewQueueItemsWithLiveStatus = createSelector(
     [(state: RootState) => state.reviewQueue.reviewQueue?.items ?? [], selectAllSessionEntities],
     (items, sessionEntities) =>
       items.map(item => ({
         ...item,
         workingState: sessionEntities[item.sessionId]?.detectedStatus ?? item.workingState,
         subStatus:    sessionEntities[item.sessionId]?.subStatus    ?? item.subStatus,
       }))
   );
   ```

4. **`ReviewQueuePanel.tsx`** — switch the items selector to `selectReviewQueueItemsWithLiveStatus`.

The 30-second fallback poll and the `WatchReviewQueue` reconnect loop with exponential backoff (Story 4.3) together ensure that any missed deltas during a brief disconnect are healed by the next snapshot without user-visible stale state.

## Consequences

### Positive
- Review queue items and session cards show the same `workingState` at all times. Divergence is structurally impossible while both slices are populated and `sessionsSlice` is live.
- Eliminates the REST round-trip on every stream event. Queue panel updates are as fast as `sessionsSlice` updates (one event cycle, typically <100ms from server event to re-render).
- `removeItem` becomes idempotent: double-remove from concurrent `WatchSessions` session-deleted and `WatchReviewQueue` item-removed events no longer corrupts the badge count.
- `createSelector` memoizes on reference equality — the selector recomputes only when `reviewQueue.items` or `sessionsSlice.entities` changes, not on every render.
- No backend changes required.

### Negative / Risks
- Review queue items no longer carry an authoritative `workingState` copy. If the session entity is absent from `sessionsSlice` (e.g., on initial load before the first `WatchSessions` snapshot arrives), the selector falls back to the queue item's own value, which is populated from the `WatchReviewQueue` initial snapshot. This is correct behavior but requires the fallback to be tested.
- If the `WatchSessions` stream is disconnected (network error), `sessionsSlice` goes stale. The joined selector then shows stale working state for both the session card and the review queue panel simultaneously — they go stale together rather than diverging. This is a more honest failure mode than the previous divergence.
- The cross-slice join introduces a coupling between `reviewQueueSelectors` and `sessionsSlice`. Future renaming of `sessionsSlice` entities requires updating both selectors.
- The Phase 1 shortcut removal is a behavioral change: any code that relied on the REST re-fetch as an implicit cache invalidation must be audited to ensure it uses the polling fallback or stream events instead.

### Residual Safety Net
The 30-second `GetReviewQueue` polling fallback is retained. Its sole purpose is resilience: if a delta is missed during a disconnect window that precedes the reconnect snapshot, the poll will eventually reconcile state. The poll calls `setReviewQueue(data)` (full state replacement), not per-item dispatches, so it cannot re-introduce the double-decrement bug.
