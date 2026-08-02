# Validation Plan: backlog-description-prominence

**Date**: 2026-08-02

## Happy Path Scenario

Given a backlog item detail view with no stored `backlog-detail-section-${itemId}-description` localStorage preference, when the user opens that item's detail page, then the Description section is rendered already expanded (`aria-expanded="true"`, content visible) with zero clicks, while Acceptance Criteria remains visible and unaffected as it always has been.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: Description expanded by default on first view (no stored preference) | `DescriptionSection.test.tsx` | `DescriptionSection_should_RenderExpanded_When_defaultExpandedTrue` | Unit | Happy path — render with `defaultExpanded={true}`, assert `aria-expanded="true"` and `backlog-description-rendered` present with zero clicks |
| REQ-1: Description expanded by default on first view (no stored preference) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | E2E | Happy path — fresh item, no localStorage key, assert `sectionHeader("description")` starts `aria-expanded="true"` |
| REQ-2: Stored per-item localStorage preference still wins over the new default | `DescriptionSection.test.tsx` | `DescriptionSection_should_RenderCollapsed_When_defaultExpandedFalse` | Unit | `defaultExpanded={false}` explicitly passed (simulates the resolved value `useSectionExpandState` would produce when a stored `"false"` exists) — asserts `aria-expanded="false"` and no rendered markdown testid |
| REQ-2: Stored per-item localStorage preference still wins over the new default | N/A | N/A | Integration | N/A — `useSectionExpandState`'s stored-value precedence logic is pre-existing and explicitly out of scope/unmodified (requirements.md Non-Functional/Constraints); no data store or external call exists in this diff to integration-test. Requirement 2 is proven at the unit layer by showing `defaultExpanded={false}` (the value the hook would resolve to when a stored preference exists) still renders collapsed — i.e., the seed is not unconditionally forced true downstream. |
| REQ-3: Acceptance Criteria unaffected (provable, not just untouched-by-diff) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | E2E | Assert the "Acceptance Criteria" heading stays `toBeVisible()` across all three Description states (initial expanded, collapsed, re-expanded) |
| REQ-4: `DescriptionSection` takes `defaultExpanded` as a prop; stale docstring updated | `DescriptionSection.test.tsx` | `DescriptionSection_should_RenderExpanded_When_defaultExpandedTrue` | Unit | Compiles and renders correctly against the new required `defaultExpanded: boolean` prop (TypeScript would fail to compile the whole suite otherwise) |
| REQ-4: `DescriptionSection` takes `defaultExpanded` as a prop; stale docstring updated | `DescriptionSection.test.tsx` | `DescriptionSection_should_RenderCollapsed_When_defaultExpandedFalse` | Unit | Same — explicit `defaultExpanded={false}` renders collapsed, proving the prop (not an internal hardcode) drives `CollapsibleSection`'s `defaultExpanded` |
| REQ-5: E2E spec updated — new expanded default, collapse/re-expand cycle exercised | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | E2E | Happy path + collapse/re-expand — asserts `aria-expanded="true"` on load, `false` after a direct click on the header, `true` again after `expandSection("description")` |
| REQ-6: No collateral changes; `make quick-check` and targeted Jest suites pass | N/A (process check, not a test file) | `make quick-check`; `npx jest --testPathPatterns="DescriptionSection.test\|BacklogItemDetail.markdown.test\|BacklogItemDetail.test"` | N/A — verification gate, not a test case | Run commands and confirm zero failures; confirm `git status --porcelain docs/registry/` is empty after `make registry-generate` |

## UX Acceptance Tests

Source: plan.md Story 1.1.2, Task 1.1.2c (`tests/e2e/backlog-item-detail-redesign.spec.ts`). No `design/ux.md` exists for this project — these are the plan's own specified e2e assertions, formalized into the validation table per the task instructions.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Description defaults expanded on first view (zero clicks) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | Playwright | 1. Seed a fresh item (no prior `backlog-detail-section-<id>-description` localStorage key). 2. Open the item detail page. 3. Assert `detailPage.sectionHeader("description")` has `aria-expanded="true"` with no click performed. |
| Description can still be collapsed by direct user interaction | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | Playwright | 1. From the expanded state above, click `detailPage.sectionHeader("description")` directly (not `expandSection`, which only opens per `BacklogItemDetailPage.ts:44-49`). 2. Assert `aria-expanded="false"`. |
| Description can be re-expanded after a manual collapse | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | Playwright | 1. From the collapsed state above, call `detailPage.expandSection("description")`. 2. Assert `aria-expanded="true"` again. |
| Acceptance Criteria stays visible throughout every Description state change | `tests/e2e/backlog-item-detail-redesign.spec.ts` | `Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria` | Playwright | 1. Locate the "Acceptance Criteria" heading via `page.getByRole("heading", { name: /Acceptance Criteria/ })`. 2. Assert `toBeVisible()` immediately after page load (Description expanded), again after the collapse click, and again after the re-expand — three checkpoints, one per Description state. |

## Test Stack
- **Unit**: Jest + React Testing Library
- **Integration**: N/A — no data store or external call in this diff
- **E2E / UX**: Playwright (`tests/e2e/backlog-item-detail-redesign.spec.ts`)

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --testPathPatterns="DescriptionSection.test|BacklogItemDetail.markdown.test|BacklogItemDetail.test" --no-coverage` | All 3 targeted suites pass (per requirements.md AC6 — no global coverage threshold change requested) |
| Playwright E2E | `cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts` | The rewritten `"Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria"` test passes; no other test in the spec regresses |
| Full pipeline | `make quick-check` | Build + test + lint all pass (AC6) |
| Registry | `make registry-generate && git status --porcelain docs/registry/` | Empty diff — no registry entries added (AC6) |

- All 6 numbered requirements: happy path covered (error paths N/A where noted — REQ-2's "integration" row is explicitly N/A per the no-data-store constraint, proven instead at the unit layer)
- UX acceptance criteria: each covered by the rewritten e2e spec (4 criteria, 1 test — all four assertions live in the same rewritten Playwright test per plan.md Task 1.1.2c)
- Migration test: N/A — no Migration Plan section exists in plan.md (no schema, data, or persisted-format change)
