# Validation: Backlog Page Performance Plan

## Adversarial Review

### Challenge 1 — Is `React.memo` enough without stable callback references?

**Risk**: If `onRowClick` and `onToggleSelect` are new function references on every render of `BacklogPageInner`, `React.memo` on `BacklogTableRow` will be a no-op — every row still re-renders.

**Verdict**: VALID CONCERN. The plan must explicitly include `useCallback` on both handlers as a prerequisite to memoizing `BacklogTableRow`. This is noted in the plan (Task 1.2 "Props to stabilize") but must be part of the implementation checklist, not optional.

**Resolution**: AC should require both `React.memo` and `useCallback` on the two handler props. Confirmed the handlers today call `setActiveItemId` and dispatch Redux actions — wrapping with `useCallback([])` is safe since they close over stable dispatch references.

---

### Challenge 2 — Will hoisting `useWatchBacklogItems` into layout break the status filter?

**Risk**: The list view and board view may use different status filter configs. If the layout owns the stream subscription with one filter, one of the two views will receive incorrect data.

**Investigation**: In `page.tsx` the hook is called `useWatchBacklogItems()` with no args (line ~273). The `useWatchBacklogItems` hook internally uses the `statusFilterKey` derived from the Redux filter state. If both views read from the same Redux store (they do), their `statusFilterKey` will be the same value. The stream is unfiltered at the streaming layer — all filtering is client-side in both views.

**Verdict**: LOW RISK. The stream is unfiltered; the hook call is already identical between list and board. Hoisting is safe.

---

### Challenge 3 — Does the warm-store guard create a race condition?

**Risk**: If the user navigates to `/backlog` with a stale Redux store (e.g., a stale item that was deleted server-side), skipping the `listBacklogItems` REST call means deleted items stay visible until the stream delivers a removal event.

**Assessment**: The `WatchBacklogItems` stream delivers `itemRemoved` events and begins each connection with a `snapshotComplete` event sequence that sends the current server state. Any item that no longer exists server-side will not appear in the stream snapshot, and after `snapshotComplete`, the client should reconcile to the server's truth.

**Verdict**: ACCEPTABLE. The stream snapshot is the authoritative reconcile mechanism. The REST snapshot is a backup/shortcut. However, the guard should only skip the REST call when the stream connection itself is established, not blindly skip it if the store has stale items but no active stream.

**Refined guard**: Check `storeHasItems && streamIsConnecting` — if the stream is already establishing, skip the REST call; it will be reconciled via stream snapshot. If no stream is active (e.g., component mounted while offline), do fire the REST call.

---

### Challenge 4 — Is the `(priority, updated_at)` index actually used by SQLite?

**Risk**: SQLite's query planner might not use a new index for `NOT IN` + ORDER BY if it estimates a full scan is cheaper (small table, or the NOT IN predicate touches most rows).

**Assessment**: For backlogs with <100 items, full scan + sort is fast enough that the index provides marginal benefit. For 200–1000 items, the filesort overhead becomes more significant. The index is low-cost to maintain (two integer/datetime columns) and has no downside. Even if SQLite occasionally chooses the full scan for small tables, it will use the index as the backlog grows.

**Verdict**: ADD THE INDEX. It is cheap, correct, and will matter for users with large backlogs.

---

### Challenge 5 — Does `annotateDuplicatePending` N+1 actually fire for most users?

**Risk**: The N+1 only fires for `status == "review"` items. Most backlogs may have zero or few review-status items.

**Assessment**: True that the blast radius is limited. However, if a user has 20 items in "review" state (a common scenario for active backlogs), that is 20 extra DB round-trips per `ListBacklogItems` call. Given `ListBacklogItems` is called on every mount AND the stream already delivers the full session list, the fix is clean with minimal code change.

**Verdict**: INCLUDE FIX. The data is already in-memory; using it is strictly better than re-querying.

---

## Acceptance Criteria Validation

All 7 acceptance criteria from requirements.md are addressed:

| AC | Plan Phase | Notes |
|---|---|---|
| AC1: Warm store renders before RPC | Phase 3 (Task 3.1) | Stream snapshot reconciles any gaps |
| AC2: `sortedItems` memoized | Phase 1 (Task 1.1) | Direct `useMemo` wrap |
| AC3: `BacklogTableRow` memoized | Phase 1 (Task 1.2) | Requires `useCallback` on handlers |
| AC4: `annotateDuplicatePending` N+1 eliminated | Phase 2 (Task 2.1) | Pass sessions by-ref from batch query |
| AC5: `(priority, updated_at)` index | Phase 2 (Task 2.2) | Needs ent-gen + migration |
| AC6: Warm mount skips redundant RPC | Phase 3 (Task 3.1) | Refined: guard on stream state |
| AC7: Shared stream across view switch | Phase 4 (Task 4.1) | Largest refactor, ship last |

## Estimated Effort

| Phase | Complexity | Notes |
|---|---|---|
| Phase 1 (render memoization) | Low | ~30 lines changed, no new files |
| Phase 2 (backend N+1 + index) | Medium | N+1 fix ~50 lines; index needs ent-gen + migration |
| Phase 3 (warm store guard) | Low | ~10 lines in hook |
| Phase 4 (shared stream layout) | High | Layout refactor + context; touches 3 files |
