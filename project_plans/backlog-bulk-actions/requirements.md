# Requirements: Bulk Actions for Backlog Items

## Item ID
f4235050-c073-484b-8e99-b1d1f6d68526

## Problem Statement

The backlog currently supports single-item archive and delete operations, but users must act on each item individually. When a sprint or project phase ends, a user may have dozens of completed (done) items to archive or many stale items to delete — forcing one-by-one operations is tedious and error-prone.

## Goals

1. **Archive completed items in bulk** — a single "Archive All Done" action archives every item currently in a `done` status without requiring item-by-item clicks.
2. **Multi-select delete** — users can select multiple arbitrary items and delete them all at once.
3. **Reuse existing behavior** — each item in a batch is handled identically to how it would be handled by the existing single-item `ArchiveBacklogItem` and `DeleteBacklogItem` operations (worktree cleanup, audit events, etc.).

## Out of Scope

- Bulk unarchive
- Bulk status transitions other than archive
- Bulk edit of fields (priority, title, description)
- Undo / rollback of bulk operations (single-item operations don't have undo either)

## Existing Architecture (Confirmed)

- **Storage**: ent ORM over SQLite. Single-item: `Storage.ArchiveBacklogItem` (soft-delete, sets `archived_at` + status → "archived"), `Storage.DeleteBacklogItem` (hard-delete). Both record a `BacklogStatusEvent`.
- **Proto**: `proto/session/v1/backlog.proto`. Bulk precedent: `BulkResetStuckRemediation`.
- **Server handlers**: `server/services/backlog_service_lifecycle.go` for single-item archive/delete; `backlog_service_stuck.go` for the only existing bulk handler.
- **Frontend list**: `web-app/src/app/backlog/page.tsx` — table with inline `BacklogTableRow`; no multi-select today.
- **Session bulk pattern**: `BulkActions.tsx` + `bulkSelectionSlice.ts` in the sessions UI are the established pattern to mirror.

## Acceptance Criteria

### AC-1 — Bulk archive all done
A user can trigger "Archive All Done" from the backlog list view toolbar. Every item in `done` status (across all visible items, ignoring current filter) is archived in a single server operation. A success notification reports the count of archived items (e.g. "7 items archived").

### AC-2 — Bulk delete via multi-select
A user can enter a "select mode" on the backlog list view. In select mode, each backlog item row shows a checkbox. The user can check individual rows (or use Select All). An action toolbar shows the count of selected items and a Delete button. Clicking Delete prompts for confirmation, then deletes all selected items.

### AC-3 — Parity with single-item behavior
Each item archived/deleted in bulk goes through the same code path as the single-item operations: worktree branches are pushed, git worktrees are cleaned up, and a `BacklogStatusEvent` audit record is written for each item.

### AC-4 — New bulk RPCs
`BulkArchiveBacklogItems` and `BulkDeleteBacklogItems` RPCs are defined in `proto/session/v1/backlog.proto` and registered on `BacklogService`.

### AC-5 — Server-side storage bulk methods
`Storage.BulkArchiveBacklogItems(ctx, []string itemIDs)` and `Storage.BulkDeleteBacklogItems(ctx, []string itemIDs)` are implemented, each iterating over the IDs and calling the existing single-item methods (so all per-item side-effects are preserved), and returning a count.

### AC-6 — Error handling: partial failures
If any item in a bulk operation fails (e.g., item not found, permission error), the operation reports which IDs failed and continues processing the rest. The RPC response includes both a `success_count` and a list of `failed_ids`.

### AC-7 — Empty-state guards
"Archive All Done" is disabled (grayed out) when no items are in `done` status. "Delete Selected" is disabled when no items are selected.

### AC-8 — Tests
Unit tests cover the storage bulk methods (success path, partial failure). The service handler tests cover the RPC layer. At least one E2E test covers the multi-select delete flow end-to-end.

## Key Files to Touch

| Layer | Files |
|---|---|
| Proto | `proto/session/v1/backlog.proto` |
| Storage interface | `session/storage.go` (add method stubs) |
| Repo impl | `session/ent_repository_backlog.go` (bulk methods) |
| Service handler | `server/services/backlog_service_lifecycle.go` (new bulk RPC handlers) |
| Frontend hook | `web-app/src/lib/hooks/useBacklogService.ts` |
| Frontend UI | `web-app/src/app/backlog/page.tsx` (select mode, toolbar) |
| Frontend components | New `BacklogBulkActions.tsx` (toolbar), `bulkBacklogSelectionSlice.ts` (or extend existing) |
| Tests | `session/ent_repository_backlog_test.go`, `server/services/backlog_service_lifecycle_test.go`, `tests/e2e/` |
