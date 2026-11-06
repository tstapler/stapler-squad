# Requirements: Bulk Triage and Auto-Implementation for Backlog Page

Item: 2ec26b1c-710b-4838-8658-e9f8968e8599
Complexity: 2

## Context

The backlog list page (`web-app/src/app/backlog/page.tsx`) already ships bulk Archive and Delete via:
- `BacklogBulkActions` component (`web-app/src/components/backlog/BacklogBulkActions.tsx`)
- `backlogBulkSelectionSlice` Redux slice (`web-app/src/lib/store/backlogBulkSelectionSlice.ts`)
- `runBulk` / `formatBulkFeedback` helpers (`web-app/src/lib/backlog/bulkActions.ts`)

This item extends that infrastructure with two new bulk actions:

## Named New Bulk Actions

### 1. Bulk Triage
- Triggers `triggerTriage(id)` (from `useBacklogService`) for each selected eligible item.
- Eligible items: status `idea` or `ready` AND `triageStatus` is NOT `"running"`.
- Skips ineligible items (already triaging, already done/archived/etc.); reports skip count in feedback.
- Fan-out is parallel (`runBulk` pattern); feedback "Triggered triage for N items" or "Triggered N of M (K failed)".
- After dispatch, clears selection on full success; leaves failures selected so user can retry.

### 2. Bulk Auto-Implementation
- Calls `spawnSessionFromItem(id, { autonomous: true })` for each selected eligible item.
- Eligible items: status `ready` (plan approved or skipPlanning=true).
- Requires an **explicit confirmation dialog** before execution — unattended autonomous sessions must never start without the user explicitly choosing them.
- Confirmation dialog text: "Start autonomous implementation for N item(s)? Sessions will run without manual steps." with a primary "Start N sessions" button and a Cancel.
- Fan-out parallel after confirmation; feedback "Started N autonomous sessions" or "Started N of M (K failed)".
- Clears selection on full success; leaves failures selected.

## Additional Bulk Actions (scoped during triage)

Beyond the two named ones, the following are identified as high-value companions:

### 3. Bulk Mark Ready (optional, lower priority)
- Calls `transitionStatus(id, "ready")` for `idea` items.
- No confirmation required (reversible via single-item controls).
- Feedback: "Marked N items ready".

## Non-Goals / Explicit Exclusions
- No new proto/server RPCs — all bulk actions fan out to single-item service calls (per prior arch decision, AC from item f4235050).
- No changes to `BacklogBoard`, `BacklogItemDetail`, or the session bulk-selection slice.
- The board page (`/backlog/board`) is NOT affected.

## Acceptance Criteria

1. "Triage selected" button appears in `BacklogBulkActions` toolbar when ≥1 item is selected.
2. Clicking "Triage selected" fans out `triggerTriage` for eligible items only; ineligible items (triaging, wrong status) are skipped silently with skip count in feedback.
3. Feedback string: "Triggered triage for N item(s)" / "Triggered N of M (K failed)" / "Nothing to triage" shown for ~4 s.
4. "Auto-implement selected" button appears in `BacklogBulkActions` toolbar when ≥1 item is selected.
5. Clicking "Auto-implement selected" opens a confirmation dialog showing the count of eligible items; Cancel closes dialog without side effects; Confirm fans out `spawnSessionFromItem(id, { autonomous: true })`.
6. Ineligible items (not `ready`, plan not approved and skipPlanning=false) are excluded from auto-implement with count shown in the confirmation dialog.
7. Confirmation text makes clear that sessions will run autonomously ("without manual steps").
8. Feedback: "Started N autonomous session(s)" / "Started N of M (K failed)" shown for ~4 s.
9. Both new actions clear selection on full success; partial failures leave failed IDs selected.
10. `proto/session/v1/backlog.proto` unchanged.
11. Jest tests cover: triage eligibility filter, auto-implement eligibility filter, confirmation dialog cancel/confirm, feedback strings, and partial-failure selection behavior.
12. Confirmation dialog is keyboard-accessible (focus trap, Escape to cancel, Enter to confirm).
13. Both buttons are disabled when `busy` is true (a prior bulk action is in flight).

## Key Files

- `web-app/src/components/backlog/BacklogBulkActions.tsx` — add new button handlers + confirmation dialog
- `web-app/src/components/backlog/BacklogBulkActions.test.tsx` — extend tests
- `web-app/src/app/backlog/page.tsx` — wire handlers using `triggerTriage` and `spawnSessionFromItem` from `useBacklogService`
- `web-app/src/lib/backlog/bulkActions.ts` — possibly add eligibility-filter helpers
- `web-app/src/lib/hooks/useBacklogService.ts` — already exposes `triggerTriage` and `spawnSessionFromItem`; no changes needed

## Open Questions

None — the two named actions are well-defined by existing single-item RPCs.
