# Research: Bulk Actions for Backlog Items

## Codebase Investigation

### Storage Layer (session/)

**`session/storage.go`** — Public `Storage` struct wraps `*EntRepository`. Relevant methods:
- `ArchiveBacklogItem(ctx, id, precondition, trigger, reason)` → delegates to `s.repo.ArchiveBacklogItem(...)`
- `DeleteBacklogItem(ctx, id)` → delegates to `s.repo.DeleteBacklogItem(...)`
- No bulk counterparts exist yet.
- `BulkResetStuckRemediation` (lines 2612+) is the only existing bulk storage method — it uses a single ent `.Where(predicates).SetX().Save()` across rows.

**`session/ent_repository_backlog.go`** — Concrete ent ORM implementation:
- `ArchiveBacklogItem` (line 1457): soft-delete, sets `archived_at + status="archived"`, records `BacklogStatusEvent`
- `DeleteBacklogItem` (line 1596): hard-delete `BacklogItem.DeleteOneID(parsedID).Exec(ctx)`, records `BacklogStatusEvent`
- `BulkResetStuckRemediation` (line 2612): uses ent bulk `.Update().Where(...).Save()` returning affected-row count

**Key insight**: `ArchiveBacklogItem` does worktree cleanup (`PushWorktreeChanges`, `CleanupWorktree`) before the DB operation. These are per-item, async-unsafe side-effects that prevent a simple single-ent-query bulk archive (unlike `BulkResetStuckRemediation` which only touches DB fields). Bulk archive must loop over IDs and call the single-item method for each.

### Service Layer (server/services/)

**`backlog_service_lifecycle.go`**:
- `ArchiveBacklogItem` handler (line 503): pushes worktree branches, calls `s.storage.ArchiveBacklogItem`, does git cleanup.
- `DeleteBacklogItem` handler (line 564): calls `s.storage.DeleteBacklogItem`.
- No bulk handlers.

**`backlog_service_stuck.go`**:
- `BulkResetStuckRemediation` (line 267): the model for new bulk handlers — calls storage, sends notification, returns count.

### Proto Layer (proto/session/v1/backlog.proto)

**Existing single-item messages:**
```protobuf
message ArchiveBacklogItemRequest { string item_id = 1; }
message ArchiveBacklogItemResponse { BacklogItem item = 1; }
message DeleteBacklogItemRequest { string item_id = 1; }
message DeleteBacklogItemResponse {}
```

**Existing bulk precedent:**
```protobuf
message BulkResetStuckRemediationRequest { ... }
message BulkResetStuckRemediationResponse { int32 reset_count = 1; ... }
```

**Design decision**: New bulk requests should carry `repeated string item_ids` for delete (specific items) and either `repeated string item_ids` or a `status_filter` (e.g., `"done"`) for archive-all-done. Including both `item_ids` and a `status_filter` on the archive request gives flexibility.

### Frontend Layer (web-app/src/)

**Backlog list** — `web-app/src/app/backlog/page.tsx`:
- `BacklogTableRow` is defined inline (not a separate file) — adding checkbox requires modifying this file.
- No select mode, no bulk toolbar today.

**Sessions bulk pattern** — the established model:
- `bulkSelectionSlice.ts` (Redux): `selectedIds`, `toggleSelection`, `selectAll`, `clearSelection`
- `BulkActions.tsx` (toolbar): shows count, action buttons, Select All / Clear
- `SessionCard.tsx`: checkbox overlay when in selectMode
- `SessionList.tsx`: "Select" toggle button → `selectMode` state → passes `isSelectMode` down

**Hook** — `useBacklogService.ts`:
- All RPC calls go through this hook's `clientRef.current.*` pattern.
- New bulk methods follow the same `async (ids: string[]) => void` shape as the existing session bulk calls.

### Comparison: Archive All Done vs. Multi-Select Delete

| Dimension | Archive All Done | Multi-Select Delete |
|---|---|---|
| Selection model | Server-side filter (status=done) | Client-side checkbox selection |
| Backend RPC | `BulkArchiveBacklogItems(status_filter)` | `BulkDeleteBacklogItems(item_ids[])` |
| Server loop needed | Yes (per-item worktree cleanup) | Yes (per-item hard-delete + audit event) |
| Confirmation | Simple "Archive N items?" prompt | "Delete N items? Cannot be undone" prompt |
| UI surface | Single toolbar button (no select mode needed) | Needs select mode checkbox overlay |

### Risks and Open Questions

1. **Partial failure handling**: If item 3 of 10 fails to archive (e.g., worktree push fails), do we continue? The description says "reuse existing behavior" which currently returns an error on any failure. For bulk, continuing-on-error with a `failed_ids` list is more user-friendly.

2. **Performance for large batches**: The loop-over-single-item approach (required for side-effects) is O(n) DB+git operations. For 100 items this is acceptable. For 1000+, a progress stream might be better. Initial implementation: synchronous loop with a timeout guard.

3. **"Archive All Done" scope**: Does it archive done items *across all filters* or only *currently visible* items? The description says "archiving all items marked done in a single action" — implies all done items globally, not filtered. This avoids confusing filter-dependent behavior.

4. **Redux slice**: Can reuse `bulkSelectionSlice.ts` (session-agnostic: just `selectedIds: string[]`) or create a `bulkBacklogSelectionSlice.ts`. Given the session slice is used by sessions, a separate slice avoids state bleed.

5. **Proto generation**: Any proto change requires `make proto-gen` and committing generated code under `gen/`.

### Pattern Confirmation

The `BulkResetStuckRemediation` pattern confirms:
- Proto: request with filter/IDs + response with count
- Handler: validate → call storage → push notification → return count
- Storage wrapper: thin delegation
- Repo: loop (or bulk query when no side-effects)

The sessions `BulkActions` + `bulkSelectionSlice` pattern confirms the frontend multi-select design is already proven in the codebase.
