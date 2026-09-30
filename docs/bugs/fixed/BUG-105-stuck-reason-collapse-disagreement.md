# BUG-105: BacklogItemDetail and BacklogBoard Collapse Multi-Reason Stuck Items to Different "Primary" Reasons [SEVERITY: Medium]

**Status**: Fixed
**Discovered**: 2026-09-11 (live UI audit)
**Fixed in**: this branch

## Problem Description

A backlog item can have several simultaneous open `StuckBacklogItem` rows —
confirmed live on item `09e91e3e-e13d-4166-a5f2-447242447f77`, which had 4 at
once (`BOUNCING`, `REWORK_BLOCKED_STALE`, `MULTIPLE_REASONS`,
`BOUNCE_CAP_EXHAUSTED`) from `ListStuckBacklogItems`. Two UI locations each
collapsed this list to a single reason, independently, with no shared order:

- `BacklogItemDetail.tsx` used `stuckItems.find(i => i.itemId === item.id)` —
  whichever row happened to be first in the array.
- `BacklogBoard.tsx` used `new Map(stuckItems.map(s => [s.itemId, s]))` —
  whichever row happened to be inserted last for that `itemId`.

For the live item, the detail panel showed "🔁 Not converging" (`BOUNCING`)
while the board card showed "🔴 Bounce cap exhausted"
(`BOUNCE_CAP_EXHAUSTED`) — two different reasons for the same item, and
neither view indicated there were 4 reasons total, not 1.

## Root Cause

Classification (per `quality:reflect-and-fix`): **Semantic/Intent**. Both call
sites correctly resolved "some open reason for this item," but neither had
any notion that more than one could be open simultaneously, so each picked
whatever its own underlying data structure (`Array.find` vs. `Map`
construction order) happened to yield first/last — an accident of iteration
order, not a deliberate choice, and the two accidents disagreed with each
other.

## Fix

Added a single shared resolution path in
`web-app/src/components/backlog-stuck/stuckReason.ts`:

- `STUCK_REASON_PRIORITY: Record<StuckReason, number>` — an explicit,
  exhaustive (compile-time-enforced, same pattern as the existing
  `STUCK_REASON_LABELS`/`_ICONS`/`_CLASS` maps) priority order. The two
  synthetic "automated remediation has given up" signals
  (`BOUNCE_CAP_EXHAUSTED`, then `MULTIPLE_REASONS`) rank above every specific
  reason, since they mean "stop auto-retrying, a human needs to look."
- `selectPrimaryStuckItem` — picks the single highest-priority row from a
  list of open rows for one item.
- `groupStuckItemsByItemId` — groups open rows by `item_id` (board case,
  O(n) once instead of a per-card filter).
- `summarizeStuckItemGroup` — resolves `{ primary, otherReasons }` for one
  item's group.

`BacklogItemDetail.tsx` and `BacklogBoard.tsx` now both call
`summarizeStuckItemGroup` instead of their own `.find()`/`Map` logic.
`BlockerChip.tsx` gained an `otherReasons?: StuckReason[]` prop rendering a
"+N more" indicator (`MoreReasonsBadge`, `data-testid="blocker-chip-more"`,
full list in the `title` tooltip) — threaded through
`LifecycleSummary.tsx` and `BacklogItemCard.tsx` — so the dropped reasons are
indicated rather than silently invisible.

## Regression Tests

- `web-app/src/components/backlog-stuck/stuckReason.test.ts`:
  `STUCK_REASON_PRIORITY` exhaustiveness + escalation-ranking tests, and
  `selectPrimaryStuckItem`/`groupStuckItemsByItemId`/`summarizeStuckItemGroup`
  tests using the exact live 4-reason fixture (asserts `BOUNCE_CAP_EXHAUSTED`
  wins, order-independence, and that `otherReasons` surfaces the 3 dropped
  reasons).
- `BacklogItemDetail.test.tsx`'s
  `BacklogItemDetail_should_ResolveBounceCapExhaustedAsPrimaryAndIndicateOthers_When_ItemHasFourOpenStuckReasons`
  and `BacklogItemCard.test.tsx`'s
  `BacklogBoard_should_ResolveBounceCapExhaustedAsPrimaryAndIndicateOthers_When_ItemHasFourOpenStuckReasons`
  render the same 4-row fixture through each real UI location and assert both
  show "Bounce cap exhausted" (never "Not converging") plus a "+3 more"
  indicator — the two components agreeing is the actual regression this bug
  was about.

`npx jest --no-coverage` (full web-app suite): 5308/5308 tests pass across
422 suites. `npx tsc --noEmit`: clean. `pnpm run lint:duplicates` (jscpd):
0.09% duplicated lines, unchanged baseline, gate passes.

## Systemic Fix / Recurring Shape

**Recurring shape**: "two UI surfaces derive the same domain fact from the
same feed independently, with no shared derivation function" — the general
class this codebase already guards against for label/icon/class rendering
(`STUCK_REASON_LABELS` etc. exist specifically so BlockerChip's two variants
never drift), but that guard didn't cover *which row* to render when more
than one is open. This fix closes that same gap for selection, not just
display: `summarizeStuckItemGroup` is now the one place either surface can
get "the" reason for an item, so a future third call site inherits agreement
for free instead of re-deriving its own tie-break.

No new lint rule was added — the enforcement is the shared function plus the
paired regression tests in `BacklogItemDetail.test.tsx`/`BacklogItemCard.test.tsx`
asserting the two real call sites agree on the same fixture, which is the
earliest point that could have caught this (a lint rule can't distinguish
"two components resolve one shared invariant independently" from ordinary
duplication).

## Related

- `web-app/src/components/backlog-stuck/stuckReason.ts` — existing
  `STUCK_REASON_LABELS`/`_ICONS`/`_CLASS` exhaustiveness pattern this fix's
  `STUCK_REASON_PRIORITY` follows.
- `session/domain/backlog.go` — `StuckReasonMultipleReasons`/
  `StuckReasonBounceCapExhausted` doc comments, the source for this fix's
  escalation-ranking rationale.
