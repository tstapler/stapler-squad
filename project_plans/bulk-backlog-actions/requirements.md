# Requirements: Bulk actions for backlog items

Item: f4235050-c073-484b-8e99-b1d1f6d68526

## Confirmed target (from codebase)
- Repo: this one (`tstapler/stapler-squad`). Surface: the **List** view, `web-app/src/app/backlog/page.tsx` (`BacklogPageInner`, inline `BacklogTableRow`) and `web-app/src/components/backlog/BacklogFilterBar.tsx` (shared with the board page `app/backlog/board/page.tsx`).
- Single-item behavior to reuse: `useBacklogService().archiveBacklogItem(id)` / `deleteBacklogItem(id)` (`web-app/src/lib/hooks/useBacklogService.ts`), which call the existing `ArchiveBacklogItem` / `DeleteBacklogItem` RPCs and **throw** on failure.
- Pattern to mirror: session `bulkSelectionSlice.ts` + `components/sessions/BulkActions.tsx`.

## Scope decision
The acceptance criteria are authoritative: **frontend-only fan-out, no proto/server change** (AC 8). An earlier draft of this plan added `BulkArchive/BulkDelete` RPCs; that contradicts AC 8 and was discarded.

## Requirements (1:1 with AC numbers, 1-indexed as shown in the item)
1. `BacklogFilterBar` shows "Archive done (N)", N = filtered items with status `done`; disabled at 0; archives exactly those. Rendered only when the page passes `onArchiveDone` (board unaffected).
2. Archive-done fans out `archiveBacklogItem` in parallel (`Promise.allSettled`); feedback "Archived N items" or "Archived N of M (K failed)" for ~4 s.
3. Rows show hover-revealed checkbox; clicking enters select mode; state in new `backlogBulkSelectionSlice` (store key `backlogBulkSelection`), independent of `bulkSelection`.
4. `BacklogBulkActions` toolbar when selectedCount > 0: "N of M selected", Archive selected, Delete selected, Select all, Clear.
5. Archive selected: only non-archived selected items; clear selection on success; feedback string.
6. Delete selected: confirmation modal with count; confirm fans out `deleteBacklogItem`; clear selection on completion.
7. Unmount of the backlog page dispatches `clearSelection`.
8. `proto/session/v1/backlog.proto` unchanged.
9. `BacklogItemDetail`, session `BulkActions`, `BacklogBoard` untouched.
10. Jest tests: slice reducers; toolbar for 0/1/N; delete confirm dialog; partial-failure feedback.

## Edge decisions
- Partial failure: succeeded ids leave selection; failed ids stay selected so the user can retry. "Clear selection on success" = all succeeded.
- Singular/plural: "Archived 1 item".
- Row click while in select mode toggles selection instead of opening detail.
