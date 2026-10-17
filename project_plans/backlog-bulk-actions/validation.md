# Validation: Bulk Actions for Backlog Items

## Adversarial Review

### Challenge 1: Loop-over-single-item performance

**Concern**: A user with 200 done items clicks "Archive All Done". The server loops over 200 items, each calling `ArchiveBacklogItem` which (a) checks for worktree, (b) optionally pushes + cleans up git, (c) runs a DB update, (d) inserts a `BacklogStatusEvent`. Under load this could run for many seconds before the RPC returns.

**Resolution**: Add a server-side cap (e.g., max 200 items per bulk request) and document it. The vast majority of real-world batches are <50 items. A progress/streaming RPC would be a v2 enhancement. For v1, the synchronous loop with a context timeout (30s) is acceptable and consistent with how `BulkResetStuckRemediation` works.

**Plan adjustment**: Add validation in both service handlers: if `len(itemIDs) > 200`, return `InvalidArgument("bulk operations are limited to 200 items; split into smaller batches")`. Document the cap in the response message.

---

### Challenge 2: "Archive All Done" scope — filtered vs. global

**Concern**: If the user has filtered the backlog list to a specific repo path, does "Archive All Done" archive only the visible done items or *all* done items across the entire backlog? The description says "archiving all items marked done in a single action" — implying global. But a user who has filtered to one project might be surprised when items from other projects are archived.

**Resolution**: The `BulkArchiveBacklogItemsRequest` includes both `status_filter` and `item_ids`. The frontend "Archive All Done" button should pass the currently-visible done item IDs (from the filtered list), not a bare status filter. This makes the scope match exactly what the user sees. The `status_filter` field stays in proto for future server-side convenience but the primary UI path uses `item_ids` computed from the filtered view.

**Plan adjustment**: Change "Archive All Done" UI: compute `doneItemIds = visibleItems.filter(i => i.status === "done").map(i => i.id)` and call `bulkArchiveBacklogItemsByStatus` with `item_ids: doneItemIds` (not `status_filter: "done"`). The button label changes to "Archive Done (N)" where N is the count of visible done items.

---

### Challenge 3: Ent ORM schema for bulk delete with audit events

**Concern**: `DeleteBacklogItem` (hard-delete) creates a `BacklogStatusEvent` before deleting. In the loop, if the `BacklogStatusEvent.Insert` succeeds but `BacklogItem.Delete` fails, we have a dangling audit event. This is a bug in the single-item implementation too, but bulk amplifies it.

**Resolution**: The existing single-item code has this same transactional risk and has been accepted. The bulk implementation will match existing behavior (AC-3). Note the risk in a code comment but don't introduce new transactional guarantees in this PR.

---

### Challenge 4: Select mode and navigation collision

**Concern**: In the backlog list, clicking a row navigates to `?item=<id>` (opens detail pane). In select mode, clicking must instead toggle selection. If the implementation isn't careful, a click in select mode could do both.

**Resolution**: In `BacklogTableRow`, when `isSelectMode` is true, `onClick` dispatches `toggleSelection(item.id)` and calls `event.preventDefault()`. The checkbox `onChange` also dispatches `toggleSelection`. Only one action fires per click. This mirrors the `SessionCard.tsx` pattern exactly.

---

### Challenge 5: Proto: mutual exclusion of status_filter and item_ids

**Concern**: `BulkArchiveBacklogItemsRequest` has both `status_filter` and `item_ids`. If a caller sets both, the server needs a clear error.

**Resolution**: Service handler validates: if both are non-empty → `InvalidArgument("specify either status_filter or item_ids, not both")`. If both are empty → `InvalidArgument("specify either status_filter or item_ids")`. This was already noted in the plan (Phase 3, Step 1); this validation check is confirmed as required.

---

### Challenge 6: Redux state cleanup after bulk delete

**Concern**: After a bulk delete succeeds, the deleted item IDs remain in `selectedIds` (Redux). If the list view is then re-filtered and the deleted items have been removed from the server, the stale selection causes no visual bug (the rows are gone) but the state is dirty.

**Resolution**: After a successful `bulkDeleteBacklogItems` call, dispatch `clearSelection()` from the frontend before re-fetching the list. This was already in the plan (Phase 6: `clearSelection()` after confirm). Confirmed as required.

---

## Validation Summary

| AC | Status | Notes |
|---|---|---|
| AC-1 Archive All Done | ✅ Confirmed | Scope: visible done items by IDs (not global status filter) |
| AC-2 Multi-select delete | ✅ Confirmed | Select mode checkbox overlay, toolbar |
| AC-3 Parity with single-item | ✅ Confirmed | Loop-over-single-item preserves all side-effects |
| AC-4 New bulk RPCs | ✅ Confirmed | Two new RPCs in proto |
| AC-5 Storage bulk methods | ✅ Confirmed | Loop over single-item methods, collect failures |
| AC-6 Partial failure handling | ✅ Confirmed | continue-on-error + failedIds response field |
| AC-7 Empty-state guards | ✅ Confirmed | Button disabled states; JS-side count check |
| AC-8 Tests | ✅ Confirmed | Unit + service + E2E |

## Amended Plan Items

1. **Cap at 200 items**: Both `BulkArchiveBacklogItems` and `BulkDeleteBacklogItems` handlers return `InvalidArgument` if `len(ids) > 200`.
2. **"Archive Done (N)" UI**: Passes visible done item IDs, not a bare `status_filter`.
3. **Navigation guard**: `event.preventDefault()` on row click in select mode.
4. **clearSelection after bulk delete**: Dispatched immediately after success, before list refresh.
