# Validation Plan — Phase 2: Action Reachability + PR Visibility
**Date**: 2026-10-12

## Happy Path Scenario

Given a backlog item with `status: "done"` and `item.prUrl` set to a GitHub PR URL,
when the user opens the item's detail panel, then the PR link is visible in the sticky
header (LifecycleSummary) without scrolling, and the ActionsSection is the first content
below the sticky header.

---

## Requirement → Test Mapping (Phase 2)

| Requirement | Test File | Test Name | Type |
|---|---|---|---|
| P2.1.1 — PR badge in header, done status | `LifecycleSummary.test.tsx` | `LifecycleSummary_should_RenderPrBadge_When_PrUrlSetAndStatusIsDone` | Unit |
| P2.1.1 — PR badge hidden for pr_pending | `LifecycleSummary.test.tsx` | `LifecycleSummary_should_NotRenderPrBadge_When_StatusIsPrPending` | Unit |
| P2.1.1 — PR badge absent when no prUrl | `LifecycleSummary.test.tsx` | `LifecycleSummary_should_NotRenderPrBadge_When_PrUrlIsUndefined` | Unit |
| P2.1.1 — PR badge renders for archived+prUrl | `LifecycleSummary.test.tsx` | `LifecycleSummary_should_RenderPrBadge_When_PrUrlSetAndStatusIsArchived` | Unit |
| P2.2.1 — ActionsSection before GateBlockingSection | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderActionsSectionBeforeGateBlockingSection_When_FullDetailRenders` | Unit |
| P2.2.1 — PlanVerdictBox immediately before ActionsSection | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderPlanVerdictBoxImmediatelyBeforeActionsSection_When_StatusIsReady` | Unit |
| P2.3.1 — VCS auto-expands for done+prUrl | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_AutoExpandVersionControlSection_When_StatusIsDoneAndPrUrlIsSet` | Unit |
| P2.3.1 — VCS stays collapsed for done+no prUrl | `BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_NotAutoExpandVersionControlSection_When_StatusIsDoneAndPrUrlIsUndefined` | Unit |

## UX Acceptance Tests (Phase 2)

| # | Criterion | Test file | Tool |
|---|---|---|---|
| 1 | PR link visible in sticky header, no scroll, for done item with prUrl | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |
| 2 | ActionsSection visible without scroll for review-status item | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |
| 3 | PR badge absent for pr_pending status (PullRequestSection covers it) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Playwright |

### Test notes

- DOM order assertion: use `el1.compareDocumentPosition(el2) & Node.DOCUMENT_POSITION_FOLLOWING`
  to verify ActionsSection precedes GateBlockingSection without coupling to nth-child counts.
- PR badge link target: assert `getByTestId("lifecycle-pr-badge").closest("a")` has `href`
  matching `item.prUrl` — not just that the element renders.
