# Validation Plan: create-rule-orphaned-fallback

**Date**: 2026-08-03

## Happy Path Scenario
Given a review-queue item whose `escalation_reason_category` metadata key is entirely absent (an orphaned approval deserialized from pre-PR-315 `pending_approvals.json`, i.e. `EscalationCategory` zero-valued and omitted by `json:",omitempty"`) but whose `tool_input_command` metadata key is present, when `ReviewQueuePanel` renders that item, then the "Create Rule" button (`create-rule-session-approval` test id) is visible — because `isCreateRuleEligibleCategory(undefined)` falls through the denylist gate the same way `"no-match"` does.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: button shows when `escalation_reason_category` is absent (orphaned approval) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueue_should_showCreateRuleButton_On_OrphanedApprovalMissingCategory` | Unit | Happy path — item has `tool_input_command` set, no `escalation_reason_category` key at all; asserts `create-rule-session-approval` is in the document (Task 1.1.2a) |
| AC1 (edge) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueue_should_hideCreateRuleButton_On_OrphanedApprovalMissingCommand` | Unit | Edge — orphaned shape (category absent) but `tool_input_command` is *also* absent; asserts button stays hidden, proving the fix only widens the category leg of the `&&` gate and doesn't accidentally drop the `tool_input_command` guard |
| AC1 — Integration | N/A | N/A | — | No data store or external call is involved in button gating; the item is a plain in-memory `ReviewQueueItem` passed through a mocked `useReviewQueueContext()` — Integration test does not apply |
| AC2: button still shows for `escalation_reason_category === "no-match"` (no regression to PR #315 happy path) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `renders create-rule button with intent=secondary when category is no-match` (existing, `ReviewQueuePanel.test.tsx:1026-1045`) | Unit | Happy path — pre-existing fixture with `escalation_reason_category: "no-match"` and `tool_input_command` set; re-run unmodified after Task 1.1.1c to confirm `isCreateRuleEligibleCategory("no-match")` still returns eligible (Task 1.1.2b) |
| AC2 (edge) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueue_should_hideCreateRuleButton_On_NoMatchCategoryWithoutCommand` | Unit | Edge — `escalation_reason_category: "no-match"` but `tool_input_command` absent; asserts button stays hidden, isolating the category-eligibility change from the pre-existing `tool_input_command` precondition |
| AC2 — Integration | N/A | N/A | — | Same reasoning as AC1 — no data store/external call; N/A |
| AC3: button stays hidden for each of the 5 known non-"no-match" `EscalationCategory` denylist values (`explicit-rule`, `domain-age`, `secret-scan`, `unclassifiable`, `unexpected`) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `omits create-rule button when category is domain-age` (existing, `ReviewQueuePanel.test.tsx:1047-1061`) | Unit | Happy path — representative denylist category (`domain-age`) with `tool_input_command` set; asserts `create-rule-session-approval` is `null` (queryBy) |
| AC3 (edge) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `omits create-rule button when category is %s` (`it.each(["explicit-rule","secret-scan","unclassifiable","unexpected"])`, Task 1.1.2c) | Unit | Edge/boundary — parametrized enumeration of the 4 remaining denylist categories, each with `tool_input_command` set; guards against any one of the 5 categories individually regressing to "shown" |
| AC3 — Integration | N/A | N/A | — | Same reasoning as AC1 — N/A |
| AC4: fix is covered by a regression test asserting the specific failure mode (missing/empty category ⇒ visible; explicit non-"no-match" category ⇒ hidden) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `denylist matches exactly the 5 known non-no-match EscalationCategory values` (Task 1.1.2d) | Unit | Happy path — asserts `NON_NO_MATCH_ESCALATION_CATEGORIES` (exported from `ReviewQueuePanel.tsx`) has size 5 and contains exactly `explicit-rule, domain-age, secret-scan, unclassifiable, unexpected` — the denylist-gating membership guard called out in research/pitfalls.md #3 |
| AC4 (edge) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `isCreateRuleEligibleCategory_should_returnTrue_When_CategoryIsUnrecognizedFutureValue` | Unit | Edge — calls `isCreateRuleEligibleCategory("some-future-category")` directly with a string that is neither `"no-match"` nor in the current denylist; asserts it returns `true` (button shown). This documents the accepted residual risk of denylist gating (an unreviewed 6th `EscalationCategory` silently defaults to "shown" instead of "hidden") that the size/membership guard test above exists to catch at review time, per plan.md's Pattern Decisions rationale |
| AC4 — Integration | N/A | N/A | — | Same reasoning as AC1 — N/A |
| AC5: no backend change required unless the chosen fix approach needs one (frontend-only fix chosen; optional backend contract-lock test) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_OmitsEscalationCategoryMetadata_WhenEmpty` (Task 1.2.1a, optional) | Unit (Go) | Happy path — seeds `ApprovalMetadata{EscalationCategory: ""}`, runs `poller.checkSession`, asserts `_, exists := item.Metadata["escalation_reason_category"]` is `false` (key entirely absent) — pins the existing, unchanged backend contract the frontend fix depends on |
| AC5 (edge) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID` (existing, `session/review_queue_poller_test.go:945-988`) | Unit (Go) | Edge/contrast case — already covers the non-empty-category path (`EscalationCategory: "no-match"` → key present with that value); no new test needed here, cited only to show the empty/non-empty contract is fully bracketed by one new test (happy) + one existing test (edge) |
| AC5 — Integration | N/A | N/A | — | No data store beyond the in-memory `pending_approvals.json`-derived `ApprovalMetadata` stub already used by the existing poller test scaffolding; this is exercised as a Go unit test, not a live-server integration test — N/A |

## UX Acceptance Tests
N/A — this fix restores existing button visibility behavior; no new user-facing surface, flow, or screen is introduced. The existing manual smoke-test path (open the app, trigger an escalated approval, confirm the button state) is covered by the Jest component tests above, which assert the same DOM the user would see.

## Test Stack
- **Unit**: Jest 30 + Testing Library 16 (frontend); Go testing package (backend, optional Epic 1.2 only)
- **Integration**: N/A — no data store or external call in scope; `ReviewQueuePanel` is tested with a mocked `useReviewQueueContext()`, and the optional Go test exercises `review_queue_poller.go` against in-memory stubs, not a live approval store or persisted JSON
- **E2E / UX**: N/A — see UX Acceptance Tests note above

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --testPathPatterns="ReviewQueuePanel" --no-coverage` (repo convention favors targeted runs over global coverage threshold per CLAUDE.md) | All new/modified assertions pass; total test count increases by 8 (2 new happy/edge for AC1 + AC2 each = 2 new tests beyond the existing AC2/AC3 happy-path tests, 4 parametrized AC3 hidden-category tests, 1 AC4 denylist-guard test, 1 AC4 unrecognized-category edge test) |
| Go (optional) | `go test ./session/... -run TestReviewQueuePoller_OmitsEscalationCategoryMetadata_WhenEmpty` (requires `make build` first per repo convention) | Passes if Epic 1.2 is included; skip is acceptable per AC5's explicit "no backend change required" |

- All 5 acceptance criteria have ≥1 corresponding test (AC1–AC4: 2 unit tests each; AC5: 1 new + 1 existing Go unit test, optional)
- Existing tests (no-match, domain-age, fallback copy, `TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID`) must still pass — zero regressions
