# Validation Plan: shipped-snapshot-display

**Date**: 2026-08-29

## Happy Path Scenario
Given a backlog item whose live PR has closed (`useVcsStatus` returns no data) but whose ship-status
snapshot was captured with a CI conclusion, review counts, and file stats, when a user opens
`BacklogItemDetail` and expands the Version Control panel, then the CI conclusion badge and review-count
aria-labels render from the durable `BacklogItemShipStatus` fields via `fromShipStatus`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC-1: Five ent-schema shipped-snapshot fields confirmed to reach the UI via `BacklogItemShipStatus` → `useBacklogItemShipStatus` → `fromShipStatus` → `VcsWidgetGithubRow`/`VcsWidget`, with cited commit/test | `project_plans/shipped-snapshot-display/implementation/plan.md` (Task 1.1.1a, citation-only) | N/A — citation verification, not an executable test | Citation/verification | Given `adapters.ts:148-150,163-182` and `BacklogItemDetail.tsx:177-182`, confirm the field mapping is unbroken and cite commit `aedd80648` + existing coverage `adapters.test.ts:130-180` |
| AC-2: `VersionControlSection` confirmed to render as the fallback once the live PR is closed and `useVcsStatus` has no data | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderShippedPillWithViewDiff_When_VcsStatusNullAndShipStatusShipped` (line 331, pre-existing, verified present) | Component (pre-existing) | Given `useVcsStatusMock` returns `{data: null}` and `useBacklogItemShipStatusMock` returns a shipped status, when `BacklogItemDetail` renders and Version Control is expanded, then "Shipped" and the view-diff affordance render from `fromShipStatus`'s fallback branch |
| AC-3a: Shipped item with captured snapshot displays CI conclusion + review counts | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` (new, Task 2.1.1b) | `BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot` | Component (new) | Given a `BacklogItemShipStatusSchema` fixture with `shippedCheckConclusion: "success"`, `shippedApprovedCount: 2`, `shippedChangesReqCount: 1`, `prUrl` and `snapshotAt` set, when the Version Control panel is expanded, then `getByText("CI: success")`, `getByLabelText("2 approved")`, and `getByLabelText("1 changes requested")` are present |
| AC-3b: Snapshot-capture-failure renders the "couldn't capture" copy | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` (new, Task 2.1.1c) | `BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue` | Component (new) | Given the same fixture shape with `snapshotAt` unset and `snapshotCaptureFailed: true` (driving `fromShipStatus`'s `hasSnapshot` to false), when the panel is expanded, then `getByText("Couldn't capture PR status at ship time")` is present |
| AC-4: Backlog item 9832b7e3-edf8-469f-af79-e128604904f6 annotated/closed as already-implemented | Backlog service (no repo file) | Task 3.2.1a — annotation + closure via backlog tooling, with explicit `item_id` guard against a mis-bound `backlog:*` skill | Verification/process step | Given the item is open and filed against `useBacklogService.ts`, when Task 3.2.1a's annotation (commit `aedd80648`, corrected call chain, plan link, new test names) is written and the item is closed, then the item's record shows a closed/resolved status discoverable without re-reading this SDD project |

## UX Acceptance Tests
N/A — no new user-facing surface; existing UI is already shipped and covered by `VcsWidgetGithubRow.test.tsx` (CI badge, review-count aria-labels, and capture-failure copy at the leaf-component level; see plan.md's Corrected Finding). The two new tests in this plan close the remaining wiring-level gap (adapter → hook → component), not a new UI surface.

## Test Stack
- **Unit/Component**: Jest 30 + React Testing Library (existing web-app setup)
- **Integration**: N/A for this task — the two new tests are wiring-level component tests (real adapter + real component tree, mocked RPC hooks), not a separate integration tier
- **E2E / UX**: N/A for this task (deferred per plan.md's Unresolved Questions — optional secondary `tests/e2e/vcs-widget.spec.ts` extension, explicitly not required for AC-3)

## Coverage Targets and How to Measure
| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail.test"` | 2 new tests passing (`BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot`, `BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue`), existing suite (including the pre-existing `..._When_VcsStatusNullAndShipStatusShipped` test proving AC-2) unaffected |
| Registry | `make registry-generate` then `git diff docs/registry/coverage-gaps.json` | No new gap entries for `backlog-item-detail` or `vcs-widget` |

- All 4 acceptance criteria from requirements.md mapped to a verification step or a concrete test:
  AC-1 and AC-4 are citation/process verification steps (no executable test applies — Complexity-1
  close-out, not new behavior); AC-2 is covered by a pre-existing passing test; AC-3 is covered by
  the two new tests added in this plan.

## Migration Plan
N/A — complexity 1, no schema changes.
