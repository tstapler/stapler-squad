# Plan
1. `web-app/src/lib/store/backlogBulkSelectionSlice.ts` (+ register in store.ts): `{selectedIds, selectMode}`; `toggleSelection` (enters select mode, leaving it when emptied), `selectAll`, `clearSelection`. Test file alongside.
2. `web-app/src/lib/backlog/bulkActions.ts`: `runBulk(ids, fn)` → `{succeeded[], failed[]}` via `allSettled`; `formatBulkFeedback(verb, succeeded, total)`. Unit-tested.
3. `BacklogFilterBar`: optional `doneCount`, `onArchiveDone`, `archiveDoneBusy`; button `data-testid=backlog-archive-done`.
4. `components/backlog/BacklogBulkActions.tsx` (+ css): toolbar + delete confirm dialog; props `selectedCount,totalCount,onArchiveSelected,onDeleteSelected,onSelectAll,onClear,feedback,busy`.
5. `page.tsx`: checkbox cell/column, row click toggles in select mode, handlers (archiveDone, archiveSelected, deleteSelected), 4 s feedback timer (cleared on unmount), unmount cleanup dispatch.
6. CSS in `backlog.css.ts` for hover-revealed checkbox.
7. Feature marker `// +feature: backlog-bulk-actions`; `make registry-generate`.

## Adversarial review
- Hidden stale ids: selection can contain items that disappear via live updates → handlers intersect with current `items` before acting. 
- Archive-selected on already-archived items would error → filtered out per AC 5.
- Double-click during in-flight run → `busy` flag disables buttons.
- Timer leak on unmount → cleared in cleanup.
- Hover-only checkbox is keyboard-inaccessible → also visible on `:focus-within` and whenever selectMode.
- Board shares filter bar → props optional; board test must pass unchanged.
Verdict: no blockers.
