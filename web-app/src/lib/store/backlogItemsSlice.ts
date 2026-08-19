import { createSlice, createSelector, PayloadAction } from "@reduxjs/toolkit";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { BacklogItem, BacklogActivityNote } from "@/gen/session/v1/backlog_pb";
import type { RootState } from "./store";

interface BacklogItemsState {
  /** Normalized map of backlog items, keyed by item id. */
  items: Record<string, BacklogItem>;
  /**
   * Per-item counter, incremented only when `upsertItem` is applied for a
   * genuine live (non-snapshot) event — never for the initial REST snapshot,
   * a reconnect resync/poll, or the forced-`is_snapshot` replay-branch copy
   * the server sends to close the double-delivery race (ADR-001; plan.md
   * Task 3.1.1c). `BacklogItemCard`'s flash-on-update treatment (Epic 6.1)
   * watches this counter change, not the item's content, so a resnapshot
   * that legitimately changes an item's fields while the client was
   * disconnected never triggers a flash (pre-mortem #4).
   */
  liveVersion: Record<string, number>;
}

const initialState: BacklogItemsState = {
  items: {},
  liveVersion: {},
};

/** Returns the epoch-ms value of a proto Timestamp, or 0 if unset (oldest possible). */
function timestampMs(ts: BacklogItem["updatedAt"]): number {
  return ts ? timestampDate(ts).getTime() : 0;
}

const backlogItemsSlice = createSlice({
  name: "backlogItems",
  initialState,
  reducers: {
    /**
     * Upserts a backlog item, guarded by a real `updatedAt`-based staleness
     * check (not sessionsSlice.upsertSession's equal-only check): an incoming
     * item strictly older than the currently-stored item for the same id is
     * dropped, so out-of-order event delivery (e.g. concurrent publishers,
     * stream replay) can never regress the store to older data.
     *
     * `isSnapshot` (default `true`) drives `liveVersion` bookkeeping only —
     * pass `false` exclusively for a genuine live `BacklogItemEvent` (i.e.
     * `event.isSnapshot === false`) so Epic 6.1's flash treatment can tell a
     * real-time change apart from a snapshot/replay/poll refresh.
     */
    upsertItem: {
      reducer(state, action: PayloadAction<{ item: BacklogItem; isSnapshot: boolean }>) {
        const { item: incoming, isSnapshot } = action.payload;
        const existing = state.items[incoming.id];
        if (existing && timestampMs(incoming.updatedAt) < timestampMs(existing.updatedAt)) {
          return;
        }
        // Backstop for a partially-loaded event push (item_sessions/activity_notes
        // dropped by a publishItemChanged call site or a non-eager-loaded snapshot
        // query) racing a fully-loaded one: never let an empty itemSessions or
        // activityNotes clobber data we already have for this item. A genuine
        // all-sessions-removed (or all-notes-removed) update would still arrive
        // with a newer updatedAt and populated (even if empty-by-intent) data from
        // a call site that *did* eager-load, so this only masks the known-bad
        // partial-load case, not real removals.
        //
        // activityNotes specifically: every BacklogChangeKind other than
        // ChangeActivityNoteAdded publishes via publishItemChanged ->
        // attachItemSessionsForPublish (session/ent_repository_backlog.go), which
        // re-populates only ItemSessions, never ActivityNotes — so their wire
        // events embed a full protoItem whose activity_notes is empty. Without
        // this guard, the very next status transition/verdict/session-attach would
        // wipe out any notes accumulated via the dedicated appendActivityNote
        // reducer below (ADR-002, Blocker 1 fix).
        let nextItem = incoming;
        if (existing) {
          const patch: Partial<BacklogItem> = {};
          if ((existing.itemSessions?.length ?? 0) > 0 && (incoming.itemSessions?.length ?? 0) === 0) {
            patch.itemSessions = existing.itemSessions;
          }
          if ((existing.activityNotes?.length ?? 0) > 0 && (incoming.activityNotes?.length ?? 0) === 0) {
            patch.activityNotes = existing.activityNotes;
          }
          if (Object.keys(patch).length > 0) {
            nextItem = { ...incoming, ...patch };
          }
        }
        state.items[incoming.id] = nextItem;
        if (!isSnapshot) {
          state.liveVersion[incoming.id] = (state.liveVersion[incoming.id] ?? 0) + 1;
        }
      },
      prepare(item: BacklogItem, isSnapshot: boolean = true) {
        return { payload: { item, isSnapshot } };
      },
    },
    /** Deletes an item from the map entirely (BacklogItemRemovedEvent — permanent delete, not upsert). */
    removeItem(state, action: PayloadAction<string>) {
      delete state.items[action.payload];
    },
    /**
     * Targeted single-note append for the dedicated activityNoteAdded live
     * event (ADR-002, backlog-item-activity-log) — appends `note` to
     * `state.items[itemId].activityNotes` without touching any other field,
     * never a call to upsertItem/a wholesale item replace. No-op if the item
     * isn't in the store yet (matches removeItem's "item not present"
     * tolerance) — the next GetBacklogItem/ListBacklogItems refresh will
     * include it.
     */
    appendActivityNote(state, action: PayloadAction<{ itemId: string; note: BacklogActivityNote }>) {
      const { itemId, note } = action.payload;
      const existing = state.items[itemId];
      if (!existing) return;
      state.items[itemId] = {
        ...existing,
        activityNotes: [...(existing.activityNotes ?? []), note],
      };
    },
  },
});

export const { upsertItem, removeItem, appendActivityNote } = backlogItemsSlice.actions;

// Base selectors
export const selectBacklogItemsMap = (state: RootState) => state.backlogItems.items;
export const selectBacklogItemById = (state: RootState, id: string): BacklogItem | undefined =>
  state.backlogItems.items[id];
/**
 * Raw `liveVersion` map — returned directly (like `selectBacklogItemsMap`)
 * rather than via `createSelector`: Immer already gives it the same
 * structural-sharing guarantee (unrelated keys keep their prior value
 * across an unrelated item's update), so no extra memoization is needed.
 */
export const selectBacklogItemsLiveVersionMap = (state: RootState) => state.backlogItems.liveVersion;

// Memoized selectors: list-shaped reads off backlogItemsSlice must not allocate
// a new array/object on every call, or every consumer (e.g. BacklogItemCard)
// re-renders whenever ANY item in the store changes, not just its own.
export const selectAllBacklogItems = createSelector(
  selectBacklogItemsMap,
  (items) => Object.values(items)
);

export const selectBacklogItemsTotal = createSelector(
  selectBacklogItemsMap,
  (items) => Object.keys(items).length
);

/** Shallow array equality: same length and same element references, in order. */
function shallowArrayEqual<T>(a: T[], b: T[]): boolean {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

/**
 * Selector factory for a status-filtered view. Use one instance per
 * component (e.g. via `useMemo(makeSelectBacklogItemsByStatus, [])`) rather
 * than a single shared selector — a shared selector's single-entry cache
 * would thrash (and stop memoizing) if multiple components filter by
 * different statuses simultaneously.
 *
 * `resultEqualityCheck: shallowArrayEqual` is what actually delivers on the
 * "unrelated item update shouldn't re-render this list" guarantee: without
 * it, the filtered array is a brand-new reference every time ANY item in the
 * store changes (because `selectAllBacklogItems` itself must return a new
 * array whenever the underlying map changes), even if none of the matching
 * items actually differ. With it, the selector still recomputes, but returns
 * the previous array reference when the filtered contents are unchanged.
 */
export const makeSelectBacklogItemsByStatus = () =>
  createSelector(
    [selectAllBacklogItems, (_state: RootState, status: string) => status],
    (items, status) => items.filter((item) => item.status === status),
    { memoizeOptions: { resultEqualityCheck: shallowArrayEqual } }
  );

export default backlogItemsSlice.reducer;
