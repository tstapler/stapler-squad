# Implementation Plan: shipped-snapshot-display

**Feature**: Close out the "durable shipped-snapshot fields aren't read by the frontend" backlog
item as already-implemented, add the one real missing regression test (wiring-level, not
adapter/component-level — see Pattern Decisions), and keep the feature registry accurate.
**Date**: 2026-08-29
**Status**: Ready for implementation
**ADRs**: None — see rationale below Pattern Decisions.

---

## Domain Glossary
Omit — Complexity 1, no new domain types.

---

## Corrected Finding (supersedes research/pitfalls.md §2 and research/stack.md §2 on one point)

Both `research/pitfalls.md` and `research/stack.md` state that no test asserts the CI-conclusion
badge, review-count spans, or capture-failure copy at the DOM level. That is **not accurate** —
verified by reading `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.test.tsx:38-92`
directly:

- `VcsWidgetGithubRow_should_RenderApprovedAndChangesRequestedWithAriaLabel_When_GithubPopulated`
  (line 38) asserts `getByLabelText("2 approved")` / `getByLabelText("1 changes requested")`.
- `VcsWidgetGithubRow_should_RenderFullCaptureFailureCopy_When_SnapshotCaptureFailedTrueAndGithubNull`
  (line 65) asserts the exact capture-failure copy with `kind: "historical"`.
- `VcsWidgetGithubRow_should_RenderPartialCaptureFailureCopyAlongsideRealData_When_GithubPartiallyPopulated`
  (line 76) asserts `CI: success` alongside the partial-capture-failure copy.

These are already registered in `docs/registry/features/frontend/vcs-widget.json`'s `testIds`
(lines 33–36), with `"tested": true` — so the registry is *already correct* on this point; no
registry update is needed for `vcs-widget.json`.

**What is genuinely still missing**, confirmed by grepping `BacklogItemDetail.test.tsx` for
`snapshotCaptureFailed|shippedCheckConclusion|shippedApprovedCount|fileStats|ShippedFileStat`
(zero hits): a test that exercises the **real** `fromShipStatus` adapter, fed a **real**
`BacklogItemShipStatus` proto (via `create(BacklogItemShipStatusSchema, {...})`), through the
**real** `useBacklogItemShipStatus` hook boundary mock already wired at
`BacklogItemDetail.test.tsx:51-54`, rendering into the real `VersionControlSection` →
`VcsWidget` → `VcsWidgetGithubRow` chain. `VcsWidgetGithubRow.test.tsx` proves the leaf component
renders correctly given a hand-built `VcsWidgetData`; it does not prove `fromShipStatus` produces
that shape from a real proto, nor that `BacklogItemDetail`'s wiring (prop names, the
`vcsStatus ? ... : shipStatus ? fromShipStatus(shipStatus) : null` branch at
`BacklogItemDetail.tsx:181`) still reaches it. This is the actual residual gap this plan closes —
narrower than research/pitfalls.md's framing, but the same root concern (wiring drift undetected
by adapter/leaf-component tests alone).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Regression test location | New test case in existing `describe("BacklogItemDetail — Story 2.2.3: VcsWidget wiring")` block, `web-app/src/components/backlog/BacklogItemDetail.test.tsx` (after line 401, following the pattern at lines 331–363) | research/pitfalls.md §2 (correct on location; its claim of *zero* related coverage anywhere is corrected above) | `tests/e2e/vcs-widget.spec.ts` extension (research/stack.md's primary recommendation) | e2e requires mocking `GetBacklogItemShipStatus` via `page.route()`, spinning up the isolated Playwright server, and re-deriving CI/review-count locators (`vcs-widget-ci-conclusion`, capture-failed text) — materially more setup for a Complexity-1 task, and it still wouldn't exercise `fromShipStatus` any more directly than the RTL test does (both go through the real adapter; RTL just skips the network/browser layer). Reserved as a "nice to have, not required" secondary — see Unresolved Questions. |
| Regression test location (rejected #2) | — | — | New/extended `web-app/src/components/backlog/detail/VersionControlSection.test.tsx` case | pitfalls.md's explicit rejection stands: that file's `makeWidgetData()` (lines 26–40) hand-builds a `VcsWidgetData` prop directly, bypassing `fromShipStatus` entirely — it would test the same thing `VcsWidgetGithubRow.test.tsx` already tests (leaf rendering given a fixture), not the adapter+hook wiring, which is the actual gap. |
| Regression test location (rejected #3) | — | — | New `VcsWidget.test.tsx` case (build-vs-buy.md's flagged alternative) | Same defect as VersionControlSection.test.tsx: `VcsWidget.test.tsx`'s existing `makeData({ shipped: true, ... })` pattern (per build-vs-buy.md §2) is also a hand-built prop fixture one layer below the adapter — confirms leaf rendering, not wiring. |
| Fixture construction for the new test | `create(BacklogItemShipStatusSchema, {...})` + `create(ShippedFileStatSchema, {...})` from `@bufbuild/protobuf`, matching the exact pattern already used at `BacklogItemDetail.test.tsx:334-339` and `adapters.test.ts:132-145` | research/pitfalls.md §2 | Hand-typed plain object cast to `BacklogItemShipStatus` | A hand-typed object can silently drift from the real generated proto shape (field renames, new required fields) without a compile error if a `Partial<>`/`as` cast is used; `create(Schema, {...})` is checked against the actual generated type and is what every other test in this file already does. |
| Registry update | Add the new test's ID to `docs/registry/features/frontend/ui/backlog-item-detail.json`'s `testIds` array and bump `lastModified`; leave `vcs-widget.json` untouched (already correct, see Corrected Finding) | research/pitfalls.md §3, `.claude/rules/feature-registry.md` | Adding a new per-feature JSON file | The display path is already represented under the existing `backlog-item-detail` (component wiring) and `vcs-widget` (leaf rendering) entries; a new file would fragment one feature across three registry entries for no benefit — `.claude/rules/feature-registry.md`'s "Existing RPC/UI feature" path (update `tested`/`testIds`/`lastModified`) applies here, not "New UI feature." |

**Why no ADR**: Per Step 5 guidance, considered whether "closing a backlog item as
already-implemented without changing product behavior" warrants one, given research/pitfalls.md
§3's recurrence-risk framing. Decided against: the recurrence risk is mitigated by writing the
finding directly into the backlog item (Task 3.1a) and the registry (Task 3.2a), which are the
durable, discoverable records this concern calls for — an ADR would duplicate that without adding
a contestable *technical* decision (the test-location choice is already captured in the Pattern
Decisions table above, which is the actual non-obvious call in this task).

---

## Migration Plan
N/A — complexity 1.

## Observability Plan
N/A — complexity 1.

## Risk Control
N/A — complexity 1.

## Unresolved Questions
- [ ] Whether to also extend `tests/e2e/vcs-widget.spec.ts` with a `snapshotCaptureFailed: true`
      case (stack.md's recommendation) as a *secondary*, lower-priority addition once the primary
      RTL test (Task 2.1a) lands — blocks nothing, not required for Acceptance Criterion 3, which
      is satisfied by the component-level test alone. Owner: whoever picks up this plan; default
      to skipping it unless e2e capacity is free, since it would duplicate coverage the RTL test
      already provides (per Pattern Decisions row 1's rejection reasoning).

## Dependency Visualization

```
Phase 1: Verification write-up
  Story 1.1.1 (confirm + cite call chain in plan — this doc)
      |
      v
Phase 2: Regression test
  Story 2.1.1 (Task 2.1a: add wiring test to BacklogItemDetail.test.tsx)
      |
      v
  Story 2.1.2 (Task 2.1b: run jest, confirm green)
      |
      v
Phase 3: Registry + closure
  Story 3.1.1 (Task 3.1a: update backlog-item-detail.json registry entry)
      |
      v
  Story 3.2.1 (Task 3.2a: annotate/close backlog item 9832b7e3-...)
```

---

## Phase 1: Verification (no code change)

### Epic 1.1: Confirm the existing display path with citations
**Goal**: Record, in a durable artifact, that the originally requested read/display path already
exists and works — satisfying Acceptance Criteria 1 and 2 before any test is written.

#### Story 1.1.1: Cite the call chain and the fallback condition
**As a** future triager re-encountering this backlog item, **I want** the call chain and the
exact fallback condition documented with file:line citations, **so that** I don't re-investigate
from scratch or re-file a duplicate item.
**Acceptance Criteria**:
- The five ent-schema fields are confirmed to reach the UI via `BacklogItemShipStatus` →
  `useBacklogItemShipStatus` → `fromShipStatus` → `VcsWidgetGithubRow`/`VcsWidget`.
  - *Given* `web-app/src/lib/vcs/adapters.ts` lines 148–150 and 163–182 (`fromShipStatus`
    reading `status.shippedCheckConclusion`, `status.shippedApprovedCount`,
    `status.shippedChangesReqCount`, `status.fileStats`, `status.snapshotCaptureFailed`), *When*
    a reader traces the chain against `web-app/src/components/backlog/BacklogItemDetail.tsx`
    line 180 (`useBacklogItemShipStatus`) and line 181 (`fromShipStatus(shipStatus)`), *Then*
    they find a direct, unbroken mapping with no missing field — already true today, no code
    change needed for this criterion.
- `VersionControlSection` is confirmed to render as the fallback once the live PR is closed.
  - *Given* `BacklogItemDetail.tsx` line 177 (`useVcsStatus` returns `null` once the work
    session's worktree is cleaned up) and line 181's ternary, *When* `vcsStatus` is falsy and
    `shipStatus` is non-null, *Then* `vcsWidgetData = fromShipStatus(shipStatus)` and
    `VersionControlSection` (rendered from `BacklogItemDetail` with that `widgetData`) renders
    the historical/shipped branch — already true today, no code change needed.
**Files**: `project_plans/shipped-snapshot-display/implementation/plan.md` (this document — the
citations above constitute the write-up; no source file is touched in this story).

##### Task 1.1.1a: No-op — citations already captured above (~2 min)
- Confirm the two Given-When-Then citations above resolve to the exact lines quoted (already
  verified while writing this plan: `web-app/src/lib/vcs/adapters.ts:148-150,163-182`,
  `web-app/src/components/backlog/BacklogItemDetail.tsx:177,180-181`).
- No file changes. This task exists only to make Acceptance Criteria 1–2 traceable to a task in
  this plan, per the template's requirement that every criterion map to a task.
- Files: none.

---

## Phase 2: Regression test

### Epic 2.1: Wiring-level regression test for the shipped-snapshot render path
**Goal**: Close the one real coverage gap (Acceptance Criterion 3) — prove `fromShipStatus`'s
output, produced from a real `BacklogItemShipStatus` proto, actually reaches the rendered DOM
through `BacklogItemDetail`'s real (non-mocked) adapter call and component tree.

#### Story 2.1.1: Add a shipped-snapshot-with-CI-and-reviews wiring test
**As a** developer refactoring `BacklogItemDetail`, `VersionControlSection`, or `VcsWidget`'s
props in the future, **I want** a test that fails if the shipped-snapshot wiring breaks, **so
that** `adapters.test.ts` and `VcsWidgetGithubRow.test.tsx` passing in isolation can't mask a
broken connection between them.
**Acceptance Criteria**:
- A shipped item with a captured snapshot (`shippedCheckConclusion: "success"`,
  `shippedApprovedCount: 2`, `shippedChangesReqCount: 1`, one `ShippedFileStat`,
  `snapshotCaptureFailed: false`) displays the CI conclusion text and both review-count
  aria-labels once the panel is expanded.
  - *Given* `useVcsStatusMock` returns `{ data: null, ... }` (live worktree gone) and
    `useBacklogItemShipStatusMock` returns `{ data: create(BacklogItemShipStatusSchema, {
    shipped: true, shippedVia: "pr", branchName: "feature/foo", branchExists: false,
    shippedCheckConclusion: "success", shippedApprovedCount: 2, shippedChangesReqCount: 1,
    fileStats: [create(ShippedFileStatSchema, { path: "server/foo.go", status:
    FileStatus.FILE_STATUS_MODIFIED, additions: 5, deletions: 1 })], snapshotAt:
    timestampFromDate(new Date("2026-07-17T10:00:00Z")), snapshotCaptureFailed: false }),
    loading: false, refetch: jest.fn() }`, *When* `BacklogItemDetail` renders and the
    "Version Control" collapsible is expanded (`fireEvent.click(screen.getByTestId(
    "collapsible-header-version-control"))`, matching line 356's existing pattern), *Then*
    `screen.getByText("CI: success")`, `screen.getByLabelText("2 approved")`, and
    `screen.getByLabelText("1 changes requested")` are all present in the DOM.
- A shipped item whose snapshot capture failed shows the "couldn't capture" copy instead.
  - *Given* the same mock setup but with `snapshotAt` left unset and `snapshotCaptureFailed:
    true` — per `adapters.ts:164,178-182`'s `hasSnapshot = status.snapshotAt != null` check and
    its comment ("when both capture groups fail, `status.snapshotAt` can stay nil while
    `snapshotCaptureFailed` is still true"), this makes `fromShipStatus` produce `github: null`
    (via `hasSnapshot` being false, `adapters.ts:174`) with `snapshotCaptureFailed: true`, which
    is exactly the real-world "nothing captured at ship time" case, not a synthetic one — *When*
    the panel is expanded, *Then* `screen.getByText("Couldn't capture PR status at ship time")`
    is present (`VcsWidgetGithubRow.tsx:38-43`'s `!data.github` branch).
**Files**: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 2.1.1a: Add imports for the new fixture types (~3 min)
- In `web-app/src/components/backlog/BacklogItemDetail.test.tsx`, extend the existing import at
  line 23 to also bring in `ShippedFileStatSchema` from `@/gen/session/v1/backlog_pb` and
  `FileStatus` from `@/gen/session/v1/types_pb` (the latter is not yet imported in this file —
  add a new `import { FileStatus } from "@/gen/session/v1/types_pb";` line next to the existing
  `VCSStatusSchema` import at line 22). `timestampFromDate` is already imported at line 19.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 2.1.1b: Write the "CI + reviews render" test case (~5 min)
- Inside `describe("BacklogItemDetail — Story 2.2.3: VcsWidget wiring", ...)` (starts line 330),
  add a new `it("BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot", async () => { ... })`
  after the existing test that ends at line 401, following the exact structure of the test at
  lines 331–363 (mock `useVcsStatusMock` to `{ data: null, ... }`, mock
  `useBacklogItemShipStatusMock` with the fixture from the Given-When-Then above, render, expand
  the collapsible, assert `getByText("CI: success")`, `getByLabelText("2 approved")`,
  `getByLabelText("1 changes requested")`).
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 2.1.1c: Write the "capture failure" test case (~5 min)
- In the same `describe` block, add
  `it("BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue", async () => { ... })`
  mirroring Task 2.1.1b's structure but with `snapshotAt` omitted (left unset) and
  `snapshotCaptureFailed: true` — this drives `fromShipStatus`'s `hasSnapshot` to `false`,
  producing `github: null` (`adapters.ts:174`) — and asserting
  `screen.getByText("Couldn't capture PR status at ship time")`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

#### Story 2.1.2: Verify the new tests pass and don't regress the suite
**As a** reviewer, **I want** proof the new tests actually run and pass, **so that** the PR isn't
merged on an untested claim (per this repo's evidence-and-claims standard).
**Acceptance Criteria**:
- `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail.test"` exits 0 with
  the two new test names listed as passing.
  - *Given* Tasks 2.1.1a–c are complete, *When* that command runs, *Then* the output includes
    `BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot`
    and
    `BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue`
    with a passing status, and the file's pre-existing test count is unchanged/still green.
**Files**: none (verification only).

##### Task 2.1.2a: Run the targeted jest command and confirm green (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail.test"`.
- If either new test fails, fix the fixture/assertion (most likely cause: an aria-label or text
  string mismatch against the real rendered copy — re-check against
  `VcsWidgetGithubRow.tsx` lines 41, 64, 66, 71, 75, 82, 86 for exact strings) and re-run.
- Files: none (or `BacklogItemDetail.test.tsx` if a fix is needed).

---

## Phase 3: Registry update and backlog closure

### Epic 3.1: Keep the feature registry accurate
**Goal**: Satisfy research/pitfalls.md §3's recurrence-risk concern — make sure a future
`make registry-generate` / coverage-gap sweep doesn't re-flag this path as untested.

#### Story 3.1.1: Update the `backlog-item-detail` registry entry
**As a** future coverage-gap sweep, **I want** the new test IDs recorded against the existing
`backlog-item-detail` feature entry, **so that** this display path isn't flagged as a gap again.
**Acceptance Criteria**:
- `docs/registry/features/frontend/ui/backlog-item-detail.json`'s `testIds` array includes both
  new test names and `lastModified` is updated.
  - *Given* the file's current `testIds` array (28 entries, `lastModified:
    "2026-07-24T00:00:00Z"`), *When* Task 3.1.1a runs, *Then* the array has 30 entries including
    `BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot`
    and
    `BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue`,
    and `lastModified` reflects today's date (2026-08-29).
**Files**: `docs/registry/features/frontend/ui/backlog-item-detail.json`

##### Task 3.1.1a: Edit the registry JSON (~3 min)
- Add the two new test name strings to the `testIds` array in
  `docs/registry/features/frontend/ui/backlog-item-detail.json`, and update `"lastModified"` to
  `"2026-08-29T00:00:00Z"`.
- Do not touch `docs/registry/features/frontend/vcs-widget.json` — its existing `testIds`
  (lines 33–36) already correctly reflect `VcsWidgetGithubRow.test.tsx`'s coverage; see Corrected
  Finding above.
- Files: `docs/registry/features/frontend/ui/backlog-item-detail.json`

##### Task 3.1.1b: Run `make registry-generate` and confirm no coverage-gap growth (~3 min)
- Run `make registry-generate` from the repo root.
- Confirm `docs/registry/coverage-gaps.json`'s count does not increase versus its current
  committed state (`git diff docs/registry/coverage-gaps.json` should show no new gap entries for
  `backlog-item-detail` or `vcs-widget`).
- Files: whatever `make registry-generate` regenerates (aggregated JSON only — do not hand-edit
  these).

### Epic 3.2: Close the backlog item as already-implemented
**Goal**: Satisfy Acceptance Criterion 4 — annotate/close backlog item
`9832b7e3-edf8-469f-af79-e128604904f6` with the finding, not as a worked net-new feature.

#### Story 3.2.1: Annotate and close the backlog item
**As a** backlog maintainer, **I want** the item closed with a citation trail, **so that** it
isn't re-opened or re-filed by someone who doesn't have this session's context.
**Acceptance Criteria**:
- Backlog item `9832b7e3-edf8-469f-af79-e128604904f6` is annotated with: (1) the finding that the
  originally-requested path already shipped in commit `aedd80648` (2026-07-18), (2) the corrected
  call chain (`BacklogItemShipStatus` → `useBacklogItemShipStatus` → `fromShipStatus` →
  `VcsWidgetGithubRow`/`VcsWidget` → `VersionControlSection`, not `useBacklogService.ts`), and
  (3) a link/reference to this plan (`project_plans/shipped-snapshot-display/`) and the new test
  names from Epic 2.1, then closed as already-implemented rather than left open or re-worked as a
  feature.
  - *Given* the item is currently open/filed against `useBacklogService.ts`, *When* Task 3.2.1a's
    annotation is written and the closure action is taken, *Then* the item's record shows the
    commit citation, the corrected call chain, and a closed/resolved status — discoverable by
    anyone who opens the item without re-reading this SDD project's files.
**Files**: none (backlog item is managed via the backlog service, not a repo file).

##### Task 3.2.1a: Write the closure annotation and close the item (~4 min)
- Using the backlog item tooling available in this environment (e.g. the `backlog:*` skills or
  the equivalent MCP/CLI surface for item `9832b7e3-edf8-469f-af79-e128604904f6`), write a
  comment/annotation containing: the commit SHA (`aedd80648`), the corrected call chain listed
  above, and a pointer to `project_plans/shipped-snapshot-display/implementation/plan.md` plus the
  two new test names from Epic 2.1. Then close/resolve the item as already-implemented.
- Files: none.
