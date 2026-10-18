# Bulk Triage & Auto-Implement: Stack Research

## 1. React/Redux/CSS patterns

**Component structure** (`BacklogBulkActions.tsx`):
- Functional component with a plain `interface BacklogBulkActionsProps` (no generic types, no union discriminants — every new action is a new optional callback prop).
- All visual state is local `useState` — currently just `confirmingDelete: boolean`. No Redux state inside the component itself; selection state lives in the Redux slice (`backlogBulkSelectionSlice`) and is passed in as derived counts/IDs from the page.
- CSS is **vanilla-extract** (`@vanilla-extract/css`). The toolbar re-uses shared class tokens from `BulkActions.css.ts` (sessions component, imported by name: `container`, `selection`, `count`, `selectAllButton`, `clearButton`, `actions`, `actionButton`, `danger`, `feedback`). Dialog-specific styles (`overlay`, `dialog`, `dialogActions`) live in `BacklogBulkActions.css.ts`, referencing design tokens via `vars` from `@/styles/theme-contract.css`.
- Analytics calls use `useAnalytics()` → `track({ name, category, component, labels? })` inline in each handler.

**Page wiring** (`page.tsx`):
- `runBulkAction(verb, ids, action)` — shared `useCallback` wrapper that sets `busyRef`/`setBulkBusy`, calls `runBulk` from `bulkActions.ts`, then calls `showBulkFeedback(formatBulkFeedback(verb, result))`.
- Each action (archive, delete) is a separate `useCallback` that filters eligible IDs before calling `runBulkAction`, then dispatches `clearSelection()` or `selectAll(failedIds)`.
- `BacklogBulkActions` receives `onArchiveSelected`, `onDeleteSelected`, `onSelectAll`, `onClear`, `selectedCount`, `totalCount`, `busy`, `feedback` as props. New actions (triage, auto-implement) follow the same pattern: add `onTriggerTriageSelected` and `onAutoImplementSelected` callbacks to the props interface.

**`runBulk` / `formatBulkFeedback`** (`lib/backlog/bulkActions.ts`):
- `runBulk(ids, action)` fans out with `Promise.allSettled` — a rejection marks that ID failed without stopping the rest. Returns `{ succeeded: string[], failed: string[] }`.
- `formatBulkFeedback(verb, result)` produces "Archived 3 items" or "Archived 2 of 5 (3 failed)".

**Eligibility filtering for new actions:**
- Bulk Triage: filter `visibleSelectedIds` to items where `status === "idea" || status === "ready"` and `triageStatus !== "running"`. Call `triggerTriage(id)` per item.
- Bulk Auto-Implement: filter to `status === "ready"` and (`planApproved === true` or `skipPlanning === true`). Call `spawnSessionFromItem(id, { autonomous: true })` per item.

## 2. Confirmation dialog pattern

The existing delete confirmation dialog in `BacklogBulkActions.tsx` is a **custom inline dialog** — no focus-trap library. Key details:
- Rendered conditionally via `{confirmingDelete && <div className={styles.overlay}>...</div>}`.
- The overlay `div` has `onClick={() => setConfirmingDelete(false)}` (click-outside-to-cancel).
- The inner dialog `div` has `role="dialog" aria-modal="true" aria-labelledby="..."` and `onClick={(e) => e.stopPropagation()}`.
- Keyboard: `onKeyDown` on the dialog div checks `e.key === "Escape"` and closes — no focus-trap, no `inert` on siblings.
- Focus is placed via `autoFocus` on the Cancel button (the safe default).

**Auto-implement confirmation should reuse this exact pattern:**
- Add `confirmingAutoImplement: boolean` local state.
- Render the same `styles.overlay` + `styles.dialog` layout.
- `autoFocus` on the Cancel button to match the existing safe-default focus placement.
- `data-testid="backlog-bulk-auto-implement-dialog"` and `data-testid="backlog-bulk-auto-implement-confirm"` for RTL targeting.

## 3. Test utilities

All tests use **Jest + React Testing Library** (`@testing-library/react`).

Patterns from `BacklogBulkActions.test.tsx`:
- `render()` + `screen.getByTestId()` / `screen.getByRole()` / `screen.getByText()` for queries.
- `fireEvent.click()` for interactions — no `userEvent` in this file.
- `jest.mock("@/lib/analytics", ...)` to stub `useAnalytics`.
- Helper `renderToolbar(props?)` factory that provides default handler stubs via `jest.fn()` and spreads overrides — the pattern to follow for new prop callbacks.
- Dialog tested by clicking the trigger, asserting `screen.getByRole("dialog")` appears, then clicking confirm/cancel and asserting the handler call count and dialog disappearance.

The `bulkActions.test.ts` unit test exercises `runBulk` and `formatBulkFeedback` directly — no rendering. New eligibility-filtering helpers should get their own unit test file following the same approach.

## 4. TypeScript prop interface pattern

`BacklogBulkActionsProps` is a plain `interface` (not `type`), with:
- Primitive props (`selectedCount: number`, `totalCount: number`, `busy?: boolean`, `feedback?: string | null`).
- Handler callbacks typed as `() => void` (synchronous from the component's perspective — the page's `useCallback` returns `Promise<void>` but the component doesn't await it).
- No generics, no discriminated unions, no `React.ComponentPropsWithoutRef` extension.

New props for the two actions follow the same shape:
```ts
onTriggerTriageSelected: () => void;
onAutoImplementSelected: () => void;
triageEligibleCount?: number;   // optional — for "Triage N items" label if desired
autoImplementEligibleCount?: number;
```

`spawnSessionFromItem` signature (from `useBacklogService.ts`):
```ts
spawnSessionFromItem: (id: string, options?: { autonomous?: boolean; force?: boolean }) => Promise<{ sessionUuid: string; queued: boolean } | null>
```

`triggerTriage` signature:
```ts
triggerTriage: (id: string, feedback?: string) => Promise<{ itemSessionId: string } | null>
```
