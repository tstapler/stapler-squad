# Spec Compliance Sweep: scrolling (SDD Phase 5 step 7)

Branch `stapler-squad-scrolling`, HEAD `f711ef051`. Inputs: `plan.md`, `validation.md`. Scope of diff: `git diff --stat origin/main...HEAD -- web-app/src tests/e2e` = 53 files, 7897 insertions, 263 deletions (no change under `tests/e2e`, none to `TerminalStreamManager.ts` or `useVisibilityResync.ts`).

Evidence basis: names and structure are grep-verified; behavior claims cite the implementing line I grepped or the covering test. Targeted jest run (this sweep): `cd web-app && npx jest --no-coverage --testPathPatterns="terminal|XtermTerminal|useTerminalGestures|useEffectiveScrollMode|ScrollingPanel|ScrollModeChip|ScrollHint|JumpToLatest|useTerminalFlowControl|TerminalOutput"` -> 48 suites passed, 724 tests passed, 0 failed. Passing a test proves the test, not the device behavior.

`ROUTING_VERIFIED`: `grep -n ROUTING_VERIFIED web-app/src/lib/terminal/scrollRouting.ts` -> `29:export const ROUTING_VERIFIED = false;` (also read at `:85`). VERIFIED still false.

## Summary counts

- Acceptance criteria (Phase 1, Phase 2, 3.1.1, 3.1.4): see AC table below; totals at the end of that section.
- Tests: 161 of 168 named tests found by exact name (169 extracted names minus 1 withdrawn). 2 of the 7 absent are present under a renamed title (functionally covered), 5 are absent because their story is skipped/optional (Story 2.1.6 x4, Story 3.1.2 e2e x1).

## 1. Test presence

Extraction command:
`grep -oE '[A-Za-z0-9]+_should_[A-Za-z0-9_]+' project_plans/scrolling/implementation/validation.md project_plans/scrolling/implementation/plan.md | sed 's/^[^:]*://' | sort -u` -> 184 tokens; 15 ending in `_` were truncated wildcard prefixes (e.g. `touchend_should_Tap_*`) and skipped; 2 names added by hand (`scrollLines_should_HaveSingleProductionCaller_InUseTerminalGestures`, `pageAccumulator_should_EmitZeroKeys_When_TravelBelowHalfPageStep`) were already in the set -> 169 names checked.
Check per name: `grep -rnF --include='*.ts' --include='*.tsx' --exclude-dir=node_modules -- "<name>" web-app/src tests/e2e` (first hit recorded). Result: 161 FOUND, 8 MISSING.

### 1a. MISSING (8)

| Name | Status | Explanation |
|---|---|---|
| `touchend_should_DoNothing_When_MovedBetween8PxAndSlop` | WITHDRAWN | plan.md:641 says it is the withdrawn dead-zone test; replaced by `touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms` (found). Not counted in the denominator. |
| `dropPendingWrites_should_EmptyBufferAndCancelScheduledFlush` | CONDITIONAL, not expected | Story 2.1.6 (Q5b-gated, device); not implemented. `grep -rn dropPendingWrites web-app/src` -> no hits. |
| `visibilityVisible_should_DropPendingWritesBeforeResync_When_Connected` | CONDITIONAL, not expected | Story 2.1.6 |
| `visibilityVisible_should_NotDrop_When_Disconnected` | CONDITIONAL, not expected | Story 2.1.6 |
| `visibilityVisible_should_RefitWithVisibilityReason` | CONDITIONAL, not expected | Story 2.1.6 |
| `terminalResize_should_MatchProposeDimensionsAndPaintNonBlank_When_ViewportShrinksAndRestores` | OPTIONAL, not done | Story 3.1.2 (optional, non-blocking); `tests/e2e/terminal-resize.spec.ts` unchanged on this branch (`git diff --stat` shows no e2e file). |
| `jumpToLatest_should_BeHiddenAndDragNoop_When_BufferEmpty` | RENAMED | present as `jumpToLatest_should_BeHidden_When_BufferEmptyOrAtLive` (`JumpToLatestButton.test.tsx:100`). Hidden-when-empty is covered; "drag is a no-op on an empty buffer" has no explicit hook test (see gap G3). |
| `scrollDrag_should_CancelAndResetNetPagesUp_When_ConnectionEpochChanges` | RENAMED | present as `scrollDrag_should_CancelGesture_When_ConnectionEpochChanges` (`useTerminalGestures.test.ts:1166`, asserts no `scrollLines` after the epoch bump) plus `jumpToLatest_should_ResetEstimate_When_ConnectionEpochChanges` (`JumpToLatestButton.test.tsx:259`). The `netPagesUp` reset itself is implemented in `TerminalOutput.tsx:918-923` and in `JumpToLatestButton.tsx:108`; the gesture test does not assert the reset (the hook does not own `netPagesUp`). |

Not in code by design (device/conditional, not expected): validation.md manual rows `UX-3`, `UX-25`, `UX-26` (manual), spike Task 0.1.2f (Q4/Q5), device checklist D1-D11 (see section 4).

### 1b. FOUND (161): name -> file:line (paths relative to `web-app/src/` unless `tests/`)

| Test name | file:line |
|---|---|
| `XtermTerminal_should_PassOverrideAndGestureFlagToHook` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:386 |
| `XtermTerminal_should_ReportScrollModeOnlyOnChange` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:364 |
| `XtermTerminal_should_SetDataGestureScrollAttribute` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:433 |
| `canFit_should_ReturnFalseAndNotFit_When_ContainerClientHeightZero` | lib/terminal/__tests__/postFitRepaint.test.ts:53 |
| `chip_should_BeHidden_When_FewerThanFiveRows` | components/sessions/__tests__/ScrollModeChip.test.tsx:48 |
| `chip_should_BeHidden_When_NoCoarsePointerAndNoTouchSeen` | components/sessions/__tests__/ScrollModeChip.test.tsx:31 |
| `chip_should_HaveAtLeast44pxTarget` | components/sessions/__tests__/ScrollModeChip.test.tsx:65 |
| `chip_should_HighlightOnce_When_LocalDragMovedNothing` | components/sessions/__tests__/ScrollModeChip.test.tsx:99 |
| `chip_should_NameRouteAndGestureState_InAccessibleName` | components/sessions/__tests__/ScrollModeChip.test.tsx:77 |
| `chip_should_NotAnnounce_When_PanelOrPickerOpen` | components/sessions/__tests__/ScrollModeChip.test.tsx:143 |
| `chip_should_NotHighlight_When_TuiRoute` | components/sessions/__tests__/ScrollModeChip.test.tsx:135 |
| `chip_should_OpenPicker_When_Tapped` | components/sessions/__tests__/ScrollModeChip.test.tsx:58 |
| `chip_should_Render_When_TouchstartSeen` | components/sessions/__tests__/ScrollModeChip.test.tsx:37 |
| `chip_should_StillShowRoute_When_GesturesOff` | components/sessions/__tests__/ScrollModeChip.test.tsx:72 |
| `clampLinesPerFrame_should_ClampToRowsBothSigns_When_LargeDelta` | lib/terminal/__tests__/scrollKinematics.test.ts:166 |
| `connectionEpoch_should_Increment_When_ReconnectOrFullSnapshot` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:297 |
| `decideScrollTarget_should_IgnoreTableAndReturnOverride_When_OverrideLocalOrTui` | lib/terminal/__tests__/scrollRouting.test.ts:54 |
| `decideScrollTarget_should_LogUnverified_When_RoutingVerifiedFalse` | lib/terminal/__tests__/scrollRouting.test.ts:73 |
| `decideScrollTarget_should_NeverReturnTuiWheel_When_PolicyNotWheel` | lib/terminal/__tests__/scrollRouting.test.ts:61 |
| `decideScrollTarget_should_ReturnPerTableRow_When_ModeMatchesRow` | lib/terminal/__tests__/scrollRouting.test.ts:41 |
| `decideScrollTarget_should_RouteAlternateToTuiPgkeys_When_DefaultTableAndAuto` | lib/terminal/__tests__/scrollRouting.test.ts:29 |
| `doubleTap_should_SelectWord_When_TwoTapsWithin300msAnd20px` | lib/hooks/__tests__/useTerminalGestures.test.ts:524 |
| `dump_should_Return500NewestEntries_When_600Logged` | lib/terminal/__tests__/mobileDebug.test.ts:36 |
| `encodePageKeys_should_EmitExactToolbarBytes_ForPgUpAndPgDn` | lib/terminal/__tests__/scrollRouting.test.ts:126 |
| `encodeWheel_should_CapReportsPerFrameAtThree_When_LargeLineDelta` | lib/terminal/__tests__/scrollRouting.test.ts:115 |
| `encodeWheel_should_ClampColRowToViewport_When_OutOfRange` | lib/terminal/__tests__/scrollRouting.test.ts:110 |
| `gestureScroll_should_FallBackToOn_When_StoredValueInvalidOrStorageThrows` | lib/terminal/__tests__/scrollOverride.test.ts:77 |
| `gestureScroll_should_PersistAndDefaultOn_When_Unset` | lib/terminal/__tests__/scrollOverride.test.ts:66 |
| `globalCss_should_SetOverscrollBehaviorNoneOnHtmlBody` | components/sessions/__tests__/XtermTerminal.overscroll.test.ts:12 |
| `handleManualResize_should_CallRefit` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:207 |
| `handleTerminalResize_should_NotPassBypass_When_1500msAfterSettleRefit` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:239 |
| `handleTerminalResize_should_PassBypass_When_Within1000msOfSettleRefit` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:225 |
| `hint_should_NotShow_When_SeenFlagSet` | components/sessions/__tests__/TerminalOutput.scrollingUi.test.tsx:247 |
| `hint_should_SetSeenFlagOnDismissOnly` | components/sessions/__tests__/ScrollHint.test.tsx:60 |
| `hint_should_ShowOnceOnFirstDragWithRouteText_And_NotOfferEscOrQ` | components/sessions/__tests__/ScrollHint.test.tsx:32 |
| `hint_should_StayUntilDismissed_And_NotAutoTimeout` | components/sessions/__tests__/ScrollHint.test.tsx:50 |
| `hook_should_DisposeBufferChangeSubscription_When_Unmounted` | lib/hooks/__tests__/useTerminalGestures.test.ts:1607 |
| `hook_should_NeverPreventDefault_When_GestureScrollOff` | lib/hooks/__tests__/useTerminalGestures.test.ts:1274 |
| `hook_should_ReRegisterListeners_When_GestureScrollTurnedOnLive` | lib/hooks/__tests__/useTerminalGestures.test.ts:1280 |
| `hook_should_RegisterNoTouchListeners_When_GestureScrollOff` | lib/hooks/__tests__/useTerminalGestures.test.ts:1268 |
| `hook_should_RegisterTouchEventListeners_AndNoPointerEventListeners_When_Mounted` | lib/hooks/__tests__/useTerminalGestures.test.ts:1238 |
| `hook_should_RemoveAllListenersAndRafs_When_Unmounted` | lib/hooks/__tests__/useTerminalGestures.test.ts:1248 |
| `isAwayFromLive_should_BeTrueOnlyWhenViewportAboveBase` | lib/terminal/__tests__/scrollPosition.test.ts:9 |
| `jumpToLatest_should_AnnounceOncePerEpisode` | components/sessions/__tests__/JumpToLatestButton.test.tsx:176 |
| `jumpToLatest_should_BeHiddenInTui_When_EstimateInvalid` | components/sessions/__tests__/JumpToLatestButton.test.tsx:81 |
| `jumpToLatest_should_BeHidden_When_FewerThanFiveRows` | components/sessions/__tests__/JumpToLatestButton.test.tsx:90 |
| `jumpToLatest_should_HaveNoTransition_When_ReducedMotion` | components/sessions/__tests__/JumpToLatestButton.test.tsx:271 |
| `jumpToLatest_should_HoldPosition_When_OutputArrivesWhileScrolledUp` | components/sessions/__tests__/JumpToLatestButton.test.tsx:165 |
| `jumpToLatest_should_KeepCorner_When_GestureInProgress` | components/sessions/__tests__/JumpToLatestButton.test.tsx:240 |
| `jumpToLatest_should_MoveTopRight_When_CursorRowIntersects` | components/sessions/__tests__/JumpToLatestButton.test.tsx:226 |
| `jumpToLatest_should_NotFocusTerminal_When_Tapped` | components/sessions/__tests__/JumpToLatestButton.test.tsx:116 |
| `jumpToLatest_should_ResetEstimate_When_ConnectionEpochChanges` | components/sessions/__tests__/JumpToLatestButton.test.tsx:259 |
| `jumpToLatest_should_SendAtMostFivePgDn_When_Tapped` | components/sessions/__tests__/JumpToLatestButton.test.tsx:127 |
| `log_should_AddNothingAndNotCallConsoleDebug_When_FlagUnsetAnd1000Calls` | lib/terminal/__tests__/mobileDebug.test.ts:47 |
| `log_should_BufferEntriesAndEmitConsoleDebug_When_FlagTrue` | lib/terminal/__tests__/mobileDebug.test.ts:26 |
| `momentumConstants_should_MatchCanonicalSet` | lib/terminal/__tests__/scrollKinematics.test.ts:63 |
| `momentum_should_CancelAndSkipTapFocus_When_TouchstartDuringCoasting` | lib/hooks/__tests__/useTerminalGestures.test.ts:1452 |
| `momentum_should_Cancel_When_BufferTypeChanges` | lib/hooks/__tests__/useTerminalGestures.test.ts:1583 |
| `momentum_should_Cancel_When_VisualViewportResizeOrOrientationChangeOrUnmount` | lib/hooks/__tests__/useTerminalGestures.test.ts:1536 |
| `momentum_should_DecayBy095AndEndWithin120Frames_When_Released1p2PxPerMs` | lib/terminal/__tests__/scrollKinematics.test.ts:82 |
| `momentum_should_DispatchDecayingScrollLinesAtMostOncePerFrame_When_FlingReleased` | lib/hooks/__tests__/useTerminalGestures.test.ts:1371 |
| `momentum_should_EmitNoFrames_When_BelowMinFlingVelocity` | lib/terminal/__tests__/scrollKinematics.test.ts:110 |
| `momentum_should_EmitNoFrames_When_ReducedMotionTrue` | lib/terminal/__tests__/scrollKinematics.test.ts:103 |
| `momentum_should_NeverDispatchTwiceInOneFrame_When_FlingRunsSixtyFrames` | lib/hooks/__tests__/useTerminalGestures.test.ts:1403 |
| `momentum_should_ResetStateAndCoastFlag_When_TouchcancelDuringCoasting` | lib/hooks/__tests__/useTerminalGestures.test.ts:1496 |
| `momentum_should_StopAtEdge_When_ViewportYAtBoundary_InLocalBuffer` | lib/hooks/__tests__/useTerminalGestures.test.ts:1624 |
| `momentum_should_StopAtPageCap_When_TuiTarget` | lib/hooks/__tests__/useTerminalGestures.test.ts:1649 |
| `momentum_should_StopImmediately_When_Cancelled` | lib/terminal/__tests__/scrollKinematics.test.ts:117 |
| `netPagesUp_should_BeInvalid_When_KeystrokeResizeModeChangeReconnectOrOverFive` | lib/terminal/__tests__/scrollPosition.test.ts:29 |
| `netPagesUp_should_FloorAtZero_When_MorePgDnThanPgUp` | lib/terminal/__tests__/scrollPosition.test.ts:17 |
| `onContextLoss_should_FallBackAndRepaint` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:259 |
| `pageAccumulator_should_CapPagesAndRateLimit_When_Fling` | lib/terminal/__tests__/scrollRouting.test.ts:166 |
| `pageAccumulator_should_EmitZeroKeys_When_TravelBelowHalfPageStep` | lib/terminal/__tests__/scrollRouting.test.ts:144 |
| `panel_should_RenderAsOverlay_When_InlineWouldLeaveFewerThanFiveRows` | components/sessions/__tests__/ScrollingPanel.test.tsx:139 |
| `panel_should_UseNativeRadiosInLabelledFieldsetWithDescriptions_And_AnnounceChange` | components/sessions/__tests__/ScrollingPanel.test.tsx:25 |
| `picker_should_CloseAndReturnFocusToChip_When_RouteSelected` | components/sessions/__tests__/ScrollingPanel.test.tsx:87 |
| `picker_should_OpenFullPanel_When_MoreSettingsPressed` | components/sessions/__tests__/ScrollingPanel.test.tsx:106 |
| `postFitRepaint_should_LogOnceWithRowsRendererAndReason_When_ForcedRefresh` | lib/terminal/__tests__/postFitRepaint.test.ts:34 |
| `postFitRepaint_should_RefreshAllRowsAndClearAtlasOnce_When_WebglActive` | lib/terminal/__tests__/postFitRepaint.test.ts:15 |
| `postFitRepaint_should_RefreshOnlyAndNeverClearAtlas_When_CanvasOrDomRenderer` | lib/terminal/__tests__/postFitRepaint.test.ts:24 |
| `push_should_EmitSymmetricLineDeltaAndCarryRemainder_When_UpAndDownThreePxFrames` | lib/terminal/__tests__/scrollKinematics.test.ts:12 |
| `push_should_ResetRemainder_When_CellHeightChanges` | lib/terminal/__tests__/scrollKinematics.test.ts:33 |
| `push_should_SeedWithOvershootOnlyAndEmitNoJump_When_Slop15Crossed` | lib/terminal/__tests__/scrollKinematics.test.ts:22 |
| `redrawButton_should_BeRenderedWhenToolbarCollapsed` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:271 |
| `redrawButton_should_CallRefitWithManualResize_When_Tapped` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:264 |
| `refit_should_CallRefreshOnceAndNotFit_When_ProposedDimsEqualApplied` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:97 |
| `refit_should_FireOncePerSettle_When_NRapidViewportResizes` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:302 |
| `refit_should_LogViewportVsContainerMismatch_When_ContainerDoesNotTrackVisualViewport` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:288 |
| `refit_should_NotRenderErrorUi_When_RetryExhausted` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:234 |
| `refit_should_RefreshAfterEveryCycle_When_200AlternatingResizeRefitCycles` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:323 |
| `refit_should_RefreshOnce_When_CanvasRendererAndDimsUnchanged` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:272 |
| `refit_should_RepaintAndClearPending_When_ObserverRestoresIdenticalSize` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:244 |
| `refit_should_RepaintWhenSamplerAlreadyActive` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:108 |
| `refit_should_RunFirstTickAtZeroMs_When_Called` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:120 |
| `refit_should_WarnRepaintAndSetPendingRefit_When_ContainerZeroFor20RafOr1000Ms` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:210 |
| `resize_should_HoldThreeThenSixSeconds_When_NoBypassAndDimsInHistory` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:657 |
| `resize_should_SendImmediatelyAndResetStreak_When_BypassBounceHoldAndDimsInHistory` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:638 |
| `resize_should_StillDedupe_When_BypassAndDimsUnchanged` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:678 |
| `sampler_should_NotRepaint_When_RoDrivenAtRest` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:126 |
| `sampler_should_RepaintOnGiveUp_When_RepaintRequested` | components/sessions/__tests__/XtermTerminal.refit.test.tsx:134 |
| `scrollDrag_should_CallScrollLinesEveryFrame_When_60FramesAtOneLineEach_InLocalBuffer` | lib/hooks/__tests__/useTerminalGestures.test.ts:771 |
| `scrollDrag_should_CallScrollLinesMinus5ThenPlus5_When_Drag90PxDownThenUp_InNormalBuffer` | lib/hooks/__tests__/useTerminalGestures.test.ts:711 |
| `scrollDrag_should_DispatchAtMostOncePerRaf_When_ManyTouchmovesInOneFrame` | lib/hooks/__tests__/useTerminalGestures.test.ts:761 |
| `scrollDrag_should_DispatchOnceAndClamp_When_200msFrameGapWith40Touchmoves` | lib/hooks/__tests__/useTerminalGestures.test.ts:784 |
| `scrollDrag_should_DropTuiPageKeyAndLog_When_InputChunking` | lib/hooks/__tests__/useTerminalGestures.test.ts:968 |
| `scrollDrag_should_IgnoreRemainingMoves_When_OrientationChangeMidGesture` | lib/hooks/__tests__/useTerminalGestures.test.ts:1154 |
| `scrollDrag_should_ResetAccumulatorAndCancel_When_ViewportResizeMidGesture` | lib/hooks/__tests__/useTerminalGestures.test.ts:1132 |
| `scrollDrag_should_ScrollBothDirections_InNormalAndAlternate` | lib/hooks/__tests__/useTerminalGestures.test.ts:735 |
| `scrollDrag_should_SendOnePgUpAndNeverScrollLines_When_11LinePostSlopDragInAlternateBuffer` | lib/hooks/__tests__/useTerminalGestures.test.ts:890 |
| `scrollDrag_should_SendPgUpBytes_When_DragDownInTuiTarget` | lib/hooks/__tests__/useTerminalGestures.test.ts:924 |
| `scrollDrag_should_StillScrollLocally_When_InputChunking` | lib/hooks/__tests__/useTerminalGestures.test.ts:988 |
| `scrollLines_should_HaveSingleProductionCaller_InUseTerminalGestures` | lib/hooks/__tests__/useTerminalGestures.test.ts:1199 |
| `scrollOverride_should_FallBackToAuto_When_StoredValueInvalid` | lib/terminal/__tests__/scrollOverride.test.ts:23 |
| `scrollOverride_should_FallBackToDefaults_When_LocalStorageThrows` | lib/terminal/__tests__/scrollOverride.test.ts:36 |
| `scrollOverride_should_PersistAndRestore_When_SetToTui` | lib/terminal/__tests__/scrollOverride.test.ts:17 |
| `scrollOverride_should_RemoveKey_When_SetToAuto` | lib/terminal/__tests__/scrollOverride.test.ts:28 |
| `selecting_should_NotScroll_When_DragInSelectionMode` | lib/hooks/__tests__/useTerminalGestures.test.ts:499 |
| `sendInput_should_ClearChunkingFlag_When_SessionChangesMidPaste` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:155 |
| `sendInput_should_NotSetChunkingFlag_When_512BytesOrFewer` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:197 |
| `sendInput_should_SetChunkingFlagUntilLastChunk_When_2000BytePaste` | lib/hooks/__tests__/useTerminalFlowControl.test.ts:134 |
| `stats_should_BeUnavailableAndZeroOverhead_When_FlagUnset` | lib/terminal/__tests__/mobileDebug.test.ts:103 |
| `stats_should_CountOverrideChangeAndMisroute_When_OverrideChangedWithin10sOfAutoDrag` | lib/terminal/__tests__/mobileDebug.test.ts:59 |
| `stats_should_CountToolbarKeyAfterDrag_When_Within5s` | lib/terminal/__tests__/mobileDebug.test.ts:77 |
| `terminalCss_should_RestoreDefaultTouchAction_When_GestureScrollOff` | components/sessions/__tests__/XtermTerminal.overscroll.test.ts:28 |
| `terminalCss_should_SetOverscrollBehaviorContainAndTouchActionNone` | components/sessions/__tests__/XtermTerminal.overscroll.test.ts:19 |
| `toggle_should_ChangeSelectionOnArrowKeys` | components/sessions/__tests__/ScrollingPanel.test.tsx:59 |
| `toggle_should_RouteNextDragToPgKeys_When_TuiSelected` | components/sessions/__tests__/ScrollingPanel.test.tsx:79 |
| `toggle_should_ShowEffectiveModeUnderAuto` | components/sessions/__tests__/ScrollingPanel.test.tsx:115 |
| `toolbarKeys_should_ScrollPages_When_LocalRoute` | components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx:105 |
| `toolbarKeys_should_SendExactBytes_When_TuiRoute` | components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx:116 |
| `toolbarPageAction_should_FallThroughToBytes_When_AutoLocalAtEdge` | lib/terminal/__tests__/scrollRouting.test.ts:222 |
| `toolbarPageAction_should_NotFallThrough_When_ExplicitLocalAtEdge` | lib/terminal/__tests__/scrollRouting.test.ts:230 |
| `toolbarPageAction_should_ReturnBytes_When_TuiRoute` | lib/terminal/__tests__/scrollRouting.test.ts:199 |
| `toolbarPageAction_should_ReturnModifiedBytes_When_ModifierArmed` | lib/terminal/__tests__/scrollRouting.test.ts:214 |
| `toolbarPageAction_should_ReturnScrollPages_When_LocalRouteAndNoModifier` | lib/terminal/__tests__/scrollRouting.test.ts:188 |
| `touchcancel_should_ResetStateAndNotCoast_When_ScrollingTouchCancelled` | lib/hooks/__tests__/useTerminalGestures.test.ts:464 |
| `touchend_should_ClearSelection` | lib/hooks/__tests__/useTerminalGestures.test.ts:434 |
| `touchend_should_ClearSelectionAndNotFocus_When_TapWhileSelectionActive` | lib/hooks/__tests__/useTerminalGestures.test.ts:434 |
| `touchend_should_NotFocusTerminal_When_ScrollCompleted` | lib/hooks/__tests__/useTerminalGestures.test.ts:1111 |
| `touchend_should_NotPreventDefaultAndLog_When_NotCancelable` | lib/hooks/__tests__/useTerminalGestures.test.ts:1097 |
| `touchend_should_PreventDefault_When_ScrollCompletedAndCancelable` | lib/hooks/__tests__/useTerminalGestures.test.ts:1089 |
| `touchend_should_RunTapPath` | lib/hooks/__tests__/useTerminalGestures.test.ts:378 |
| `touchend_should_RunTapPathAndFocusOnce_When_TotalDy5And100ms` | lib/hooks/__tests__/useTerminalGestures.test.ts:378 |
| `touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms` | lib/hooks/__tests__/useTerminalGestures.test.ts:387 |
| `touchmove_should_NotPreventDefaultAndLog_When_NotCancelable` | lib/hooks/__tests__/useTerminalGestures.test.ts:1054 |
| `touchmove_should_NotScrollOrPreventDefault_When_HorizontalFirstPastSlop` | lib/hooks/__tests__/useTerminalGestures.test.ts:626 |
| `touchmove_should_PreventDefault_When_FirstMovePastSlopAndCancelable` | lib/hooks/__tests__/useTerminalGestures.test.ts:1047 |
| `touchstart_should_CancelGesture_When_TwoTouches` | lib/hooks/__tests__/useTerminalGestures.test.ts:574 |
| `touchstart_should_EnterSelecting_When_Stationary400ms` | lib/hooks/__tests__/useTerminalGestures.test.ts:482 |
| `touchstart_should_IgnoreGesture_When_TouchBeganOnScrollbarOrSelectionHandle` | lib/hooks/__tests__/useTerminalGestures.test.ts:1066 |
| `useEffectiveScrollMode_should_FollowTable_When_Auto` | lib/hooks/__tests__/useEffectiveScrollMode.test.ts:19 |
| `useEffectiveScrollMode_should_KeepRoute_When_GesturesOff` | lib/hooks/__tests__/useEffectiveScrollMode.test.ts:25 |
| `useEffectiveScrollMode_should_ReturnOverrideTarget_When_OverrideLocalOrTui` | lib/hooks/__tests__/useEffectiveScrollMode.test.ts:14 |
| `viewportResize_should_CallRefitOncePerSettle_And_NotAfterUnmount` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:156 |
| `viewportSettle_should_ArmOnVisualViewportScroll_When_OnlyOffsetTopChanges` | lib/terminal/__tests__/viewportSettle.test.ts:100 |
| `viewportSettle_should_ArmOnWindowResize_When_NoVisualViewport` | lib/terminal/__tests__/viewportSettle.test.ts:156 |
| `viewportSettle_should_FireAtThirdStableFrame_When_HeightStableAndOffsetTopChanges0To40` | lib/terminal/__tests__/viewportSettle.test.ts:83 |
| `viewportSettle_should_FireOnceAtThirdStableFrame_When_HeightSequence800To480` | lib/terminal/__tests__/viewportSettle.test.ts:49 |
| `viewportSettle_should_FireWithin400msOfFirstEvent_When_HeightStabilizesWithin300ms` | lib/terminal/__tests__/viewportSettle.test.ts:186 |
| `viewportTouch_should_NotDoubleScroll_When_HookOwnsDrag` | lib/hooks/__tests__/useTerminalGestures.test.ts:1224 |
| `visibilityTrue_should_CallRefit_NotSetTimeoutFit` | components/sessions/__tests__/TerminalOutput.refit.test.tsx:196 |

Counts: `wc -l < /tmp/res.txt` = 169 checked; `grep -c MISSING /tmp/res.txt` = 8; `grep -v MISSING /tmp/res.txt | wc -l` = 161. Totals: **161 / 168 present by exact name** (withdrawn name excluded); 163 / 168 counting the two renamed equivalents; 5 absent because Story 2.1.6 / Story 3.1.2 are skipped (expected).

## 2. Acceptance criteria (top-level AC bullets per story; nested Given/When/Then grouped under their bullet)

Status key: MET = implemented and covered by a passing jest test (device behavior still separate, see section 4); PARTIAL; NOT MET; DEVICE-GATED/UNVERIFIED; SKIPPED = conditional story not implemented by design.

| Story | AC bullet | Status | Implementing code / covering test / reason |
|---|---|---|---|
| 1.1.1 | Remainder carried, up/down symmetric | MET | `lib/terminal/scrollKinematics.ts` (`ScrollAccumulator`); `push_should_EmitSymmetricLineDeltaAndCarryRemainder_When_UpAndDownThreePxFrames` |
| 1.1.1 | First-slop overshoot seed, `SLOP_PX`=15 | MET | `scrollKinematics.ts:10` `SLOP_PX = 15`; `push_should_SeedWithOvershootOnlyAndEmitNoJump_When_Slop15Crossed` |
| 1.1.2 | Decision table routing, override, unverified log, injected rows, wheel invariant | MET | `lib/terminal/scrollRouting.ts` (`decideScrollTarget`, `:29` `ROUTING_VERIFIED=false`, `:85`); `decideScrollTarget_should_*` (6 tests found) |
| 1.1.2 | Encoders exact bytes, clamped coords, X10 unreachable | MET | `scrollRouting.ts:136-137` page bytes; `encodeWheel_*`, `encodePageKeys_*` tests found |
| 1.1.2 | Page-key mode accumulated, rate-limited, capped | MET | `scrollRouting.ts:146-147` (`PAGE_RATE_LIMIT_MS=100`, `PAGE_FLING_CAP=5`); `pageAccumulator_*` tests |
| 1.1.3 | Decay with injected clock; canonical constants | MET | `scrollKinematics.ts:13` `MOMENTUM_CONSTANTS`; `momentum_should_DecayBy095...`, `momentumConstants_should_MatchCanonicalSet` |
| 1.1.3 | Reduced motion disables | MET | `momentum_should_EmitNoFrames_When_ReducedMotionTrue` |
| 1.1.3 | Cancel immediate | MET | `momentum_should_StopImmediately_When_Cancelled` |
| 1.1.4 | `clampLinesPerFrame` symmetric, excess not carried | MET | `clampLinesPerFrame_should_ClampToRowsBothSigns_When_LargeDelta` |
| 1.1.4 | No timer throttle on 1:1 path | MET | `scrollDrag_should_CallScrollLinesEveryFrame_When_60FramesAtOneLineEach_InLocalBuffer` |
| 1.1.4 | Stalled frames do not replay | MET | `scrollDrag_should_DispatchOnceAndClamp_When_200msFrameGapWith40Touchmoves` |
| 1.2.4 | Regression anchors (tap, long-press, double-tap, multi-touch, PgUp bytes) | MET | `touchend_should_RunTapPathAndFocusOnce_*`, `touchstart_should_EnterSelecting_*`, `doubleTap_*`, `touchstart_should_CancelGesture_When_TwoTouches`, `encodePageKeys_*` all found; `lib/terminal/gestureMachine.ts` extracted (Task 1.2.4b) |
| 1.2.1 | Both directions, normal buffer | MET | `useTerminalGestures.ts` + `scrollDrag_should_CallScrollLinesMinus5ThenPlus5_*` |
| 1.2.1 | TUI target forwards bytes, never `scrollLines` | MET | `scrollDrag_should_SendOnePgUpAndNeverScrollLines_*` |
| 1.2.1 | First move past slop `preventDefault` (cancelable guard + log) | MET | `touchmove_should_PreventDefault_*`, `touchmove_should_NotPreventDefaultAndLog_*` |
| 1.2.1 | Mode re-evaluated per frame | MET | routing re-read per frame in the hook; mode-flip case pinned in the `SendOnePgUp...` group (validation.md row REQ-2) |
| 1.2.1 | Drag never focuses | MET | `touchend_should_NotFocusTerminal_When_ScrollCompleted` |
| 1.2.1 | `touchend` click suppression | MET | `touchend_should_PreventDefault_When_ScrollCompletedAndCancelable`, `..._NotPreventDefaultAndLog_*` |
| 1.2.1 | Mid-gesture cancel (viewport, orientation, override, epoch) + reset remainders | MET | `scrollDrag_should_ResetAccumulatorAndCancel_*`, `..._IgnoreRemainingMoves_*`, `useTerminalGestures.test.ts:1166` (epoch), `:1177` (override); `netPagesUp` reset on epoch lives in `TerminalOutput.tsx:918-923` |
| 1.2.2 | Fling continues then stops | MET | `momentum_should_DispatchDecayingScrollLinesAtMostOncePerFrame_*`, `..._NeverDispatchTwiceInOneFrame_*` |
| 1.2.2 | Cancellation sources incl. buffer change, unmount | MET | `momentum_should_Cancel_When_BufferTypeChanges`, `..._VisualViewportResizeOrOrientationChangeOrUnmount`, `hook_should_DisposeBufferChangeSubscription_When_Unmounted` |
| 1.2.2 | Touch-to-stop is not a tap (COASTING, `consumedByCoast`) | MET | `momentum_should_CancelAndSkipTapFocus_When_TouchstartDuringCoasting`, `momentum_should_ResetStateAndCoastFlag_When_TouchcancelDuringCoasting`; `gestureMachine.ts` has `COASTING`/`momentumEnd` |
| 1.2.10 | `touchcancel` in SCROLLING resets, no coast | MET | `gestureMachine.ts:118-120` (-> IDLE via `abort()`); `touchcancel_should_ResetStateAndNotCoast_*`. Discrepancy with ux.md S9, see section 3 |
| 1.2.10 | Horizontal-first ignored, no `preventDefault` | MET | `gestureMachine.ts` (`absDx > absDy` -> stay PENDING, long-press timer kept; release is not a tap via the dx-aware tap check); `touchmove_should_NotScrollOrPreventDefault_When_HorizontalFirstPastSlop` |
| 1.2.10 | Tap tolerance = slop | MET | `touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms` |
| 1.2.10 | Tap with selection active clears only | MET | `touchend_should_ClearSelectionAndNotFocus_When_TapWhileSelectionActive` |
| 1.2.3 | No page-level overscroll | PARTIAL | static part MET: `app/globals.css:232` `overscroll-behavior: none`, `XtermTerminal.css.ts:62` `overscrollBehavior: "contain"`, tests `globalCss_should_SetOverscrollBehaviorNoneOnHtmlBody`, `terminalCss_should_SetOverscrollBehaviorContainAndTouchActionNone` (`XtermTerminal.overscroll.test.ts`); real pull-to-refresh is device D4, UNVERIFIED |
| 1.2.5 | Control + persistence (radios, `terminal-scroll-override`, announce, reload, Auto removes key) | MET | `lib/terminal/scrollOverride.ts`, `components/sessions/ScrollingPanel.tsx` (+ `SCROLL_OPTIONS`); `scrollOverride_should_*`, `panel_should_UseNativeRadios...`, `toggle_should_RouteNextDragToPgKeys_When_TuiSelected` |
| 1.2.5 | Active mode visible (Auto row "now:", chip, gestures-off suffix, accessible name) | MET | `ScrollModeChip.tsx` (`routeLabel`), `useEffectiveScrollMode.ts`; `toggle_should_ShowEffectiveModeUnderAuto`, `chip_should_StillShowRoute_When_GesturesOff`, `chip_should_NameRouteAndGestureState_InAccessibleName` |
| 1.2.5 | Compact picker closes on select, returns focus, one announcement | MET | `picker_should_CloseAndReturnFocusToChip_When_RouteSelected`, `picker_should_OpenFullPanel_When_MoreSettingsPressed`, `chip_should_NotAnnounce_When_PanelOrPickerOpen` |
| 1.2.5 | Panel never starves terminal (overlay under 5 rows) | MET | `shouldRenderPanelAsOverlay` imported at `TerminalOutput.tsx:65`; `panel_should_RenderAsOverlay_When_InlineWouldLeaveFewerThanFiveRows`. Real keyboard-up behavior not run on a device |
| 1.2.5 | Misroute cue (heavier border, "!", 5 s, once per 30 s) | MET | `ScrollModeChip.tsx:10-12` (`MISROUTE_HIGHLIGHT_MS=5000`, `MISROUTE_ANNOUNCE_INTERVAL_MS=30000`); `chip_should_HighlightOnce_When_LocalDragMovedNothing`, `chip_should_NotHighlight_When_TuiRoute` |
| 1.2.5 | Chip visibility rule (coarse pointer or touchstart; hidden under 5 rows) | MET | `ScrollModeChip.tsx:106`; `chip_should_BeHidden_When_NoCoarsePointerAndNoTouchSeen`, `chip_should_Render_When_TouchstartSeen`, `chip_should_BeHidden_When_FewerThanFiveRows` |
| 1.2.5 | Gesture scrolling Off (no listeners, no `preventDefault`, default touch-action, PgUp/PgDn still scroll) | MET | 1.2.5c: `XtermTerminal.css.ts:60` `'&[data-gesture-scroll="on"]': { touchAction: "none" }`, `XtermTerminal.tsx:1573` sets `data-gesture-scroll`; `hook_should_RegisterNoTouchListeners_When_GestureScrollOff`, `..._NeverPreventDefault_*`, `..._ReRegisterListeners_*`, `terminalCss_should_RestoreDefaultTouchAction_*`. TalkBack must-pass (D8) UNVERIFIED |
| 1.2.5 | Storage failure falls back to defaults | MET | `scrollOverride_should_FallBackToDefaults_When_LocalStorageThrows`, `gestureScroll_should_FallBackToOn_*` |
| 1.2.5 | Targets and focus (44x44, 200% font) | PARTIAL | 44px CSS: `ScrollingPanel.css.ts:38-39,60,68-69`; `chip_should_HaveAtLeast44pxTarget`. 200% font-size usability (D8) UNVERIFIED |
| 1.2.5 | First-use hint (status, "Got it", no timeout, seen flag on dismiss) | MET | `components/sessions/ScrollHint.tsx` mounted at `TerminalOutput.tsx:2095`; `hint_should_*` (4 tests) in `ScrollHint.test.tsx`. Note: tests live in `ScrollHint.test.tsx` not `ScrollingPanel.test.tsx` (validation.md named the latter) |
| 1.2.5 mounts | Chip in always-visible actions row; panel/picker mounted | MET | `TerminalOutput.tsx:1882` `<ScrollModeChip`, `:2097` `<ScrollingPanel`, `:2095` `<ScrollHint`; `TerminalOutput.scrollingUi.test.tsx` passes |
| 1.2.7 | Jump button visible iff away + enough rows; TUI estimate gating | MET | `JumpToLatestButton.tsx:101`; `isAwayFromLive_*`, `jumpToLatest_should_BeHiddenInTui_When_EstimateInvalid`, `..._BeHidden_When_FewerThanFiveRows`. Mounted at `TerminalOutput.tsx:2183` via `JumpToLatestMount.tsx` |
| 1.2.7 | Placement (top-right on cursor intersect; corner kept during gesture) | MET | `jumpToLatest_should_MoveTopRight_When_CursorRowIntersects`, `..._KeepCorner_When_GestureInProgress` |
| 1.2.7 | Tap scrolls without focusing; TUI sends min(netPagesUp,5) PgDn | MET | `jumpToLatest_should_NotFocusTerminal_When_Tapped`, `..._SendAtMostFivePgDn_When_Tapped`; `netPagesUp.markLive()` at `JumpToLatestButton.tsx:183` |
| 1.2.7 | Output while scrolled holds position; one announcement per episode | MET | `jumpToLatest_should_HoldPosition_When_OutputArrivesWhileScrolledUp`, `..._AnnounceOncePerEpisode` |
| 1.2.7 | Reduced motion no transition | MET | `jumpToLatest_should_HaveNoTransition_When_ReducedMotion` |
| 1.2.7 | Reconnect resets estimate; empty buffer hides and drag is a no-op | PARTIAL | reconnect MET (`TerminalOutput.tsx:918-923`, `JumpToLatestButton.tsx:108`, test `jumpToLatest_should_ResetEstimate_When_ConnectionEpochChanges`); empty-buffer hide MET (renamed test `:100`); "drag is a no-op on empty buffer" has no dedicated hook test or guard (no `empty`/`length === 0` match in `useTerminalGestures.ts`) |
| 1.2.7 invalidations | `netPagesUp` invalidated by keystroke, resize, mode/override, epoch, over 5 | MET | keystroke `TerminalOutput.tsx:835`; resize `:987` and settle `:1358`; mode/override `:925-932` (`modeKey` includes override); epoch `:918-923`; overflow `scrollPosition.ts:66`; `netPagesUp_should_BeInvalid_When_KeystrokeResizeModeChangeReconnectOrOverFive` |
| 1.2.8 | Always-visible "Redraw" button, aria-label, `refit({reason:'manual-resize'})`, refresh at unchanged dims | MET | `TerminalOutput.tsx:1869-1879`; `redrawButton_should_CallRefitWithManualResize_When_Tapped`. Sends resize after fit: `TerminalOutput.tsx:1695` passes `onFitted`; `XtermTerminal.tsx:1163-1164,1345-1346,1555` (`RefitOptions.onFitted` collected and flushed once when the sampler run ends; type in `lib/terminal/postFitRepaint.ts:31`) |
| 1.2.8 | 360 px viewport, 44x44 with toolbar collapsed | DEVICE-GATED | `redrawButton_should_BeRenderedWhenToolbarCollapsed` covers rendering only; size measure is D10 |
| 1.2.8 | Device checks D8/D10 | DEVICE-GATED | not run |
| 1.2.9 | Local route + no modifier: `scrollPages`, no bytes | MET | `toolbarPageAction` (`scrollRouting.ts`), wired at `TerminalOutput.tsx:962`; `toolbarPageAction_should_ReturnScrollPages_*`, `toolbarKeys_should_ScrollPages_When_LocalRoute` |
| 1.2.9 | TUI route sends exact bytes, counts toward `netPagesUp` | MET | `TerminalOutput.tsx:974-979`; `toolbarKeys_should_SendExactBytes_When_TuiRoute` |
| 1.2.9 | Modifier armed sends modified bytes | MET | `toolbarPageAction_should_ReturnModifiedBytes_When_ModifierArmed` |
| 1.2.9 | Auto + edge falls through; explicit local does not | MET | `..._FallThroughToBytes_When_AutoLocalAtEdge`, `..._NotFallThrough_When_ExplicitLocalAtEdge` |
| 1.2.9 | Residual risk recorded (normal-buffer TUI local) | DEVICE-GATED | D5 record is a PR-body item, not run |
| 1.2.6 | Chunked paste exposes in-flight signal | MET | `useTerminalFlowControl.ts:48,316,524` (`isInputChunking`, ref-backed); `useTerminalStream.ts:83,733`; `sendInput_should_*Chunking*` (3 tests) |
| 1.2.6 | TUI page steps dropped (not queued) while paste in flight; local unaffected | MET | `useTerminalGestures.ts:92,409,425` (`isInputBusy` gate at two TUI dispatch sites); threaded `TerminalOutput.tsx:885` -> `XtermTerminal.tsx:163,339`; `scrollDrag_should_DropTuiPageKeyAndLog_When_InputChunking`, `..._StillScrollLocally_When_InputChunking` |
| 1.2.6 | Toolbar PgUp/PgDn not gated | MET | `sendKey` path unchanged (`TerminalOutput.tsx:979`); no `isInputBusy` reference outside the gesture hook |
| 2.1.1 | `postFitRepaint` renderer-aware | MET | `lib/terminal/postFitRepaint.ts`; `postFitRepaint_should_RefreshAllRowsAndClearAtlasOnce_When_WebglActive`, `..._RefreshOnlyAndNeverClearAtlas_When_CanvasOrDomRenderer` |
| 2.1.1 | Zero-size guard `canFit` | MET | `canFit_should_ReturnFalseAndNotFit_When_ContainerClientHeightZero` |
| 2.1.2 | Confirmed fit repaints | MET | `refit_should_RefreshOnce_When_CanvasRendererAndDimsUnchanged` and sampler tests in `XtermTerminal.refit.test.tsx` |
| 2.1.2 | `refit()` at unchanged dims repaints (3 cases) | MET | `refit_should_CallRefreshOnceAndNotFit_When_ProposedDimsEqualApplied`, `..._RepaintWhenSamplerAlreadyActive`, `sampler_should_NotRepaint_When_RoDrivenAtRest`; `XtermTerminal.tsx:1555` `requestFitRef.current({forceRepaint:true,...})` |
| 2.1.2 | Bypasses RO debounce | MET | `refit_should_RunFirstTickAtZeroMs_When_Called` |
| 2.1.2 | Retry exhaustion bounded, non-silent | MET | `refit_should_WarnRepaintAndSetPendingRefit_When_ContainerZeroFor20RafOr1000Ms`, `refit_should_NotRenderErrorUi_When_RetryExhausted` |
| 2.1.2 | Zero-then-restore repaints | MET | `refit_should_RepaintAndClearPending_When_ObserverRestoresIdenticalSize` |
| 2.1.2 | Context loss recovers | MET | `onContextLoss_should_FallBackAndRepaint` |
| 2.1.2 | Canvas renderer covered | MET | `refit_should_RefreshOnce_When_CanvasRendererAndDimsUnchanged`, `refit_should_RefreshAfterEveryCycle_When_200AlternatingResizeRefitCycles` |
| 2.1.3 | No `isFittingRef` / timer-wrapped fit in TerminalOutput | MET | `grep -nE "isFittingRef|\.fit\(\)|setTimeout\(.*fit" TerminalOutput.tsx` -> only 3 comment lines (109, 1391, 1408) |
| 2.1.3 | Visibility effect uses `refit()` | MET | `visibilityTrue_should_CallRefit_NotSetTimeoutFit`, `handleManualResize_should_CallRefit` |
| 2.1.3 | Remaining bare `fit()` calls enumerated | MET | `XtermTerminal.tsx` has 3 `fitAddon.fit()` call lines (`:610` fallback, `:825` initial, `:1206` sampler), matching the plan's 3 "keep" rows. Task 2.1.3d (desktop manual regression record) is a PR item, not run |
| 2.1.4 | Trailing, not leading-edge | MET | `lib/terminal/viewportSettle.ts`; `viewportSettle_should_FireOnceAtThirdStableFrame_When_HeightSequence800To480` |
| 2.1.4 | Offset-aware; arms on vv resize and scroll | MET | `viewportSettle_should_FireAtThirdStableFrame_When_HeightStableAndOffsetTopChanges0To40`, `..._ArmOnVisualViewportScroll_*`. Tuning against device deferred (D7) |
| 2.1.4 | Timeout fallback | MET | `viewportSettle_should_FireOnceAtMaxWaitWithLatestHeight_When_HeightNeverStabilizes` |
| 2.1.4 | No `visualViewport` fallback | MET | `..._UseInnerHeightAndNotThrow_When_NoVisualViewport`, `..._ArmOnWindowResize_*` |
| 2.1.4 | Not slower than the 400 ms path | MET | `viewportSettle_should_FireWithin400msOfFirstEvent_*`. Device median is D7, UNVERIFIED |
| 2.1.4 | Decoupled from the gesture hook | MET | wiring in `TerminalOutput.tsx:1352-1358` (`createViewportSettle(window.visualViewport, createRafScheduler(), ...)`); settle stamp `:1356`. Not re-checked: absence of `lib/hooks` imports in `viewportSettle.ts` (not grepped) |
| 2.1.5 | Bypass sends immediately on a bounce | MET | `useTerminalFlowControl.ts:324` (`bypassed`), `:348,362` logs; `resize_should_SendImmediatelyAndResetStreak_When_BypassBounceHoldAndDimsInHistory` |
| 2.1.5 | Non-bypass unchanged; dedup kept | MET | `resize_should_HoldThreeThenSixSeconds_*`, `resize_should_StillDedupe_When_BypassAndDimsUnchanged` |
| 2.1.5 | Only settle-driven resizes bypass (1000 ms window) | MET | `TerminalOutput.tsx:1086-1088`; `handleTerminalResize_should_PassBypass_When_Within1000msOfSettleRefit`, `..._NotPassBypass_When_1500msAfterSettleRefit` |
| 2.1.5 | Debug log `bypassed: true|false` | MET | `useTerminalFlowControl.ts:348,362` (`bypassed` field in `mobileDebug.log('resize', ...)`) |
| 2.1.6 | Stale queue dropped when connected | SKIPPED (conditional) | Q5b not run; no `dropPendingWrites` in src |
| 2.1.6 | Repaint after resume (`reason:'visibility'`) | SKIPPED (conditional) | no `reason: "visibility"` caller outside tests (grep empty) |
| 2.1.6 | Stall watchdog still recovers | SKIPPED (conditional) | `useVisibilityResync.ts` untouched on this branch (existing behavior) |
| 3.1.1 | jest green; coverage targets; `lint:duplicates`; `make lint` | PARTIAL | jest subset green (724/724, command above). Coverage (`--coverage`), `pnpm run lint:duplicates`, `make lint` not run in this sweep |
| 3.1.4 | `make quick-check` and `make ready` pass; registry-generate clean | UNVERIFIED | not run in this sweep (read-only constraint, long gates) |

Totals (81 bullets): **MET 70**, PARTIAL 4 (1.2.3, 1.2.5 targets, 1.2.7 empty-buffer, 3.1.1), DEVICE-GATED 3 (1.2.8 x2, 1.2.9 residual-risk record), SKIPPED-conditional 3 (2.1.6), UNVERIFIED 1 (3.1.4), NOT MET 0. Excluding the 3 skipped and the 1 unrun gate: 70 MET of 77.
Recount: `70 + 4 + 3 + 3 + 1 = 81`.

## 3. Specific items

- (a) Story 1.2.5 UI: MET. `ScrollModeChip.tsx` (exports `routeLabel`, `useTouchCapable`, `useMisrouteCue`, `ScrollModeChip`) and `ScrollingPanel.tsx` (re-exports the chip, plus `SCROLL_OPTIONS`, `WIDE_OUTPUT_NOTE`, `TMUX_NOTE`) and `ScrollHint.tsx`; mounted in `TerminalOutput.tsx` at `:1882` (chip, in the always-visible actions row next to Redraw), `:2095` (hint), `:2097` (panel). Plan said the chip lives in `ScrollingPanel.tsx`; it is a separate file re-exported there. Hint tests are in `ScrollHint.test.tsx`, not `ScrollingPanel.test.tsx`.
- (b) Story 1.2.5c: MET. `XtermTerminal.css.ts:60` keys `touchAction: "none"` on `[data-gesture-scroll="on"]`; `XtermTerminal.tsx:1573` emits the attribute. Other `touchAction` rules (`:31,133` manipulation; `:188,199,223` none) are overlays and were left alone per Task 1.2.3a.
- (c) Story 1.2.7: MET. `JumpToLatestMount.tsx` mounted at `TerminalOutput.tsx:2183`; all five invalidations (keystroke, resize, mode/override, epoch, over 5) present (list in AC table).
- (d) Story 1.2.8: MET. `onFitted` is a field of `RefitOptions` (`postFitRepaint.ts:31`); `XtermTerminal.tsx` queues callbacks while a sampler run is coalescing and flushes them after the fit; `TerminalOutput.tsx:1695` uses it to send the server resize after the fit.
- (e) Story 1.2.10 S9: multi-touch `touchstart` -> CANCELLED is implemented (`gestureMachine.ts:64-65`, `:75`). SCROLLING `touchcancel` intentionally returns to IDLE (`gestureMachine.ts:118-120`, with an explanatory comment). **Plan/ux discrepancy**: plan.md Story 1.2.10 AC says IDLE; design/ux.md S9 says CANCELLED. The code follows the plan AC and argues the two are observably identical because the next `touchstart` starts a fresh PENDING from either state. The plan, ux.md and code were not reconciled; one of the two docs should be amended.
- (f) Story 1.2.6 paste guard: MET (details in AC table). Note the plan puts it in Tier B; it is implemented.
- (g) Story 2.1.6: skipped (conditional on Q5b, device-gated). No code, no tests. Expected per plan.md:971 and :383.
- (h) Story 3.1.2 e2e: not done (optional, plan.md:1003). `tests/e2e` has no diff on this branch.

## 4. Device checklist (not verified)

Nothing below was run in this sweep or evidenced in the repo. Every row is NOT RUN.

| Item | Source | Status |
|---|---|---|
| Spike Q1 (X10/SGR wheel encoding accepted) | plan.md Spike section, Task 0.1.2 | NOT RUN |
| Spike Q2 (mode matrix: tmux shell, Claude Code, tracking modes; confirms or edits default routing rows, flips `ROUTING_VERIFIED`) | Task 0.1.2 | NOT RUN (`ROUTING_VERIFIED` still false) |
| Spike Q3 (blank terminal root cause: container tracks viewport; bounce hold >= 3 s with blank canvas) | Task 0.1.2 | NOT RUN |
| Spike Q4 (streaming path, FlowControl honored) | Task 0.1.2f | NOT RUN |
| Spike Q5 (`RECONNECT_V2` value, stale flash or blank on hidden-tab return; gates 2.1.6) | Task 0.1.2f | NOT RUN |
| Baseline blank rate `b` (Task 0.1.2d) | plan.md | NOT RUN |
| D1 10+10 drag threshold, plain shell | plan.md:1013 | NOT RUN |
| D1b tmux plain shell routing | plan.md:1014 | NOT RUN |
| D2 Claude Code 10+10, `dump()` bytes | plan.md:1015 | NOT RUN |
| D2b panel/picker/chip/hint/misroute trial, 2x10-min sessions per scenario | plan.md:1016 | NOT RUN |
| D2c mid-drag rotate/keyboard | plan.md:1017 | NOT RUN |
| D2d paste while dragging | plan.md:1018 | NOT RUN |
| D2e direction check via `dump()` | plan.md:1019 | NOT RUN |
| D2f jump to latest on device | plan.md:1020 | NOT RUN |
| D3 slow drag, fling, touch-to-stop | plan.md:1021 | NOT RUN |
| D4 pull-to-refresh / bounce | plan.md:1022 | NOT RUN |
| D5 toolbar equivalence and regression | plan.md:1023 | NOT RUN |
| D6 reduced motion | plan.md:1028 | NOT RUN |
| D7 keyboard open/close N cycles, 0 blanks; first-vv-event-to-refit median <= 400 ms; `bypassed:true` on close | plan.md:1029-1030 | NOT RUN (cannot be claimed; baseline also unmeasured) |
| D8 200% font, TalkBack Off/On, must-pass focus-and-type with Gesture scrolling Off | plan.md:1024 | NOT RUN |
| D9 jank trace (no frame over 32 ms), SLOP_PX tuning, drifting tap 8-14 px | plan.md:1025 | NOT RUN |
| D10 Redraw visible and >= 44x44 at 360 px, recovers a forced blank | plan.md:1026 | NOT RUN |
| D11 landscape under 5 rows, reconnect while scrolled | plan.md:1027 | NOT RUN |
| Tuning constants (`SLOP_PX`=15, half-page step 11 lines at 24 rows, `MOMENTUM_CONSTANTS` window 100 ms / min 0.3 px/ms / decay 0.95 / stop 0.02 px/ms / cap 8 px/ms / 120 frames, `PAGE_RATE_LIMIT_MS`=100, `PAGE_FLING_CAP`=5, settle 3 frames / 600 ms) | plan.md D9, "tune on device" | defaults shipped, NOT TUNED |
| Task 2.1.3d desktop regression record (manual resize, side panel, tab switch, log count) and running `tests/e2e/terminal-resize.spec.ts` | plan.md:907 | NOT RUN |
| Coverage run, `pnpm run lint:duplicates`, `make lint`, `make quick-check`, `make ready`, `make registry-generate` | Stories 3.1.1, 3.1.4 | NOT RUN in this sweep |

## 5. Real gaps, ranked

1. **Plan/ux.md S9 `touchcancel` mismatch is unresolved** (section 3e). Code is IDLE per plan; ux.md says CANCELLED. Doc fix needed (or code change), otherwise a reviewer reading ux.md will flag it.
2. **Empty-buffer drag no-op has no guard or test** (AC 1.2.7 last bullet; validation.md row `jumpToLatest_should_BeHiddenAndDragNoop_When_BufferEmpty`). Only the hide half is tested (renamed test). Likely harmless in xterm, but UNVERIFIED.
3. **`netPagesUp` reset on epoch is not asserted at the hook level**; the validation.md-named `scrollDrag_should_CancelAndResetNetPagesUp_When_ConnectionEpochChanges` was replaced by a cancel-only test plus the button-level reset test. Implementation exists (`TerminalOutput.tsx:918-923`).
4. **Story 2.1.6 (hidden-tab drop and visibility repaint) absent**: expected skip, but plan.md:383 says that if Q5b is never run the "drop and record why" step is also missing; no Spike Findings entry exists to justify the skip.
5. **Story 3.1.2 e2e absent** (optional; validation.md REQ-4 lists it as partial proof for the blank-terminal fix, so the only automated end-to-end anchor for REQ-4 is missing).
6. **Hint tests are in `ScrollHint.test.tsx`**, validation.md maps them to `ScrollingPanel.test.tsx`; a paperwork mismatch only.
7. Unrun gates (`make ready`, jscpd, coverage) and every device item (section 4) remain before the PR can claim the defects fixed; D7 and D1/D2 cannot be claimed from this evidence.

## 5. Hygiene results (coordinator, 2026-10-02; supersedes the "not run" notes above for 3.1.1 / 3.1.4)

| Check | Result |
|---|---|
| `cd web-app && pnpm exec tsc --noEmit -p .` | 0 output lines (clean) |
| `cd web-app && pnpm exec jest --no-coverage` (full) | 441/442 suites, 5701/5704 tests pass; the 3 failures are all `LocalFileBrowser.test.tsx` |
| `LocalFileBrowser` on `origin/main` (temp worktree, now removed) | 6/6 pass there. Our branch does not touch `components/files`, `components/ui` or `useRepoPathSuggestions`; `origin/main` has a 6-line change to `LocalFileBrowser.test.tsx` that this branch lacks. Pre-existing on this branch's base, not caused by it |
| `pnpm run lint:duplicates` | First run 0.12% (> 0.1% threshold) because `TerminalOutput.scrollingUi.test.tsx` copied xterm mock setup; fixed by sharing `capturingXtermTerminalMockModule`/`createMockXtermHandle` from `terminalOutputTestMocks.ts`; rerun passed (10 clones, all in files this branch does not touch) |
| `make lint` | Web build OK; `lint-custom` fails with 4 findings in untouched Go files (`pkg/portguard/portguard_test.go`, `server/services/connectrpc_websocket_test.go`, `session/git/ops_bench_test.go`, `server/services/autonomous_orchestration_service.go`). `git diff` shows no `.go` changes on this branch, so unrelated |
| `CI=true make registry-generate` | Ran. No tracked change under `docs/registry` is attributable to this work; it emitted 6 untracked backend RPC JSONs for unrelated Go RPCs (not committed) and the scanner's pnpm install touched tracked files under `tools/scanner/frontend/node_modules` (not committed). The scanner did not emit per-feature files for the new markers `terminal-scroll-settings` and `terminal-jump-to-latest` (reason not investigated) |
| `make quick-check`, `make ready` | Not run |
| Coverage (`--coverage`) | Not run |

## 6. Follow-up pass (gap closure, 2026-10-02)

| Gap | Result |
|---|---|
| Registry: no per-feature files for `terminal-scroll-settings`, `terminal-jump-to-latest` | Root cause: the frontend scanner (`tools/scanner/frontend/src/main.ts`, `make registry-generate-frontend`) writes only the gitignored monolithic `frontend-features.json`; per-feature files under `docs/registry/features/frontend/` are hand-authored. Markers were already detected (ran the scanner to `/tmp`: both IDs found, `terminal-pre-sizing` too). Added `terminal-scroll-settings.json` and `terminal-jump-to-latest.json` by hand; `aggregate.py` reads them (128 features). `terminal-pre-sizing` still has no per-feature file (pre-existing, not touched). `tools/scanner/README.md` still claims the frontend scanner writes per-feature files; not edited. |
| Gap 1 (S9 touchcancel mismatch) | CLOSED: ux.md S9 SCROLLING touchcancel row now says IDLE; plan.md Story 1.2.10 task wording aligned; validation.md had no conflicting wording. |
| Gap 2 (empty-buffer drag no-op) | CLOSED: `useTerminalGestures.ts` `xterm-local` dispatch returns when `buffer.active.length === 0`; `scrollDrag_should_BeNoop_When_BufferEmpty` fails without the guard (verified) and passes with it. Applies to the local route only; TUI routes still send page keys. |
| Gap 3 (netPagesUp reset on epoch) | CLOSED: `netPagesUp_should_Invalidate_When_ConnectionEpochChanges` in `TerminalOutput.scrollingUi.test.tsx` (full-snapshot bump -> `invalidate("reconnect")`, jump button hidden). |
| Extra tests | `netPagesUp_should_Invalidate_When_ViewportSettles` (settle path calls `invalidate("resize")`); `jumpToLatest_should_NotInvalidateEstimate_When_ItsOwnPageDownIsSent`; `jumpToLatest_should_UseRecreatedTerminal_When_ResizeReportsANewInstance`; `toggle_should_RouteNextDragToPgKeys_When_TuiSelected` as a real-drag hook test in `useTerminalGestures.test.ts`. |
| JumpToLatestMount terminal capture | No code bug found: `TerminalOutput.handleTerminalResize` calls `setJumpTerminal(xtermRef.current?.terminal ?? null)` on every resize report, not only the first, so a new instance is picked up on the next `onResize`. Residual: a recreated terminal that never fires `onResize` is not picked up (not changed). |
| Gap 6 (hint test file) | CLOSED by updating validation.md (REQ-10, UX-29, REQ-15, UX-21, UX-32 rows) to the real file/test names. |
| Hygiene | `tsc --noEmit` 0 lines; terminal jest subset 48 suites / 730 tests pass; full jest 441/442 suites, 5707/5710 tests pass (3 failures only in `LocalFileBrowser.test.tsx`, pre-existing); `pnpm run lint:duplicates` exit 0. |

Still open: gap 4 (Story 2.1.6), gap 5 (Story 3.1.2 e2e), all device items, `ROUTING_VERIFIED=false`, `make ready`/`make quick-check`.
