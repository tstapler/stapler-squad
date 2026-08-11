# Requirements: bulk-select-ux

**Date**: 2026-06-23
**Type**: feature addition
**Complexity**: 3 — system design (multi-component UX overhaul with keyboard model, undo, and E2E coverage)

## Problem Statement

The session list's bulk selection capability exists but is only wired for **card view** — the default **row view** (`viewMode="row"`, used by most users) has no checkbox or selection support on `SessionRow`. The "Select" button in the header does appear in row mode and the `BulkActions` toolbar renders, but there is no mechanism to actually select rows. Users who exclusively use row mode (the default) effectively have no bulk action capability at all. Additionally, even in card mode, the feature has discoverability gaps: no keyboard range-select, no visual affordance before entering select mode, and no undo for destructive bulk operations.

## Baseline

Today without this feature:
- **Row mode users**: Must open each session individually and trigger single-session actions (delete, pause) one at a time.
- **Card mode users**: Bulk selection works but requires clicking "Select" to enter a mode before any checkboxes appear (low discoverability). No keyboard shortcuts for range select. No undo after bulk delete.
- Workaround for most users: none; they manually repeat per-session actions.

## Users / Consumers

- Developers running many parallel Claude/Aider sessions who need to clean up completed or stopped sessions in batch.
- Users managing sessions across projects who want to pause or resume a group at once.
- Power users who prefer keyboard-driven workflows.

## Success Metrics

1. **Row mode parity**: Selecting sessions in row mode works identically to card mode — checkboxes appear, `BulkActions` toolbar reflects selection count, all bulk operations execute correctly.
2. **Discoverability**: Checkboxes are hover-revealed on both row and card items (no need to click "Select" first to see them).
3. **Keyboard range-select**: Shift+click selects a contiguous range; Cmd/Ctrl+A selects all visible sessions.
4. **Undo on bulk delete**: A toast with "Undo" appears after bulk delete, reversible within 5 seconds.
5. **E2E coverage**: At least one Playwright test covering row-mode bulk delete and bulk pause, asserting correct session state after each action.

## Appetite

**Large (1–2 weeks)**
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must not break existing card mode selection behavior.
- Row mode uses a virtualized list (`@tanstack/react-virtual`) — checkbox state must work correctly with virtualized rendering (visible items only in DOM at any time).
- Undo for bulk delete is client-side only (soft delete or restore RPC must exist, or scope undo to a "pending delete" pattern before the server call).
- No new proto RPCs unless strictly required — prefer batching existing `DeleteSession` / `PauseSession` calls.

## Non-functional Requirements

- **Performance SLO**: Selecting 100+ sessions must not cause perceptible lag; checkbox state updates must be O(1) (Set-based, already the case).
- **Scalability**: Virtualized row list already handles 1000+ sessions; selection state lives in a `Set<string>` — no scalability concerns.
- **Security classification**: Internal tool, no regulated data.
- **Data residency**: No special requirements.

## Scope

### In Scope

- Wire `selectMode`, `isSelected`, `onToggleSelect` props into `SessionRow` — render a checkbox column (leftmost) that auto-enters select mode on click.
- Hover-reveal checkbox affordance on `SessionRow` (visible on hover even before select mode, matching card mode intent).
- Shift+click range select: clicking a row while holding Shift selects the contiguous range from the last-clicked row to the current.
- Cmd/Ctrl+A keyboard shortcut to select all visible (filtered) sessions.
- Escape key exits select mode and clears selection.
- Undo toast for bulk delete: show a snackbar with "Undo" button for 5 seconds; clicking it restores sessions (either via restore RPC or by re-creating from stored session data — investigate in research phase).
- Update `SessionRow.css.ts` and `SessionRow.tsx` to add checkbox column to the grid template.
- Update `BulkActions.tsx` if needed (e.g., keyboard hint labels).
- Playwright E2E tests: row-mode bulk delete, row-mode bulk pause, Shift+click range select, Escape to exit.

### Out of Scope

- Right-click context menu ("Select all with same status", etc.) — potential follow-on.
- Bulk rename or bulk move to project from row mode beyond what `BulkActions` already supports.
- Persistent selection across page navigations or server restarts.
- Mobile-specific touch-and-hold to enter select mode.
- New backend RPCs for batch operations (use existing per-session calls in parallel).

## Rabbit Holes

- **Undo for bulk delete**: If there is no `RestoreSession` or `UndeleteSession` RPC, implementing undo requires either a "soft delete" pattern (server-side) or a client-side "pending delete" approach (hold deletes locally, fire on undo timeout). Research must determine which exists. If neither, undo should be descoped to "confirmation modal only" to stay within appetite.
- **Virtualized checkbox column**: The grid template in `session-columns.ts` / `buildRowGridTemplate` must be updated to include a checkbox column. All row items in the virtualizer are re-measured on layout changes — ensure adding a fixed-width checkbox column doesn't cause a re-measure cascade.
- **Shift+click range select across groups**: Sessions in row mode are rendered in a flat virtualized list but are logically grouped. If the last-clicked session and current session are in different groups, Shift+click should still select the contiguous flat range. Ensure the range logic operates on `flatItems` (the flat virtualizer array), not per-group arrays.

## Alternatives Considered

- **Click-to-select without a dedicated "Select" button**: Enter select mode on any checkbox click (already done in card mode). This was partially implemented; extend to row mode.
- **Dedicated bulk-action page**: Rejected — would require navigation and lose context.
- **Server-side batch RPC**: Out of scope for this iteration; existing parallel per-session calls are sufficient.

## Feasibility Risks

- No `RestoreSession` RPC currently visible — undo for bulk delete may need to be descoped or implemented as a client-side pending-delete pattern.
- `buildRowGridTemplate` changes must not break existing column picker behavior.
- Shift+click range state must be managed carefully with `useCallback` dependencies to avoid stale closure bugs.

## Observability Requirements

Standard request logging sufficient. No new metrics or alerts needed — this is a pure UI feature with existing backend RPCs.

## Risk Control

- **Feature flag**: Not needed — row mode checkboxes are additive and do not change existing card mode behavior.
- **Rollback**: Revert the `SessionRow` prop changes and CSS update; `BulkActions` and `SessionList` wiring are backward-compatible (new props are optional).
- **Staged rollout**: Not required.

## Open Questions

1. Does a `RestoreSession` or `UndeleteSession` RPC exist on `SessionService`? (Determines whether undo is viable.)
2. Does `buildRowGridTemplate` in `session-columns.ts` support a fixed prepended column, or does every column need a `ColumnKey` entry?
3. Is there a keyboard event handler already on `SessionList` (for the existing `G` grouping shortcut) that can absorb Cmd+A and Escape, or do new `keydown` listeners need to be added?
