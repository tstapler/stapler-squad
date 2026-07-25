# BUG-037: The Backlog Kanban Board Renders Only 5 of 9 Item Statuses — `queued`, `pr_pending`, and `refining` Items Vanish Entirely, With No Card, No Count, No Error [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — while investigating a user-reported concern ("a lot of stuff has been queued for a while and I want to make sure we understand why") and separately confirming, via the live `/backlog/board` page, that a known-stuck item (`9264efe7-b4c2-455a-9e2a-ab0196a63ecd`, status `pr_pending`) was invisible on the board: the Review column showed "0 / No items" despite the item actively existing and being polled by the reconciliation loop every 60 seconds.
**Fixed**: 2026-07-22 — `web-app/src/components/backlog/BacklogBoard.tsx`
**Impact**: `BacklogItemStatus` has 9 real values (`idea, refining, ready, queued, in_progress, review, pr_pending, done, archived`), but `BacklogBoard.tsx`'s `COLUMNS` array only defines 5 (`idea, ready, in_progress, review, done`), and every item→column assignment used an exact `item.status === column.status` match. An item whose status was `queued`, `pr_pending`, or `refining` matched none of the 5 columns and was silently dropped from the board entirely — not shown anywhere, not counted anywhere, no loading state, no error. A user looking at the board has no way to tell these items exist short of opening item detail (where the pipeline stepper *does* correctly show them, via a `modifier` badge) or checking the separate stuck-items feed. This directly explains reports of "items queued for a while with no visibility into why" — the board, the primary at-a-glance view of backlog state, was structurally incapable of showing them.

## Root Cause

The item-detail page's `StageTracker` component already solved this exact problem correctly: `deriveStageDisplay(status)` is a pure function mapping all 9 statuses onto the 5 visible lifecycle stages, folding `queued` into `in_progress` (with a "Queued" badge), `pr_pending` into `review` (with a "PR pending" badge), and `refining` into `idea` — with `archived` handled as a distinct dimmed/ribboned state. `BacklogBoard.tsx` never reused this mapping; its own column-membership filter (`items.filter((i) => i.status === column.status)`) and its enter/exit-animation bookkeeping (`COLUMN_STATUSES.has(...)`, `fromStatus === item.status`, etc.) all compared raw status values directly, so any status the 5-column enum didn't literally include had no possible column to land in.

## Fix Applied

`web-app/src/components/backlog/BacklogBoard.tsx`: added `stageOf(status): Stage | null`, a thin wrapper around the same `deriveStageDisplay` the item-detail `StageTracker` already uses (`archived: true` → `null`, meaning "excluded from the board" — preserving the pre-existing behavior that archived items never appeared on the board either). Replaced every raw-status comparison with `stageOf(...)`:
- The column-membership filter (`items.filter((i) => stageOf(i.status) === column.status)`), so `queued`/`pr_pending`/`refining` items now render in their mapped column.
- The exit-animation `exitingForColumn` filter and the flap-protection check (`pendingExit.fromStatus === item.status` → `stageOf(pendingExit.fromStatus) === stageOf(item.status)`), so a transition between two statuses that map to the *same* column (e.g. `review` ↔ `pr_pending`) correctly produces no animation at all (nothing visually moved), while a transition that crosses columns still animates correctly.
- The `COLUMN_STATUSES.has(...)` gates guarding whether an exit/enter animation should be scheduled, replaced with `stageOf(...) !== null`.

The now-redundant `COLUMN_STATUSES` constant was removed.

## Files Affected

- `web-app/src/components/backlog/BacklogBoard.tsx` — `stageOf` helper, column filter, animation-tracking logic
- `web-app/src/components/backlog/BacklogBoard.hiddenStatuses.test.tsx` — new regression test file

## Verification

- `BacklogBoard_should_RenderInProgressColumn_When_StatusIsQueued`, `..._RenderReviewColumn_When_StatusIsPrPending`, `..._RenderIdeaColumn_When_StatusIsRefining`, `..._RenderNoColumn_When_StatusIsArchived` — new tests asserting each previously-hidden status now renders in its mapped column (or, for `archived`, confirming the pre-existing correct exclusion is preserved).
- **Verified to fail against pre-fix code**: `git stash` on `BacklogBoard.tsx` alone reproduces the exact live symptom in the 3 new positive tests — `getByTestId("backlog-column-review")` renders "Review / 0 / No items" instead of containing the item, matching precisely what was observed live on `/backlog/board`.
- All 4 pre-existing `BacklogBoard.columnTransition.test.tsx` tests (Epic 6.4 fade/flash/reduced-motion/flap-settle behavior) pass unmodified — the animation-logic generalization from raw status to mapped stage doesn't change behavior for any status that was already one of the 5 literal column values.
- `npx tsc --noEmit` clean. `next lint` on the changed files shows only two pre-existing, unrelated warnings (ref-cleanup timing in an effect this fix didn't touch).
- Broader `jest --testPathPatterns="backlog"` run: 451/452 tests pass; the one failure (`BacklogEmptyState.test.tsx`) is a pre-existing, unrelated worker-crash confirmed to reproduce identically in isolation with this fix fully reverted.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Type Safety Gap bordering on API Contract Gap — `BacklogItemStatus` is a 9-value union (`KnownBacklogStatus | (string & {})`), but `COLUMNS`' status field is typed as the same broad `BacklogItemStatus`, so TypeScript had no way to flag that only 5 of 9 known values were ever actually reachable as a column. A `Record<Stage, ...>`-shaped exhaustiveness check (as `StageTracker`'s own `STAGE_LABELS: Record<Stage, string>` already demonstrates) would have made "every status maps to *some* stage" a compile-time-checked invariant instead of a filter that silently no-ops for anything not explicitly listed.

**Earliest achievable enforcement**: The regression tests are the practical level actually applied here. A stronger, cheaper structural guard worth flagging for a follow-up: have `BacklogBoard.tsx` import `STAGE_ORDER`/`deriveStageDisplay` as the single source of truth for "which stages exist" (already done by this fix) rather than maintaining its own separate `COLUMNS` array — the duplication between `COLUMNS` and `STAGE_ORDER`/`STAGE_LABELS` is itself a residual smell (two hand-maintained lists of the same 5 stages, in two files) this fix didn't fully collapse, since `COLUMNS` also carries the board-specific `label` text. Not resolved here to keep the diff minimal and low-risk; worth a follow-up if a 6th stage is ever added and someone updates one list but not the other.

**Recurring shape**: A `StageTracker`-shaped canonical mapping already existed and was correct; a sibling component (`BacklogBoard`) was built without discovering or reusing it, and independently re-implemented a narrower, incomplete version of the same status→stage logic. Distinct from this session's other recurring shape ("cleanup path forgotten") — this is "the correct abstraction already exists elsewhere in the codebase, but a new call site didn't find it and drifted." Worth a note for a future architecture-review pass: grep for other places doing ad hoc `item.status === "..."` comparisons in the frontend that should instead route through `deriveStageDisplay`.

## Related

- `web-app/src/components/backlog/detail/StageTracker.tsx` — the pre-existing correct mapping this fix reuses.
- Discovered while live-diagnosing backlog item `9264efe7-b4c2-455a-9e2a-ab0196a63ecd`, the same item behind BUG-029, BUG-031, BUG-032, and BUG-036 earlier the same day.
