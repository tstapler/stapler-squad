# Validation Plan: plan-approval-ux

**Date**: 2026-08-01

## Happy Path Scenario

Given a `ready`-status backlog item with a freshly-generated `plan.md` at
`plan_artifacts_path` and `plan_approved=false`, when the user opens the item
detail view (sees `PlanVerdictBox` reading "Pending review" and the rendered
plan markdown in `PlanArtifactsSection`, auto-expanded) and clicks
"Request Changes" → types a reason → clicks Submit, then `PlanVerdictBox`
transitions to "Changes requested" with the reason persisted and visible, and
a "Regenerate Plan with This Feedback" button appears; clicking it calls
`triggerTriage(item.id, item.planRejectionReason)`, which on completion clears
both `plan_rejection_reason` and resets `plan_approved=false` for the freshly
regenerated (never-yet-reviewed) plan.

---

## Requirement → Test Mapping

### Success Criterion 1 — Visible approval state (5-state persistent indicator)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SC1 happy | `web-app/src/lib/backlog/planReviewStatus.test.ts` | `derivePlanReviewStatus_should_ReturnPendingReview_When_PlanArtifactsPathSetAndNotApprovedNotRejected` | Unit (TS) | Item with `planArtifactsPath` set, `planApproved=false`, no rejection reason → `"pending_review"`. |
| SC1 error/edge | `web-app/src/lib/backlog/planReviewStatus.test.ts` | `derivePlanReviewStatus_should_ReturnSkipped_When_SkipPlanningTrueRegardlessOfOtherFields` | Unit (TS) | `skipPlanning=true` and no plan artifacts → `"skipped"`, not `"no_plan"` (Task 5.1.1's own worked example; precedence order matters). |
| SC1 edge (defensive) | `web-app/src/lib/backlog/planReviewStatus.test.ts` | `derivePlanReviewStatus_should_ReturnChangesRequested_When_RejectionReasonNonEmptyEvenIfApprovedTrue` | Unit (TS) | Stale `planApproved=true` alongside a non-empty `planRejectionReason` (shouldn't happen post-Task 3.1.4, but derivation must stay defensively correct) → `"changes_requested"` wins. |
| SC1 edge | `web-app/src/lib/backlog/planReviewStatus.test.ts` | `derivePlanReviewStatus_should_ReturnNoPlan_When_NoArtifactsPathAndNotSkipped` | Unit (TS) | No plan artifacts, not skipped → `"no_plan"`. |
| SC1 edge | `web-app/src/lib/backlog/planReviewStatus.test.ts` | `derivePlanReviewStatus_should_ReturnApproved_When_PlanApprovedTrueAndNoRejectionReason` | Unit (TS) | Clean approved state. |
| SC1 integration | `web-app/src/components/backlog/PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_RenderMatchingIconAndLabel_When_GivenEachOfFiveStatuses` | Integration (TS, RTL component) | All 5 states render distinguishable icon+label pairs (never color-only, per `BlockerChip.tsx:16-18`). |
| SC1 integration | `web-app/src/components/backlog/PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_SetStatusRoleAndAriaLivePolite_When_Rendered` | Integration (TS, RTL component) | Root carries `role="status" aria-live="polite" aria-atomic="true"`, matching `GateVerdictBox.tsx:253-257`. |

### Success Criterion 2 — Consistent gate (queued vs. ready, detection widened)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SC2 happy | `session/backlog_lifecycle_test.go` | `TestReconcilePlanNotApprovedItems_ReadyStatusStalePlan_MarksStuck` | Unit (Go) | `ready`-status item, plan pending review >5 min (via `PlanArtifactsSetAt` fallback since `QueuedAt` is nil — updated post-Phase-4-patch, see plan.md §10 "Phase 4 Patches Applied"), never queued → marked stuck with `StuckReasonPlanNotApproved` (regression guard for the `MarkStuck` hardcoded-`expectedStatus` fix, Task 8.1.1). |
| SC2 pre-mortem P1 regression | `session/backlog_lifecycle_test.go` | `TestReconcilePlanNotApprovedItems_UnrelatedFieldEditDoesNotResetStaleness` | Integration (Go) | Stale `ready`-status item, edit an unrelated field (e.g. `Title`) which bumps `UpdatedAt` but not `PlanArtifactsSetAt` → detector still fires (Task 8.1.3b; the specific case pre-mortem.md P1 item #2 flagged). |
| SC2 error/edge | `session/backlog_lifecycle_test.go` | `TestSelfHealStuck_PlanNotApproved_ResolvesOnApprovalEvenWhenStatusUnchanged` | Unit (Go) | `ready`-status item marked stuck, then approved without a status transition (`item.Status` stays `"ready"`) → `selfHealStuck` must resolve based on `PlanApproved`/`SkipPlanning`, not a status-anchored condition (Task 8.1.1b fix regression test — the pre-fix bug would leave it stuck forever). |
| SC2 integration | `session/backlog_lifecycle_test.go` | `TestReconcilePlanNotApprovedItems_QueuedStatusStalePlan_StillMarksStuck` | Integration (Go, ent storage + reconciliation sweep) | Seeded `queued`-status item with the same staleness condition, run through the full `ListBacklogItems` → `MarkStuck` → notification path, confirming the widened `Statuses` filter and `item.Status`-passthrough didn't silently break the pre-existing queued-only case. |

### Success Criterion 3 — Reject / request-changes with reason

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SC3 happy | `server/services/backlog_service_test.go` | `TestRejectPlan_HappyPath_SetsReasonAndTimestamp` | Unit (Go) | `RejectPlan(item_id, reason)` on a `ready` item with a pending plan → response has `planRejectionReason` set, non-nil `planRejectedAt`, derives to `changes_requested`. |
| SC3 error | `server/services/backlog_service_test.go` | `TestRejectPlan_EmptyReason_ReturnsInvalidArgument` | Unit (Go) | Whitespace-only/empty `reason` → `CodeInvalidArgument`, no partial write. |
| SC3 error | `server/services/backlog_service_test.go` | `TestRejectPlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition` | Unit (Go) | No plan artifacts exist yet → `CodeFailedPrecondition` ("run TriggerTriage first"). |
| SC3 integration | `server/services/backlog_service_test.go` | `TestRejectPlan_ClearsExistingApproval` | Integration (Go, RPC + storage + spawn gate) | Approve a plan, then reject it → `PlanApproved == false` in both the response and a fresh `GetBacklogItem` read, **and** the backend spawn gate (`SpawnSessionFromItem`) independently confirms it still blocks — the concrete architecture-review.md Blocker 3 case (stale `plan_approved=true` co-existing with a rejection reason must never happen). |
| SC3 integration | `server/services/backlog_service_test.go` | `TestApprovePlan_ClearsExistingRejectionReason` | Integration (Go, RPC + storage) | Reject then approve → `plan_rejection_reason == ""`, closing the reverse gap. |
| SC3 integration | `web-app/src/components/backlog/PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_DisableRejectSubmit_When_ReasonTextIsEmptyOrWhitespace` | Integration (TS, RTL component) | Submit stays `aria-disabled`+`disabled` while trimmed textarea is empty (mirrors `manual-review-summary` guard, `ActionsSection.tsx:302`). |
| SC3 integration | `web-app/src/components/backlog/PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_ShowRegenerateButton_When_StatusIsChangesRequestedOnly` | Integration (TS, RTL component) | "Regenerate Plan with This Feedback" only renders in `changes_requested` state — the ADR-002 two-click flow's second half. |

### Success Criterion 4 — Plan content viewable in-app

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SC4 happy | `server/services/backlog_service_test.go` | `TestGetPlanArtifactContent_HappyPath_ReturnsContentAndMtime` | Unit (Go) | `GetPlanArtifactContent(item_id, "plan.md")` on an item with a valid artifacts dir → returns file text, `sizeBytes`, `modifiedAtUnixMs`. |
| SC4 error | `server/services/backlog_service_test.go` | `TestGetPlanArtifactContent_TraversalAttempt_ReturnsInvalidArgument` | Unit (Go) | `filename: "../../../etc/passwd"` → `CodeInvalidArgument` via `isAllowedPlanArtifactFilename` + `resolveAndValidatePath`, never resolved as a client-supplied path (security-critical error path). |
| SC4 error | `server/services/backlog_service_test.go` | `TestGetPlanArtifactContent_DisallowedFilename_ReturnsInvalidArgument` | Unit (Go) | Filename outside the allowlist (`plan.md`, `requirements.md`, `validation.md`, `research/*.md`) → `CodeInvalidArgument`. |
| SC4 error | `server/services/backlog_service_test.go` | `TestGetPlanArtifactContent_MissingFile_ReturnsNotFound` | Unit (Go) | Allowlisted filename but file absent on disk → `CodeNotFound` ("may have been moved or deleted"). |
| SC4 integration | `web-app/src/components/backlog/detail/PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_RenderFetchedMarkdownContent_When_ContentLoadsSuccessfully` | Integration (TS, RTL component + mocked RPC hook) | Content fetched via `getPlanArtifactContent` renders as formatted markdown (`react-markdown`+`remark-gfm`) inside `data-testid="backlog-plan-content-rendered"`, not the raw path. |
| SC4 integration | `web-app/src/components/backlog/detail/PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_ShowInlineError_When_FetchFails` | Integration (TS, RTL component) | RPC failure → `InlineError` with `customMessage`, retry/dismiss present. |
| SC3+SC4 cross-artifact BLOCKER regression | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ThreadFetchedMtimeIntoApproveAndReject_When_PlanContentHasLoaded` | Integration (TS, RTL component + mocked RPC hook) | Content fetch reports a mtime via `onMtimeChange`; subsequent Approve/Reject client calls carry that value as `expectedModifiedAtUnixMs`, not `0n` (Task 6.1.5 — the specific gap the cross-artifact-consistency review found: the freshness token was defined server-side but never wired up from the UI). |

### Success Criterion 5 — Line-level feedback capability

**No tests designed.** Per plan.md P6 (§3 Pattern Decisions) and requirements.md's own
"may land as a follow-up" hedge, line-level/section-anchored feedback is **deferred
entirely** to a follow-up project — not built in any reduced form in this pass. The
free-text `RejectPlan.reason` field (tested under SC3 above) is the accepted substitute
for this pass; a full test design belongs to whichever future project implements
heading/paragraph-anchored comments (build-vs-buy.md's recommended shape).

### Foundational correctness fix (precedes SC1/SC2, ships independently — Epic 1)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Approval invalidation on regen | `server/services/backlog_service_test.go` | `TestTriggerTriage_RefineWithFeedback_ResetsPlanApproved` | Unit (Go) | Approve a plan, then `TriggerTriage(item_id, feedback)` regenerates it → `item.PlanApproved == false` afterward, so the spawn gate re-blocks until re-review. |
| Rejection reason not stale after regen | `server/services/backlog_service_test.go` | `TestTriggerTriage_RefineWithFeedback_ClearsRejectionReason` | Integration (Go, async completion path) | Reject a plan with a reason, then regenerate via `TriggerTriage(feedback)` → `item.PlanRejectionReason == ""`, so the freshly-generated plan derives to `pending_review`, not stale `changes_requested` text. |
| Reset on backward transition | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_SendBackToIdea_ClearsRejectionReason` | Integration (Go, transition + storage) | Item in `changes_requested`, sent back to `idea`/`refining` → `plan_rejection_reason` cleared in the same update that already clears `plan_approved`/`plan_artifacts_path`. |

### Optimistic-concurrency token (`expected_modified_at_unix_ms`, backs SC1 + SC4's staleness guarantees)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Stale-token happy/error | `server/services/backlog_service_test.go` | `TestApprovePlan_StaleContentToken_ReturnsFailedPrecondition` | Unit (Go) | `ApprovePlan` called with a mismatched `expected_modified_at_unix_ms` → `CodeFailedPrecondition`. |
| Stale-token happy/error | `server/services/backlog_service_test.go` | `TestRejectPlan_StaleContentToken_ReturnsFailedPrecondition` | Unit (Go) | Same for `RejectPlan`. |
| Fail-closed on missing file | `server/services/backlog_service_test.go` | `TestRejectPlan_PlanFileGoneMidRegeneration_FailsClosed` | Unit (Go) | Non-zero token, `plan.md` deleted/renamed mid-regeneration → `checkPlanArtifactFreshness` fails **closed** (`FailedPrecondition`), not silently passing (adversarial-review.md Blocker remediation). |
| Fail-closed on missing file | `server/services/backlog_service_test.go` | `TestApprovePlan_PlanFileGoneMidRegeneration_FailsClosed` | Unit (Go) | Same for `ApprovePlan`. |

---

## UX Acceptance Tests

Source: `project_plans/plan-approval-ux/design/ux.md` §1–§6 (20 numbered ACs). Most are
covered as Playwright e2e in `tests/e2e/plan-review.spec.ts` (extends the base flow test
from Task 9.3.2/9.3.3); fine-grained accessibility-attribute assertions are covered as
Jest+RTL component tests colocated with the component under test, consistent with how
`GateVerdictBox.test.tsx` covers its own analogous ACs today. Color-contrast ACs rely on
the project's existing Axe Core CI gate (`.claude/... UX analysis CI`, runs on PRs
touching `web-app/src/`) rather than a bespoke per-pixel test.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-1.1 (icon+label paired, never color-only) | `PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_RenderMatchingIconAndLabel_When_GivenEachOfFiveStatuses` | Jest+RTL | Render each of 5 statuses; assert accessible name includes the label text, icon has `aria-hidden="true"`. |
| AC-1.2 (`role="status" aria-live="polite" aria-atomic="true"`) | `PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_SetStatusRoleAndAriaLivePolite_When_Rendered` | Jest+RTL | Query root element attributes on render. |
| AC-1.3 (keyboard: real `<button>`s, Tab + Enter/Space) | `tests/e2e/plan-review.spec.ts` | `plan-verdict-box keyboard access > user can Tab to and activate "Request Changes" via keyboard` | Playwright | Tab from a known preceding element to the button (`getByRole('button', { name: /request changes/i })`), press Enter, assert form opens. |
| AC-1.4 (contrast ≥4.5:1, all 5 variants incl. new `skipped`) | *(Axe Core CI, existing gate)* | — | Axe Core (CI) | No bespoke test; flag `PlanVerdictBox.css.ts`'s net-new `skipped` variant for explicit visual QA on first PR since it's the one color pair not proven compliant elsewhere. |
| AC-2.1 (focus moves into textarea on open) | `PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_MoveFocusToTextarea_When_RequestChangesFormOpens` | Jest+RTL | Click toggle, assert `document.activeElement` is the textarea. |
| AC-2.2 (`aria-disabled` guard enforced in handler, not just attribute) | `PlanVerdictBox.test.tsx` | `PlanVerdictBox_should_NotInvokeOnReject_When_SubmitClickedWithEmptyReason` | Jest+RTL | Force-click submit with empty/whitespace textarea (bypassing the `disabled` attribute via `fireEvent`); assert `onReject` was never called. |
| AC-2.3 (Cancel returns focus to the toggle button — validates the one-line fix ux.md §7.1 recommends) | `tests/e2e/plan-review.spec.ts` | `plan-verdict-box reject form > Cancel returns focus to the "Request Changes" toggle button` | Playwright | Open form, click Cancel, assert focus is on the toggle button (`page.locator('[data-testid="backlog-action-reject-plan"]')` is focused). |
| AC-2.4 (3-click completion) | `tests/e2e/plan-review.spec.ts` | `plan-review flow > user completes request-changes in exactly 3 interactions` | Playwright | Click "Request Changes" → fill textarea → click "Submit"; assert no intermediate confirmation dialog appears and status updates to "Changes requested". |
| AC-3.1 (plain `<div>`, no ARIA widget role) | `PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_RenderMarkdownContainerAsPlainDiv_When_ContentPresent` | Jest+RTL | Assert the `data-testid="backlog-plan-content-rendered"` element has tag `div` and no `role` attribute. |
| AC-3.2 (GFM table contrast ≥4.5:1) | *(Axe Core CI, existing gate)* | — | Axe Core (CI) | No bespoke test; covered by the existing gate once `plan.md` fixtures with tables are in a rendered PR page. |
| AC-3.3 (`data-testid` present iff `content !== null`) | `PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_SetDataTestId_When_ContentIsNotNull` | Jest+RTL | Assert testid absent before fetch resolves, present after. |
| AC-4.1 (`role="status" aria-live="polite"` on stale notice) | `PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_UsePoliteLiveRegion_When_StaleNoticeShown` | Jest+RTL | Trigger a background re-fetch returning a different mtime; assert notice's live-region attributes. |
| AC-4.2 (Reload is primary-styled, keyboard reachable, sole exit) | `tests/e2e/plan-review.spec.ts` | `plan-content-viewer > "Reload" button is keyboard-reachable and applies the newer plan` | Playwright | Simulate a background regeneration (trigger a second `TriggerTriage` while the detail page is open), Tab to "Reload", press Enter, assert content updates. |
| AC-4.3 (old content stays intact/un-truncated until Reload) | `PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_PreserveOldContentUnchanged_UntilReloadClicked` | Jest+RTL | Mock a second fetch returning different content+mtime; assert displayed content is unchanged until Reload is clicked, then assert it flips. |
| AC-5.1 (`role="alert" aria-live="assertive"` for genuine failures) | `PlanArtifactsSection.test.tsx` | `PlanArtifactsSection_should_UseAssertiveAlertRole_When_FetchErrorOccurs` | Jest+RTL | Mock RPC rejection; assert `InlineError`'s role/live-region attributes. |
| AC-5.2 (every error state has an explicit exit action) | `tests/e2e/plan-review.spec.ts` | `plan-content-viewer error states > every error surfaces Retry and/or Dismiss with no dead end` | Playwright | Force a fetch failure (e.g. stop the mock backend mid-request or seed an item with a missing plan file), assert Retry and Dismiss are both present and actionable. |
| AC-5.3 (no raw RPC/connect error string leaks to the user) | `tests/e2e/plan-review.spec.ts` | `plan-content-viewer error states > NotFound error shows human-readable copy, not a raw connect error string` | Playwright | Request an item whose `plan.md` was deleted out-of-band; assert the rendered error text does not contain `"not_found:"` or other raw connect-error prefixes. |
| AC-6 (reject ≤3 clicks; regenerate = 1 click) | `tests/e2e/plan-review.spec.ts` | `plan-review flow > reject-with-reason completes in 3 clicks, regenerate completes in 1 click` | Playwright | Full flow: Request Changes → type → Submit (3 interactions) → assert `changes_requested` → click "Regenerate Plan with This Feedback" (1 click) → assert triage re-triggers. |
| AC-7 (no dead ends across all error/notice states) | `tests/e2e/plan-review.spec.ts` | `plan-review flow > no error or notice state is a dead end` | Playwright | Enumerate each flagged state (missing file, failed RPC, stale content, stale token if wired) and assert each exposes at least one of Retry/Dismiss/Reload/Cancel. |
| AC-8 (status visible without user action, all 5 states, Success Criterion 1) | `tests/e2e/plan-review.spec.ts` | `plan-review flow > PlanVerdictBox status is visible immediately on page load for all 5 states` | Playwright | Load item-detail pages for 5 fixture items (one per status) and assert the status text is visible without any click, immediately on render. |

---

## Test Stack

- **Unit (Go)**: standard library `testing` + `github.com/stretchr/testify` (assertion
  library already used throughout `server/services/backlog_service_test.go` and
  `session/backlog_lifecycle_test.go` — confirmed by existing test files in those
  packages, not assumed).
- **Unit (TS)**: Jest + React Testing Library (`@testing-library/react`), matching
  `GateVerdictBox.test.tsx`'s existing pattern.
- **Integration**: Go tests against the ent/SQLite test harness already used in
  `server/services/backlog_service_test.go` (in-memory/temp-file SQLite via the same
  `newTestBacklogService`-style helper the existing `TestApprovePlan_*` suite uses) —
  no new harness needed.
- **E2E / UX**: Playwright (`tests/e2e/`), per `.claude/rules/e2e-test-conventions.md`:
  `data-testid`/ARIA locators only, no `waitForTimeout`, `// @feature` header required
  on every new spec file.

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `cd web-app && npx jest --coverage` | ≥80% line |

- All public service methods (`RejectPlan`, `GetPlanArtifactContent`, extended
  `ApprovePlan`): happy path + error paths covered (see Requirement → Test Mapping
  above — every handler has ≥3 Go tests).
- All external integrations (ent storage writes, spawn-gate cross-checks,
  `TriggerTriage`'s async completion write): unit mocked + at least one integration
  test that exercises the real storage layer (see the "Integration (Go...)" rows above).
- UX acceptance criteria: all 20 numbered ACs in `design/ux.md` have a corresponding
  test or an explicit reliance on an existing CI mechanism (Axe Core), documented above
  — none are silently uncovered.
- Frontend registry (`docs/registry/features/`) and `make registry-generate` /
  `coverage-gaps.json` diff: run as Task 9.4.1's own terminal step, not a test per se,
  but a required gate before this feature is considered shippable.

---

## Migration / Backward-Compatibility Test

Per plan.md §4's "Backward compatibility checkpoint": after Epic 2's ent schema change
(two new nullable columns, `plan_rejection_reason`/`plan_rejected_at`), re-run the
existing `TestApprovePlan_*` suite **unmodified**.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Additive schema doesn't break existing callers | `server/services/backlog_service_test.go` | `TestApprovePlan_*` (existing suite, run unmodified — no new assertions added) | Migration | `ApprovePlanRequest{item_id}` (no `expected_modified_at_unix_ms` set, defaults to `0`) must still succeed exactly as before the schema/proto change, proving the two new nullable columns and the new optional request field didn't alter default behavior for pre-existing callers. |

**Up/down reversibility**: not applicable here. Both new columns are nullable/optional
with no backfill and no default beyond ent's own zero-value handling — SQLite/ent
applies additive nullable columns automatically via `client.Schema.Create` at startup
(the same mechanism every prior additive field on this schema, e.g. `category`,
`pipeline_mode`, already relied on). There is no destructive schema change and therefore
no meaningful "down" path to test; the backward-compatibility checkpoint above is the
complete migration-safety test for this feature.

---

## Summary

- **19 Go tests** (unit + integration) across `server/services/backlog_service_test.go`
  and `session/backlog_lifecycle_test.go`, all named verbatim from plan.md's task list
  (none invented beyond what plan.md already specified).
- **20 TypeScript/Jest tests**: 5 pure-function unit tests (`planReviewStatus.test.ts`),
  6 `PlanVerdictBox.test.tsx` component tests, 9 `PlanArtifactsSection.test.tsx`
  component tests — 5 of the 20 are newly named here (not explicitly named in plan.md's
  task list, which only described them narratively) to reach full AC coverage; the rest
  reuse plan.md's own test descriptions verbatim.
- **9 new Playwright e2e tests** in `tests/e2e/plan-review.spec.ts` (extends the base 2
  tests plan.md's Task 9.3.2/9.3.3 already specifies) plus **1 required regression run**
  of the existing `tests/e2e/plan-gate.spec.ts` unmodified (Task 9.3.1).
- **Requirements coverage: 4/5 success criteria** have designed tests (SC1, SC2, SC3,
  SC4). **SC5 (line-level feedback) has zero tests by design** — explicitly deferred to
  a follow-up project per plan.md's P6 and requirements.md's own scope hedge; this is a
  documented scope cut, not a coverage gap.
- **20/20 UX acceptance criteria** from `design/ux.md` have either a dedicated test or
  an explicit, named reliance on the project's existing Axe Core CI gate (AC-1.4,
  AC-3.2 — contrast checks).
- **1 migration/backward-compatibility test** (re-run of `TestApprovePlan_*`
  unmodified); full up/down reversibility test declared not applicable (additive
  nullable columns only, no destructive change).
