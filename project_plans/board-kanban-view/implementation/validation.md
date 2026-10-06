# Validation Plan: board-kanban-view

**Date**: 2026-08-07

## Happy Path Scenario

Given a workspace with sessions in `ACTIVE`, `PAUSED`, and `STOPPED` states and the dashboard
in List view, when the user clicks the "Board" toggle (or presses `b`), drags an `ACTIVE`
session's `BoardCard` from the "Running" column onto the "Paused" column, then reloads the
browser, then the card renders under "Paused" within 1 second of drop (optimistic,
`updateSession` fired with `{status: SessionStatus.PAUSED}`), and after reload the dashboard
restores Board view (not List) with the session still shown as Paused.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: Toggle switches view w/o reload, filters preserved | `web-app/src/lib/hooks/useSessionViewMode.test.ts` | `useSessionViewMode_should_ReturnListDefault_When_NoStoredValueForWorkspace` | Unit | Happy path |
| AC1: Toggle switches view w/o reload, filters preserved | `web-app/src/lib/hooks/useSessionViewMode.test.ts` | `useSessionViewMode_should_FallBackToListInMemory_When_LocalStorageThrows` | Unit | Error path (localStorage unavailable/throws, e.g. private-browsing quota) |
| AC1: Toggle switches view w/o reload, filters preserved | `web-app/src/components/pane/__tests__/PaneSplitRenderer.viewToggle.test.tsx` | `SessionListPaneBody_should_PreserveSearchQueryAndSelection_When_TogglingListToBoard` | Integration | Toggle click reads/writes real `localStorage`, no `listSessions()` refetch fires |
| AC2: Cards bucket into 4 status columns matching real `SessionStatus`/`SubStatus` | `web-app/src/lib/board/columns.test.ts` | `getBoardColumnKey_should_ReturnNeedsReview_When_StatusActiveAndSubStatusNeedsApproval` | Unit | Happy path |
| AC2: Cards bucket into 4 status columns matching real `SessionStatus`/`SubStatus` | `web-app/src/lib/board/columns.test.ts` | `getBoardColumnKey_should_ReturnRunning_When_StatusUnspecified` | Unit | Error path (defensive fallback for `SESSION_STATUS_UNSPECIFIED`) |
| AC2: Cards bucket into 4 status columns matching real `SessionStatus`/`SubStatus` | N/A | N/A | Integration | N/A — pure function over already-loaded `Session` data, no store/RPC involved |
| AC3: Column header shows a count badge | `web-app/src/components/sessions/BoardColumn.test.tsx` | `BoardColumn_should_RenderCountBadgeWithSessionsSuffix_When_SessionsPresent` | Unit | Happy path (badge text is "N sessions", not a bare number — feeds UX #6) |
| AC3: Column header shows a count badge | `web-app/src/components/sessions/BoardColumn.test.tsx` | `BoardColumn_should_RenderZeroCountBadge_When_ColumnHasNoMatchingSessions` | Unit | Error/edge path |
| AC3: Column header shows a count badge | N/A | N/A | Integration | N/A — derived from already-loaded props, no external call |
| AC4: Legal drag fires `updateSession`/`ResolveApproval` and moves the card | `web-app/src/lib/board/statusForColumnMove.test.ts` | `statusForColumnMove_should_ReturnPaused_When_TargetColumnIsPausedFromActive` | Unit | Happy path |
| AC4: Legal drag fires `updateSession`/`ResolveApproval` and moves the card | `web-app/src/lib/board/statusForColumnMove.test.ts` | `statusForColumnMove_should_ReturnNull_When_TargetIsRunningFromHibernated` | Unit | Error path (needs `resumeHibernatedSession`, not `updateSession` — wrong-RPC guard) |
| AC4: Legal drag fires `updateSession`/`ResolveApproval` and moves the card | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `onDragEnd_should_CallUpdateSessionWithPausedStatus_When_DraggingActiveCardToPausedColumn` | Integration | Component-level: simulated `DragEndEvent` → mocked `updateSession` RPC call assertion |
| AC4: Legal drag fires `updateSession`/`ResolveApproval` and moves the card | `server/services/session_service_test.go` | `TestUpdateSession_should_StopSession_When_TargetIsStoppedFromActive` | Integration | Backend: `UpdateSession` → `Instance.StopByUser()` → `transitionToLocked(Stopped)`, real state machine |
| AC5: Illegal/rejected drag bounces back with visible error | `web-app/src/lib/board/transitions.test.ts` | `isLegalBoardDragForSession_should_ReturnFalse_When_SessionStatusIsCreatingRegardlessOfColumnTable` | Unit | Happy path (client-side legality guard correctly fires) |
| AC5: Illegal/rejected drag bounces back with visible error | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `attemptColumnMove_should_ReturnRejectedByServer_When_UpdateSessionResolvesNull` | Unit | Error path (server rejects via Redux `sessions.error`, not a thrown rejection) |
| AC5: Illegal/rejected drag bounces back with visible error | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `onDragEnd_should_SkipRpcAndBounceCardBack_When_DropTargetIsIllegal` | Integration | Component-level: illegal drop never calls mocked `updateSession`, toast/live-region assertion |
| AC5: Illegal/rejected drag bounces back with visible error | `session/instance_state_test.go` | `TestStopByUser_should_RejectTransition_When_SessionIsRestoring` | Integration | Backend: `StopByUser` returns `ErrInvalidTransition`, `classifyStopErr` maps to `FailedPrecondition` |
| AC6: Swimlane axis reuses `GroupingStrategy` for rows, status stays columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `SessionBoard_should_RenderOneSwimlaneRowPerBranch_When_GroupingStrategyIsBranch` | Unit | Happy path |
| AC6: Swimlane axis reuses `GroupingStrategy` for rows, status stays columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `SessionBoard_should_OmitEmptyGroupRow_When_NoSessionsMatchThatGroupValue` | Unit | Error/edge path |
| AC6: Swimlane axis reuses `GroupingStrategy` for rows, status stays columns | N/A | N/A | Integration | N/A — `groupSessions()` operates on already-loaded client state, no store/RPC |
| AC7: Instant search filters cards across all columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `SessionBoard_should_BucketFromFilteredSessions_When_SearchQueryIsSet` | Unit | Happy path |
| AC7: Instant search filters cards across all columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `SessionBoard_should_ShowEmptyStateInEveryColumn_When_SearchMatchesZeroSessions` | Unit | Error/edge path |
| AC7: Instant search filters cards across all columns | N/A | N/A | Integration | N/A — client-side filter over already-loaded `sessions` prop, no RPC refetch |
| AC8: Bulk select/actions work across columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `SessionBoard_should_ComputeSelectedCountAcrossColumns_When_CardsSelectedInDifferentColumns` | Unit | Happy path |
| AC8: Bulk select/actions work across columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `bulkAction_should_ReportPerSessionOutcome_When_OneOfTwoSelectedSessionsRejectsTransition` | Unit | Error path (partial bulk failure) |
| AC8: Bulk select/actions work across columns | `web-app/src/components/sessions/SessionBoard.test.tsx` | `onPauseAll_should_CallUpdateSessionOncePerSelectedId_When_BulkPauseTriggeredAcrossColumns` | Integration | Mocked `updateSession` RPC called once per selected session ID |
| AC9: Last-used view mode persists per workspace | `web-app/src/lib/hooks/useSessionViewMode.test.ts` | `useSessionViewMode_should_ScopeStorageKeyByCurrentWorkspaceId_When_ModeIsSet` | Unit | Happy path |
| AC9: Last-used view mode persists per workspace | `web-app/src/lib/hooks/useSessionViewMode.test.ts` | `useSessionViewMode_should_NotLeakModeAcrossWorkspaces_When_SwitchingCurrentId` | Unit | Error/edge path (isolation check) |
| AC9: Last-used view mode persists per workspace | `web-app/src/lib/hooks/useSessionViewMode.test.ts` | `useSessionViewMode_should_PersistAndRestoreBoardMode_When_LocalStorageRoundTrips` | Integration | Real `localStorage` (jsdom), not mocked — round-trips a `setItem`/reload cycle |
| AC10: Mobile/touch layout + non-drag `MoveToMenu` fallback | `web-app/src/components/sessions/MoveToMenu.test.tsx` | `MoveToMenu_should_ListOnlyLegalTargetColumns_When_OpenedForRunningColumnCard` | Unit | Happy path |
| AC10: Mobile/touch layout + non-drag `MoveToMenu` fallback | `web-app/src/components/sessions/MoveToMenu.test.tsx` | `MoveToMenu_should_ShowNoMovesAvailableMessage_When_CardIsInCompleteColumn` | Unit | Error/edge path |
| AC10: Mobile/touch layout + non-drag `MoveToMenu` fallback | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `attemptColumnMove_should_ProduceIdenticalOutcomeAsMoveToMenu_When_SameLogicalMoveTriggeredByDragOrMenu` | Integration | Shared `attemptColumnMove` path exercised via both call sites against mocked RPC |
| AC11: Existing List view is unaffected (no regression) | `web-app/src/lib/hooks/useFilteredGroupedSessions.test.ts` | `useFilteredGroupedSessions_should_ProduceIdenticalOutput_When_GivenSameInputsAsPreExtractionSessionList` | Unit | Happy path (extraction is behavior-preserving) |
| AC11: Existing List view is unaffected (no regression) | `web-app/src/lib/hooks/useFilteredGroupedSessions.test.ts` | `useFilteredGroupedSessions_should_ReturnEmptyGroups_When_SessionsArrayIsEmpty` | Unit | Error/edge path |
| AC11: Existing List view is unaffected (no regression) | `web-app/src/components/sessions/SessionList.collapse.test.tsx` (existing suite) | full pre-existing suite, unchanged assertions | Integration | Regression gate: `npx jest --testPathPatterns="SessionList"` passes with zero changed expectations post-extraction |
| AC12: Drop/move into "Complete" requires confirmation (added 2026-08-07, UX-lens triad blocker fix) | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `attemptColumnMove_should_ShowConfirmDialogAndSkipMutation_When_TargetIsComplete` | Unit | Happy path (confirm dialog shown, mutation gated on it) |
| AC12: Drop/move into "Complete" requires confirmation | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `attemptColumnMove_should_ProduceCancelledOutcomeAndNotCallUpdateSession_When_UserCancelsCompleteConfirmation` | Unit | Error/edge path (Cancel → no RPC, card returns to origin) |
| AC12: Drop/move into "Complete" requires confirmation | `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx` | `attemptColumnMove_should_CallStopByUserBranch_When_UserConfirmsCompleteConfirmation` | Integration | Mocked `updateSession` called only after confirmation resolves true |

## UX Acceptance Tests

All specs live under `tests/e2e/`, start with the required `// @feature` header, use
`data-testid`/ARIA locators only, and avoid `waitForTimeout` per
`.claude/rules/e2e-test-conventions.md`.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Toggle preserves search+filter in <300ms, card counts match | `session-board-view.spec.ts` | `toggleBoard_should_PreserveSearchAndFilteredCardCounts_When_SwitchingFromListWithActiveQuery` | Playwright | Set search query + status filter in List, note visible row count, click "Board", assert same count of cards across columns, assert `<300ms` via `expect.poll`/network-idle timing |
| 2. Reload after switching to Board restores Board (no List flash) | `session-board-view.spec.ts` | `reload_should_RestoreBoardView_When_LastModeWasBoardForWorkspace` | Playwright | Toggle to Board, `page.reload()`, assert Board container (`data-testid="session-board"`) is the first view-mode container to become visible, List container never mounts |
| 3. Workspace switch does not carry over view-mode choice | `session-board-view.spec.ts` | `switchWorkspace_should_NotCarryOverViewMode_When_TargetWorkspaceHasNoStoredPreference` | Playwright | Set workspace A to Board, switch to workspace B via workspace switcher, assert B renders List by default |
| 4. `b` while typing in search inserts the letter, no toggle | `session-board-view.spec.ts` | `pressB_should_InsertCharacterNotToggleView_When_SearchBoxIsFocused` | Playwright | Focus search input, type "bug", assert input value contains "bug" and Board/List container unchanged |
| 5. All 4 columns visible/reachable in fixed order, even when empty | `session-board-view.spec.ts` | `board_should_RenderAllFourColumnsInFixedOrder_When_OneOrMoreColumnsAreEmpty` | Playwright | Seed sessions covering only 2 of 4 columns, assert `data-testid="board-column-running|needs_review|paused|complete"` all present in that DOM order |
| 6. Count badge is screen-reader readable as "N sessions" | `session-board-accessibility.spec.ts` | `columnHeader_should_ExposeAccessibleNameWithSessionsSuffix_When_InspectedViaAriaSnapshot` | Playwright | `page.getByRole("status", { name: /\d+ sessions/ })` (or `aria-label` assertion) per column header |
| 7. `PAUSED` and `HIBERNATED` both land in "Paused", distinguished only by status chip | `session-board-view.spec.ts` | `pausedColumn_should_ContainBothPausedAndHibernatedSessions_When_BothStatusesPresent` | Playwright | Seed one `PAUSED` + one `HIBERNATED` session, assert both cards render under the Paused column with distinct status-chip text |
| 8. Running→Paused drag completes in one gesture, card visible within 1s | `session-board-view.spec.ts` | `dragCard_should_LandInPausedColumnWithinOneSecond_When_DraggedFromRunningToPaused` | Playwright | `dragTo()` from drag-handle locator to Paused column locator, `expect(pausedColumn.getByText(title)).toBeVisible({timeout: 1000})` |
| 9. Illegal drag returns card within 1s with a specific-reason toast | `session-board-view.spec.ts` | `dragCard_should_BounceBackWithSpecificReasonToast_When_DroppedOnIllegalColumn` | Playwright | Drag a `STOPPED` (Complete) card onto Needs Review, assert card reappears in Complete within 1s and toast text names the specific rule (not "Error") |
| 10. Drop-target column shows distinct valid/invalid treatment mid-drag | `session-board-view.spec.ts` | `dragCard_should_HighlightValidTargetDifferentlyFromInvalidTarget_When_HoveringDuringDrag` | Playwright | Start drag, hover over a legal column and assert one CSS class/attribute, hover over an illegal column and assert a distinguishable one |
| 11. Card never appears in two columns or zero columns, including on forced network failure | `session-board-view.spec.ts` | `dragCard_should_AppearInExactlyOneColumn_When_NetworkFailsDuringMutation` | Playwright | `page.context().setOffline(true)` mid-drag, drop on a legal column, assert card count across all 4 columns for that session ID is exactly 1 after settling |
| 12. Every card has a `Tab`-reachable non-drag Move-to control | `session-board-view.spec.ts` | `moveToMenu_should_BeReachableByTabAloneAndOperableByKeyboard_When_NoMouseUsed` | Playwright | `page.keyboard.press("Tab")` repeatedly to reach trigger, `Enter` to open, `ArrowDown`+`Enter` to select, assert move completes with zero mouse events |
| 13. Complete-column card's menu shows "No moves available" or is absent, never blank | `session-board-view.spec.ts` | `moveToMenu_should_ShowNoMovesAvailableOrBeAbsent_When_CardIsInCompleteColumn` | Playwright | Open menu on a Complete-column card, assert either the explicit message text or that the trigger is absent — assert menu is never open-but-empty |
| 14. Move-to and drag produce identical end state | `session-board-view.spec.ts` | `moveToMenuAndDrag_should_ProduceIdenticalEndState_When_PerformingTheSameLogicalMove` | Playwright | Perform Running→Paused via drag on card A, via `MoveToMenu` on card B, assert both land in Paused with matching toast/announcement pattern |
| 15. Focus lands on card/trigger in new column after successful Move-to | `session-board-accessibility.spec.ts` | `moveToMenu_should_PlaceFocusOnCardInNewColumn_When_MoveSucceeds` | Playwright | Complete a `MoveToMenu` move via keyboard, assert `document.activeElement` (via `page.evaluate`) is within the target column's DOM subtree, `Tab` afterward moves to the next element in that context |
| 16. Live region announces pick-up/move/illegal-rejection/server-rejection distinctly | `session-board-accessibility.spec.ts` | `liveRegion_should_AnnounceDistinctTextForEachOfFourOutcomeTypes_When_DragLifecycleEventsFire` | Playwright | Assert `[role="status"][aria-live="polite"]` text content changes uniquely for pickup, `{type:"moved"}`, `{type:"rejected_illegal"}`, `{type:"rejected_by_server"}` (mock the server-rejection path) |
| 17. Column header/badge text meets 4.5:1 contrast in light+dark theme | `session-board-accessibility.spec.ts` | `columnHeaderAndBadge_should_MeetMinimumContrastRatio_When_CheckedInLightAndDarkTheme` | Playwright (axe-core) | Run Axe Core against the rendered board in both themes, assert zero `color-contrast` violations on column headers/badges |
| 18. Every card control has a visible, non-color-only focus indicator | `session-board-accessibility.spec.ts` | `cardControls_should_ShowVisibleFocusIndicator_When_ReachedViaTab` | Playwright | Tab to drag handle, Move-to trigger, and select checkbox in turn; assert a non-color focus style (outline/box-shadow) via computed style snapshot |
| 19. Branch grouping adds one row per branch, all still show 4 columns | `session-board-swimlanes.spec.ts` | `boardSwimlanes_should_ShowFourColumnsPerRow_When_GroupingByBranch` | Playwright | Select "Branch" grouping, count swimlane rows × 4 against visible column-shell count |
| 20. Multi-tag session visible in both matching tag rows simultaneously | `session-board-swimlanes.spec.ts` | `boardSwimlanes_should_ShowSameSessionInBothTagRows_When_SessionHasTwoTags` | Playwright | Select "Tag" grouping, assert the session title is `visible` in both matching row locators at once |
| 21. Search narrows to 2/4 columns, others show empty state, badges reflect filtered counts | `session-board-search-bulk.spec.ts` | `search_should_ShowEmptyStateInNonMatchingColumnsAndFilteredBadgeCounts_When_QueryMatchesTwoOfFourColumns` | Playwright | Type a query matching sessions in only 2 columns, assert the other 2 show empty-state text and every badge equals its visible card count |
| 22. Cross-column 2-select shows exact "2 selected", bulk action applies to both regardless of column | `session-board-search-bulk.spec.ts` | `bulkSelect_should_ShowExactSelectedCount_When_SelectingOneCardInRunningAndOneInNeedsReview` | Playwright | Select one card each in Running and Needs Review, assert bar text is exactly "2 selected", click Pause, assert both sessions' status chips update |
| 23. Selection survives Board→List switch | `session-board-search-bulk.spec.ts` | `viewToggle_should_PreserveSelectionCount_When_SwitchingFromBoardToListWithTwoSelected` | Playwright | Select 2 cards in Board, toggle to List, assert bulk-action bar reappears showing the same 2-item selection |
| 24. Empty column shows header, "0" badge, and a "No sessions" message (never a blank area) | `session-board-empty-volume.spec.ts` | `emptyColumn_should_ShowHeaderZeroBadgeAndNoSessionsMessage_When_NoSessionsMatch` | Playwright | Seed a workspace where one column has zero sessions, assert header, "0" badge, and empty-state text are all present |
| 25. 100+-session column scrolls smoothly, badge always shows true total | `session-board-empty-volume.spec.ts` | `highVolumeColumn_should_ShowTrueTotalBadgeRegardlessOfVirtualizedVisibleCount_When_ColumnHas100PlusSessions` | Playwright | Seed 150 sessions in one column, assert badge reads "150" while DOM query shows far fewer mounted card nodes; scroll and assert no dropped frames via a basic scroll-completion check |
| 26. 375px viewport shows one column at a time, swipe snaps cleanly | `session-board-mobile.spec.ts` | `board_should_ShowOneColumnAndSnapOnSwipe_When_ViewportIs375px` | Playwright | `page.setViewportSize({width:375, height:800})`, assert only one `board-column` fully in viewport bounding box, simulate horizontal scroll, assert settled scroll position equals a column boundary |
| 27. Move-to trigger is directly tappable, ≥44×44px, no hover-reveal | `session-board-mobile.spec.ts` | `moveToMenuTrigger_should_MeetMinimumTouchTargetSize_When_ViewportIsMobile` | Playwright | At 375px viewport, measure trigger bounding box via `boundingBox()`, assert width/height ≥ 44px, assert visible without a prior hover event |
| 28. Vertical swipe on card body scrolls the column, does not start a drag | `session-board-mobile.spec.ts` | `verticalSwipeOnCardBody_should_ScrollColumn_When_NotStartedOnDragHandle` | Playwright | Touch-simulate a vertical drag starting on the card body (not the handle), assert column `scrollTop` changed and no drag-pending visual state appeared |
| 29. Full Running→Complete move achievable touch-only, no drag attempted | `session-board-mobile.spec.ts` | `touchOnlyUser_should_CompleteRunningToCompleteMove_When_UsingOnlyTapGestures` | Playwright | At mobile viewport, use only `tap()` calls (no `mouse.down`/`dragTo`) to open `MoveToMenu` and select "Complete", assert final column placement |
| 30. Every interrupted flow returns to a known-good state without a page reload | `session-board-view.spec.ts` | `board_should_SelfResolveToKnownGoodState_When_DragIsCancelledOrRejectedOrNetworkFails` | Playwright | Table-driven: (a) `Escape` mid-drag, (b) illegal-drop bounce-back, (c) offline-mode network failure — assert after each that the board shows no stuck pending state and all sessions are visible, without calling `page.reload()` |
| 31. First load shows column-shell skeletons, never empty-state copy | `session-board-view.spec.ts` | `board_should_ShowSkeletonPlaceholders_When_RenderedBeforeInitialSessionsFetchResolves` | Playwright | Delay the mocked `watchSessions` response, assert skeleton elements are present and "No sessions here" text is absent during the delay |
| 32. Toggle buttons expose `aria-pressed` state | `session-board-accessibility.spec.ts` | `viewToggle_should_ExposeAriaPressedState_When_EitherViewIsActive` | Playwright | Assert active button has `aria-pressed="true"`, inactive has `"false"`, for both List-active and Board-active states |
| 33. View switch fires a live-region announcement naming the new view and count | `session-board-accessibility.spec.ts` | `viewToggle_should_AnnounceNewViewAndSessionCount_When_SwitchingListToBoard` | Playwright | Toggle List→Board, assert the shared `[aria-live]` region's text matches `/Board view, showing \d+ sessions/` |
| Drop into "Complete" shows a confirm dialog; Cancel leaves the card in place | `session-board-view.spec.ts` | `dropIntoComplete_should_ShowConfirmDialog_When_UserDropsCardOnCompleteColumn` | Playwright | Drag a Running card onto Complete, assert a confirm dialog appears before any mutation; click Cancel, assert card remains in Running and no toast/status change occurred |

## Test Stack

- **Unit**: Jest + React Testing Library for `web-app/src/lib/board/*.ts` (pure functions:
  `getBoardColumnKey`, `isLegalBoardDrag`/`isLegalBoardDragForSession`, `statusForColumnMove`,
  `DragOutcome` construction) and `web-app/src/lib/hooks/*.ts` (`useSessionViewMode`,
  `useFilteredGroupedSessions`), run via `cd web-app && npx jest --no-coverage
  --testPathPatterns="lib/board|useSessionViewMode|useFilteredGroupedSessions"`. Go unit tests
  for `Instance.StopByUser()`/`classifyStopErr` via `go test ./session/... ./server/services/...`.
- **Integration**: Jest + React Testing Library component tests that exercise multiple modules
  together against mocked RPC boundaries (`SessionBoard.dragdrop.test.tsx`,
  `PaneSplitRenderer.viewToggle.test.tsx`, `MoveToMenu.test.tsx`) — mocking only the network
  boundary (`updateSession`, `resolveApproval`, the Redux `sessions` slice), not the internal
  module wiring. Go integration tests exercise the real state machine
  (`server/services/session_service_test.go`'s `TestUpdateSession_*`, driving
  `Instance.StopByUser()` end-to-end through `transitionToLocked`).
- **E2E / UX**: Playwright specs under `tests/e2e/` (`session-board-view.spec.ts`,
  `session-board-accessibility.spec.ts` w/ axe-core, `session-board-swimlanes.spec.ts`,
  `session-board-search-bulk.spec.ts`, `session-board-empty-volume.spec.ts`,
  `session-board-mobile.spec.ts`), run against the isolated `--test-mode` server per
  `tests/e2e/global-setup.ts`; each file opens with a `// @feature session-board-view` (or
  `session:update` where the backend `Stopped` branch is exercised) header per
  `.claude/rules/e2e-test-conventions.md`.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% branch coverage on `stopByUserLocked`'s new error paths specifically (mirrors the existing `pauseLocked` bar) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line, with `web-app/src/lib/board/*.ts` held to 100% line coverage (pure, easily-testable domain logic — no excuse for gaps) |

- All public service methods (`Instance.StopByUser`, `useFilteredGroupedSessions`,
  `useSessionViewMode`, `attemptColumnMove`): happy path + error paths covered per the mapping
  table above.
- All external integrations (`UpdateSession` RPC, `ResolveApproval` RPC, `localStorage`,
  `watchSessions` live-push race): unit mocked (Jest) + at least one integration test that
  exercises the real boundary (Go `UpdateSession` test against the real state machine; jsdom
  `localStorage` round-trip test).
- UX acceptance criteria: all 30 criteria in `design/ux.md` have a corresponding Playwright test
  in the table above — no criterion is left as "manual step only," since every one of them
  describes a DOM-observable or measurable outcome (contrast ratio, bounding-box size, text
  content, focus target).

## Migration Plan Test — N/A

Per plan.md's own "Migration Plan" section: there is no database/storage schema migration for
this feature. The single backend change (`Instance.StopByUser()` and a new `→ Stopped` branch
inside the existing `UpdateSession` handler) is purely additive Go code — no new proto messages,
no new database columns, no data backfill. The reversible-migration test that would normally
appear in this section is therefore skipped as not applicable; `StopByUser`'s correctness is
instead covered by the Go unit/integration tests in the Requirement → Test Mapping table above
(AC4/AC5 rows), which is the appropriate test category for an additive method, not a migration.
