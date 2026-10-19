# Research (codebase findings)
- `archiveBacklogItem`/`deleteBacklogItem` return `Promise<boolean>`, throw on RPC error, and also `setLastError` — fan-out must use `Promise.allSettled`.
- `BacklogFilterBar` consumers: list page, board page, `BacklogFilterBar.source.test.tsx`, `BoardPage.stuckItems.test.tsx` → new props must be optional.
- `filteredItems` (page.tsx) already excludes archived unless "Show archived"; `done` is a distinct status from `archived`.
- Table has 6 columns (`colSpan={6}` group headers) → adding a checkbox column needs `colSpan={7}` and an extra `<th>`.
- Store (`lib/store/store.ts`) combines slices by key; `bulkSelectionSlice` is `{selectedIds}` only, so a separate backlog slice is required (AC 3).
- Existing modal markup precedent: `role="dialog"` overlay in page.tsx using `styles.modalOverlay/modalBox`.
