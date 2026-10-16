# Pitfalls — shipped-snapshot-display

## 1. False-negative risk in closing this as "already implemented"

`project_plans/backlog-already-implemented/requirements.md` is about the *inverse* trust
problem — a work-agent claiming "no diff needed, already done" and a reviewer having to
verify that claim against the live codebase before crediting it, because "empty diff,
already implemented" and "empty diff, agent did nothing" look identical at the diff level.
Its load-bearing guardrail: **never credit an already-implemented claim without independent
verification against the current codebase.**

That guardrail transfers directly here, and this session's own investigation already
satisfies it rather than merely asserting it:

- The claim is backed by a specific commit (`aedd80648`, 2026-07-18, on `main`) and a
  concrete call chain (`GetBacklogItemShipStatus` RPC → `useBacklogItemShipStatus` →
  `fromShipStatus`/`fromShipStatusGithub` → `VcsWidgetGithubRow.tsx`/`VcsWidget.tsx` →
  `VersionControlSection.tsx` → `BacklogItemDetail.tsx`), not a bare assertion.
- Unit coverage exists today at `web-app/src/lib/vcs/adapters.test.ts:132-190`, verified by
  reading it directly (`fromShipStatus_should_MapGithubAndFileStatsFromSnapshot_When_...`,
  `..._PreservePartiallySuccessfulGithubGroup_When_SnapshotCaptureFailedTrueAndFileStatsEmpty`).

Residual risk if this gets closed without the regression test in §2: the *adapter* mapping
is proven, but nothing proves the mapped data actually reaches the rendered DOM through the
real hook boundary. If a future refactor breaks the wiring between `BacklogItemDetail` and
`VersionControlSection` (e.g. a prop renamed, a conditional that hides the widget for a
status this item never hits in tests), `adapters.test.ts` would keep passing while the
screen goes dark — the same "green but not actually verified" gap the
`backlog-already-implemented` project exists to prevent, just at the UI-wiring layer instead
of the review-gate layer. That's exactly why the item-closure recommendation should pair
with the DOM-level regression test rather than close on adapter coverage alone.

## 2. Test-writing pitfalls — where to add the DOM-level test, and how not to fake it

**Do not add this test to `VersionControlSection.test.tsx`.** That file's `makeWidgetData()`
helper (lines 26–40) hand-builds a `VcsWidgetData` object with hardcoded fields including
`shipped: false` and no snapshot fields at all. A test added there would pass a hand-rolled
fixture straight to the component as a prop, never touching `fromShipStatus` — it would
verify only that the component *can* render given fields, not that the real adapter produces
those fields from a real `BacklogItemShipStatus` proto. This is precisely the failure mode
the task description warns about ("could drift from the real proto shape").

**Add it to `BacklogItemDetail.test.tsx` instead, following the existing pattern at lines
330–363** (`describe("BacklogItemDetail — Story 2.2.3: VcsWidget wiring")`). That pattern
already does this correctly:

- `useBacklogItemShipStatus` is mocked only at the hook boundary
  (`jest.mock("@/lib/hooks/useBacklogItemShipStatus", ...)`, line 52), returning
  `{ data: create(BacklogItemShipStatusSchema, {...}), loading, refetch }` — a real
  proto object built via `create()` from the generated schema, not a hand-typed shape.
- `fromShipStatus` runs for real inside `BacklogItemDetail`/`VersionControlSection` — nothing
  downstream of the hook is mocked, so the adapter mapping is exercised end-to-end into the
  DOM.
- `useVcsStatus` is separately mocked to `{ data: null, ... }` so the "live status wins over
  historical" branch (also tested at line 395) doesn't shadow the historical/shipped path.

Verified gap: grepping that file for `snapshotCaptureFailed|shippedCheckConclusion|
shippedApprovedCount|fileStats|ShippedFileStat` returns zero hits — none of the existing
`BacklogItemDetail.test.tsx` cases set the shipped-snapshot fields, only `shipped`,
`shippedVia`, `branchName`, `branchExists`. A new test should follow the exact same
`useBacklogItemShipStatusMock.mockReturnValue({ data: create(BacklogItemShipStatusSchema, {
shippedCheckConclusion: ..., shippedApprovedCount: ..., shippedChangesReqCount: ...,
fileStats: [create(ShippedFileStatSchema, {...})], snapshotAt: ..., snapshotCaptureFailed:
... }), ... })` shape used in `adapters.test.ts:132-145`, then assert on rendered text/testids
(check-conclusion badge, approved/changes-requested counts, file stat rows, capture-failed
messaging) the way the existing "Shipped" pill test asserts `screen.getByText("Shipped")`.

**If instead a Playwright e2e test is chosen** (stack.md's other option): `.claude/rules/
e2e-test-conventions.md` requires a `// @feature` header, no `waitForTimeout`, and
`data-testid`/ARIA-only locators. The specific pitfall for *this* feature: an e2e test would
need a real backend-captured snapshot (a `BacklogItem` whose `shipped_*` ent fields are
actually populated) fixtured into the isolated test-mode server
(`STAPLER_SQUAD_TEST_DIR`/`--test-mode`), which is materially more setup than mocking one
hook in Jest — for a Complexity-1 task, the component test above is the proportionate choice
unless e2e fixture support for shipped snapshots already exists (not confirmed in this
research pass).

## 3. Recurrence / re-filing risk if the item is closed without a durable trace

Closing the backlog item as "already implemented" without linking back to this research
(and to commit `aedd80648`) leaves nothing for future automation or a future triager to find
before re-investigating from scratch. Concretely: this repo's `docs/registry/features/`
convention ties frontend features to `filePath`/`tested`/`testIds` per
`.claude/rules/feature-registry.md` — if the shipped-snapshot display isn't represented
there (or is represented with `tested: false`), a future `make registry-generate` pass or a
coverage-gap sweep would flag it as an untested/missing feature again, and someone without
this session's context could easily re-open or re-file the same backlog item, believing the
frontend gap still exists. The closure step should therefore explicitly note the commit and
call chain in the backlog item itself (not just in this SDD project's files, which live in a
worktree), and check whether a `docs/registry/features/frontend/*.json` entry exists for
this display path and needs its `tested`/`testIds` fields updated once the new regression
test lands.
