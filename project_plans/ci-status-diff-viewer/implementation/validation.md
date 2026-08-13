# Validation Plan: ci-status-diff-viewer

**Date**: 2026-08-02

## Happy Path Scenario

Given a session with an associated GitHub PR whose CI is failing
(`GitHubCheckConclusion == "failure"`) and the `review:block-approval-on-ci-failure`
feature flag enabled, when a reviewer opens the session's diff viewer and then attempts
to click "Approve" in the notification panel, then the diff-viewer header shows a
`CIStatusBadge` reading "Failing" (linked to the PR's Checks page) and the Approve
action is rejected with a visible inline explanation ("Approval blocked: CI is failing
on this branch — review before approving.") rather than silently succeeding or
silently doing nothing — proving AC1/AC2/AC5/AC7 work together end to end, with an
"Approve anyway" override available so the block is a speed bump, not a wall (Story
2.2.4). All other acceptance criteria (AC3, AC4, AC6, AC8–AC10) are variations on or
supporting infrastructure for this flow, not independent priorities.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: badge shows passing/failing/pending/no-checks | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_RenderPassingBadge_When_CheckConclusionIsSuccess` | Unit | Happy path — also add sibling cases for `failure`→`prBadgeBlocking`/❌, `pending`→`prBadgePending`/⏳ |
| AC1 | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_RenderNoChecksBadge_When_CheckConclusionIsEmptyOrNeutral` | Unit | Edge — `""`/`"neutral"` both collapse to `prBadgeUnknown`/"No checks" |
| AC1 | `tests/e2e/ci-status-badge.spec.ts` | `ci-status-badge > shows the expected badge text for each CI conclusion fixture` (Task 4.1.1b) | E2E | Happy path — all 4 states rendered from a real session fixture |
| AC2: badge links to Actions run/check page | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_LinkToChecksPage_When_PrUrlProvided` | Unit | Happy path — `href === "${prUrl}/checks"`, `target="_blank" rel="noopener noreferrer"` |
| AC2 | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_OmitHref_When_PrUrlMissing` | Unit | Error path — no `prUrl` on a PR session (shouldn't happen but defend against it) |
| AC2 | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_OpenChecksPageInNewTab_When_Clicked` | E2E | Happy path — click badge, assert new tab/window opens `.../pull/N/checks` |
| AC3: fetched via existing `gh`/GitHub API integration, no new dependency | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_NotInvokeAnyRPCOrFetch_When_Rendering` | Unit | Happy path — spy on the session RPC client / global `fetch`, assert zero calls; component is purely presentational over props |
| AC3 | *(manual — code review, not a test)* | Confirm no new GitHub API client/fetch code added outside `github/client.go`, `github/user_pr_cache.go`, `session/backlog_plugin_github_prs.go` | Checklist | Diff review during PR — this criterion is about absence of new code, not runtime behavior |
| AC4: CI updates reach frontend via `WatchSessions`, not new polling | `session/pr_status_poller_test.go` | `TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange` (Task 3.2.2a) | Unit | Happy path — `onUpdated` fires when only `GitHubCheckConclusion` changes |
| AC4 | `session/pr_status_poller_test.go` | `TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange` (same test, negative assertion) | Unit | Error/regression path — `onUpdated` NOT called when neither priority nor conclusion changed (changed-only-publish guard) |
| AC4 | *(no new integration test)* | Reuses existing `WatchSessions`/EventBus transport, already covered by pre-existing tests; this plan only adds a new *trigger condition* (Task 3.2.1a/b), not new transport | N/A | Verified via AC4's unit test above + the diff-viewer badge live-updating in `tests/e2e/ci-status-badge.spec.ts` |
| AC5: configurable block on manual Approve with visible explanation | `server/services/approval_service_test.go` | `TestResolveApproval_BlocksOnFailingCI_WhenFlagEnabled` (Task 2.2.3a) | Integration | Happy path — flag on, PR CI failing → `CodeFailedPrecondition` returned before `approvalStore.Resolve` |
| AC5 | `server/services/approval_service_test.go` | `TestResolveApproval_AllowsOnFailingCI_WhenFlagDisabled` (Task 2.2.3a) | Integration | Error/negative path — flag off → block never fires, `Resolve` proceeds |
| AC5 | `server/services/approval_service_test.go` | `TestResolveApproval_FailsOpen_WhenStorageLookupErrors` (Task 2.2.3a) | Integration | Error path — storage lookup fails/not-found → fails open (does not block, does not panic) |
| AC5 | `tests/e2e/approval-ci-block.spec.ts` (new) | `ApprovalBlock_should_ShowInlineExplanation_When_ApproveClickedWithFailingCIAndFlagOn` | E2E | Happy path — inline warning text + "View CI run" link render next to Approve/Deny, not a disabled button |
| AC6: `ci_passing` condition combinable with existing conditions | `pkg/classifier/classifier_test.go` | `TestClassify_RequireCIPassing_Success_AutoAllow` (Task 1.1.5a) | Unit | Happy path — `RequireCIPassing: true` + `CIStatus: "success"` → matches |
| AC6 | `pkg/classifier/classifier_test.go` | `TestClassify_RequireCIPassing_Failure_Escalate` (Task 1.1.5a) | Unit | Error path — `CIStatus: "failure"` → falls through to Escalate |
| AC6 | `pkg/classifier/classifier_test.go` | `TestClassify_RequireCIPassing_NoPR_Escalate` (Task 1.1.5a) | Unit | Error path — `CIStatus: ""` (no PR) → falls through |
| AC6 | `pkg/classifier/classifier_test.go` | `TestClassify_RequireCIPassing_CommandPatternAnd_BothMustMatch` (Task 1.1.5b) | Unit | Happy path — AC6's literal example: regex AND `ci_passing` must both hold |
| AC6 | `server/services/approval_handler_test.go` | `TestHandlePermissionRequest_StaleCIStatus_TreatedAsUnknown` (Task 1.1.2c) | Integration | Error path — stale `LastPRStatusCheck` (> 2×poll interval) forces `CIStatus` to `""` even if cached conclusion is `"success"`, closing the stale-CI auto-approve race |
| AC7: no PR → no badge, unaffected by CI-blocking rule | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_ReturnNull_When_PrNumberIsZero` | Unit | Happy path — `prNumber: 0` → renders `null`, no chip |
| AC7 | `server/services/approval_service_test.go` | `TestResolveApproval_UnaffectedWhenNoPR` (Task 2.2.3a) | Integration | Happy path — `GitHubPRNumber == 0` short-circuits the block check entirely |
| AC7 | `tests/e2e/ci-status-badge.spec.ts` | `ci-status-badge > shows no badge (not an empty placeholder) for the no-PR fixture` (Task 4.1.1b) | E2E | Happy path — `getByTestId("ci-status-badge")` absent, not present-but-empty |
| AC8: unit test coverage for `ci_passing` + e2e coverage for badge states | *(cross-reference, not a new test)* | Satisfied by the AC6 unit-test rows above (classifier coverage) + the AC1/AC7 e2e rows above (`tests/e2e/ci-status-badge.spec.ts`, Task 4.1.1b) | Checklist | Confirm both halves exist before considering AC8 done — no standalone test needed |
| AC9: session-creation registry untouched (no new session type) | *(manual — code review, not a test)* | Diff `proto/session/v1/types.proto`'s `SessionType` enum and `session/instance.go`'s `SessionType` constants; confirm neither gained a new value in this plan's changeset | Checklist | This plan introduces no new session creation mode — verified by absence of change, not by a test |
| AC10: feature registry entries + `make registry-generate` clean | *(manual — command run, not a unit test)* | Run `make registry-generate` after Tasks 4.2.1a–c land; diff `docs/registry/coverage-gaps.json` before/after; confirm no unexplained increase | Checklist | Task 4.2.1d in the plan is exactly this verification step |

## UX Acceptance Tests

Source: `design/ux.md`'s "UX Acceptance Criteria" section (14 numbered criteria across
Task completion, Error states, No dead ends, and Accessibility). One test per criterion;
several reuse the same underlying spec/test where the criterion is a different facet of
the same interaction.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Badge visible in 0 clicks at page load | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_BeVisibleImmediately_When_DiffTabOpensWithPR` | Playwright | Navigate to session detail with PR fixture → open Diff tab → assert `getByTestId("ci-status-badge")` visible without any additional interaction |
| 2. View CI run detail in 1 click | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_OpenChecksPageInNewTab_When_Clicked` | Playwright | Click badge → assert a new tab/page opens with URL `.../pull/{n}/checks` |
| 3. Add CI-passing requirement in 2 total actions (checkbox + Save) | `tests/e2e/rule-builder-ci-passing.spec.ts` (new) | `RuleBuilder_should_SubmitRequireCiPassingTrue_When_CheckboxCheckedAndSaved` | Playwright | Open Rule Builder → check "Require CI passing on this branch" → click Save → assert submitted rule has `requireCiPassing: true` (via API response or rule list row) |
| 4. Blocked reviewer sees why Approve failed in 0 additional clicks | `tests/e2e/approval-ci-block.spec.ts` (new) | `ApprovalBlock_should_ShowInlineExplanation_When_ApproveClickedWithFailingCIAndFlagOn` | Playwright | Click Approve on a failing-CI/flag-on fixture → assert inline warning text appears on the same panel, no separate "why?" click needed |
| 5. Block message text + "View CI run" link, not a disabled button | `tests/e2e/approval-ci-block.spec.ts` | `ApprovalBlock_should_OfferViewCIRunLink_When_Blocked` | Playwright | After block fires, assert exact text "Approval blocked: CI is failing on this branch — review before approving." and a clickable "View CI run" link; assert Approve button is still enabled (not `disabled` attribute) |
| 6. No-PR session → badge renders nothing | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_RenderNothing_When_SessionHasNoPR` | Playwright | Navigate to a one-off/no-PR session fixture's Diff tab → assert `getByTestId("ci-status-badge")` is not present |
| 7. Known limitation: "not yet fetched" / "poll failed" / "genuinely no CI" render identically | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_RenderIdenticalNoChecksBadge_When_CIDataMissingRegardlessOfCause` | Playwright | Render badge with 3 distinct fixtures (`checkConclusion: ""`, no `lastChecked`, and stale `lastChecked`) → assert all 3 show the same "No checks" text/class — documents the accepted limitation as a regression guard, not a claim it's fixed |
| 8. Blocked-approve override ("Approve anyway") — resolves the gap `design/ux.md` flagged as "FAILS, as currently planned" | `tests/e2e/approval-ci-block.spec.ts` | `ApprovalBlock_should_AllowApproveAnyway_When_ReviewerOverridesBlock` | Playwright | After block fires, click "Approve anyway" (Task 2.2.4c) → assert approval resolves to "Approved" and no error is shown — confirms Story 2.2.4's `overrideCiBlock` closes the no-scoped-override gap `ux.md`'s Exit-Path Analysis identified |
| 9. Diff badge has no dead end (read-only, non-blocking) | `tests/e2e/ci-status-badge.spec.ts` | `CIBadge_should_OpenInNewTabWithoutNavigatingAwayFromDiffView_When_Clicked` | Playwright | Click badge → assert the original Diff tab/page is still on the same URL (new tab only, no navigation away, nothing to get "stuck" in) |
| 10. Rule-builder checkbox has no dead end (fully reversible) | `tests/e2e/rule-builder-ci-passing.spec.ts` | `RuleBuilder_should_RevertCheckboxState_When_FormCancelled` | Playwright | Check the box → click Cancel → reopen form → assert checkbox is unchecked (state fully reverted, no partial commit) |
| 11. Keyboard navigation — badge is a native `<a>`, Approve/Deny/checkbox are native elements | `tests/e2e/ci-status-badge.spec.ts`, `tests/e2e/approval-ci-block.spec.ts`, `tests/e2e/rule-builder-ci-passing.spec.ts` | `CIBadge_should_BeReachableAndActivatableViaKeyboard_When_Tabbed`; `ApprovalBlock_should_RemainKeyboardOperable_When_ApproveDenyButtonsRendered`; `RuleBuilder_should_BeKeyboardOperable_When_CheckboxFocused` | Playwright | Tab to each element, assert focus ring/visible focus state, press Enter/Space, assert the expected action fires (new tab / approve or deny / checkbox toggles) |
| 12. Screen-reader labels — `role="status"` + `aria-label="CI status: <label>"` | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` | `CIStatusBadge_should_ExposeStatusRoleAndAriaLabel_When_Rendered` | Jest/RTL | Render each conclusion state, assert `getByRole("status")` and `aria-label` text matches `"CI status: <label>"` exactly |
| 13. Color contrast ≥ 4.5:1 (all 4 badge variants, esp. gray-on-gray "No checks") | *(existing CI, no new test)* | Axe Core WCAG AA gate — this repo's UX analysis CI already runs Axe Core on PRs touching `web-app/src/` (per root `CLAUDE.md`) and blocks on violations | Axe Core CI + manual spot-check | Rely on the existing Axe Core gate for automated coverage; add one manual visual spot-check of `prBadgeUnknown` during implementation review since gray-on-gray is the variant most likely to sit near the threshold |
| 14. Color is never the sole signal (glyph + text pairing) | `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx`, `tests/e2e/approval-ci-block.spec.ts` | `CIStatusBadge_should_PairIconWithTextLabel_When_EachStateRenders`; `ApprovalBlock_should_PairWarningIconWithText_When_BlockMessageRenders` | Jest/RTL + Playwright | Assert each of the 4 badge states renders both a distinct glyph (✅/❌/⏳/⬤) and a text label (not glyph-only or color-only); assert the blocked-Approve message pairs ⚠️ with its warning text |

## Test Stack
- **Unit**: Go `testing` + testify (backend), Jest + React Testing Library (frontend)
- **Integration**: Go testing with in-memory storage doubles (`ApprovalService`/`RuleBasedClassifier` tests that exercise `session.Storage` lookups via a fake/stub, not a real GitHub API call)
- **E2E / UX**: Playwright (`tests/e2e/`), per `.claude/rules/e2e-test-conventions.md` — `data-testid`/ARIA locators only, no `waitForTimeout`, every spec starts with a `// @feature` header
- **Accessibility**: existing Axe Core CI gate (runs on PRs touching `web-app/src/`, blocks on WCAG AA violations) — no new tooling needed for UX criterion 13

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this plan (`RuleBasedClassifier.Classify`/`matchesRule`, `ApprovalService.ResolveApproval`, `ApprovalHandler.HandlePermissionRequest`, `PRStatusPoller.applyPRUpdate`): happy path + error paths covered per the table above.
- All external integrations: this plan adds none (AC3) — existing GitHub CI-status fetch code (`github/client.go`, `session/pr_status_poller.go`) is reused as-is, already covered by its own pre-existing tests; this plan's new tests only cover the new *read* call sites, not new fetch logic.
- UX acceptance criteria: all 14 criteria in `design/ux.md` have a corresponding test or documented manual/CI-gate step in the table above; criterion 8 (blocked-approve override) specifically closes the gap `design/ux.md`'s own Exit-Path Analysis flagged as "FAILS, as currently planned" — verify Story 2.2.4 (`overrideCiBlock`/"Approve anyway") actually ships before treating AC8 as passing, not just the base block (Story 2.2.2/2.2.3).
- Run `go test ./pkg/classifier/... ./session/... ./server/services/...` and `cd web-app && npx jest --testPathPatterns="CIStatusBadge"` before considering Phases 1–3 done, per Task 1.1.1e's explicit regression-gate instruction (highest-blast-radius change: `matchesRule`/`classifySingle` run on every tool-call classification in the app).
