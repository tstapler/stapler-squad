# Implementation Plan: Bulk Actions for Backlog Items

## Overview

Two bulk capabilities: (1) "Archive All Done" — one-click server-side filter archive; (2) Multi-select delete — checkbox selection + batch delete. Both reuse the existing single-item code paths for per-item side-effects.

## Architecture Decision

**Loop-over-single-item** (not single-query bulk update) for both operations, because:
- `ArchiveBacklogItem` performs git worktree cleanup per item — cannot be collapsed into a single ent `.Update().Where(...).Save()`.
- `DeleteBacklogItem` records per-item `BacklogStatusEvent` audit records — also per-item.
- Parity with single-item behavior (AC-3) demands this.
- Performance is acceptable for the expected batch sizes (tens to low hundreds of items).

**Partial-failure model**: continue on error, accumulate `failed_ids` in the response. This is more user-friendly than aborting on first failure, and the description says each item is handled as it would be alone — a single item's failure would not prevent other items from being processed.

---

## Phase 1 — Proto Changes

**File**: `proto/session/v1/backlog.proto`

Add after the existing `DeleteBacklogItemResponse`:

```protobuf
message BulkArchiveBacklogItemsRequest {
  // Archive items matching this status filter (e.g. "done"). Mutually exclusive with item_ids.
  string status_filter = 1;
  // Explicit list of IDs to archive. Mutually exclusive with status_filter.
  repeated string item_ids = 2;
}
message BulkArchiveBacklogItemsResponse {
  int32 success_count = 1;
  repeated string failed_ids = 2;
}
message BulkDeleteBacklogItemsRequest {
  repeated string item_ids = 1;
}
message BulkDeleteBacklogItemsResponse {
  int32 success_count = 1;
  repeated string failed_ids = 2;
}
```

Add to `service BacklogService`:
```protobuf
rpc BulkArchiveBacklogItems(BulkArchiveBacklogItemsRequest) returns (BulkArchiveBacklogItemsResponse);
rpc BulkDeleteBacklogItems(BulkDeleteBacklogItemsRequest) returns (BulkDeleteBacklogItemsResponse);
```

Run `make proto-gen` after changes. Commit generated output under `gen/`.

---

## Phase 2 — Storage Layer

**File**: `session/storage.go`

Add two new methods to the `Storage` struct (delegating to `s.repo`):

```go
func (s *Storage) BulkArchiveBacklogItems(ctx context.Context, statusFilter string, itemIDs []string) (successCount int, failedIDs []string, err error)
func (s *Storage) BulkDeleteBacklogItems(ctx context.Context, itemIDs []string) (successCount int, failedIDs []string, err error)
```

**File**: `session/ent_repository_backlog.go`

Implement both on `*EntRepository`:

`BulkArchiveBacklogItems`:
1. If `statusFilter != ""`, query all item IDs with that status using `r.client.BacklogItem.Query().Where(backlogitem.Status(statusFilter)).IDs(ctx)`.
2. Loop over resolved IDs, call `r.ArchiveBacklogItem(ctx, id, nil, TriggeredByUser, "bulk-archive")` for each.
3. Collect successes and failures.
4. Return count, failed IDs.

`BulkDeleteBacklogItems`:
1. Loop over `itemIDs`, call `r.DeleteBacklogItem(ctx, id)` for each.
2. Collect successes and failures.
3. Return count, failed IDs.

---

## Phase 3 — Service Handlers

**File**: `server/services/backlog_service_lifecycle.go`

Add two new handlers following the `BulkResetStuckRemediation` pattern:

`BulkArchiveBacklogItems`:
1. Validate: both `status_filter` and `item_ids` non-empty → `InvalidArgument`.
2. Call `s.storage.BulkArchiveBacklogItems(ctx, req.Msg.StatusFilter, req.Msg.ItemIds)`.
3. If `successCount > 0`, send notification: `"N items archived"`.
4. Return `BulkArchiveBacklogItemsResponse{SuccessCount: int32(n), FailedIds: failedIDs}`.

`BulkDeleteBacklogItems`:
1. Validate: `item_ids` must be non-empty → `InvalidArgument`.
2. Call `s.storage.BulkDeleteBacklogItems(ctx, req.Msg.ItemIds)`.
3. If `successCount > 0`, send notification.
4. Return `BulkDeleteBacklogItemsResponse{SuccessCount: int32(n), FailedIds: failedIDs}`.

Register both handlers in `server/server.go` (or wherever `BacklogService` routes are registered).

---

## Phase 4 — Frontend: Hook

**File**: `web-app/src/lib/hooks/useBacklogService.ts`

Add two methods following the existing pattern:
```ts
bulkArchiveBacklogItemsByStatus: async (statusFilter: string) => Promise<{successCount: number; failedIds: string[]}>
bulkDeleteBacklogItems: async (ids: string[]) => Promise<{successCount: number; failedIds: string[]}>
```

---

## Phase 5 — Frontend: Redux Slice

**New file**: `web-app/src/lib/store/bulkBacklogSelectionSlice.ts`

Mirror `bulkSelectionSlice.ts` (sessions) with:
- State: `{ selectedIds: string[]; selectMode: boolean }`
- Reducers: `toggleSelection(id)`, `selectAll(ids[])`, `clearSelection()`, `toggleSelectMode()`

Wire into the Redux store root reducer.

---

## Phase 6 — Frontend: Backlog List UI

**File**: `web-app/src/app/backlog/page.tsx`

Changes to `BacklogTableRow`:
- Accept `isSelectMode: boolean` and `isSelected: boolean` props.
- When `isSelectMode`, render a `<td>` with `<input type="checkbox">` as first column.
- `onClick` in select mode → dispatch `toggleSelection(item.id)` instead of navigating.

Changes to `BacklogPageInner`:
- Import and use `bulkBacklogSelectionSlice` selectors/actions.
- Add "Select" toggle button in the toolbar (header row above the table).
- Pass `isSelectMode` and `isSelected` down to each `BacklogTableRow`.
- Keyboard: `Ctrl/Cmd+A` → `selectAll(visibleItemIds)`, `Escape` → `clearSelection + toggleSelectMode(false)`.

**New file**: `web-app/src/components/backlog/BacklogBulkActions.tsx`

Toolbar component rendered when `selectMode || hasAnyDoneItems`, with:
- "Archive All Done" button (disabled if no done items): confirms → calls `bulkArchiveBacklogItemsByStatus("done")`.
- "Delete Selected" button (disabled if `selectedIds.length === 0`): confirms → calls `bulkDeleteBacklogItems(selectedIds)` → calls `clearSelection()`.
- Item count display: "N selected".

**File**: `web-app/src/app/backlog/backlog.css.ts`

Add styles for checkbox column, bulk actions toolbar (following `BulkActions.tsx` styles in sessions).

---

## Phase 7 — Tests

**Backend unit tests** — `session/ent_repository_backlog_test.go`:
- `TestBulkArchiveBacklogItems_AllDone`: seed 3 done + 2 not-done items → archive by status="done" → 3 archived, 2 untouched.
- `TestBulkArchiveBacklogItems_SpecificIDs`: archive by IDs, one ID doesn't exist → 1 failure in failedIDs, rest archived.
- `TestBulkDeleteBacklogItems_Success`: seed 3 items → delete all → 0 remain, successCount=3.
- `TestBulkDeleteBacklogItems_PartialFailure`: one unknown ID → failedIDs contains it, rest deleted.

**Service handler tests** — `server/services/backlog_service_lifecycle_test.go`:
- RPC-level tests for `BulkArchiveBacklogItems` and `BulkDeleteBacklogItems` (validation, success, partial failure).

**E2E test** — `tests/e2e/backlog-bulk-actions.spec.ts`:
- `// @feature backlog:bulk-actions`
- Enter select mode → select 2 items → click Delete → confirm → verify 2 items removed from list.
- Seed 2 done items → click "Archive All Done" → confirm → verify they no longer appear.

---

## Rollout / Risk Notes

- No DB schema change: bulk operations use existing columns (`archived_at`, `status`).
- No migration needed.
- The `BulkArchiveBacklogItems` status-filter path is additive — callers that don't use it are unaffected.
- Feature registry: add `// +api: backlog:bulk-actions` annotation on the new RPC handlers; `// +feature: backlog-bulk-actions` in `BacklogBulkActions.tsx`; run `make registry-generate`.
