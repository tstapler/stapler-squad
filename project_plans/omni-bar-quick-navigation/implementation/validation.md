# Validation Plan: omni-bar-quick-navigation

**Date**: 2026-04-21
**Tech stack**: React + TypeScript (Jest + React Testing Library + Playwright), Go (stdlib testing)
**Note**: FBG Spring Boot testing guidelines do not apply — this is a TypeScript/React frontend with a Go backend.

---

## Requirement → Test Mapping

### REQ-1: Keyboard Navigation Completeness

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: Arrow keys navigate result list | `useModeReducer.test.ts` | `modeReducer_should_returnDiscovery_When_resetToDiscovery` | Unit | Happy path — reset action |
| REQ-1: Arrow keys navigate result list | `useModeReducer.test.ts` | `modeReducer_should_returnCreation_When_detectLocalPath` | Unit | Happy path — creation transition |
| REQ-1: Arrow keys navigate result list | `useModeReducer.test.ts` | `modeReducer_should_notTransition_When_unknownDetectionType` | Unit | Error path — unrecognised detection type stays in discovery |
| REQ-1: scrollIntoView on navigation | `OmnibarResultList.test.tsx` | `OmnibarResultList_should_scrollHighlightedItemIntoView_When_indexChanges` | Component | Happy path — `scrollIntoView` called on highlighted element |
| REQ-1: No scroll when nothing highlighted | `OmnibarResultList.test.tsx` | `OmnibarResultList_should_notScroll_When_highlightedIndexIsNegative` | Component | Error path — index < 0 suppresses scroll |
| REQ-1: Escape first press clears highlight | `Omnibar.test.tsx` | `Omnibar_should_clearHighlight_When_EscapeFirstPress` | Component | Happy path — resultHighlightIndex reset to -1 |
| REQ-1: Escape second press closes | `Omnibar.test.tsx` | `Omnibar_should_close_When_EscapeSecondPress` | Component | Happy path — `onClose` called on second Escape |
| REQ-1: Tab from result opens creation panel | `Omnibar.test.tsx` | `Omnibar_should_openCreationPanel_When_TabPressedOnHighlightedResult` | Component | Happy path — mode transitions to creation_with_repo |

---

### REQ-2: Fast Session Creation Addon (Inline Creation Panel)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-2: Branch auto-slug from session name | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_preFillBranch_When_sessionNameChanges` | Component | Happy path — branch slug updated on name change |
| REQ-2: Slug utility handles edge cases | `slugify.test.ts` | `slugify_should_returnKebabCase_When_givenMixedCaseString` | Unit | Happy path — "My Feature" → "my-feature" |
| REQ-2: Slug utility handles empty input | `slugify.test.ts` | `slugify_should_returnEmpty_When_givenEmptyString` | Unit | Error path — empty string in → empty string out |
| REQ-2: Slug utility strips leading/trailing hyphens | `slugify.test.ts` | `slugify_should_stripLeadingAndTrailingHyphens_When_inputHasSpecialChars` | Unit | Error path — " / feature / " → "feature" |
| REQ-2: Compact view renders by default | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_renderCompactView_When_mounted` | Component | Happy path — only type selector + branch visible |
| REQ-2: Advanced fields hidden by default | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_hideAdvancedFields_When_disclosureIsClosed` | Component | Happy path — name/category/auto-yes not visible |
| REQ-2: Advanced disclosure toggle reveals fields | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_showAdvancedFields_When_disclosureToggled` | Component | Happy path — toggle opens advanced section |
| REQ-2: Enter submits form | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_callOnSubmit_When_EnterPressed` | Component | Happy path — onSubmit callback fires |
| REQ-2: Submit disabled while submitting | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_disableSubmit_When_isSubmittingIsTrue` | Component | Error path — double-submit prevented |
| REQ-2: Arrow keys cycle session type | `SessionTypeRadioGroup.test.tsx` | `SessionTypeRadioGroup_should_cycleToNext_When_ArrowDownPressed` | Component | Happy path — new_worktree → directory |
| REQ-2: Arrow keys cycle backward | `SessionTypeRadioGroup.test.tsx` | `SessionTypeRadioGroup_should_cycleToPrev_When_ArrowUpPressed` | Component | Happy path — directory → new_worktree |
| REQ-2: Session type wraps around | `SessionTypeRadioGroup.test.tsx` | `SessionTypeRadioGroup_should_wrapAround_When_atLastOption` | Component | Error path — wraps from last to first |
| REQ-2: ARIA attributes on radio group | `SessionTypeRadioGroup.test.tsx` | `SessionTypeRadioGroup_should_setAriaChecked_When_optionSelected` | Component | Happy path — role="radio" + aria-checked |
| REQ-2: Tab moves focus out of radio group | `SessionTypeRadioGroup.test.tsx` | `SessionTypeRadioGroup_should_moveFocusOut_When_TabPressed` | Component | Happy path — Tab does not cycle within group |
| REQ-2: Pre-selected repo shown in panel | `OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_displayPath_When_pathPropProvided` | Component | Happy path — path displayed read-only above form |

---

### REQ-3: Mode Modifier Shortcuts

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-3: modeReducer handles open_creation_direct | `useModeReducer.test.ts` | `modeReducer_should_transitionToCreation_When_openCreationDirect` | Unit | Happy path — Cmd+Shift+K action |
| REQ-3: modeReducer handles new_prefix_typed | `useModeReducer.test.ts` | `modeReducer_should_transitionToCreationWithRepo_When_newPrefixTyped` | Unit | Happy path — "new/" typed |
| REQ-3: NewSessionDetector detects "new/" | `detector.test.ts` | `NewSessionDetector_should_detectNewPrefix_When_inputStartsWithNew` | Unit | Happy path — "new/" → InputType.NewSession |
| REQ-3: NewSessionDetector ignores non-prefix | `detector.test.ts` | `NewSessionDetector_should_returnNull_When_inputDoesNotStartWithNew` | Unit | Error path — "stapler" → null |
| REQ-3: NewSessionDetector extracts query | `detector.test.ts` | `NewSessionDetector_should_parseQueryAfterPrefix_When_inputIsNewSlashFoo` | Unit | Happy path — "new/stapler" → parsedValue="stapler" |
| REQ-3: NewSessionDetector empty query | `detector.test.ts` | `NewSessionDetector_should_returnEmptyParsedValue_When_inputIsJustNewSlash` | Unit | Happy path — "new/" → parsedValue="" |
| REQ-3: NewSessionDetector case-insensitive | `detector.test.ts` | `NewSessionDetector_should_detectPrefix_When_inputIsUppercaseNEW` | Unit | Error path — "NEW/thing" still matches |
| REQ-3: Mode badge shows correct label | `OmnibarModeBadge.test.tsx` | `OmnibarModeBadge_should_showJumpLabel_When_inDiscoveryMode` | Component | Happy path |
| REQ-3: Mode badge create label | `OmnibarModeBadge.test.tsx` | `OmnibarModeBadge_should_showCreateLabel_When_inCreationMode` | Component | Happy path |
| REQ-3: Mode badge calls onToggle | `OmnibarModeBadge.test.tsx` | `OmnibarModeBadge_should_callOnToggle_When_inactiveButtonClicked` | Component | Happy path |
| REQ-3: Active badge button not re-triggerable | `OmnibarModeBadge.test.tsx` | `OmnibarModeBadge_should_notCallOnToggle_When_activeButtonClicked` | Component | Error path — active button is no-op |
| REQ-3: ARIA pressed attributes | `OmnibarModeBadge.test.tsx` | `OmnibarModeBadge_should_setAriaPressed_When_modeMatches` | Component | Happy path — aria-pressed=true on active |
| REQ-3: Cmd+Shift+K opens creation mode | `OmnibarContext.test.tsx` | `OmnibarContext_should_openInCreationMode_When_CmdShiftKPressed` | Integration | Happy path — global keydown triggers isOpen + creation |
| REQ-3: Cmd+K unaffected by Cmd+Shift+K | `OmnibarContext.test.tsx` | `OmnibarContext_should_openInDiscoveryMode_When_CmdKPressed` | Integration | Error path — existing shortcut behavior preserved |
| REQ-3: Omnibar switches on new/ input | `Omnibar.test.tsx` | `Omnibar_should_switchToCreationMode_When_newPrefixTyped` | Integration | Happy path — "new/" input triggers mode transition |

---

### REQ-4: Omnibar Action Registry (Architecture)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-4: navigate_session dispatches | `dispatch.test.ts` | `dispatchOmnibarAction_should_callNavigate_When_navigateSessionAction` | Unit | Happy path |
| REQ-4: navigate_session calls close | `dispatch.test.ts` | `dispatchOmnibarAction_should_callClose_When_navigateSessionAction` | Unit | Happy path — modal closes after navigation |
| REQ-4: create_session dispatches | `dispatch.test.ts` | `dispatchOmnibarAction_should_callCreateSession_When_createSessionAction` | Unit | Happy path |
| REQ-4: pause_session dispatches | `dispatch.test.ts` | `dispatchOmnibarAction_should_callPauseSession_When_pauseSessionAction` | Unit | Happy path |
| REQ-4: resume_session dispatches | `dispatch.test.ts` | `dispatchOmnibarAction_should_callResumeSession_When_resumeSessionAction` | Unit | Happy path |
| REQ-4: delete_session dispatches | `dispatch.test.ts` | `dispatchOmnibarAction_should_callDeleteSession_When_deleteSessionAction` | Unit | Happy path |
| REQ-4: clone_session dispatches createSession | `dispatch.test.ts` | `dispatchOmnibarAction_should_callCreateSession_When_cloneSessionAction` | Unit | Happy path — clone = createSession with source path |
| REQ-4: TypeScript exhaustive switch | `types.ts` (compile check) | TypeScript compile error if new variant added without case | Static | TypeScript will not compile if action type added without dispatch case |

---

### REQ-5: Session List Cleanup

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-5: Session card has exactly 4 actions (running) | `SessionCard.test.tsx` | `SessionCard_should_renderExactlyFourActions_When_sessionIsRunning` | Component | Happy path — Open, Pause, Clone, Delete |
| REQ-5: Session card has exactly 4 actions (paused) | `SessionCard.test.tsx` | `SessionCard_should_renderExactlyFourActions_When_sessionIsPaused` | Component | Happy path — Open, Resume, Clone, Delete |
| REQ-5: Fork button not rendered | `SessionCard.test.tsx` | `SessionCard_should_notRenderForkButton_When_rendered` | Component | Error path — fork removed |
| REQ-5: Duplicate button not rendered | `SessionCard.test.tsx` | `SessionCard_should_notRenderDuplicateButton_When_rendered` | Component | Error path — duplicate removed |
| REQ-5: Clone button calls onClone | `SessionCard.test.tsx` | `SessionCard_should_callOnClone_When_cloneButtonClicked` | Component | Happy path |
| REQ-5: Clone button on result row | `OmnibarSessionResult.test.tsx` | `OmnibarSessionResult_should_showCloneButton_When_highlighted` | Component | Happy path — visible on highlighted row |
| REQ-5: Clone button fires callback | `OmnibarSessionResult.test.tsx` | `OmnibarSessionResult_should_callOnClone_When_cloneButtonClicked` | Component | Happy path |
| REQ-5: Clone button stops propagation | `OmnibarSessionResult.test.tsx` | `OmnibarSessionResult_should_notNavigate_When_cloneButtonClicked` | Component | Error path — click doesn't select the session |

---

### REQ-6: TUI Removal

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-6: Go build succeeds after deletion | CI / `make ci` | `go build .` exits 0 | Build | Happy path — no compilation errors |
| REQ-6: Go tests pass after deletion | CI / `make test` | `go test ./...` exits 0 | Build | Happy path — no broken imports |
| REQ-6: bubbletea removed from go.mod | Shell assertion | `grep "bubbletea" go.mod` returns empty | Static | Happy path — dependency gone |
| REQ-6: No tea imports in production code | Shell assertion | `grep -r "bubbletea" --include="*.go" .` returns empty | Static | Happy path — dead imports eliminated |

---

## Test Stack

### Frontend (TypeScript / React)

- **Unit (logic)**: Jest + TypeScript — pure functions, reducers, detectors
- **Component**: Jest + React Testing Library — component rendering, keyboard events, ARIA assertions
- **Integration**: Jest + React Testing Library — multi-component interaction (OmnibarContext + Omnibar + keyboard)
- **E2E** (smoke, optional): Playwright — open omnibar, type "new/", verify creation panel appears

### Backend (Go)

- **Build gate**: `go build .` — compile-time verification of TUI removal
- **Test gate**: `go test ./...` — no regressions from file deletions

---

## File Locations

Tests colocated with source files following the existing project pattern:

```
web-app/src/
  lib/omnibar/
    modes/
      useModeReducer.test.ts        ← REQ-1, REQ-3 (reducer unit tests)
    actions/
      dispatch.test.ts              ← REQ-4 (dispatcher unit tests)
    detector.test.ts                ← REQ-3 (NewSessionDetector — extend existing file)
    slugify.test.ts                 ← REQ-2 (utility unit tests)
  components/sessions/
    Omnibar.test.tsx                ← REQ-1, REQ-2, REQ-3 (Escape, Tab, mode switch)
    OmnibarContext.test.tsx         ← REQ-3 (Cmd+Shift+K global shortcut)
    OmnibarResultList.test.tsx      ← REQ-1 (scrollIntoView)
    OmnibarCreationPanel.test.tsx   ← REQ-2 (form, compact/advanced, submit)
    SessionTypeRadioGroup.test.tsx  ← REQ-2 (arrow key navigation, ARIA)
    OmnibarModeBadge.test.tsx       ← REQ-3 (badge labels, toggle, ARIA)
    OmnibarSessionResult.test.tsx   ← REQ-5 (clone button)
    SessionCard.test.tsx            ← REQ-5 (4 actions, no fork/duplicate)
```

---

## Coverage Targets

- Unit test coverage: all public functions have happy path + at least one error/edge case
- Component coverage: all interactive elements have keyboard + click + ARIA tests
- Action registry: all 6 action types have dispatch tests; TypeScript exhaustiveness verified by compiler
- Mode reducer: all 5 action kinds covered; all 3 state types reachable
- Detector: happy path, null return, case-insensitive, empty query

---

## Phase Quality Gates (from plan.md)

Before each implementation phase ships, run:

```bash
# All phases
npx jest --no-coverage                   # All frontend tests pass
go test ./...                            # All backend tests pass
make lint                                # Lint passes

# Phase 2 specific manual smoke tests
# - Type "new/" → mode badge switches to "Create"
# - Press Cmd+Shift+K → omnibar opens in creation mode
# - Arrow keys navigate list → highlighted item scrolls into view

# Phase 3 manual smoke tests
# - Select repo from results → creation panel appears inline
# - Arrow keys cycle session type radio group
# - Tab moves focus out of radio group (not cycle within)
# - Cmd+Enter creates session from compact panel

# Phase 4 manual smoke test
# - Session card shows exactly: Open, Pause/Resume, Clone, Delete
# - Clone opens omnibar pre-filled with source session path

# Phase 5 build gates
go build .
grep "bubbletea" go.mod   # must return nothing
go test ./...
```
