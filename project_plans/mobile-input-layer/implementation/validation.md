# Validation Plan: mobile-input-layer

**Date**: 2026-11-08

**Path aliases** (used in the File columns): `MI` = `web-app/src/lib/terminal/mobileInput`; `CMP` = `web-app/src/components/sessions`; `HOOKS` = `web-app/src/lib/hooks`; `SVC` = `server/services`; `E2E` = `tests/e2e`. Story and task ids refer to `plan.md`. "Creating task" is the plan task that authors the test file. PR-A1, PR-A2 and PR-B..PR-E are the delivery slices in plan.md (A1 = layer + gate + minimal diagnostics, A2 = Phase 2 observability). Phase 4 plan-repair (2026-11-08) added tasks 0.1.2b, 0.1.3b, 0.1.4a/b, 1.1.4e, 1.2.1d, 1.2.2d, 1.3.1b, 1.3.2e, 1.4.1f, 1.4.2b, 2.2.3b, 2.3.1a, 5.1.1d, 5.1.4c; see the last table of the mapping and the Gaps section. Triad-review repair (2026-11-08) added Stories 0.1.5, 1.2.5, 1.4.3, 1.4.4 and Tasks 0.1.3c, 1.1.4f, 1.1.4g, 3.1.5b, 4.1.2c, 4.2.2c, 5.1.1e, 5.1.2d, 5.1.4d, 5.1.4e and split Task 1.1.4b into three batches (plan.md now has 120 tasks in 43 stories, counted by command); row ownership for them is in the "Triad-review repair additions" section and UX-31..UX-39.

## Happy Path Scenario

Given the Baseline (Android Chrome, `ghostty_renderer` on, soft-keyboard letters never reach the pane and the server cannot say whether they arrived), when the owner taps the terminal on the phone and types "the quick brown fox" with one Gboard autocorrect (`teh` -> `the`) and one swipe word, then the soft keyboard opens on the first tap, the pane shows exactly the text a hardware keyboard would produce (once, with no prompt text eaten), and `/actuator/terminal-input` plus `terminal_input_frames_total{kind="text",listener="remote",stage="received"}` show the frames arriving. Everything below is a variation of that flow. Its automated proxy is the Pixel 7 Playwright spec (wiring only); the real-keyboard proof is Story 5.1.4 on the owner's phone.

## Requirement -> Test Mapping

Requirement ids are assigned here for traceability to `requirements.md`: PS = Problem Statement, SM = Success Metric, SC = In-Scope item, NFR, OBS = Observability Requirement, RC = Risk Control, CON = Constraint, OQ = Open Question. Every row has a creating task. Type "Manual" marks a human or owner-device proof.

### Problem statement and Success Metrics

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| PS-1: root cause UNVERIFIED (ghostty-web ignores keyCode 229, preventDefault on every beforeinput) | `project_plans/mobile-input-layer/implementation/spike-findings.md` | spike_should_RecordActualGhosttyWebConditions_When_GrepOfInstalled040Copy | Manual | Evidence record. Greps installed `ghostty-web/dist/ghostty-web.js` for `keyCode === 229` and `beforeinput`; findings list condition text. [S0.1.1 / T0.1.1a, PR-A, blocks 1.4.1c] |
| PS-1 | `MI/useMobileInput.test.tsx` | mount_should_PlaceLayerTextareaOutsideOpenElement_When_GhosttyOpenElementCarriesBlanketPreventDefault | Unit | Happy path: fake `open()` records its element; layer events never reach its listeners, `defaultPrevented` false. [S1.4.1 / T1.4.1c] |
| PS-1 | `MI/useMobileInput.test.tsx` | mount_should_MakeGhosttyOwnTextareaInert_When_LayerActiveOnTouch | Unit | Error path: ghostty's own textarea `inputmode="none"`, `tabindex=-1`, so a bounce of focus cannot double-send. [S1.4.1 / T1.4.1c] |
| PS-2: UNVERIFIED that flag off restores phone typing | `spike-findings.md` (owner results table) | ownerChecklist_should_RecordPlainLettersArrive_When_GhosttyOffThenOn | Manual | Owner phone, `stelekit`, ghostty off then on, steps from the checklist. Device-only. [S0.1.3 / T0.1.3a] |
| PS-3: Baseline silent drop (`console.warn` only) | `HOOKS/useTerminalFlowControl.test.ts` | sendInput_should_ReturnDropped_When_StreamDisconnected | Unit | Happy path for the drop signal; old warn text retained in PR-A. [S1.2.4 / T1.2.4a, PR-A1] |
| PS-3 | `HOOKS/useTerminalFlowControl.test.ts` | existingCases_should_PassUnchanged_When_ReturnTypeWidened | Unit | Error/regression path: existing callers unaffected by widened `SendResult | void`. [S3.1.1 / T1.2.4a] |
| SM-1: replayed Gboard stream == hardware bytes, both renderers, 100% scenarios | `MI/replay.test.ts` | replay_should_EmitHardwareEquivalentBytes_When_EachFixtureReplayed (table, one case per `__fixtures__/*.json`) | Unit | Happy path: `teh` then `\x7f\x7fhe`, ModelTerminal line `the`; covers typing, autocomplete, autocorrect, swipe, Backspace, Enter, emoji. [S1.1.4 / T1.1.4c, PR-A1] |
| SM-1 | `MI/replay.test.ts` | replay_should_FailNamingFixture_When_ExpectedBytesDiffer | Unit | Error path: mutated `expectedBytes` fails with the fixture id. [S1.1.4 / T1.1.4c] |
| SM-1 | `MI/replay.test.ts` | corpus_should_HaveAtLeast14SyntheticFixturesEachWithSource_When_DirectoryScanned | Unit | Counts the 14 named scenarios plus the 5 Story 1.2.4 fixtures plus the 2 Task 1.1.4e fixtures (21). [S1.1.4 / T1.1.4b, T1.1.4f, T1.1.4g, T1.1.4e] |
| SM-1 | `MI/replay.test.ts` | summary_should_PrintSyntheticAndDeviceCountsSeparately_When_SuiteRuns | Unit | Reads `synthetic 21/21, device 0/0`. [S1.1.4 / T1.1.4c] |
| SM-1 | `MI/rewrite.test.ts` | computeRewrite_should_ReturnTwoBackspacesAndHe_When_TehBecomesThe | Unit | Happy path (also emoji ZWJ, empty `sent`). [S1.1.1 / T1.1.1c] |
| SM-1 | `MI/rewrite.test.ts` | computeRewrite_should_ReportDeferredWithBoundExceeded_When_Region65GraphemesOrNewline | Unit | Error path; plus fast-check `backspaces <= graphemes(sent)`. [S1.1.1 / T1.1.1c] |
| SM-1 | `MI/composition.test.ts` | reduce_should_EmitLoAndStayComposing_When_InsertCompositionTextGrowsHelToHello | Unit | Happy path, one case per ECP row. [S1.1.2 / T1.1.2b] |
| SM-1 | `MI/composition.test.ts` | reduce_should_EmitNothingAndNotPreventDefault_When_Keydown229 | Unit | Error/edge path (also orphaned `input` -> `Degraded`, selection-in-region, stuck-composition tick). [S1.1.2 / T1.1.2b] |
| SM-1 | `MI/composition.test.ts` | reduce_should_EmitExactlyOneCarriageReturn_When_EnterWrappedInComposition (3 orderings from Story 1.1.2) | Unit | Enter never reorders or duplicates. [S1.1.2 / T1.1.2b] |
| SM-1 | `MI/composition.property.test.ts` | reduce_should_YieldIdenticalBytesAndNeverOverDelete_When_CompositionEndAndBeforeInputPermuted | Unit | Property: DEL count <= sent (ghostty PR #110 order case). [S1.1.2 / T1.1.2c] |
| SM-1 | `MI/keymap.test.ts` | keymap_should_EmitCtrlC_When_ArmedCtrlAndSingleCharC | Unit | Happy path (multi-char commit ignores modifier, Alt prefixes ESC). [S1.1.3 / T1.1.3b] |
| SM-1 | `MI/keymap.test.ts` | keymap_should_FlattenNewlines_When_MultilinePasteAndBracketedPasteUnavailable | Unit | Error path: `a\nb` -> `a b`, never raw `\r`. [S1.1.3 / T1.1.3b] |
| SM-1 | `MI/attachMobileInput.test.ts` | attach_should_CallOnDataTehThenAtomicRewrite_When_GboardAutocorrectEventsDispatched | Integration | jsdom real DOM events through normalize + reducer + executor, 3 fixtures reused. [S1.2.2 / T1.2.2c] |
| SM-1 (both renderers) | `CMP/__tests__/mobileInputRenderers.test.tsx` | renderers_should_DeliverIdenticalHelloBytes_When_GboardTypingFixtureDispatched (`describe.each(["xterm","ghostty"])`) | Integration | Fake ghostty module + real `XtermTerminal` over stubbed xterm; fake `open()` element sees no layer events. [S1.4.2 / T1.4.2a, PR-A1] |
| SM-1 (both renderers) | `CMP/__tests__/mobileInputRenderers.test.tsx` | renderers_should_NotDoubleSend_When_FocusBouncesToRendererTextarea | Integration | Error path: bounce focus, second keystroke, each sent exactly once. [S1.2.2 AC no-double-send / T1.2.2c; S1.4.2] |
| SM-2: mobile-emulated e2e, text appears in live pane, both renderers | `E2E/mobile-input-layer.spec.ts` | typing_should_ShowTextInPaneExactlyOnce_When_ImeComposedAndCommitted[xterm] | E2E | Pixel 7, `imeCompose(page,"ls")`, `imeCommit`, pane shows `ls`. Proves wiring only, not Gboard. [S5.1.1 / T5.1.1b, PR-A1] |
| SM-2 | `E2E/mobile-input-layer.spec.ts` | typing_should_ShowTextInPaneExactlyOnce_When_ImeComposedAndCommitted[ghostty] | E2E | Same, `ghostty_renderer` on via `helpers/feature-flag.ts`. Real ghostty-web behavior is only exercised here. [S5.1.1 / T5.1.1b] |
| SM-2 | `E2E/mobile-input-layer.spec.ts` | tap_should_LeaveMobileInputTextareaActiveAndTypeAbcOnce_When_FirstTapOnNeverFocusedDockedTerminal | E2E | Error/edge path: layer attached on dock, not on focus; no double-send. [S5.1.1 / T5.1.1b] |
| SM-2 | `E2E/mobile-input-layer.spec.ts` | layer_should_NotMount_When_IphoneEmulatedContext | E2E | Platform gate negative (`devices['iPhone 14']`). [S5.1.1 / T5.1.1b] |
| SM-2 | `E2E/helpers/ime.ts` (spike result) | imeHelper_should_FallBackToInsertTextPlusSyntheticBeforeInput_When_CdpDoesNotFireInsertCompositionText | Manual | Spike 0.1.2 records the real CDP event order; spec header states which events are real. [S0.1.2 / T0.1.2a, T5.1.1a] |
| SM-3: server per-session frames/bytes, escape vs text | `SVC/terminal_input_metrics_test.go` | TestClassifyInput_ShouldReturnTextEscapeOther_When_LsCRArrowUpCtrlC | Unit | Happy path: `ls\r`=Text, `\x1b[A`=Escape, `0x03`=Other. [S2.1.1 / T2.1.1a, PR-A2] |
| SM-3 | `SVC/terminal_input_metrics_test.go` | TestClassifyInput_ShouldScanAtMost64Bytes_When_LongFrameWithLateControlByte | Unit | Error/edge path. [S2.1.1 / T2.1.1a] |
| SM-3 | `SVC/terminal_input_metrics_test.go` | TestRecordTerminalInput_ShouldIncrementReceivedForwardedAndBytes_When_RemoteTextFrameOnCM | Unit | Happy path (5-byte frame, exactly four labels). [S2.1.1 / T2.1.1b] |
| SM-3 | `SVC/terminal_input_metrics_test.go` | TestTally_ShouldEvictOldestAndHoldSize256_When_300SessionsRecorded | Unit | Error/bound path. [S2.1.1 / T2.1.1b] |
| SM-3 | `SVC/terminal_input_sites_test.go` | TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtEachOfFiveSites | Integration | One subtest per site (shared read loop, capture, shell, stream, `:1653`/`:2489` closures). [S2.1.2 / T2.1.2a, T2.1.2b] |
| SM-3 | `SVC/terminal_input_sites_test.go` | TestInputSites_ShouldRecordReceivedWithoutForwarded_When_ReadOnlyOrCanSendCommandFalse | Integration | Error path: denial is visible as `received - forwarded`. [S2.1.2 / T2.1.2a] |
| SM-3 | `SVC/terminal_input_sites_test.go` | TestMuxInput_ShouldIncrementTally_When_TerminalMuxClientSendsInput | Integration | `/api/ws/terminal-mux` reaches a counted site. [S2.1.2 / T2.1.2c] |
| SM-3 | `SVC/terminal_input_sites_test.go` | TestInputSites_ShouldFail_When_NewInputReadLoopParamsWiringNotInAllowList | Unit | Enumeration guard against a future blind transport. [S2.1.2 / T2.1.2c] |
| SM-3 | `server/actuator_terminal_input_test.go` | TestActuatorTerminalInput_ShouldReturnJSONWithFramesBytesByKindLastAt_When_TwoSessionsRecorded | Integration | `GET /actuator/terminal-input`; no content field; `?format=text` also tested. [S2.1.3 / T2.1.3a] |
| SM-3 (diagnosable < 1 min) | manual instance, `docs/how-to/debug-phone-typing.md` | diagnose_should_ShowClientTypedServerReceived_When_FollowingHowToOnManualInstance | Manual | Run the how-to on ports 62871/62872: type in Pixel-7 emulation, read `/actuator/terminal-input`, compare with the `cm.input emitted` rate; stopwatch under 60 s. The pass criterion is the timed drill in the "Timed diagnosis drill" section below (three scripted faults, each named correctly in under 60 s). [S5.1.2 / T5.1.2b, T5.1.2d] |
| SM-4: client counts drops and shows indicator | `MI/attachMobileInput.test.ts` | executor_should_IncrementDroppedDisconnectedCounter_When_SinkReturnsDropped | Unit | PR-A counter `cm.input{event=dropped_disconnected}` (no chip yet). [S1.2.4 / T1.2.4b (A1), S2.2.2 / T2.2.2b (A2)] |
| SM-4 | `MI/useMobileInput.test.tsx` | chain_should_PropagateDroppedFromSendInputToLayer_When_PoolOnDataReturnsDropped | Integration | `SendResult` flows through `deliverTerminalData`, `handleTerminalData`, pool `onData`, renderer `onDataRef`. [S1.2.4 / T1.2.4b] |
| SM-4 | `CMP/InputStatusChip.test.tsx` | chip_should_RenderNotBeingSentWithinFakeTimers250ms_When_OnInputDroppedFires | Unit | Happy path (PR-B, `mobile_input_queue`). [S3.1.2 / T3.1.2a] |
| SM-4 | `HOOKS/useTerminalFlowControl.test.ts` | sendInput_should_KeepOldConsoleWarn_When_SubFlagOff | Unit | Error path: flag-off restores exact old warn. [S3.1.1 / T3.1.1a] |
| SM-4 | `E2E/mobile-input-layer.spec.ts` | disconnect_should_ShowTerminalInputDroppedChip_When_WebSocketBlockedAndKeyTyped | E2E | Default expect timeout, `data-testid` locator, no `waitForTimeout`. [S5.1.1 / T5.1.1c, PR-B] |
| SM-5: desktop latency/behavior unchanged | `MI/platform.test.ts` | shouldAttachMobileInput_should_BeFalse_When_DesktopIphoneIpadOsTouchLaptopFinePointerAndroid | Unit | Environment table: only {coarse, no-hover, Android} is true. [S1.2.1 / T1.2.1a] |
| SM-5 | `MI/platform.test.ts` | isTouchPrimary_should_BeTrue_When_AnyCoarsePointerRow | Unit | Error path: iPhone/iPad/touch laptop keep today's link-tap menu and selection handles. [S1.2.1 / T1.2.1a] |
| SM-5 | `MI/attachMobileInput.test.ts` | attach_should_ReturnNullAndAddZeroListenersAndNodes_When_DesktopIphoneOrTouchLaptop | Unit | Zero cost off-platform. [S1.2.2 / T1.2.2c] |
| SM-5 | `MI/useMobileInput.test.tsx` | mount_should_CreateNoMobileInputTextarea_When_NonAndroidDevice | Integration | Both renderers. [S1.4.1 / T1.4.1d] |
| SM-5 | `MI/platform.test.ts` | terminalScope_should_ContainPointerCoarseMatchMediaOnlyInPlatformTs_When_Scanned | Unit | Pinned-regex grep test with negative fixtures (`any-pointer`, `hover: none`). [S1.3.2 / T1.3.2c] |
| SM-5 | `web-app` full existing jest run (`bunx jest --no-coverage`) | existingTerminalTests_should_StayGreen_When_BranchBuilt | Integration | Regression gate over XtermTerminal/GhosttyTerminal/TerminalOutput tests. [S5.1.3 / T5.1.3a] |
| SM-5 | `E2E/mobile-input-layer.spec.ts` | desktop_should_RenderNoLayerTextareaOrNewChrome_When_FinePointerChromium | E2E | Desktop project; also existing terminal e2e specs stay green. [S5.1.1 / T5.1.1b] |
| SM-5 (latency) | owner phone and desktop | latency_should_AddAtMostOneFrameP95_When_ComparedWithBaseline | Manual (measured) | Not a feel check: desktop keydown-to-echo p50/p95 on the branch vs the Task 0.1.3c baseline on the same machine, added p95 <= 16.7 ms; phone `beforeinput`-to-`onData` delta p95 <= 16.7 ms from the diagnostics report; recorded, never a CI threshold. Automated proxy: NFR-1 fake-timer test. [S0.1.3 / T0.1.3c; S5.1.4 / T5.1.4e] |
| SM-6: owner phone acceptance in `stelekit` | `spike-findings.md` | ownerAcceptance_should_ShowSameFinalText_When_TypingAutocompleteAutocorrectSwipeOnDeployedBuild | Manual | Device-only: tap opens keyboard in one tap; "the quick brown fox" with one autocorrect and one swipe; both renderers; Gboard and Samsung if available. [S5.1.4 / T5.1.4b] Thresholds (per renderer): the 22-phrase scripted list (plan Story 5.1.4) passes 22/22 exact, zero eaten canary characters, zero duplicated characters, and a 200-character script arrives 100% (chars in pane / chars typed); baseline from Task 0.1.3c is `0/22` expected with ghostty on. [T5.1.4b, T5.1.4c, T5.1.4e] |
| SM-6 | `spike-findings.md` | killSwitch_should_RestoreXtermAfterSecondLoad_When_FlagFlippedOffOnDevice | Manual | Device-only: reload twice. [S5.1.4 / T5.1.4b] |

### In-scope items

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SC-1: hidden-element layer (beforeinput types, composition, diff rewrite, 229, attribute policy, orphaned input) | `MI/inputElement.test.ts` | createInputElement_should_SetAutocapitalizeNoneAndOmitSpellcheck_When_Created | Unit | Happy path (also `role`, `aria-label`, `data-testid`, vanilla-extract class, not `display:none`). [S1.2.1 / T1.2.1b] |
| SC-1 | `MI/inputElement.test.ts` | attributesFor_should_AddAutocorrectOffOnlyWhenEchoOff_When_PolicyEvaluated | Unit | Error/secret path (ADR-008): default omits `autocorrect`. [S1.2.1 / T1.2.1a, T1.2.1c] |
| SC-1 | `MI/composition.test.ts` | reduce_should_EmitDeltaOnceAndResetTextarea_When_OrphanedInputWithoutBeforeInput | Unit | Orphan fallback. [S1.1.2 / T1.1.2b] |
| SC-1 | `MI/attachMobileInput.test.ts` | attach_should_NotThrowAndNotLogTypedData_When_OnDataThrows | Integration | Handler exception guard. [S1.2.2 / T1.2.2b] |
| SC-1 | `MI/attachMobileInput.test.ts` | attach_should_KeepTextareaValue_When_CompositionUpdateDispatched | Unit | Reset only at commit boundaries. [S1.2.2 / T1.2.2c] |
| SC-1 | `MI/attachMobileInput.test.ts` | attach_should_SuppressEmissionAndSetInputmodeNone_When_IsReadOnlyTrue | Unit | Read-only. [S1.2.2 / T1.2.2c] |
| SC-2: both renderers via one seam | `MI/useMobileInput.test.tsx` | hook_should_AttachWhenDockedAndDisposeWhenUndocked_When_VisibilityChanges | Unit | Attach on visible/dock, not focus. [S1.4.1 / T1.4.1a] |
| SC-2 | `MI/useMobileInput.test.tsx` | hook_should_SendLayerBytesUnfilteredToOnData_When_OutputLooksLikeMouseReport | Unit | Bypasses `isMouseReport`. [S1.4.1 / T1.4.1a] |
| SC-2 | `MI/useMobileInput.test.tsx` | hook_should_SetAndRestoreHelperTextareaAttrs_When_AttachedThenDisposedOnXterm | Unit | `inputmode=none`, `tabindex=-1`. [S1.4.1 / T1.4.1b] |
| SC-2 | `MI/useMobileInput.test.tsx` | handleFocus_should_FocusLayerTextarea_When_LayerActive | Unit | Handle `focus` -> `controller?.focus() ?? term.focus()`. [S1.4.1 / T1.4.1a] |
| SC-2 | `MI/useMobileInput.test.tsx` | hook_should_PassStickyCtrlAsX03AndClearIt_When_SoftKeyboardC | Integration | Modifiers from `TerminalOutput`. [S1.4.1 / T1.4.1d] |
| SC-2 | `MI/useMobileInput.test.tsx` | hook_should_CallInvalidate_When_SessionIdGenerationConnectionPoolReuseOrLongHiddenChanges (one test per trigger) | Integration | Boundary wiring. [S1.2.4 / T1.4.1a] |
| SC-2 | `CMP/__tests__/mobileInputRenderers.test.tsx` | (see SM-1 both-renderer rows) | Integration | Shared setup helper, no duplicated `jest.mock` blocks. [S1.4.2 / T1.4.2a] |
| SC-3: server observability at all input entry points | `SVC/terminal_input_sites_test.go` | (see SM-3 rows: five sites, denial case, mux, enumeration) | Integration | [S2.1.2 / T2.1.2a-c] |
| SC-4: client observability (counter, indicator, class-only beacon) | `server/analytics/client_metrics_test.go` | TestClientMetrics_ShouldAcceptCmInputAndImeAnomaly_When_FixedEnumValues | Unit | Happy path. [S2.2.1 / T2.2.1a] |
| SC-4 | `server/analytics/client_metrics_test.go` | TestClientMetrics_ShouldRejectEvent_When_RendererFooOrKindNotInImeAnomalyEnum | Unit | Error path. [S2.2.1 / T2.2.1a] |
| SC-4 | `web-app/src/lib/telemetry/inputTelemetry.test.ts` | sender_should_SendAtMostFiveBeacons_When_50OrphanedInputAnomaliesInOneSecond | Unit | Rate limit 1 per kind per 10 s, burst 5 (fake clock). [S2.2.2 / T2.2.2a] |
| SC-4 | `web-app/src/lib/telemetry/inputTelemetry.test.ts` | beacon_should_ContainNoTypedSubstring_When_EmittedTextIsSecret | Unit | Class/enum only. [S2.2.2 / T2.2.2a] |
| SC-5: corpus, replay harness, e2e | `MI/fixtureLint.test.ts` | lint_should_FailNamingFile_When_DeviceFixtureNotScrubbedOrNoSignoff | Unit | Soft guard, glob over all committed fixtures, runs in CI. [S1.1.4 / T1.1.4d] |
| SC-5 | `MI/fixtureLint.test.ts` | lint_should_FailSecretScan_When_SyntheticFixtureContainsGhpPrefix | Unit | Secret-shape scan. [S1.1.4 / T1.1.4d] |
| SC-5 | `MI/replay.test.ts` | harness_should_ApplyLinkDownLinkUpAndSessionSwitchSteps_When_FixtureUsesThem | Unit | Five Story 1.2.4 fixtures. [S1.2.4 / T1.2.4d] |
| SC-6: renderer gate | `web-app/src/lib/terminal/rendererPolicy.test.ts` | resolveRenderer_should_ReturnXterm_When_GhosttyOnLayerOffAttachable | Unit | Happy path of the 8-combination table; `{true,true,true}` -> ghostty. [S1.3.2 / T1.3.2a] |
| SC-6 | `web-app/src/lib/terminal/rendererPolicy.test.ts` | resolveRenderer_should_ReturnGhostty_When_GhosttyOnLayerOffNotAttachable | Unit | Error/unchanged path: desktop and iOS keep ghostty. [S1.3.2 / T1.3.2a] |
| SC-6 | `web-app/src/lib/terminal/useMobileInputLayerActive.test.ts` (in `flagStore.test.ts` per plan) | useMobileInputLayerActive_should_BeFalse_When_FlagTrueAndIphoneUa | Unit | [S1.3.2 / T1.3.2b] |
| SC-6 | `E2E/mobile-input-layer.spec.ts` | gate_should_RenderXtermContainer_When_GhosttyOnLayerOffPixel7 | E2E | `data-testid` for renderer kind. [S5.1.1 / T5.1.1c] |

### Non-functional requirements

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| NFR-1: no added latency beyond one animation frame | `MI/attachMobileInput.test.ts` | executor_should_CallOnDataSynchronouslyInHandler_When_BeforeInputDispatchedWithFakeTimersAndNoRaf | Unit | Happy path: with fake timers and a rAF spy, `onData` is called before any timer advances. [S1.2.2 / T1.2.2b "rAF-free design", T1.2.2d] |
| NFR-1 | `MI/attachMobileInput.test.ts` | executor_should_NotScheduleTimersOrRaf_When_NotComposing | Unit | Error path: no per-keystroke scheduler work (watchdog tick only while composing). [S1.2.2 / T1.2.2b] |
| NFR-1 (desktop zero work) | `MI/attachMobileInput.test.ts` | attach_should_AddZeroListeners_When_DesktopHardwareKeyboard | Unit | (same as SM-5 row; one assertion, two requirements) [S1.2.2 / T1.2.2c] |
| NFR-1 (batching preserved) | `HOOKS/useTerminalFlowControl.test.ts` | existingInputBatchingCases_should_PassUnchanged_When_SendInputReturnsSendResult | Unit | `terminal-input-batching` regression. [S1.2.4 / T1.2.4a] |
| NFR-1 | `HOOKS/useTerminalFlowControl.test.ts` | sendInput_should_IssueExactlyOneWebSocketSend_When_AtomicEdit600Bytes | Unit | Atomic edit bypasses chunking; never split inside a code point. [S3.1.1 / T3.1.1b, PR-B] |
| NFR-1 | owner phone | typingLatency_should_FeelInstant_When_TypingOnPixelOrSamsung | Manual | Device-only; no automated wall-clock threshold (they flake in this repo). [S5.1.4 / T5.1.4b] |
| NFR-2: scalability, many terminals, layer attaches to visible/docked terminals, only one holds focus | `MI/useMobileInput.test.tsx` | hook_should_FocusOnlyTerminalB_AndSetAInputmodeNone_When_TwoDockedTerminalsAndBGainsFocus | Unit | Happy path. [S1.4.1 / T1.4.1a] |
| NFR-2 | `MI/useMobileInput.test.tsx` | hook_should_DisposeController_When_TerminalHiddenOrUndocked | Unit | Error path: no leaked listeners/DOM nodes for hidden terminals. [S1.4.1 / T1.4.1a] |
| NFR-3: input bytes never logged (security) | `MI/attachMobileInput.test.ts` | consoleError_should_ExcludeTypedData_When_OnDataThrowsAfterTypingSecret | Unit | Every `console.error` arg excludes `secret`; `onAnomaly` payload class only. [S1.2.2 / T1.2.2b] |
| NFR-3 | `SVC/terminal_input_metrics_test.go` | TestInputLogging_ShouldContainNoPayload_When_FirstInputAndSummaryEmitted | Unit | Capture injected logger output; asserts frame bytes absent and only path/listener/frames/bytes/kind fields. [S2.1.3 / T2.1.3b] |
| NFR-3 | `MI/diagnostics.test.ts` | report_should_ContainTextFiveAndNotHello_When_HelloTyped | Unit | Content-free report (PR-A1, pulled forward from PR-C). [S4.1.1 / T4.1.1b] |
| NFR-4: data residency | n/a | no test | N/A | "No special requirements" in requirements.md. |

### Observability requirements

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| OBS-1: `terminal_input_frames_total{kind}`, `terminal_input_bytes_total`, bounded labels | `SVC/terminal_input_metrics_test.go` | TestRecordTerminalInput_ShouldUseOnlyKindListenerPathStageLabels_When_Recorded | Unit | Happy path (label set exactly four; no session or title label). [S2.1.1 / T2.1.1a] |
| OBS-1 | `SVC/terminal_input_metrics_test.go` | TestRecordTerminalInput_ShouldAddNothingToBytes_When_EmptyFrame | Unit | Error/edge path. [S2.1.1 / T2.1.1b] |
| OBS-1 | `SVC/terminal_input_sites_test.go` | TestInputSites_ShouldExposeRegisteredCounters_When_MetricsGathered | Integration | Gather from the real registry after a site dispatch; confirms the exact series names. [S2.1.2 / T2.1.2c] |
| OBS-2: Info log on first input per stream + throttled summary | `SVC/terminal_input_metrics_test.go` | TestInputLog_ShouldEmitOneFirstInputLineAndZeroSummaries_When_100FramesIn10s | Unit | Happy path (injected clock/logger). [S2.1.3 / T2.1.3b] |
| OBS-2 | `SVC/terminal_input_metrics_test.go` | TestInputLog_ShouldEmitOneSummary_When_61sElapsed | Unit | Error/throttle path; at most 1 per minute per stream. [S2.1.3 / T2.1.3b] |
| OBS-3: entry in actuator view | `server/actuator_terminal_input_test.go` | TestActuatorTerminalInput_ShouldReturnTextFormat_When_FormatTextQuery | Integration | Plus the JSON case (see SM-3). [S2.1.3 / T2.1.3a] |
| OBS-4: alert condition, remote stream with client typing but zero text frames | `docs/reference/client-network-telemetry.md` + `docs/observability/grafana/dashboards/client-network.json` | dashboard_should_ParseAndHaveTerminalInputPanel_When_JsonLoaded | Unit | Go or Jest JSON check for the panel titled "Terminal input" and the documented diagnostic query. [S2.2.3 / T2.2.3a (dashboard JSON test), PR-A2] |
| OBS-4 | alert rule file beside `docs/observability/` | alertRule_should_ParseAndReferenceOnlyRegisteredSeries_When_Validated | Unit | Non-paging rule for client `emitted` > 0 with server `received` == 0 for 5 min on `listener="remote"`; if no provisioning convention exists the task stops and records the owner decision instead. [S2.2.3 / T2.2.3b, PR-A2] |
| OBS-4 | manual instance (VictoriaMetrics/Grafana or `/actuator/terminal-input`) | diagnosticQuery_should_ShowClientEmittedPositiveAndServerReceivedZero_When_ServerFrameStreamBlocked | Manual | Block the server path on the manual instance, confirm the documented query condition fires and `received` flat-positive/`forwarded` zero in the read-only case. The alert rule (T2.2.3b) is non-paging; if no provisioning convention exists, the owner decision recorded in requirements.md applies (see Gaps). [S2.2.3 / T2.2.3a, S5.1.2 / T5.1.2b] |
| OBS-5: client `input_dropped_disconnected_total`, `ime_event_sequence_unrecognized_total`, composition anomalies via beacon | `web-app/src/lib/telemetry/inputTelemetry.test.ts` | sender_should_MapDroppedDisconnectedAndUnrecognizedSequenceToCmSeries_When_Reported | Unit | Name mapping per Observability Plan: `cm.input{event=dropped_disconnected}`, `cm.ime_anomaly{kind=unrecognized_sequence}`. [S2.2.2 / T2.2.2b] |
| OBS-5 | `MI/attachMobileInput.test.ts` | executor_should_ReportAnomalyKindOnlyOncePerKindOnConsole_When_RepeatedAnomalies | Unit | Error path: `console.error` once per kind. [S1.2.2 / T1.2.2b] |
| OBS-5 | `docs/reference/client-network-telemetry.md` | doc_should_ListCmInputAndImeAnomalyRowsWithNameMapping_When_Grepped | Unit | Doc grep test (or review checklist) for both series and the requirement-name mapping table. [S2.2.3 / T2.2.3a] |
| OBS-6: never log input content | (see NFR-3 rows) | | Unit | Same three tests; additionally `inputTelemetry.test.ts` beacon-excludes-secret. [S1.2.2, S2.1.3, S2.2.2, S4.1.1] |

### Risk Control and Constraints

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| RC-1: live-settable flag `mobile_input_layer`, global + per-scope | `SVC/feature_flag_service_test.go` | TestListFeatureFlags_ShouldIncludeMobileInputLayerWithMergedDefaultAndKillSwitchDescription_When_FreshServer | Unit | Happy path; the asserted default is the Task 5.1.4c outcome (true on a pass, false plus owner override on a fail). [S1.3.1 / T1.3.1a] |
| RC-1 | `SVC/feature_flag_service_test.go` | TestFeatureFlag_ShouldReturnTrueForWorkspaceW_When_ScopeOverrideTrue | Unit | Per-scope override. [S1.3.1 / T1.3.1a] |
| RC-1 | `SVC/config_surface_proof_test.go` | TestConfigSurface_ShouldEnumerateMobileInputLayerFlag_When_FlagListEnumerated | Integration | Existing enumerating test updated. [S1.3.1 / T1.3.1a] |
| RC-1 (client mirror of default) | `web-app/src/lib/features/flagStore.test.ts` | preload_should_EqualGoDefaultConstant_When_NothingPersisted | Unit | Asserts `MOBILE_INPUT_LAYER_DEFAULT` equals the server default (Go-side flag list fixture). [S1.3.2 / T1.3.2b] |
| RC-1 (no env var) | `SVC/feature_flag_service_test.go` | TestMobileInputFlags_ShouldNotBeReadFromEnvironment_When_EnvVarSet | Unit | Error path: setting a lookalike env var changes nothing. Requires a non-parallel `t.Setenv`-free design (inject env reader) so it keeps `t.Parallel()`. [S1.3.1 / T1.3.1b] |
| RC-2: ghostty gated on touch unless layer active | (see SC-6 rows) | | Unit / E2E | `resolveRenderer` matrix and the Pixel 7 flag-off spec. [S1.3.2 / T1.3.2a; S5.1.1 / T5.1.1c] |
| RC-3: rollback by flipping flag off, no rebuild | `web-app/src/lib/features/flagStore.test.ts` | persistFlags_should_WriteZeroAndPreloadReturnFalse_When_ServerReportsDisabled | Unit | Happy path (also `"1"` when enabled). [S1.3.2 / T1.3.2d] |
| RC-3 | `web-app/src/lib/features/flagStore.test.ts` | preload_should_ApplyOffOnlyOnSecondLoad_When_FlagFlippedAfterFirstLoadPersisted | Integration | Simulates load 1 (server off, persists `"0"`) then load 2 (preload `false`). [S1.3.2 / T1.3.2d] |
| RC-3 | `E2E/mobile-input-layer.spec.ts` | rollback_should_RenderXterm_When_FlagSetOffAndPageReloadedTwice | E2E | Uses `helpers/feature-flag.ts`. [S5.1.1 / T5.1.1c] |
| RC-3 | owner phone | rollback_should_RestoreXtermPath_When_FlagFlippedOffInPanelOnPhone | Manual | Device-only (duplicate of SM-6 kill switch). [S5.1.4 / T5.1.4b] |
| CON-1: renderer-agnostic, swap cannot break phone typing | `MI/architecture.test.ts` | mobileInputModules_should_NotImportXtermOrGhosttyOrDom_When_ImportGraphScanned | Unit | Reducer files (`composition.ts`, `rewrite.ts`, `keymap.ts`) import no DOM/renderer module. [S1.2.1 / T1.2.1d] |
| CON-1 | `CMP/__tests__/mobileInputRenderers.test.tsx` | (see SM-1 both-renderer rows) | Integration | Same fixture, both renderers. [S1.4.2] |
| CON-2: Bun-only, `unset npm_config_user_agent` | CI / `make ready` | build_should_Succeed_When_UserAgentUnsetAndBunUsed | Integration | Part of `make ready`; spike 0.1.1a runs `bun install` with the unset. [S5.1.3 / T5.1.3a; S0.1.1 / T0.1.1a] |
| CON-3: repo gates (`make ready`, dupl, jscpd 0.14%, zero-warning lint, e2e conventions, nav facade, vanilla-extract, no hex) | CI `make ready` and `bun run lint:duplicates` | gates_should_ExitZero_When_MakeReadyRun | Integration | Output quoted in PR. No Go test filename contains "gate"; new Go tests call `t.Parallel()`. [S5.1.3 / T5.1.3a] |
| CON-3 | `E2E/mobile-input-layer.spec.ts` | specHeader_should_PassConventionCheck_When_FirstLineIsFeatureAnnotation | Integration | `// @feature terminal:input, terminal:mobile-input`, no `waitForTimeout`, `data-testid`/ARIA locators only. [S5.1.1 / T5.1.1b] |
| CON-3 | `CMP/*.css.ts`, `MI/mobileInput.css.ts` | styles_should_UseThemeTokensAndNoHexLiterals_When_Linted | Unit | Lint/grep test over new `.css.ts` files. [S1.2.1 / T1.2.1b; S3.1.2 / T3.1.2a] |
| CON-4: process (no force push, PR ready not draft, `gh pr merge --repo`, isolated manual instance, no `make install-service` pre-merge) | PR checklist | prChecklist_should_BeFollowed_When_PrOpened | Manual | Reviewer/owner checklist; pre-merge manual runs use ports 62871/62872 and a private tmux socket. [S5.1.3] |

### Open questions (each must be answered by a recorded result)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| OQ-1: ghostty off alone restores typing? | `spike-findings.md` | (see PS-2) | Manual | Device-only. No longer a blocker; wording in how-to only. [S0.1.3 / T0.1.3a; S5.1.2 / T5.1.2b] |
| OQ-2: exact Gboard/Samsung sequences | `MI/__fixtures__/*.json` (`source:"device"`) | deviceFixture_should_PassReplayWithScrubbedTrueAndSignoff_When_CapturedFromOwnerPhone | Manual | Device-only; captured via Story 4.1.3, committed after the scrub checklist. Summary reads `synthetic N/N, device k/k`. [S5.1.4 / T5.1.4a, PR-C] |
| OQ-3: beforeinput sufficient vs EditContext | `spike-findings.md` | editContextDecision_should_BeRecordedAsNeededOrNotNeeded_When_DeviceCorpusReviewed | Manual | [S6.1.1 / T6.1.1a, PR-E] |
| OQ-3 | `MI/editContextSource.test.ts` | attach_should_UseEditContextSource_When_SubFlagOnAndEditContextInWindow | Unit | And `textarea` when absent. Conditional on OQ-3 decision. [S6.1.1 / T6.1.1b] |
| OQ-4: how other web terminals handle IME | `project_plans/mobile-input-layer/research/*.md` | researchDocs_should_CoverNamedTerminals_When_Reviewed | Manual | Document review (xterm.js #3600, Codeman PR 65, hterm, VS Code, Termux, Blink, CodeMirror 6). Phase 2 deliverable. |
| OQ-5: metric home | `SVC/terminal_input_metrics_test.go` + `server/actuator_terminal_input_test.go` | (see OBS-1, OBS-3) | Unit / Integration | Decision (Prometheus counters plus actuator JSON) is verified by both existing. ADR-005. |

### Plan-derived safety invariants (not in requirements.md but gating the flag-ON-at-merge decision)

| Source | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.2.4 `no-DEL-across-boundary` | `MI/deliveryInvariant.property.test.ts` | deleteCount_should_NeverExceedCurrentEpochGraphemes_When_RandomLinkSessionImeInterleavings | Unit | fast-check fixed seed + 200 runs. [T1.2.4e] |
| Story 1.2.4 | `MI/deliveryLedger.test.ts` | ledger_should_EmitInsertOnlyTailLp_When_HeSentLDroppedAndRegionBecomesHelp | Unit | And zero bytes when the region autocorrects to `world`. [T1.2.4c] |
| Story 1.2.4 | `MI/attachMobileInput.invalidate.test.ts` | invalidate_should_EmitZeroBytesAndClearTextarea_When_SessionIdChangesMidComposition | Unit | One test per reason (`session`, `pool_reuse`, `visibility`, `queue_discard`, `transport`), plus a stale-region variant (trailing `compositionend`, orphaned `input`, Backspace `beforeinput` emit zero bytes). [T1.2.4e] |
| Story 1.2.4 | `HOOKS/useTerminalFlowControl.test.ts` | sendInput_should_ReturnDropped_When_PushMessageThrows | Unit | Mid-paste abort is covered by the isConnected/sessionId invalidate triggers, not a return value. Strict-FIFO contract test with stub queue also here. [T1.2.4a] |
| Story 1.2.4 | `MI/replay.test.ts` | fiveBoundaryFixtures_should_HoldFinalTextAndInvariant_When_Replayed | Unit | `partial-delivery-then-drop`, `keystroke-after-reconnect-before-flush`, `session-switch-mid-composition`, `sent-then-close-then-autocorrect`, `enter-after-mid-region-drop`. [T1.2.4d] |
| Story 3.1.3 | `web-app/src/lib/terminal/InputQueue.test.ts` | queue_should_AcceptOnlyPlainTextUnitsAndDiscardWholeQueue_When_ControlByteOrOverflowOrTtl | Unit | PR-B; fake clock; units never split/merged. [T3.1.3a] |
| Story 3.1.3 | `HOOKS/useTerminalFlowControl.test.ts` | flush_should_KeepStrictFifoAndDiscardOnSessionChange_When_LiveKeystrokeDuringFlush | Integration | PR-B. [T3.1.3b, T3.1.3c] |
| Story 3.1.4 | `MI/replay.test.ts` | disconnectAutocorrectReconnect_should_YieldDollarSpaceThe_When_ReplayedThroughRealInputQueue | Integration | PR-B. [T3.1.4b] |
| ADR-008 | `MI/inputElement.test.ts` | attributes_should_OmitAutocorrectOff_When_NoEchoOffSignal | Unit | Limitation is documented in `docs/reference/mobile-input-layer.md` (grep test, S5.1.2). |
| Story 4.1.3 | `MI/captureForFixture.test.ts` | capture_should_MakeZeroNetworkCallsAndClearOnClose_When_ThePhraseTyped | Unit | Spies on `fetch`, `XMLHttpRequest`, `sendBeacon`. PR-C. [T4.1.3a] |
| Story 4.2.1 | `CMP/ComposeBar.test.tsx` | send_should_WrapInBracketedPasteAndStrip201Tilde_When_Mode2004On | Unit | PR-D. Fallback `a\nb` -> `a b`. [T4.2.1b] |

### Phase 4 plan-repair additions (pre-mortem P1-P3, consistency B1-B3, validation gaps)

| Requirement / source | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| RC-1 conditional default (B1, pre-mortem P1-1) | `spike-findings.md` | preMergeGate_should_DecideMergedDefault_When_OwnerPhoneTypesOnIsolatedManualInstance | Manual, device-only | Owner phone on `:62871`/`:62872`: tap opens keyboard, letters, autocorrect, swipe, Backspace after commit, Enter, both renderers. Pass: default ON committed; fail: default OFF + per-scope override + Copy report. [S5.1.4 / T5.1.4c, owner blocker] |
| RC-3 same-load kill switch (P1-2) | `web-app/src/lib/features/flagStore.test.ts` | preload_should_ReturnFalseInSameLoadAndPersist_When_QueryParamMobileInput0 (and `=1`, garbage ignored) | Unit | Param read before first mount; writes the separate `ssq:flag-override:` key, which survives `persistFlags` and is cleared by `recordFlagWrite` (tests `override_should_SurviveServerRefreshAndStillWin_...`, `override_should_BeClearedAndServerValueGovern_...`, `useMobileInputLayerActive_should_HonorOverrideAheadOfServerValue_...`). [S1.3.2 / T1.3.2e] |
| RC-3 | `E2E/mobile-input-layer.spec.ts` | queryParamMobileInput0_should_RenderXtermAndFlagOffHint_When_OpenedOnPixel7 | E2E | Same-load escape; hint visible. [S5.1.1 / T5.1.1c] |
| P1-2 flag-off never silent | `CMP/useMobileInputChrome.test.tsx` | chrome_should_ShowFlagOffHint_When_AndroidClassAndFlagFalse (absent on desktop/iOS) | Jest | Hint text per plan Story 1.3.2. [S1.3.2 / T1.4.1e] |
| P1-3 Backspace on empty (Story 0.1.x) | `MI/composition.test.ts` + `MI/__fixtures__/backspace-on-empty-after-commit.json` | reduce_should_EmitOneDelExactly_When_BackspaceAfterCommitOnResetTextarea (keydown-8 only, beforeinput only, both) | Unit | Design from spike. [S1.1.2 / T1.1.2b, T1.1.4e; spike T0.1.2b, T0.1.3b] |
| P1-3 | `spike-findings.md` | backspaceOnEmpty_should_BeRecordedForCdpAndOwnerDevice_When_ProbeRun | Manual | CDP and device results table; chooses sentinel vs keydown-8. [S0.1.2 / T0.1.2b; S0.1.3 / T0.1.3b] |
| P1-4 tap target | `MI/attachMobileInput.focus.test.ts` | bindTapTarget_should_FocusSynchronouslyOnce_When_PointerupTouchendClickAndGestureHandlersPreventDefault (incl. pointercancel then click) | Jest | Bound to the visible surface, not the 1px textarea. [S1.2.3 / T1.2.3c] |
| P1-4 | `spike-findings.md` | tapOpensKeyboard_should_HoldOnPhone_When_GestureHandlersActive | Manual, device-only | [S0.1.3 / T0.1.3b; S5.1.4 / T5.1.4c] |
| P1-1 minimal diagnostics in A1 | `CMP/MinimalInputDiagnostics.test.tsx` | (see UX-19 PR-A1 row) | Jest | [S2.3.1 / T2.3.1a] |
| P2-5 dependency bump | `web-app/src/lib/terminal/ghosttyVersionPin.test.ts` | pin_should_FailWithGreppingInstruction_When_GhosttyWebVersionChanges | Unit | [S1.4.2 / T1.4.2b] |
| P2-5 | CI workflow path filter | ghosttyDoubleSendE2e_should_RunAsRequiredCheck_When_PackageJsonOrLockfileChanges | CI | Within the CI job budget. [S5.1.1 / T5.1.1d] |
| P2-6 platform cases | `MI/platform.test.ts` | shouldAttachMobileInput_should_BeTrue_When_AndroidPlainHttpNoUserAgentDataOrStandalonePwa | Unit | UA fallback and display-mode rows. [S1.2.1 / T1.2.1a] |
| P2-6 listener label | `SVC/terminal_input_sites_test.go` | TestInputSites_ShouldRecordListenerRemote_When_MuxAndRemoteListenerPaths | Integration | [S2.1.2 / T2.1.2c, PR-A2] |
| P2-6 origins | `spike-findings.md` | originsMatrix_should_PassOnRemote8444PwaAndPlainHttp_When_OwnerRuns | Manual, device-only | [S5.1.4 / T5.1.4b, T5.1.4c] |
| P2-7 redeploy | `spike-findings.md` | acceptance_should_StartWithBuildIdCheckAndSurviveRestart_When_InstallServiceRedeploy | Manual, device-only | Build id in the `?inputdebug=1` header matches the deployed commit; restart is a transport boundary. [S5.1.4 / T5.1.4b] |
| P2-8 gates | `spike-findings.md` | baseline_should_RecordSmokeAndMakeReady_When_StubBranchRun | Manual | bun install, make build, go and jest smoke, `make ready`, `.jscpd.json` fixture exclusion. [S0.1.4 / T0.1.4a, T0.1.4b] |
| P2-8 | `web-app/.jscpd.json` + `bun run lint:duplicates` | duplicates_should_StayWithin0_14Percent_When_21FixturesPresent | CI | [S5.1.3 / T5.1.3a] |
| P3-11 hardware keyboard | `MI/__fixtures__/hardware-keyboard-keydown.json` + `E2E/mobile-input-layer.spec.ts` | hardwareKeyboard_should_TypeOnceOnly_When_PlaywrightKeyboardOnPixel7 | Unit + E2E | [S1.1.4 / T1.1.4e; S5.1.1 / T5.1.1b] |
| P3-11 | `spike-findings.md` | bluetoothKeyboardAndDex_should_TypeOnce_When_AttachedToPhone | Manual, device-only | [S5.1.4 / T5.1.4b] |
| P3-13 Ctrl-W | `spike-findings.md` | ctrlW_should_DeleteWordOrFallbackToBoundedDel_When_LongPressBackspaceOnPhone | Manual, device-only | Result also decides T1.1.2a mapping. [S0.1.3 / T0.1.3a; S5.1.4 / T5.1.4b] |
| NFR-1 latency budget (C4) | `MI/attachMobileInput.test.ts` | executor_should_CallOnDataBeforeAnyTimerAdvances_When_BeforeInputDispatched | Unit | [S1.2.2 / T1.2.2d] |
| NFR-1 batching (C4) | `HOOKS/useTerminalFlowControl.test.ts` | existingInputBatchingCases_should_PassUnchanged_When_SendInputReturnsSendResult | Unit | [S1.2.4 / T1.2.4a] |
| Gap: env var | `SVC/feature_flag_service_test.go` | TestMobileInputFlags_ShouldNotBeReadFromEnvironment_When_EnvVarSet | Unit | [S1.3.1 / T1.3.1b] |
| Gap: alert | alert rule file | (see OBS-4 alert row) | Unit | [S2.2.3 / T2.2.3b] |

### Triad-review repair additions (2026-11-08)

| Source gap | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Product: numeric acceptance thresholds | `spike-findings.md` | acceptance_should_PassAll22PhrasesExactWithZeroEatenCanaryChars_When_RunPerRenderer | Manual, device-only | 22 scripted phrases (plain 5, autocorrect 4, suggestion picks 4, swipe 3, Backspace runs 4, Enter and symbols 2) at a prompt holding `ssq-prompt: `; 22/22 exact per renderer; pre-merge (T5.1.4c) and post-redeploy (T5.1.4b). [S5.1.4 / T5.1.4b, T5.1.4c] |
| Product: baseline measured | `spike-findings.md` | baseline_should_RecordExactCountLossAndDesktopLatency_When_RunOnCurrentMain | Manual | Phone exact N/22 per renderer, chars received / typed, desktop p50/p95 keydown-to-echo. [S0.1.3 / T0.1.3c] |
| Product: loss and latency re-measure | `spike-findings.md` | remeasure_should_ShowLossZeroAndAddedP95WithinOneFrame_When_ComparedToBaseline | Manual | [S5.1.4 / T5.1.4e] |
| Product: diagnosable in under a minute | `spike-findings.md` | diagnosisDrill_should_NameFailingHopInUnder60sEach_When_ThreeFaultsInjected | Manual (timed) | See "Timed diagnosis drill" below. [S5.1.2 / T5.1.2d] |
| UX gap 1: TalkBack typed characters (UX-31) | `MI/attachMobileInput.test.ts` | commit_should_WriteCommittedWordOnceToPoliteRegionAndKeepRingClassOnly_When_HelloCommitted | Jest | Textarea value `""` after commit; Backspace run announces nothing; read-only announces nothing. [S1.4.4 / T1.4.4b] |
| UX gap 1 (real) | `spike-findings.md` | talkback_should_AnnounceCommittedWordOnceAndNameElement_When_TypingHelloOnDevice | Manual, device-only | Owner phone, TalkBack on, pre-merge session. [S5.1.4 / T5.1.4d] |
| UX gap 2: polite vs assertive (UX-38) | `MI/announce.test.ts` | announce_should_RouteEachMessageToItsRegionAndWriteOnce_When_StateRerenders | Jest | Table over the classification (chip states polite; discard and stuck assertive); 5 re-renders = 1 write; assertive never mirrored in polite. [S1.4.4 / T1.4.4a] |
| UX gap 3: flicker (UX-32) | `MI/replay.test.ts` | rewrite_should_ShowAtMostOneIntermediateFrameAndNeverShortenPrompt_When_AutocorrectSampledPerFrame | Jest | Per-frame sampler on the autocorrect fixtures. [S1.2.5 / T1.2.5a] |
| UX gap 3 | `MI/attachMobileInput.test.ts` | rewrite_should_CallOnDataOnceBeforeAnyTimerOrRaf_When_InsertReplacementText | Jest (fake timers) | One frame budget. [S1.2.5 / T1.2.5a] |
| UX gap 3 | `E2E/mobile-input-layer.spec.ts` | autocorrect_should_ShowAtMostOneIntermediateStateAndNoBlankFrame_When_SampledEachAnimationFrame[xterm, ghostty] | E2E | Structural count of distinct intermediate lines, no wall-clock threshold. [S5.1.1 / T5.1.1e] |
| UX gap 3 (DeferredCommit) | `CMP/useMobileInputChrome.test.tsx` | indicator_should_ShowCharactersWaitingAfter300msAndClearOnCommit_When_100GraphemeDictation | Jest (fake timers) | Also cleared by the stuck watchdog. [S1.2.5 / T1.2.5b] |
| UX gap 4: minimum rows (UX-37) | `E2E/mobile-input-layer.spec.ts` | terminal_should_KeepMinimumRows_When_ToolbarComposeChipAndKeyboardViewport[360x340, 320x300, 740x170, 360x240] | Playwright | Rows >= 5, 5, 3, 3 via `floor(containerHeight / cellHeight)`. [S1.4.3 / T1.4.3b] |
| UX gap 4 | `MI/chromeBudget.test.ts` | chromeBudget_should_ReturnCompactAndBadgeChip_When_LandscapeKeyboardViewport | Jest | Density and priority rules. [S1.4.3 / T1.4.3a] |
| UX gap 4 (360px) | `E2E/mobile-input-layer.spec.ts` | chip_should_NotOverlapOrScrollHorizontally_When_360pxDisconnectedWithComposeOpen | Playwright | Buttons >= 44x44, gaps >= 8px, no intersecting boxes. [S1.4.3 / T1.4.3b] |
| UX gap 5: keyboard resize (UX-33) | `spike-findings.md` | keyboardResize_should_RecordModeAndCursorRowVisibility_When_ChromeSamsungSplitFloatingProbed | Manual, device-only | Before/after table of `innerHeight`, `visualViewport.height/offsetTop`, `clientHeight`, `100dvh`; named mode. [S0.1.5 / T0.1.5a, T0.1.5b] |
| UX gap 5 | `E2E/mobile-input-layer.spec.ts` | cursorRow_should_StayInsideViewportAboveToolbar_When_VisualViewportShrunkOnPixel7 | Playwright | Emulation proxy. [S0.1.5 / T5.1.1b, T5.1.1e] |
| UX gap 6: compose offer on anomalies (UX-34) | `MI/fallbackDetector.test.ts` | detector_should_OfferComposeVariant_When_3AnomaliesWithin60s (and not when spread over 90 s, not after opt-out) | Jest | [S4.2.2 / T4.2.2c] |
| UX gap 7: flag-off copy (UX-39, UX-28) | `CMP/useMobileInputChrome.test.tsx` | hint_should_UsePlainLanguageWithoutQueryOrFlagIdentifier_When_AndroidClassAndFlagFalse | Jest | Text equals the Story 1.3.2 string; asserts no `?` and no `mobile_input`. [S1.3.2 / T1.4.1e] |
| UX gap 8: stuck copy (UX-35) | `CMP/InputStatusChip.test.tsx` | chip_should_ShowSlowReconnectFlushSlowAndFlushStuckCopy_When_FakeTimers10s5sAnd15s | Jest (fake timers) | Stuck state announced once via the assertive region; completion replaces all. [S3.1.5 / T3.1.5b] |
| UX gap 9: diagnostics entry (UX-36) | `CMP/InputDiagnosticsSheet.test.tsx` | sheet_should_OpenFromSettingsEntryInTwoTapsWithoutLongPress_When_PhoneViewport | Jest | Plus chip `contextmenu` suppressed, `user-select:none`. [S4.1.2 / T4.1.2c] |
| Eng: fixture split | `MI/replay.test.ts` | corpus_should_HaveAtLeast14SyntheticFixturesEachWithSource_When_DirectoryScanned | Unit | Unchanged assertion; produced by T1.1.4b + T1.1.4f + T1.1.4g (5/5/4 files) and the 5 + 2 fixtures from T1.2.4d and T1.1.4e. [S1.1.4] |
| Eng: OBS-4 owner-decision fallback | `requirements.md` Observability | alertRuleOrRecordedOwnerDecision_should_ExistWhenTask223bComplete | Manual | Either the rule file validates (expression parses, registered series) or `requirements.md` carries the OPEN owner-decision line. [S2.2.3 / T2.2.3b] |

### Timed diagnosis drill (SM-3 "diagnosable in under a minute")

Run once after PR-A1 and PR-A2 are merged and deployed to the isolated manual instance (ports 62871/62872); procedure and records in `spike-findings.md`, script in plan Task 5.1.2d. A facilitator injects one fault at a time without naming it; the operator has only `docs/how-to/debug-phone-typing.md`, a phone and a laptop, and a stopwatch runs from "typing is broken" to the operator naming the failing hop.

| Drill | Injected fault | Expected diagnosis (failing hop) | Evidence the operator must use | Pass |
|---|---|---|---|---|
| A | `?mobile_input=0` on the phone (layer off) | client emits nothing | `?inputdebug=1` header `layer: off`, `cm.input` rate 0 | named correctly, < 60 s |
| B | Read-only session with the layer on | client sends, server receives but denies | `/actuator/terminal-input`: `received` rising, `forwarded` flat | named correctly, < 60 s |
| C | WebSocket blocked | client drops while disconnected | `cm.input{event=dropped_disconnected}` and diagnostics `dropped` counter | named correctly, < 60 s |

A wrong hop or a time over 60 s fails that drill: revise the how-to and repeat until all three pass; record every attempt.

## UX Acceptance Tests

Every criterion UX-1..UX-39 has at least one test (UX-31..UX-39 were added by the triad-review repair; their rows are in the "Triad-review repair additions" table above and are summarized in the UX rows below). "Device-only" marks proofs that cannot be completed in CI or on the dev machine. PR-A1 has no chip, compose, full diagnostics sheet or fallback notice (it does have the minimal `?inputdebug=1` panel and the flag-off hint); their rows apply when PR-B/C/D ship.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-1: one tap opens keyboard, cursor row visible | `E2E/mobile-input-layer.spec.ts` | tap_should_FocusLayerTextareaInOneTapAndKeepCursorRowInViewport_When_FirstTapOnDockedTerminal | Playwright (Pixel 7) | Tap terminal; assert `activeElement` is `mobile-input-textarea`; shrink viewport to emulate IME and assert cursor row bounding box is inside it. [S5.1.1 / T5.1.1b, PR-A1] |
| UX-1 (real keyboard) | `spike-findings.md` | keyboardOpens_should_HoldOnGboardAndSamsung_When_TappedOnDevice | Manual, device-only | Phone: tap once, keyboard appears, cursor row visible above toolbar. [S5.1.4 / T5.1.4b] |
| UX-2: autocomplete/autocorrect/swipe/Backspace match hardware | `MI/replay.test.ts` | (SM-1 table: autocomplete pick, autocorrect after space, swipe word, Backspace-in-composition fixtures) | Jest replay | Automated proxy on synthetic fixtures. [S1.1.4] |
| UX-2 | `E2E/mobile-input-layer.spec.ts` | typing_should_ShowTheInPane_When_ImeComposesTehThenReplacesWithThe | Playwright (Pixel 7, both renderers) | `imeCompose "teh"`, replace with `the`, assert pane line `the`, prompt text untouched. [T5.1.1b] |
| UX-2 (real) | `spike-findings.md` | sameSentence_should_MatchHardwareKeyboard_When_TypedOnOwnerPhone | Manual, device-only | Type "the quick brown fox" with one autocorrect and one swipe word; compare with a hardware keyboard. [T5.1.4b] |
| UX-3: toolbar keys immediate, never close keyboard | `MI/attachMobileInput.focus.test.ts` | toolbarCtrl_should_KeepTextareaActive_When_PointerDownPreventDefault | Jest (jsdom) | Focus textarea, `pointerdown` on Ctrl button, `activeElement` unchanged. [S1.2.3 / T1.2.3b] |
| UX-3 | `E2E/mobile-input-layer.spec.ts` | toolbarEsc_should_SendEscapeAndKeepTextareaFocused_When_Tapped | Playwright (Pixel 7) | Tap Esc/arrow keys; assert byte effect in pane and `activeElement` still the layer textarea. [T5.1.1b] |
| UX-3 (real keyboard stays open) | `spike-findings.md` | keyboardStaysOpen_should_HoldAfterToolbarTaps_When_OnDevice | Manual, device-only | Tap Esc, Tab, arrows, Ctrl; keyboard never closes. [T5.1.4b] |
| UX-4: Enter never reorders | `MI/composition.test.ts` | reduce_should_EmitLsThenCarriageReturnOnce_When_EnterAndCompositionEndInEitherOrder | Jest | Three orderings from Story 1.1.2. [T1.1.2b] |
| UX-4 | `E2E/mobile-input-layer.spec.ts` | enter_should_SubmitLsOnce_When_ImeCommitThenEnter | Playwright (Pixel 7) | Compose `ls`, Enter, pane shows `ls` executed once. [T5.1.1b] |
| UX-4 (real) | `spike-findings.md` | enter_should_NotReorder_When_PressedAfterWordOnGboard | Manual, device-only | Type a word then Enter in `stelekit`. [T5.1.4b] |
| UX-5: desktop no new UI, typing unchanged | `E2E/mobile-input-layer.spec.ts` | desktop_should_ShowNoLayerOrChipOrComposeDom_When_HardwareKeyboardTyping | Playwright (desktop Chromium) | Type into terminal; assert no `mobile-input-textarea`, `terminal-input-dropped`, `compose-bar-input`; existing terminal specs green. [T5.1.1b] |
| UX-5 | `MI/useMobileInput.test.tsx` | mount_should_CreateNoDomOrListeners_When_NonAndroid | Jest | [T1.4.1d] |
| UX-5 (feel) | desktop manual | desktopTyping_should_FeelUnchanged_When_HardwareKeyboard | Manual | Not device-only; any desktop. [T5.1.4b] |
| UX-6: chip within 250 ms of first typed char while disconnected | `CMP/InputStatusChip.test.tsx` | chip_should_ContainNotBeingSent_When_FakeTimers250msAfterOnInputDropped | Jest (fake timers) | PR-B. The 250 ms bound is asserted only here. [T3.1.2a] |
| UX-6 | `E2E/mobile-input-layer.spec.ts` | disconnect_should_ShowChipAboveToolbar_When_StreamBlockedAndKeyTyped | Playwright | Default expect timeout; chip above toolbar by bounding boxes. [T5.1.1c] |
| UX-7: Reconnect >= 44x44, one tap | `E2E/mobile-input-layer.spec.ts` | reconnectButton_should_Measure44x44AndStartReconnect_When_TappedOnce | Playwright (Pixel 7) | `boundingBox()` >= 44 both axes; tap starts reconnect, label "Reconnecting..." and `aria-busy`. [T3.1.2a, T5.1.1c] |
| UX-7 | `CMP/InputStatusChip.test.tsx` | reconnect_should_CallReconnectAndDisableButton_When_Tapped | Jest | [T3.1.2a] |
| UX-8: flush once, in order, count confirmed, gone within 4 s | `HOOKS/useTerminalFlowControl.test.ts` | flush_should_SendQueuedUnitsOnceInOrder_When_ReconnectToSameSessionWithin30s | Jest (fake clock) | [T3.1.3b] |
| UX-8 | `CMP/InputStatusChip.test.tsx` | chip_should_ShowSentNCharsAndBeRemoved_When_4sFakeTimeElapses | Jest | [T3.1.5a] |
| UX-9: Esc/arrows/Ctrl never replayed | `web-app/src/lib/terminal/InputQueue.test.ts` | queue_should_RejectEscArrowCtrlAndLoneBackspace_When_Disconnected | Jest | Results `dropped,dropped,dropped,queued`; after reconnect sink receives none of them. [T3.1.3a] |
| UX-9 | `CMP/InputStatusChip.test.tsx` | chip_should_ShowKeysNotSentWithoutQueueCount_When_OnlyControlInput | Jest | [T3.1.5a] |
| UX-10: overflow, TTL, session change each show "Discarded N chars ..." with dismiss | `CMP/InputStatusChip.test.tsx` | discardNotice_should_ShowCauseSpecificTextDismissAndRestoreFocus_When_OverflowTtlOrSessionChange (3 variants) | Jest | Texts are exactly `Discarded N chars typed while offline.` / `Discarded N chars - offline for more than 30 s.` / `Discarded N chars - the session changed.`; Dismiss is a 44px button; focus returns to the layer textarea. [T3.1.5a] |
| UX-10 | `web-app/src/lib/terminal/InputQueue.test.ts` | queue_should_DiscardWholeQueueAndEmitOnQueueDiscard_When_4106BytesOr31sOrSessionChange | Jest | [T3.1.3a, T3.1.3c] |
| UX-11: failed reconnect | `CMP/InputStatusChip.test.tsx` | chip_should_ShowStillDisconnectedAndReenableButton_When_ReconnectFails | Jest | [T3.1.5a] |
| UX-12: read-only does not open keyboard; hint once per 10 s | `MI/useMobileInput.test.tsx` | readOnlyTap_should_SetInputmodeNoneAndRenderStatusHint_When_SessionReadOnly | Jest | `role="status"`, `aria-live="polite"`. [T1.4.1d] |
| UX-12 | `E2E/mobile-input-layer.spec.ts` | readOnly_should_NotOpenKeyboardAndShowViewOnlyHint_When_TappedTwiceWithin10s | Playwright (read-only session) | Second tap does not stack a second hint (existing `TerminalOutput.tsx:1068-1080` rate limit). [T5.1.1b] |
| UX-13: extra-keys row and Compose toggle hidden when read-only | `CMP/ComposeBar.test.tsx` | toggle_should_BeHiddenAndOpenBarClosed_When_SessionBecomesReadOnly | Jest | PR-D. [T4.2.1d] |
| UX-13 | `CMP/__tests__/TerminalOutput.readOnly.test.tsx` (new; Task 1.4.1f) | extraKeysRow_should_NotRender_When_ReadOnly | Jest | Verify existing behavior or add. [S1.4.1 / T1.4.1f] |
| UX-14: Compose toggle 44px, opens textarea with Send/Send+Enter, off by default | `CMP/ComposeBar.test.tsx` | compose_should_RenderInputSendAndSendEnter_When_TogglePressedAndBeAbsentByDefault | Jest | `data-testid="compose-bar-input"`, `aria-label="Send"`, `aria-pressed`. PR-D. [T4.2.1a] |
| UX-14 | `E2E/mobile-input-layer.spec.ts` | composeToggle_should_Measure44pxAndOpenBarInOneTap_When_Tapped | Playwright (Pixel 7) | [T4.2.1c] |
| UX-15: Send as one unit, Send+Enter one `\r` | `CMP/ComposeBar.test.tsx` | send_should_EmitBracketedPasteWithNoCarriageReturn_When_Mode2004On / sendEnter_should_AppendOneCarriageReturn | Jest | Fallback `a b`. [T4.2.1b, T4.2.1d] |
| UX-16: > 4096 chars offline not cleared, error shown | `CMP/ComposeBar.test.tsx` | send_should_KeepDraftAndShowTooLongError_When_Disconnected5000Chars | Jest | Exact string "Too long to hold while offline (4096 chars max)." [T4.2.1d] |
| UX-17: draft kept on close | `CMP/ComposeBar.test.tsx` | reopen_should_RestoreDraftAbc_When_ClosedAfterTypingAbc | Jest | Draft discarded on session change. [T4.2.1d] |
| UX-18: fallback notice after 5 keydown-229 with no text (the anomaly-window trigger is UX-34) | `MI/fallbackDetector.test.ts` | detector_should_FireNoticeAndUnrecognizedSequence_When_5Keydown229AndZeroBeforeInput | Jest | PR-D. [T4.2.2a] |
| UX-18 | `CMP/ComposeBar.test.tsx` (chrome hook test) | notice_should_PersistOptOutAndSuppressAfterReload_When_TurnOffTapped | Jest | "Got it" suppresses until reload only; localStorage blocked works per session. [T4.2.2b] |
| UX-19: sheet in <= 2 taps or `?inputdebug=1`, titled "Input diagnostics" | `CMP/InputDiagnosticsSheet.test.tsx` | sheet_should_ShowDialogTitledInputDiagnostics_When_InputdebugQueryParam | Jest | PR-C. [T4.1.2a, T4.1.2b] |
| UX-19 (PR-A1 minimal) | `CMP/MinimalInputDiagnostics.test.tsx` | panel_should_ShowDialogWithBuildIdAndCopyContentFreeReport_When_InputdebugQueryParam | Jest | Read-only panel, build id header, Copy report has `text(1)` entries and no typed character; without the param nothing renders and buffer length stays 0. [S2.3.1 / T2.3.1a, PR-A1] |
| UX-19 | `E2E/mobile-input-layer.spec.ts` | diagnostics_should_OpenInTwoTaps_When_OverflowThenInputDebug | Playwright | [T4.1.2b; the `?inputdebug=1` entry is already covered in PR-A1 by T2.3.1a] |
| UX-20: header shows renderer + fallback reason | `CMP/InputDiagnosticsSheet.test.tsx` | header_should_ReadRendererXtermFallbackLayerOff_When_GhosttyOnLayerOffAndroid | Jest | [T4.1.2a] |
| UX-21: Copy/Clear/Pause; copied JSON has no text even with Reveal | `MI/diagnostics.test.ts` | copiedReport_should_ContainNoTypedText_When_RevealOn | Jest | [T4.1.1b] |
| UX-21 | `CMP/InputDiagnosticsSheet.test.tsx` | clear_should_EmptyTableAndResetCounters_When_3EventsThenClear | Jest | Pause freezes, Resume unfreezes; revealBuffer empty on close. [T4.1.2a, T4.1.1b] |
| UX-22: closes via Close/Back/scrim/Escape; focus returns to opener | `CMP/InputDiagnosticsSheet.test.tsx` | sheet_should_CloseAndRestoreOpenerFocus_When_CloseBackScrimOrEscape (4 variants) | Jest + Playwright | 390px: `aria-modal`, focus trap; 1280px: non-modal, Tab leaves. [T4.1.2a] |
| UX-23: clipboard blocked | `CMP/InputDiagnosticsSheet.test.tsx` | copy_should_ShowSelectableReportWithManualGuidance_When_ClipboardRejects | Jest | [T4.1.2a] |
| UX-24: >= 44x44 with 8px spacing, accessible name, focus ring 3:1 | `E2E/mobile-input-layer.spec.ts` | newControls_should_Measure44x44With8pxGaps_When_ChipComposeSheetOpen | Playwright (Pixel 7) | `boundingBox()` over each new control plus accessible-name role locators. [T5.1.1c, T4.x] |
| UX-24 (focus ring contrast) | manual | focusRing_should_Meet3to1_When_TabbedThroughNewControls | Manual | Tab through with a hardware keyboard in light and dark; not device-only. |
| UX-25: TalkBack announces once; hidden input named "Terminal input, edit box" | `MI/inputElement.test.ts` + `CMP/InputStatusChip.test.tsx` | aria_should_HaveRoleTextboxLabelDescribedByAndSingleStatusRegion_When_Rendered | Jest | Static aria contract: `role="textbox"`, `aria-multiline`, `aria-label`, `aria-describedby`; chip `role="status"`; discard `role="alert"` once. [T1.2.1b, T1.4.1d, T3.1.2a, T3.1.5a] |
| UX-25 (real) | `spike-findings.md` | talkback_should_AnnounceChipDiscardAndHintOnce_When_OnDevice | Manual, device-only | TalkBack on the owner's phone; also confirms "Terminal input, edit box". |
| UX-26: theme tokens, contrast >= 4.5:1 / 3:1, status not color alone | `E2E/mobile-input-layer.spec.ts` | axe_should_ReportNoWcagAaViolations_When_ChipComposeSheetShownInLightAndDark | Playwright + Axe Core | Repo UX-analysis CI already blocks on AA. [T5.1.1c] |
| UX-26 | `CMP/InputStatusChip.test.tsx` | chip_should_RenderIconAndText_When_AnyStatusState | Jest | Status never color alone; token lint (see CON-3). [T3.1.2a] |
| UX-27: chip/compose/toolbar above open keyboard; sheet within viewport | `E2E/mobile-input-layer.spec.ts` | surfaces_should_StayInsideVisualViewport_When_ViewportShrunkToEmulateKeyboard | Playwright (Pixel 7) | Resize viewport to keyboard-open height; chip, compose, toolbar `boundingBox()` inside; sheet height <= 40% visual viewport. [T5.1.1c] |
| UX-27 (real) | `spike-findings.md` | surfaces_should_StayAboveRealKeyboard_When_OnDevice | Manual, device-only | Real IME height and `visualViewport` behavior differ from emulation. |
| UX-28: flag off, none of the layer's own interactive surfaces, behavior as today (carve-outs: the plain-language flag-off hint on the Android class, UX-39, and the diagnostics surfaces) | `CMP/useMobileInputChrome.test.tsx` | chrome_should_RenderNothingAndKeepConsoleWarn_When_LayerFlagOff | Jest | [T1.4.1e, T3.1.1a] |
| UX-28 | `E2E/mobile-input-layer.spec.ts` | flagOff_should_RenderXtermAndOnlyTheFlagOffHint_When_LayerFlagFalseOnPixel7 | Playwright | [T5.1.1c] |
| UX-29: no dead-end error state | `CMP/InputStatusChip.test.tsx` | everyErrorState_should_ExposeDismissRetryReconnectOrClose_When_ParametrizedOverStateTable | Jest | Table-driven over S3 error table, S5, S7 error tables. [T3.1.5a, T4.2.1d, T4.1.2a] |
| UX-29 | manual | errorStates_should_AllHaveAnExit_When_WalkedOnManualInstance | Manual | Walk the exit-path matrix of `design/ux.md` once per sub-flag PR. |
| UX-30: reduced motion, no animation | `E2E/mobile-input-layer.spec.ts` | surfaces_should_HaveNoAnimation_When_ReducedMotionEmulated | Playwright (`emulateMedia({reducedMotion:'reduce'})`) | Computed `animation-name`/`transition` none on chip and sheet. [T3.1.5a, T4.1.2a] |
| UX-31: TalkBack announces the committed word once | `MI/attachMobileInput.test.ts` | (see triad table) | Jest | [T1.4.4b] |
| UX-31 (real) | `spike-findings.md` | (see triad table) | Manual, device-only | [T5.1.4d] |
| UX-32: at most one intermediate state; DeferredCommit indicator | `MI/replay.test.ts`, `E2E/mobile-input-layer.spec.ts`, `CMP/useMobileInputChrome.test.tsx` | (see triad table) | Jest + Playwright | [T1.2.5a, T1.2.5b, T5.1.1e] |
| UX-33: cursor row visible under keyboard emulation and real modes | `E2E/mobile-input-layer.spec.ts`, `spike-findings.md` | (see triad table) | Playwright + Manual, device-only | [T0.1.5a, T0.1.5b, T5.1.1e] |
| UX-34: compose offer on 5x229 and on 3 anomalies in 60 s | `MI/fallbackDetector.test.ts` | (see triad table) | Jest | [T4.2.2c] |
| UX-35: stuck and timeout copy | `CMP/InputStatusChip.test.tsx` | (see triad table) | Jest (fake timers) | [T3.1.5b] |
| UX-36: diagnostics reachable without long-press | `CMP/InputDiagnosticsSheet.test.tsx` | (see triad table) | Jest | [T4.1.2c] |
| UX-37: minimum terminal rows and 360px layout | `E2E/mobile-input-layer.spec.ts`, `MI/chromeBudget.test.ts` | (see triad table) | Playwright + Jest | [T1.4.3a, T1.4.3b] |
| UX-38: polite vs assertive, announce once | `MI/announce.test.ts` | (see triad table) | Jest | [T1.4.4a] |
| UX-39: plain-language flag-off hint | `CMP/useMobileInputChrome.test.tsx` | (see triad table) | Jest | [T1.4.1e, plan Story 1.3.2] |

## Owner-device-only proofs

These cannot be closed by CI, jsdom or Playwright emulation (Playwright's CDP IME is not Gboard; the e2e proves wiring only). Items 0a and 0b happen BEFORE merge:

0a. Task 0.1.3b: Backspace-on-empty and tap-opens-keyboard (physical phone, gesture handlers active), origins list.
0b. Task 5.1.4c: pre-merge phone gate on the isolated manual instance; sets the merged default of `mobile_input_layer`.

1. SM-6 / Story 5.1.4: real Gboard (and Samsung Keyboard) typing, autocomplete pick, autocorrect, swipe word, Backspace, Enter in `stelekit`, both renderers.
2. UX-1, UX-2, UX-3, UX-4 on the real keyboard (keyboard opens in one tap, stays open after toolbar taps, no Enter reordering).
3. UX-25 and UX-31 TalkBack announcements (Task 5.1.4d, pre-merge session).
4. UX-27 and UX-33 with the real soft-keyboard height and the Chrome/Samsung split/floating/one-handed modes (Story 0.1.5; emulated viewport shrink is only a proxy).
5. NFR-1 latency on the phone, measured (Task 5.1.4e) against the Task 0.1.3c baseline, and the 22-phrase threshold run (Tasks 5.1.4b, 5.1.4c).
6. OQ-1 (ghostty off alone), OQ-2 (real event sequences), Unresolved Question on Gboard/Samsung sequences, and device fixtures (`source:"device"`, scrub plus owner sign-off, PR-C onward).
7. RC-3 kill switch confirmed on the phone (two page loads from the panel; `?mobile_input=0` in one).
9. Origins and environments (Task 5.1.4b): `:8444` private-CA HTTPS, LAN/Tailscale, installed PWA, plain-HTTP LAN; hardware/Bluetooth keyboard and DeX; Ctrl-W; redeploy restart behavior; build-id check first.
8. Real ghostty-web IME/focus/229 behavior is only exercised in Playwright with the real module; jsdom uses a fake ghostty (Story 1.4.2 header says so).

## Test Stack
- **Unit**: Jest (Bun, `bunx jest --no-coverage`) + fast-check for properties (`rewrite`, `composition.property`, `deliveryInvariant`); Go `testing` + testify with `t.Parallel()` and injected clock/logger.
- **Integration**: Jest + jsdom with the fake-ghostty module mock and a stubbed xterm (`mobileInputTestSetup.ts`), real `InputQueue` with fake clock in the replay harness; Go httptest + registry gather for the five input sites and the actuator.
- **E2E / UX**: Playwright + Allure in `tests/e2e/` with `devices['Pixel 7']`, `devices['iPhone 14']`, desktop Chromium, CDP `Input.imeSetComposition`/`Input.insertText` helper (`helpers/ime.ts`), Axe Core, `data-testid`/ARIA locators, per-slot `baseUrl()`; manual checklist for owner-device items.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (`server/services`, `server/analytics`, `server`) | `go test ./server/... -coverprofile=coverage.out && go tool cover -func=coverage.out` (after `make build`) | >= 80% line for `terminal_input_metrics.go`, the new handler and the allow-list code |
| TypeScript/Jest | `cd web-app && unset npm_config_user_agent && bunx jest --coverage --collectCoverageFrom='src/lib/terminal/mobileInput/**' --coverageThreshold='{"global":{"lines":80}}'` | >= 80% line for `mobileInput/**`; reducer, rewrite, keymap, ledger, queue target >= 90% |
| E2E | `cd tests/e2e && bun run test mobile-input-layer.spec.ts` | All specs green for both renderers; no coverage figure |

- All public service methods: happy path + error paths covered (above).
- All external integrations (WebSocket send, clipboard, localStorage, beacon): unit mocked + at least one integration test (`sendInput` chain, chrome hook, telemetry allow-list).
- UX acceptance criteria: each of UX-1..UX-39 has a test or manual step above.
- Migration test: N/A (no schema or persisted-data changes; plan Migration Plan says not applicable).

## Gaps and plan feedback (status after Phase 4 plan-repair, 2026-11-08)

1. **Tests with no creating task: RESOLVED.** `MI/architecture.test.ts` (Task 1.2.1d), `TerminalOutput.readOnly.test.tsx` for the extra-keys half of UX-13 (Task 1.4.1f), the RC-1 env-var-not-read test (Task 1.3.1b), the OBS-4 dashboard JSON/panel test (named in Task 2.2.3a).
2. **OBS-4 "Alert condition": RESOLVED by task, with the fallback now explicit.** Task 2.2.3b adds a non-paging alert rule. If the repo has no alert-provisioning convention that task stops and records an owner decision in `requirements.md` that the panel plus documented query satisfy the requirement; that decision is OPEN until the owner accepts it. Not claimed as accepted here.
3. **NFR-2 wording: RESOLVED.** `requirements.md` now reads "attaches to visible/docked terminals; only one holds focus", matching Story 1.4.1 and ADR-006.
4. **SM-4 timing**: still true by design. `requirements.md` now states the visible indicator ships in PR-B behind `mobile_input_queue`; PR-A1 ships only the drop result and PR-A2 the counter. The SM-4 chip rows are PR-B.
5. **Chip message variants: RESOLVED.** Stories 3.1.3 and 3.1.5 now specify the three discard texts from `design/ux.md` S3 (overflow, TTL, session change) and UX-10 asserts them.
6. **Requirement id traceability**: unchanged. `requirements.md` has no ids; the ids above are assigned here and should be copied into PR descriptions.
7. **Flakiness controls**: unchanged. No wall-clock thresholds in e2e (the 250 ms bound and the latency budget are fake-timer unit tests); disconnect e2e uses default expect timeout; new Go tests call `t.Parallel()` and avoid `t.Setenv`; no Go test filename contains "gate" (BUG-087 trap).
8. **Remaining physical-world dependencies** (cannot be closed by a task alone): Tasks 0.1.3b, 5.1.4c and 5.1.4b need the owner's phone and are wall-clock blockers; the merged default of `mobile_input_layer` is unknown until 5.1.4c runs.
9. **Triad-review gaps (2026-11-08): RESOLVED by task, device rows pending.** UX gaps 1-9, the product threshold, drill, Open Questions and ICE items, and the engineering fixture split are owned by the tasks in the "Triad-review repair additions" table. Still physical-world dependent: Tasks 0.1.3c, 0.1.5a, 5.1.4d and 5.1.4e (owner phone) and the Task 5.1.2d drill (facilitator plus a second device).
