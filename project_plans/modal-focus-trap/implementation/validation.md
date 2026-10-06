# Validation Plan: modal-focus-trap

**Date**: 2026-08-30

## Happy Path Scenario

Given one of the 7 target modals/dialogs is open, when a keyboard user presses `Tab` repeatedly, then focus cycles among that modal's own focusable elements and never reaches the backgrounded page.

## Requirement → Test Mapping

Naming convention matches this repo's own precedent for these exact files —
`web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx` already uses
`ComponentName_should_ExpectedBehavior_When_Condition` (contrast with
`SessionActionsOverflow.test.tsx`, which uses plain-English `it()` strings; the
`should_..._When_...` form is what the two named-modal test files in this project already
use, so new tests in those files and in the net-new hook/component test files follow it for
consistency).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: `ReviewChangesModal` traps Tab/Shift+Tab; Escape unchanged | `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx` | `ReviewChangesModal_should_wrapFocusToFirstElement_When_TabPressedOnLastElement` | Unit (happy) | `sessionId` set, diff still loading → focusable = `[Terminal link, Close]`; Tab on Close wraps to Terminal link, Shift+Tab on Terminal link wraps to Close (Task 2.1.1b) |
| AC1 | `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx` | `ReviewChangesModal_should_keepFocusOnCloseButton_When_TabPressedAndOnlyCloseButtonIsFocusable` | Unit (edge — **gap in plan**: no adopter test covers `first === last`) | `sessionId` unset (no Terminal link) and diff loading, so the container has exactly one focusable element; Tab and Shift+Tab both leave focus on Close rather than throwing or moving off-container |
| AC1 | `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx` | `ReviewChangesModal_should_callOnClose_When_EscapePressed` | Unit (regression) | Confirms the existing capture-phase `Escape` listener (lines 59-68) is untouched by the `useFocusTrap` wiring |
| AC1, AC5 | `tests/e2e/accessibility.spec.ts` | e2e Tab-loop block for `ReviewChangesModal` (Task 4.1.1a) | E2E | Real browser: open the modal from `BacklogItemDetail`, press Tab up to 30x (existing loop pattern, lines 266-272), assert `document.activeElement` stays within the dialog's `[role="dialog"]` subtree and cycles back to the first element |
| AC2: `BacklogFileBrowserModal` traps Tab/Shift+Tab; Escape unchanged | `web-app/src/components/backlog/__tests__/BacklogFileBrowserModal.test.tsx` | `BacklogFileBrowserModal_should_wrapFocusToLastElement_When_ShiftTabPressedOnFirstElement` | Unit (happy) | Focusable = `[Terminal link, Close, first FileTree row]`; Shift+Tab on Terminal link wraps to the last FileTree row (Task 2.1.2b) |
| AC2 | `web-app/src/components/backlog/__tests__/BacklogFileBrowserModal.test.tsx` | `BacklogFileBrowserModal_should_callOnClose_When_EscapePressed` | Unit (edge/regression) | Existing `Escape` listener (lines 50-59) fires unchanged after the mount-focus effect is removed and `useFocusTrap` takes over initial-focus duty |
| AC2, AC5 | `tests/e2e/accessibility.spec.ts` | e2e Tab-loop block for `BacklogFileBrowserModal` (Task 4.1.1b) | E2E | Same real-browser Tab-loop technique as AC1's e2e test, applied to the file browser dialog |
| AC3: `VaguenessPromptModal` migrated off hand-rolled trap | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` | `VaguenessPromptModal_should_wrapFocusToRefineButton_When_TabPressedOnProceedButton` | Unit (happy) | Focusable = `[refineButtonRef, proceedButtonRef]`; Tab on Proceed wraps to Refine via `useFocusTrap`, replacing the deleted hand-rolled branch (Task 3.1.1b) |
| AC3 | `web-app/src/components/backlog/VaguenessPromptModal.test.tsx` | `VaguenessPromptModal_should_remainOpen_When_EscapePressed` | Unit (edge — guards a deliberate design choice a naive migration could accidentally "fix") | `useFocusTrap` has no Escape handling; confirms the dialog's intentional no-Escape-dismissal behavior survives the swap |
| AC3: `GateVerdictBox` skip-confirm migrated off hand-rolled trap | `web-app/src/components/backlog/GateVerdictBox.test.tsx` | `GateVerdictBox_should_wrapFocusToCancelButton_When_TabPressedOnConfirmButton` | Unit (happy) | Focusable = `[cancelRef, confirmRef]`; Tab on Confirm wraps to Cancel via `useFocusTrap(skipConfirmDialogRef, showSkipConfirm, skipLinkRef)` (Task 3.1.2b) |
| AC3 | `web-app/src/components/backlog/GateVerdictBox.test.tsx` | `GateVerdictBox_should_restoreFocusToSkipLink_When_CancelOrConfirmClicked` | Unit (edge — covers the flagged behavior change) | Plan calls out that focus-restore now also fires on Cancel/Confirm clicks (not just Escape) because those paths now go through the same `showSkipConfirm` transition the hook observes; this is the one behavior delta in the whole plan and needs its own assertion, not just an Escape-restore check |
| AC3: `CommitPushModal` gets its first Tab trap | `web-app/src/components/unfinished/CommitPushModal.test.tsx` (new) | `CommitPushModal_should_wrapFocusToTextarea_When_TabPressedOnCommitButton` | Unit (happy) | Focusable = `[textarea, Cancel, Commit & Push]`; Tab on Commit & Push wraps to the textarea — closes a gap where no Tab handling existed at all (Task 3.1.3b) |
| AC3 | `web-app/src/components/unfinished/CommitPushModal.test.tsx` (new) | `CommitPushModal_should_submitOnCtrlEnter_And_closeOnEscape_When_ExistingHandlersInvoked` | Unit (edge/regression) | Confirms Escape-to-close and Cmd/Ctrl+Enter-to-submit (existing `handleKeyDown`, lines 64-67) are unaffected by adding `useFocusTrap` alongside them |
| AC3: `WorktreeDiffModal` gets its first Tab trap | `web-app/src/components/unfinished/WorktreeDiffModal.test.tsx` (new) | `WorktreeDiffModal_should_wrapFocusToCloseButton_When_TabPressedOnLastStaticElement` | Unit (happy) | Focusable = `[Close]` before the diff resolves; Tab wraps to itself/stays contained (Task 3.1.4b) |
| AC3, AC4 | `web-app/src/components/unfinished/WorktreeDiffModal.test.tsx` (new) | `WorktreeDiffModal_should_includeAsyncRenderedControl_When_TabPressedAfterDiffFetchResolves` | Unit (edge, directly exercises ADR-001) | Mocks a resolved diff fetch so `DiffRenderer` renders a new focusable control after mount; Tab from Close must now reach that control, proving the trap re-queries rather than using a stale mount-time snapshot — this is the plan's only component-level test of the re-query fix (Task 3.1.4b) |
| AC3: `BacklogQueueSection` import dialog gets its first Tab trap | `web-app/src/components/unfinished/BacklogQueueSection.test.tsx` | `BacklogQueueSection_should_wrapFocusWithinImportDialog_When_TabPressedOnLastElement` | Unit (happy) | Opens the import dialog (`data-testid="import-github-issue-button"`), mocked `GitHubIssuePicker` renders `[input, Cancel]`; Tab on Cancel wraps to input (Task 3.1.5b) |
| AC3 | `web-app/src/components/unfinished/BacklogQueueSection.test.tsx` | `BacklogQueueSection_should_notCloseImportDialog_When_EscapePressed` | Unit (edge — guards against scope creep) | Per the plan's explicit Pattern Decision *not* to add Escape-to-close here; confirms Escape is a no-op post-migration, same as pre-migration (this dialog previously had zero keyboard handling of any kind) |
| AC4: `useFocusTrap` re-queries focusable elements per keypress; 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx) unaffected | `web-app/src/lib/hooks/__tests__/useFocusTrap.test.ts` (new — **gap in plan**: no direct hook-level test file exists today; all current coverage is indirect via adopter components) | `useFocusTrap_should_focusDynamicallyAddedElement_When_TabPressedAfterElementRendersPostActivation` | Unit (happy) | Render a container with one button; activate the hook; append a second button to the DOM after activation (simulating async content); press Tab on the first button and assert focus moves to the newly-added second button, not back to the first (proves re-query, isolated from any one component's async-fetch mocking) |
| AC4 | `web-app/src/lib/hooks/__tests__/useFocusTrap.test.ts` (new) | `useFocusTrap_should_keepFocusOnSoleElement_When_OnlyOneFocusableElementExists` | Unit (edge — **gap in plan**: `first === last` never exercised at the hook level) | Container has exactly one focusable element; Tab and Shift+Tab both leave focus on that element without throwing or escaping the container |
| AC4 | `web-app/src/lib/hooks/__tests__/useFocusTrap.test.ts` (new) | `useFocusTrap_should_preventDefaultWithoutMovingFocus_When_NoFocusableElementsExist` | Unit (edge — **gap in plan**: `focusable.length === 0` branch, lines 38-41 of `useFocusTrap.ts`, has zero test coverage today) | Container has no focusable descendants; Tab calls `preventDefault()` and focus does not move to `document.body` or any background element |
| AC4 | 5 existing suites: `ResumeSessionModal`, `WorkspaceSwitchModal`, `TagEditor`, `SessionActionsOverflow`, `DebugMenu` | Regression run (Task 1.1.1b): `npx jest --no-coverage --testPathPatterns="ResumeSessionModal\|WorkspaceSwitchModal\|TagEditor\|SessionActionsOverflow\|DebugMenu"` | Integration (existing suites re-run unmodified against the patched hook) | Proves the ADR-001 requery change is behavior-preserving for all 5 static-content adopters — zero new failures, zero changed assertions needed |
| AC5: new automated tests (not just Axe) prove focus cannot escape via keyboard | `tests/e2e/accessibility.spec.ts` | Tab-loop tests for `ReviewChangesModal` and `BacklogFileBrowserModal` (Tasks 4.1.1a/b, same rows as AC1/AC2) | E2E | Real-browser proof, since Axe's static scan structurally cannot detect a Tab-escape (per `ux.md`/plan.md Phase 4 rationale) |
| AC5 | Manual + PR note (Task 4.1.2a) | Sibling-modal reachability check | Manual | With `ReviewChangesModal` open, confirm the "Files" trigger behind the backdrop is inert (unreachable/no-effect); repeat with `BacklogFileBrowserModal` open and the "Review Changes" trigger — guards against two simultaneous `document`-level Tab listeners being reachable at once, per Story 4.1.2 |
| AC6: `cd web-app && npx jest --no-coverage` passes for every touched file | Full suite | Task 5.1.1c gate | Gate (not a behavior test — see Coverage Targets) | Run after all of the above land; zero failures across the full Jest suite, not just the touched files' own suites |

## UX Acceptance Tests

There is no `design/ux.md` for this project. Per the plan, the accessibility fix's own
Phase 4 verification tasks (e2e Tab-loop tests + the manual sibling-modal check) **are** the
UX acceptance criteria — a keyboard user must never be able to Tab out of an open dialog,
and opening one dialog must not leave a second dialog's trigger reachable underneath it.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Tab/Shift+Tab never moves focus out of `ReviewChangesModal` into the backgrounded page | `tests/e2e/accessibility.spec.ts` | New test in the `Accessibility (WCAG 2.1 AA)` describe block (Task 4.1.1a) | Playwright | 1) `createBacklogItemDirect` a review-status item. 2) Navigate to its detail view via `BacklogPage`. 3) Open `ReviewChangesModal`. 4) Press Tab up to 30x (existing loop pattern, lines 266-272). 5) Assert `document.activeElement`'s closest `[role="dialog"]` ancestor is always the modal, and the sequence eventually cycles back to the first focusable element |
| Tab/Shift+Tab never moves focus out of `BacklogFileBrowserModal` into the backgrounded page | `tests/e2e/accessibility.spec.ts` | New test in the `Accessibility (WCAG 2.1 AA)` describe block (Task 4.1.1b) | Playwright | Same steps as above, opening the file browser dialog instead |
| Opening one of the two named modals does not leave the other modal's trigger keyboard/click-reachable underneath the backdrop | Manual (PR description note, Task 4.1.2a) | Sibling-modal reachability check | Manual click-through | 1) Open `ReviewChangesModal`; attempt to activate the "Files" trigger button behind the backdrop — confirm no effect. 2) Close it, open `BacklogFileBrowserModal`; attempt to activate the "Review Changes" trigger — confirm no effect. 3) Record the result in the PR description; if a gap is found, file a new backlog item rather than expanding this fix's scope (per requirements.md's explicit "no `inert`/`aria-hidden` in this fix" boundary) |

## Test Stack

- **Unit**: Jest + React Testing Library (`@testing-library/react`), matching every existing test file under `web-app/src/components/**/__tests__/` and `web-app/src/lib/hooks/`. `fireEvent.keyDown(document, { key: "Tab" | "Escape", shiftKey?: true })` is the mechanism throughout, since `useFocusTrap` and each modal's own key handlers attach at the `document` level, not the element level.
- **Integration**: Re-running the 5 existing adopter suites unmodified against the patched hook (Task 1.1.1b) — the cheapest possible regression signal for a shared-hook change with no code changes required in those 5 components.
- **E2E / UX**: Playwright, extending `tests/e2e/accessibility.spec.ts`'s existing Tab-loop helper (lines 242-280) rather than introducing a new spec file or technique — this is the only layer that can prove real keyboard reachability end-to-end (Axe's static scan cannot detect a Tab-escape, per `ux.md`/plan.md).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --no-coverage --testPathPatterns="useFocusTrap\|ReviewChangesModal\|BacklogFileBrowserModal\|VaguenessPromptModal\|GateVerdictBox\|CommitPushModal\|WorktreeDiffModal\|BacklogQueueSection"` | Zero failures across every touched/new test file (targeted pre-flight check before the full-suite gate) |
| TypeScript/Jest (full gate, AC6) | `cd web-app && npx jest --no-coverage` | Zero failures repo-wide — this is the literal text of AC6, not a coverage percentage |
| E2E | `cd tests/e2e && npx playwright test accessibility.spec.ts` | Both new Tab-loop tests pass; no `waitForTimeout` used (repo convention, `.claude/rules/e2e-test-conventions.md`) |

- All public methods of `useFocusTrap` (its one exported function, exercised across its 3 parameters — `ref`, `isActive`, `triggerRef`): happy path (wrap-around, re-query) + error/edge paths (zero focusable elements, single focusable element) covered by the new `useFocusTrap.test.ts`.
- All 7 target components: each has at least one Tab-wrap unit test and one regression test proving pre-existing Escape/submit/no-Escape behavior is unchanged.
- External integrations: N/A — this is a pure client-side DOM/keyboard-event fix with no network or backend surface (Migration Plan and Observability Plan in plan.md are both N/A/none for the same reason).
- UX acceptance criteria: both apply-to-named-modals criteria have an e2e test; the sibling-modal-reachability criterion has an explicit manual step with a documented PR-note requirement (not silently skipped).

## Migration Plan

N/A — plan.md's own Migration Plan section is N/A (no schema, data, or persisted-state changes; this is a client-side keyboard-interaction fix only). No migration tests apply.
