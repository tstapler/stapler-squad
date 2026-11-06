# Validation Plan — Phase 3: AcCriteriaList + Section Grouping + Description Default
**Date**: 2026-10-17

---

## Happy Path Scenario

Given a `review`-status item with 8 acceptance criteria and a non-empty description,
when the user opens the item's detail panel, then:
1. The description is immediately visible (DescriptionSection expanded by default).
2. The acceptance criteria count badge is visible in the group header without expanding.
3. All acceptance criteria are visible without scrolling (default expanded for review).
4. The Sessions section header is visible without scrolling (it is no longer hidden below an
   always-expanded criteria list).
5. Five group labels (Decision, Reference, Monitoring, Audit Trail, Admin) divide the
   15 collapsible sections.

---

## Requirement → Test Mapping (Phase 3)

| Requirement | Test File | Test Name | Type |
|---|---|---|---|
| P3.1.1 — AcCriteriaSection inside CollapsibleGroup | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderAcCriteriaSectionInsideCollapsibleGroup` | Unit |
| P3.1.1 — AcCriteriaSection default-expanded for review | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_DefaultExpandAcCriteriaSection_When_StatusIsReview` | Unit |
| P3.1.1 — AcCriteriaSection default-collapsed for in_progress | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_DefaultCollapseAcCriteriaSection_When_StatusIsInProgress` | Unit |
| P3.1.1 — AcCriteriaList present inside AcCriteriaSection | `AcCriteriaSection.test.tsx` | `AcCriteriaSection_should_RenderAcCriteriaList_Inside_Section` | Unit |
| P3.1.1 — AcCriteriaSection header shows count | `AcCriteriaSection.test.tsx` | `AcCriteriaSection_should_ShowCountInHeader_When_SomeCriteriaAreDone` | Unit |
| P3.2.1 — "Decision" group label for review status | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderDecisionGroupLabel_When_StatusIsReview` | Unit |
| P3.2.1 — "Decision" label absent for idea status | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_NotRenderDecisionGroupLabel_When_StatusIsIdea` | Unit |
| P3.2.1 — "Reference" group label always present | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderReferenceGroupLabel_Always` | Unit |
| P3.2.1 — Group labels are aria-hidden | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_MarkGroupLabelsAsAriaHidden` | Unit |
| P3.3.1 — DescriptionSection default-expanded | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_DefaultExpandDescriptionSection_When_NoStoredPreference` | Unit |
| P3.3.1 — Stored "false" preference collapses Description | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RespectStoredCollapsedPreference_For_DescriptionSection` | Unit |
| P3.4.1 — DescriptionSection before AcCriteriaSection in DOM | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderDescriptionSectionBeforeAcCriteriaSection_In_Reference_Group` | Unit |

---

## UX Acceptance Tests (Phase 3)

| # | Criterion | Test file | Tool |
|---|---|---|---|
| 1 | AcCriteriaList not visible above CollapsibleGroup (no longer between PlanningSection and group) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |
| 2 | Group labels visible in collapsed accordion for review-status item | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |
| 3 | Description visible without expanding anything (first open, no stored prefs) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |
| 4 | Criteria expanded by default for review-status item | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |

---

## Regression Checks

The following existing tests must still pass without modification:

- `BacklogItemDetail.test.tsx` — all existing testid-based queries should find the same
  elements (only their structural position changes, not their testid or content).
- `BacklogItemDetail.regression.test.tsx` — update the Description default-collapsed
  assertion (see Task P3.3.1b).
- `BacklogItemDetail.loadGuard.test.tsx` — no changes expected.
- `BacklogItemDetail.markdown.test.tsx` — no changes expected.
- `AcCriteriaList.tsx` component itself is unchanged; only its containing wrapper changes.

### Test notes

- DOM order assertion for Description before AcCriteria: use
  `descEl.compareDocumentPosition(acEl) & Node.DOCUMENT_POSITION_FOLLOWING`
  (same pattern as P2.2.1b in `validation-phase2.md`).
- Group label aria-hidden: `getByText("Reference")` should have `aria-hidden="true"`.
- For the stored-preference test: set `localStorage.setItem("section-expand-${itemId}-description", "false")`
  before rendering, then assert `aria-expanded="false"`.
