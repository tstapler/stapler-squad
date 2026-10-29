# Validation Plan: attention-inbox-consolidation

**Date**: 2026-10-23

Sources: `requirements.md`, `implementation/plan.md` (Domain Glossary terms used in all signatures), `design/ux.md` (UX-01..UX-49), `decisions/ADR-001..007`. Nothing here was run; test files are planned names and do not exist yet (INFERRED from the plan's Files lists).

## Happy Path Scenario

Given a running Claude session "docs-site" whose agent finished its turn with the plain-prose message "Want A or B for the router?" (no question tool) and flag `attention_prose_question_detection` on (baseline today: falls to `ReasonIdle`/nothing and is invisible), when the Stop hook posts to `/api/hooks/stop`, then `Instance.RecordTurnEnd` stores `TurnEnded{Kind=Question, Source=Hook}`, `Determine` adds a `prose_question` (Medium) `ReviewItem` with the excerpt, a `NOTIFICATION_TYPE_INPUT_REQUIRED` toast is delivered, the row "Asked a question" shows in the Attention list with a timeline reachable in <= 2 clicks, the item survives a server restart, and `LastMeaningfulChangeAt` on the card is identical after a browser reload.

## Requirement IDs

Derived from `requirements.md` (the file has no numbering; IDs are assigned here).

| ID | Source in requirements.md | Short description |
|---|---|---|
| R1 | Problem gap 1 | Prose questions at an idle prompt become a visible, Medium, needs-me item |
| R2 | Problem gap 2 | Finished-awaiting-context becomes a visible Low item (not suppressed like idle) |
| R3 | Problem gap 3 | Missed notifications recoverable from a durable surface |
| R4 | Problem 4 / In Scope event log | Durable per-session event log (what/when/why) + UI |
| R5 | Problem 5 / In Scope timer | Card timer stable across reload; defined semantics |
| R6 | In Scope ADR + migration/redirect | Surface consolidation decision (ADR-001), flag-gated redirect, no broken links |
| R7 | In Scope secondary | Unexpectedly suspended sessions surfaced without reviving |
| R8 | In Scope root-cause audit | Pipeline audit: suppression is explainable, skips are logged |
| SM1 | Success Metric Coverage | 100% of waiting sessions on the surface within one poll/event interval, per-program fixture corpus, measured FN rate, compared with the measured missed-rate baseline B1 |
| SM2 | Success Metric Durability | Un-acted item stays until acked or state changes; recoverable beyond the tray |
| SM3 | Success Metric Event log | Last N transitions viewable in <= 2 clicks from surface and card; survive reload and restart |
| SM4 | Success Metric Timer | Card value identical before/after reload and after server restart |
| SM5 | Success Metric Usage | `usePageView` analytics for the surface, with the measured baseline U1 (observational) |
| SM6 | Success Metric Alert guarantee (triad iter 1) | Every needs-human item and every finished-waiting episode on a visible session produces at least one operator-facing channel (alert or non-alerting unseen cue); testable, with a negative control |
| NFR1 | NFR Performance | Surface within existing poll/event latency; no new tight polling loop |
| NFR2 | NFR Scalability | Tens to low hundreds of sessions; bounded event log (per-session and global) |
| NFR3 | NFR Security | No secrets persisted/sent in excerpts |
| OB1 | Observability counters | Created/cleared by reason, emitted/delivered/suppressed, event-log writes and retention drops |
| OB2 | Observability log line | Log when needs-operator session yields no item, with skip reason |
| OB3 | Observability coverage query | Gap count (needs-operator with no active item) on actuator, target 0 |
| RC1 | Risk Control flags | Live-settable flags default off; rollback by flag flip; additive schema |
| C1 | Constraint lock-free reads | `instance-lock-free-reads.md` |
| C2 | Constraint noarchivedrevival | Never revive archived/paused sessions by surfacing |
| C3 | Constraint nav-facade/RouteMeta | `@/lib/nav`, RouteMeta rows |
| C4 | Constraint flags/no install-service | Live-settable flags (no env), trial on separate instance |
| C5 | Constraint mobile+desktop, proto/registry, Bun, e2e conventions | Both form factors; `make proto-gen`, `make registry-generate`; Bun only; e2e rules |
| OOS | Out of Scope | No auto-revive/resume/answer; no new channels; PR tabs not rewritten |

## Requirement -> Test Mapping

Go tests live in `session/`, `server/` etc. as `TestXxx_Should_When`; jest tests in `web-app/src/**` as `*.test.ts(x)`. "Unit" tests use fakes (`SessionEventRepository` fake, fake sink/recorder, injected clock, `DeterminerOptions`); "Integration" tests use in-memory SQLite ent client or real `AcknowledgeSession`/`RecordTurnEnd` paths. Per `deterministic-fast-tests`: injected clocks, no real sleeps, no `t.Setenv`; per `fix-flaky-tests-dont-defer`: pin `Math.random` in jittered tests.

### R1 Prose questions

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R1 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_AddProseQuestionMedium_When_TurnEndedQuestionFromHookAndFlagOn | Unit | Happy path |
| R1 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_MatchLegacyGoldensByteIdentical_When_ZeroDeterminerOptions | Unit | Error/regression path (flag off) |
| R1 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_MapEveryTurnEndKind_When_KindIsQuestionFinishedErrorUnknown | Unit | Exhaustive sum-type mapping (Unknown -> no signal, Error -> ErrorState) |
| R1 | session/turn_end_classifier_test.go | TestClassifyTurnEnd_Should_MeetPrecision90Recall80_When_RunOnLabelledClaudeCorpus | Unit | Happy path (>= 100 real messages from >= 10 repos, owner-labelled) |
| R1 | session/turn_end_classifier_test.go | TestClassifyTurnEnd_Should_ReturnFinished_When_InputUncertainOrRhetorical | Unit | Error path |
| R1 | session/turn_end_classifier_test.go | TestClassifyTurnEnd_Should_IgnoreInstructionText_When_AdversarialInjectionInMessage | Unit | Adversarial |
| R1 | session/turn_end_classifier_test.go | TestClassifyTurnEnd_Should_NeverReturnQuestion_When_ProgramIsNonClaudeWithoutCorpus | Unit | Per-program gate |
| R1 | session/turn_end_llm_test.go | TestLLMTiebreak_Should_FailOpenToFinishedAndWarn_When_HeadlessCallTimesOut | Unit | Error path |
| R1 | session/turn_end_llm_test.go | TestLLMTiebreak_Should_NotRunOnPollGoroutine_When_AmbiguousMessage | Unit | Concurrency |
| R1 | server/services/hook_turn_end_test.go | TestHandleStop_Should_RecordQuestionTurnEnd_When_TranscriptLastAssistantAsksQuestion | Unit | Happy path |
| R1 | server/services/hook_turn_end_test.go | TestHandleStop_Should_LogOnceAndReturn200_When_SessionTitleUnknown | Unit | Error path |
| R1 | session/transcript_tail_test.go | TestTranscriptTail_Should_ReadAtMost256KB_When_TranscriptIs10MB | Unit | Bound |
| R1 | session/transcript_tail_test.go | TestTranscriptTail_Should_RefuseAndWarn_When_PathEscapesProjectsDirViaDotDotOrSymlink | Unit | Security error path |
| R1 | server/services/hook_turn_end_test.go | TestHandleStop_Should_YieldOneTurnEndedAndOneNotification_When_ScriptAndEndpointBothFire | Unit | Dedup (HookDeduper) |
| R1 | session/turn_end_test.go | TestRecordTurnEnd_Should_NotBlockSnapshotReaders_When_SinkPersistBlocks | Unit | Lock discipline |
| R1 | session/turn_end_test.go | TestRecordTurnEnd_Should_SetSnapshotFactAndNotPanic_When_InstanceIsBareLiteral | Unit | Nil-safe collaborators |
| R1 | session/hook_to_item_integration_test.go | TestStopHookToQueueItem_Should_AddProseQuestionAndEmitInputRequired_When_WiredThroughPollerAndBus | Integration | End-to-end happy path |
| R1 | session/program_signal_table_test.go | TestProgramSignalTable_Should_PinAllowlistToRealPatternSet_When_ForEachProgram | Unit | Per-program pinning |
| R1 | session/turn_end_screen_test.go | TestScreenFallback_Should_YieldSourceScreenFinished_When_AiderProseQuestionWithoutHook | Unit | Non-hook program |
| R1 | ADR-003 / Spike 0.1.4 tables | phase0Exit_should_RecordGoOrNamedFallbackF1F2BeforePhase3_When_CoverageTableEvaluatedAgainst80Percent | Manual | Story 0.1.5 go/no-go gate (wall-clock blocker: owner confirms threshold) |
| R1 | session/turn_end_transcript_poll_test.go | TestTranscriptPoll_Should_RecordOneTurnEndPerUuidAndCapReadsAt10_When_FallbackF1SelectedAnd12IdleSessions | Unit | F1 fallback (Task 3.1.1e) |
| R1 | session/turn_end_transcript_poll_test.go | TestTranscriptPoll_Should_DedupWithHook_When_BothFireForSameUuidAndRefusePathEscape | Unit | F1 dedup and security |
| R1 | session/turn_end_classifier_test.go | TestExtractQuestionSentence_Should_ReturnTrailingQuestionNotOpening_When_LongPreambleMultiQuestionOrNoQuestionMark | Unit | Which text is stored (Story 3.1.2) |

### R2 Finished, awaiting direction

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_AddAwaitingDirectionLow_When_TurnEndedFinishedAndNoHigherItem | Unit | Happy path |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_KeepApprovalPrecedence_When_TurnEndedArrivesWithOpenApproval | Unit | Error/precedence path |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_ReplaceTaskCompleteWithAwaitingDirection_When_StatusSuccessAndFinishedAndOptionOn | Unit | One item per session |
| R2 | web-app/src/lib/utils/reviewQueueVisibility.test.ts | reviewQueueVisibility_should_ShowAwaitingDirectionAndProseQuestion_When_IdleStillHidden | Unit | Visibility (ADR-002) |
| R2 | web-app/src/lib/utils/reviewQueueVisibility.test.ts | reviewQueueVisibility_should_HideBareIdle_When_ReasonIsIdle | Unit | Regression path |
| R2 | server/adapters/attention_reason_adapter_test.go | TestAttentionReasonAdapter_Should_RoundTripValues11And12_When_ProtoMapped | Unit | Proto wiring |
| R2 | server/review_queue_manager_test.go | TestPublish_Should_MapAwaitingDirectionToTaskCompleteWithMetadata_When_ReasonRaised | Unit | Delivery mapping |
| R2 | session/review_queue_poller_chain_test.go | TestCheckSession_Should_KeepAwaitingDirection_When_BackgroundTaskFooterOrAckAfterOutput | Integration | Suppression path (gap 2 suppressors) |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_ReturnIdleReason_When_IdleClockOutputOnAndOutputOlderThanThreshold | Unit | Idle clock (Story 1.4.1) |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_ApplyWaitingForAgent30MinThreshold_When_OutputClockStaleButUpdatedAtFresh | Unit | Error path/clock |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_ClearAwaitingDirection_When_NewTurnStartsOrOperatorInputFollowsTurnEnd | Unit | Episode over (Story 3.2.1) |
| R2 | session/review_queue_determiner_golden_test.go | TestDetermine_Should_AutoExpireAwaitingDirection_When_Older24Hours | Unit | Informational auto-expiry |
| R2 | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_ProduceOneTrayRecordAndOnePushAndNoToastNoSlack_When_VisibleSessionFinishesTurn | Unit | Delivered, low urgency (owner to confirm) |
| R2 | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_NotRedeliver_When_SameEpisodeRepublishedOrQueueRebuiltAfterRestart | Integration | Dedup by episode |
| R2 | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_SuppressPushButKeepTrayRecordAndCount_When_SecondEpisodeWithin5Minutes | Unit | Flood bound (push limiter) |
| R2 | server/review_queue_manager_test.go | TestProseQuestionDelivery_Should_ReachToastPushAndSlackAsInputRequired_When_VisibleSession | Unit | ProseQuestion keeps INPUT_REQUIRED sinks |

### R3 Missed notifications recoverable / R3 + SM2 durability

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndPersistence_Should_RoundTripFactAndAckedEpisode_When_ToInstanceDataSaveLoad | Integration | Happy path |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndPersistence_Should_ReraiseProseQuestionWithoutNewNotification_When_ServerRestartsWithOpenItem | Integration | Restart survival |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndPersistence_Should_RaiseNothingForAckedEpisodeAndRaiseNewOne_When_RestartThenNewEpisode | Integration | Ack survival |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndPersistence_Should_ExerciseClearBranch_When_NewEpisodeClearsAckedEpisodeID | Integration | Cleared state |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestAcknowledgeSession_Should_PersistAckedEpisodeWithoutFullSave_When_LiveAndNotInPollerBranches | Integration | Narrow-writer durability |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestRecordTurnEnd_Should_PersistFiveColumnsWithoutFullSave_When_StopHookProcessed | Integration | Narrow-writer durability |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestFromInstanceData_Should_YieldZeroTurnEndedAndRaiseNothing_When_LegacyRowHasNullColumns | Integration | Error/legacy path |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndPersistence_Should_StoreFactAndRaiseNoItem_When_DetectionFlagOff | Integration | Flag-off variant |
| R3, SM2 | session/startup_scanner_test.go | TestStartupScanner_Should_SkipItemForAckedEpisode_When_AckedEpisodeIDMatches | Unit | Startup path |
| R3, SM2 | session/startup_scanner_test.go | TestStartupScanner_Should_AddProseQuestionWithStoredExcerpt_When_AckedEpisodeEmpty | Unit | Startup happy path |
| R3, SM2 | session/review_queue_suppression_test.go | TestEpisodeAck_Should_KeepItemAcked_When_OnlySpinnerOutputArrives | Unit | Happy path |
| R3, SM2 | session/review_queue_suppression_test.go | TestEpisodeAck_Should_ResurfaceAsNewItem_When_NewEpisodeE2Arrives | Unit | New-episode escalation |
| R3, SM2 | session/review_queue_suppression_test.go | TestAckSnooze_Should_ExpireAndReturnItem_When_SnoozeOlderThan4Hours | Unit | Expiry |
| R3, SM2 | session/review_queue_suppression_test.go | TestEpisodeAck_Should_ApplyUnconditionally_When_BypassFixFlagOff | Unit | Flag-order safety |
| R3, SM2 | session/review_queue_suppression_test.go | TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput | Unit | Ack bypass fix, flag on |
| R3, SM2 | session/review_queue_suppression_test.go | TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff | Unit | Characterization, flag off |
| R3, SM2 | session/review_queue_suppression_test.go | TestAckSnooze_Should_KeepApprovalPending_When_BypassFixOnOrOff | Unit | Error/approval unchanged |
| R3, SM2 | session/review_queue_suppression_test.go | TestSuppressionPolicy_Should_EnforceRuleOrder_When_TwoRulesWouldFire | Unit | Ordered contract |
| R3, SM2 | session/review_queue_suppression_test.go | TestReaddLimit_Should_NotSuppress_When_NewEpisodeWithin2Minutes | Unit | Escalation |
| R3, SM2 | session/review_queue_suppression_test.go | TestGrace_Should_NotRemoveItem_When_NewEpisodeWithin5MinutesOfAck | Unit | Escalation |
| R3, SM2 | session/review_queue_poller_chain_test.go | TestCheckSession_Should_PreserveCharacterizedSuppressionChain_When_RefactoredToSuppressionPolicy | Integration | Behavior preservation |
| R3, SM2 | session/review_queue_poller_chain_test.go | TestAcknowledgeEpisode_Should_ReachLogShowingAcknowledgedE1ThenRaisedE2_When_RealSuppressionPolicy | Integration | E1/E2 through real policy |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestLostUpdate_Should_KeepNarrowWrittenColumns_When_StaleFullSaveInterleavesAtBarrier | Integration | HARD GATE (merge blocker for PR B, never skipped or quarantined) [3.1.4d]. Merge rules: narrow write survives stale full save |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestLostUpdate_Should_KeepNewerInMemoryValue_When_NarrowWriteIsOlderThanFullSave | Integration | Merge, not skip |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestTurnEndAndAckPersistence_Should_PassRaceDetectorCount20_When_RunWithRace | Integration | `go test -race -count=20` |
| R3, SM2 | server/services/review_queue_service_test.go | TestUnacknowledgeSession_Should_ClearAckedEpisodeDurablyAndReRaiseItemWithUndoneEvent_When_NoNewerEpisode | Integration | Undo (Story 3.2.5) |
| R3, SM2 | server/services/review_queue_service_test.go | TestUnacknowledgeSession_Should_ReturnAlreadySupersededAndChangeNothing_When_NewerEpisodeExists | Unit | Undo error path |
| R3, SM2 | server/services/review_queue_service_test.go | TestAcknowledgeSessions_Should_AckInformationalRejectNeedsHumanAndReportPerItemErrors_When_MixedBatch | Integration | Server-side informational-only batch |
| R3 | server/services/ui_rpc_surface_test.go | TestUIRpcSurface_Should_ClassifyUnacknowledgeSessionAndAcknowledgeSessionsAsWrite_When_SinkguardTagRun | Unit | RPC gate |
| R3 | web-app/src/components/ui/NotificationItem.test.tsx | NotificationItem_should_NavigateToReviewQueueSessionParam_When_RowClicked | Unit | Tray -> list (tray never only record) |
| R3 | web-app/src/components/ui/NotificationItem.test.tsx | NotificationItem_should_LeaveAttentionItemUntouched_When_TrayDismissClicked | Unit | Error/independence path |

### R4 + SM3 Event log

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R4, SM3 | session/session_event_repository_test.go | TestSessionEventRepository_Should_ReturnNewestFirst_When_ThreeEventsAppended | Unit | Happy path (fake) |
| R4, SM3 | session/session_event_repository_test.go | TestSessionEventRepository_Should_NotErrorAndKeepIndexes_When_SchemaCreateRunsTwice | Integration | In-memory ent, idempotent |
| R4, SM3 | session/session_event_repository_test.go | TestSessionEventRepository_Should_SurviveReopen_When_DatabaseClosedAndReopened | Integration | Restart survival |
| R4, SM3 | session/session_event_repository_test.go | TestGeneratedEnt_Should_NotAppearInGitStatus_When_TreeRegenerated | Integration | Repo hygiene |
| R4 | session/session_event_recorder_test.go | TestRecorder_Should_WriteOneRow_When_TwoConsecutivePollsRaiseSameAttention | Unit | Write-on-change |
| R4 | session/session_event_recorder_test.go | TestRecorder_Should_WriteOneTurnEndedAndOneAttentionRaised_When_ThreePathsReportSameEpisode | Integration | No double count |
| R4 | session/session_event_recorder_test.go | TestRecorder_Should_LogTurnEndedAndRaiseNoItem_When_SessionIsHidden | Integration | Hidden-session decision |
| R4 | session/session_event_recorder_test.go | TestRecorder_Should_WriteNothingButTimerStillAdvances_When_EventLogFlagOff | Unit | Flag-off independence |
| R4 | session/session_event_recorder_test.go | TestRecorder_Should_BeCalledAtSingleOwnerPerKind_When_EventOwnershipTableEnumerated | Unit | One owner per kind |
| R4 | session/session_event_recorder_test.go | TestPoller_Should_InvokeRecordAttentionRaisedAndCleared_When_ItemAddedThenRemoved | Unit | Seam (no-op recorder) |
| R4 | session/session_event_service_test.go | TestListSessionEvents_Should_ReturnOnlyRequestedSessionNewestFirstWithTrimmedFlag_When_RetentionDropped | Unit | Happy path |
| R4 | server/services/session_event_service_test.go | TestListSessionEvents_Should_CapLimitAt500AndFilterByKindAndSince_When_CrossSessionQuery | Unit | Recently finished view |
| R4 | server/services/session_event_service_test.go | TestListSessionEvents_Should_ReturnFlagDisabledError_When_SessionEventLogOff | Unit | Error path |
| R4 | server/adapters/session_event_kind_adapter_test.go | TestSessionEventKindAdapter_Should_RoundTripEveryKind_When_ProtoMapped | Unit | Exhaustive enum |
| R4 | server/services/ui_rpc_surface_test.go | TestUIRpcSurface_Should_ClassifyListSessionEventsAsRead_When_SinkguardTagRun | Unit | RPC gate |
| R4 | session/session_event_prune_test.go | TestPrune_Should_DeleteTwelveRowsAndCountDrops_When_SessionDeleted | Integration | Delete path |
| R4 | session/session_event_prune_test.go | TestPrune_Should_KeepRows_When_SessionArchivedUntilRetention | Integration | Archive path (Story 2.1.4 is prune-on-delete only) |
| R4 | web-app/src/lib/hooks/useSessionEvents.test.ts | useSessionEvents_should_LoadFirstPageAndAppendOlder_When_LoadOlderCalled | Unit | Happy path |
| R4 | web-app/src/lib/hooks/useSessionEvents.test.ts | useSessionEvents_should_ExposeFlagOffState_When_ServerDoesNotAdvertiseEventLog | Unit | Error path |
| R4 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_RenderOlWithTimeEntriesAndTrimmedRow_When_EventsLoaded | Unit | Happy path |
| R4 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_RenderUnknownKindFallbackWithoutCrash_When_KindNotInWordingMap | Unit | Error path |
| R4, SM3 | server/services/session_event_service_test.go | TestListSessionEvents_Should_PageWithBeforeCursorWithoutGapOrOverlapAndTieBreakById_When_120Events | Unit | Cursor (Task 2.1.3d) |
| R4 | proto/session/v1 (buf/proto-gen) | protoGen_should_CompileWithoutNameClash_When_RowMessageIsSessionLogEntry | Integration | `session.v1.SessionEvent` already exists (`events.proto:11`) |
| R4, R3 | web-app/src/lib/hooks/useRecentSessionEvents.test.ts | useRecentSessionEvents_should_SendKindsSinceAndBeforeCursorAndAppendPages_When_FiltersChangedAndLoadOlderCalled | Unit | Cross-session hook |
| R4, R3 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_ListFinishedSessionsIncludingDoneAndNeverRaised_When_TurnEndedEventsLoaded | Unit | Finished (24 h) (Story 5.2.3) |
| R4, R3 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_ShowQuietNowFromSessionsSliceWithoutRpc_When_EventLogFlagOff | Unit | Quiet now works flag-off |
| R4 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_FilterByKindChipAndTimeWindow_When_ChangedChipAnd1hSelected | Unit | Filters |
| R4, SM2 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_ListDoneRecentlyWithUndoAndShowSupersededText_When_ShowDoneOn | Unit | Show done |
| R4 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_AppendOlderPageKeepFocusAndAnnounceInOwnRegion_When_LoadOlderActivated | Unit | Load older |
| R4 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_HideHiddenSessionsUnlessToggledAndTagThem_When_IncludeHiddenToggled | Unit | Hidden toggle |
| R4 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_ShowExplainerRetryAndKeepLoadedEntries_When_FlagOffRpcErrorOrLoadOlderFails | Unit | Error/flag-off states |
| R4, SM3 | tests/e2e/attention-inbox.spec.ts | recent_should_BeReachableInTwoClicksFromNavBadgeAndListFinishedSessions_When_DesktopAnd375px | E2E | Reachability both form factors |

### R5 + SM4 Timer

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R5, SM4 | session/instance_state_test.go | TestTransitionTo_Touch_Should_AdvanceLastMeaningfulChangeBeforeSnapshot_When_StatusChanges | Unit | Happy path, `-race -timeout 30s` |
| R5, SM4 | session/instance_state_test.go | TestUpdateTimestamps_Should_NotAdvanceLastMeaningfulChange_When_RenameOrPin | Unit | Error/non-semantic path |
| R5, SM4 | session/instance_state_test.go | TestUpdateTimestamps_Should_RebuildSnapshotOncePerMeaningfulFrameAndZeroOtherwise_When_CountingHook | Unit | Lock-share, no second rebuild |
| R5, SM4 | session/instance_state_test.go | TestRecordTurnEnd_Should_AdvanceLastMeaningfulChange_When_TurnEnded | Unit | Advance rule |
| R5, SM4 | session/session_activity_test.go | TestSessionActivity_Should_AdvanceWithEventLogFlagOff_When_OutputArrives | Unit | Flag-off |
| R5, SM4 | session/session_activity_persistence_test.go | TestLastMeaningfulChange_Should_EqualPreShutdownValueExactly_When_GracefulSaveReopenLoad | Integration | Restart (time.Equal) |
| R5, SM4 | session/session_activity_persistence_test.go | TestLastMeaningfulChange_Should_ReturnLastSavedValue_When_CrashWithoutFurtherSave_AcceptedLossWindow | Integration | Documented loss window |
| R5, SM4 | session/session_activity_persistence_test.go | TestUpdateTimestamps_Should_IssueZeroStatements_When_100MeaningfulFramesProcessed | Integration | Query-count, no hot-path DB work |
| R5, SM4 | session/session_activity_persistence_test.go | TestFromInstanceData_Should_BackfillFromLastMeaningfulOutputThenCreatedAt_When_ColumnNull | Integration | Legacy backfill |
| R5, SM4 | session/instance_state_test.go | TestUpdatedAt_Should_RemainUnchangedOnTouchAndSerialize_When_ADR005CharacterizationRuns | Unit | `UpdatedAt` untouched |
| R5 | server/adapters/instance_adapter_test.go | TestInstanceAdapter_Should_MapLastMeaningfulChangeAtToProto_When_SnapshotHasValue | Unit | Proto mapping |
| R5 | web-app/src/lib/store/sessionsSlice.test.ts | sessionsSlice_should_KeepUpdatedAtSortAndDedupBehavior_When_LastMeaningfulChangeAtPresent | Unit | Characterization of other consumers |
| R5 | web-app/src/lib/hooks/useFilteredGroupedSessions.test.ts | useFilteredGroupedSessions_should_SortByUpdatedAtUnchanged_When_NewFieldAdded | Unit | Characterization |
| R5, SM4 | web-app/src/lib/unfinished/relativeTime.test.ts | formatRelativeTime_should_Return5MinAgoAbsoluteAfter7dAndNoActivityRecorded_When_CasesGiven | Unit | Happy + missing value |
| R5 | web-app/src/lib/utils/timestamp.test.ts | formatTimeAgo_should_PassExistingTests_When_WrapperOverFormatRelativeTime | Unit | Regression |
| R5 | web-app/src/lib/hooks/useNowTicker.test.ts | useNowTicker_should_UseSingleIntervalPauseWhenHiddenAndRefreshOnVisibility_When_200RowsMounted | Unit | Fake timers |
| R5 | web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx | StuckItemDetail_should_UseNowTickerAndNoLocalUseNowRemain_When_Migrated | Unit | Grep AC + render |
| R5, SM4 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_RenderIdenticalLastActivityText_When_RenderedTwiceWithSamePropsAndNow | Unit | Reload invariance |
| R5 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_FallBackToUpdatedLabel_When_LastMeaningfulChangeAtAbsent | Unit | Error/older server |
| R5 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_ShowNoActivityRecorded_When_ValueMissing | Unit | Error path |
| R5 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_OpenDrawerWithAbsoluteTime_When_LabelTapped | Unit | Touch path |
| R5 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_ShowCreatedLabelRelativeUnder24hAndNoUpdatedLabel_When_Rendered | Unit | Created label (UX S7) |

### R6 Consolidation, redirect, no broken links

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R6 | web-app/src/app/review-queue/page.test.tsx | ReviewQueuePage_should_RedirectToUnfinishedSessionsTabPreservingParams_When_ConsolidationFlagOn | Unit | Happy path |
| R6 | web-app/src/app/review-queue/page.test.tsx | ReviewQueuePage_should_RenderLegacyPage_When_FlagOffOrFlagFetchFails | Unit | Error/fail-to-legacy |
| R6 | web-app/src/components/unfinished/UpNextTabs.test.tsx | UpNextTabs_should_ShowSessionsTabWithBadgeEqualToNavBadge_When_FlagOn | Unit | Happy path |
| R6 | web-app/src/lib/hooks/useUpNextTab.test.ts | useUpNextTab_should_ResolveTabQueryThenSessionsWhenNeedsMeThenStoredThenPrs_When_Cases | Unit | Default-tab rule table |
| R6 | web-app/src/components/unfinished/UpNextTabs.test.tsx | UpNextTabs_should_RenderExactlyCurrentTabsAndSameStreamSubscriptionCount_When_FlagOff | Unit | Regression |
| R6 | web-app/src/app/unfinished/UnfinishedTab.test.tsx | UnfinishedTab_should_KeepSingleAttentionSubscription_When_TabsSwitched | Unit | No duplicated state |
| R6 | web-app/src/app/route-meta.test.ts | routeMeta_should_ContainReviewQueueRowAndMatchPageTitles_When_ParsedFromSources | Unit | RouteMeta drift |
| R6 | web-app/src/lib/nav/touchpoints.test.ts | navTouchpoints_should_ResolveOrRepointEveryReviewQueueReference_When_GreppedSlackOmnibarShortcutsOnboarding | Unit | Deep link inventory |
| R6 | docs/registry (CI) | registryGenerate_should_ProduceCleanDiff_When_RunAfterRoutesChange | Integration | `make registry-generate` |
| R6 | project_plans/.../decisions/ADR-001 | adr001_should_BeAcceptedOrAmendedBeforeEpic6_When_OwnerReplies | Manual | Wall-clock gate (owner) |
| R6 | web-app/src/lib/nav/linkProducers.test.ts | linkProducers_should_KeepSessionParamAndLandOnSessionsTab_When_PushClickActionSlackBuilderMobileDeepLinkAndOmnibarEachExercised | Unit | Task 6.2.1d, one case per producer |
| R6 | tests/e2e/unfinished-sessions-tab.spec.ts | consolidationFlag_should_KeepOneStreamSubscriptionAndCorrectTab_When_FlippedOnThenOff | E2E | Flip-on / flip-off with a live stream |

### R7 Suspended sessions without reviving, plus C2 and OOS

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R7, C2, OOS | session/review_queue_poller_test.go | TestShouldSkipSession_Should_SkipPausedStoppedCrashedHibernatedAndCountEach_When_Evaluated | Unit | Happy path |
| R7, C2, OOS | server/services/hook_turn_end_test.go | TestHandleStop_Should_RecordTurnEndButNeverCallStartOrRecover_When_SessionArchived | Unit | Error path (fake fails on Start/Recover) |
| R7, C2, OOS | tools/lint/noarchivedrevival (analysistest) | TestNoArchivedRevival_Should_ReportNothing_When_RunOnPlanFiles | Integration | `go run ./tools/lint/noarchivedrevival` clean |
| R7 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowCollapsedPausedGroupWithoutNeedsYouStylingOrResume_When_PausedSessionPresent | Unit | Happy path |
| R7 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_OfferOnlyOpenAction_When_PausedRowRendered | Unit | Error path (no resume on view) |
| OOS | server/deliverygate/sink_guard_test.go | TestSinkGuard_Should_RegisterNewBusConsumerInReviewedConsumers_When_Added | Unit | No new channels |

### R8 Pipeline audit, suppression explainability, and OB2

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R8, OB2 | session/review_queue_suppression_test.go | TestSuppressionLogging_Should_EmitOneDebugLine_When_GraceSuppressesThreeConsecutivePolls | Unit | Happy path |
| R8, OB2 | session/review_queue_suppression_test.go | TestSuppressionLogging_Should_EmitAgainOnDecisionChange_When_RuleChanges | Unit | Error/edge path |
| R8, OB2 | session/review_queue_poller_test.go | TestShouldSkipSession_Should_LogOncePerSessionAndIncrementSkipCounter_When_HiddenAndPausedSkipped | Unit | Skip visibility |
| R8 | session/review_queue_determiner_test.go | TestDetermine_Should_StayUnderComplexityGateAndHaveNoRawUpdatedAtReads_When_Refactored | Unit | Grep/complexity AC (lint) |
| R8 | session/review_queue_determiner_test.go | TestDetermine_Should_ReadNoFlagResolver_When_GrepOfDeterminerFiles | Unit | Purity AC |
| R8 | session/review_queue_options_test.go | TestDeterminerOptions_Should_ReadResolverOncePerCycle_When_ThreeSessionsPolled | Unit | Counting fake |
| R8 | session/review_queue_options_test.go | TestRuleOptions_Should_ReadAckBypassFlagOncePerCycleAndNotEnterDeterminer_When_ThreeSessionsPolled | Unit | Counting fake |
| R8 | session/review_queue_determiner_test.go | TestStatusDeterminerFakes_Should_CompileWithOptionsSignature_When_BuildAndVetRun | Integration | `go build ./... && go vet` |

### SM1 Coverage (per-program fixture corpus, FN rate) + OB3

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM1 | session/review_queue_determiner_golden_test.go | TestDetermineGolden_Should_MatchCommittedGoldens_When_RunOverClaudeAiderAgyPiGeminiOpencodeCorpus | Unit | Happy path (>= 5 cases per program with patterns) |
| SM1 | session/review_queue_determiner_golden_test.go | TestDetermineGolden_Should_ReportMissingSignalPerProgram_When_ProgramRowIsNone | Unit | Error path ("cannot detect") |
| SM1 | session/program_coverage_report_test.go | TestProgramCoverage_Should_ComputeFalseNegativeRatePerProgram_When_OperatorLabelledSamplesReplayed | Integration | Measured FN rate |
| SM1 | session/review_queue_poller_test.go | TestPoller_Should_SurfaceItemWithinOnePollInterval_When_FakeClockAdvancesOneCycle | Unit | Latency bound |
| SM1, OB3 | server/actuator_test.go | TestActuatorAttentionCoverage_Should_ReportGap1WithSkipRule_When_InputRequiredSessionSuppressed | Integration | Gap detected |
| SM1, OB3 | server/actuator_test.go | TestActuatorAttentionCoverage_Should_ReportGap0_When_ItemPresent | Integration | Happy path |
| SM1, OB3 | server/actuator_test.go | TestActuatorAttentionCoverage_Should_ListHiddenSuspendedArchivedNotStartedUnderSkippedByDesign_When_Present | Integration | Not counted as gap |
| SM1, OB3 | server/actuator_test.go | TestActuatorAttentionCoverage_Should_UseGroundTruthIndependentOfDetermine_When_UnackedQuestionPersisted | Integration | Independence |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_FailFloodCheck_When_ItemsPerHourPerSessionExceeds6 | Unit | Soak gate error |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_PassPrecision90Recall80_When_LabelsMeetBars | Unit | Soak gate happy |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_ComputeHookCoverageRatioPerProgramAndBlockDetectionFlip_When_AnyProgramBelowExitCriterion | Unit | Flip precondition (Story 7.3.2) |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_RecommendFlagOff_When_DismissRateAbove20Percent | Unit | Dismiss-rate watch |
| SM1 | docs/how-to/roll-out-attention-inbox.md | soak_should_PassCoverageGapZeroAndPrecisionBars_When_3SessionsPerProgramOnIsolatedInstance | Manual | Soak on isolated instance (wall-clock blocker) |

### SM2 additional (surface durability in UI)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_KeepAwaitingDirectionItem_When_SessionOpenedAndRemoveOnlyOnDone | Unit | Seen vs Done |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_OmitDoneButtonAndAnnounceReason_When_DKeyPressedOnNeedsMeRow | Unit | Error path |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RemoveProseItemAndFireDismissKind_When_NotAQuestionClicked | Unit | Dismissal |
| SM2 | server/services/review_queue_service_test.go | TestAcknowledgeSession_Should_IncrementDismissedCounterLogEventAndRemoveItem_When_DismissKindNotAQuestion | Integration | Dismissal server side |
| SM2 | server/services/review_queue_service_test.go | TestAcknowledgeSession_Should_ReturnNotFoundAndLeaveStateConsistent_When_ItemAlreadyCleared | Unit | Error path |
| SM2 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_CountOnlyNeedsHumanReasonsAndShareOneSelectorWithTabAndTitle_When_AwaitingDirectionPresent | Unit | Count rule (Task 5.2.1f) |
| SM2 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_ClassifyEveryAttentionReasonValue_When_EnumEnumerated | Unit | Fails when a reason is missing from the classification |
| SM2 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_KeepLegacyCount_When_DetectionFlagOff | Unit | Flag-off regression |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_CallUnacknowledgeAndShowSupersededToast_When_UndoActivatedAfterNewerReply | Unit | Undo UI |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_SendOnlyInformationalRowsAndRetryRemaining_When_MarkAllFinishedHasOneFailure | Unit | Bulk partial failure |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowSeenMarkerFromServerLastViewedAndShowNoneWhenFieldAbsent_When_LastViewedAfterTurnEnd | Unit | Seen is server-derived (ADR-007; replaces the per-viewer localStorage test) |
| R7, SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_BuildPausedGroupFromSessionsSliceWithoutQueueChange_When_PausedStoppedHibernatedPresent | Unit | Paused source (client join) |
| R3 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowClearedUnknownSessionAndFilterResetBanners_When_DeepLinkedToClearedDeletedOrFilteredOutItem | Unit | Tray edge banners (UX S5) |
| SM5 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RenderAttentionTitleAndMatchRouteMeta_When_RouteMetaTestRuns | Unit | Copy rename + RouteMeta |

### SM3 Event log reachable in <= 2 clicks (browser)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM3 | tests/e2e/session-event-drawer.spec.ts | timeline_should_OpenFromSessionCardAndQueueRowInTwoClicks_When_EventsExist | E2E | Desktop + 375 px |
| SM3 | tests/e2e/session-event-drawer.spec.ts | timeline_should_ShowSameEventsAfterPageReload_When_ReloadedWithoutRestart | E2E | Reload survival |
| SM3 | tests/e2e/session-event-drawer.spec.ts | timeline_should_ShowSameEventsAfterServerRestart_When_IsolatedServerRestarted | E2E | Restart survival (global-setup restart helper) |

### SM4 Timer (browser)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM4 | tests/e2e/session-card-timer.spec.ts | cardTimer_should_ShowIdenticalLastActivityText_When_PageReloadedWithoutRestart | E2E | Reload |
| SM4 | tests/e2e/session-card-timer.spec.ts | cardTimer_should_ShowIdenticalLastActivityText_When_IsolatedServerRestartedAndPageReloaded | E2E | Restart |
| SM4 | docs ADR-005 / Spike 0.1.2 | spike_should_NameWriterOfReloadReset_When_ReloadThenRestartReadViaGetSession | Manual | Root cause repro (Spike 0.1.2a) |

### SM5 Usage

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM5 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_CallUsePageViewOnceOnMount_When_Rendered | Unit | Happy path |
| SM5 | web-app/src/components/unfinished/UpNextTabs.test.tsx | SessionsTab_should_CallUsePageViewWithSessionsTabId_When_TabSelected | Unit | Consolidated surface still counted |
| SM5 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_NotThrowAndStillRender_When_AnalyticsUnavailable | Unit | Error path |

### NFR1 Performance (no tight polling)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| NFR1 | session/review_queue_poller_test.go | TestPoller_Should_ReuseExistingTickerAndAddNoNewPollLoop_When_TurnEndedFactArrivesViaHook | Unit | Event-driven path |
| NFR1 | session/session_activity_persistence_test.go | TestUpdateTimestamps_Should_IssueZeroStatements_When_100MeaningfulFramesProcessed | Integration | (shared with R5) no hot-path DB |
| NFR1 | session/turn_end_test.go | TestRecordTurnEnd_Should_ObserveTurnEndToItemHistogramUnder100msBudgetOnFakeClock_When_Recorded | Unit | Latency metric |
| NFR1 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_UseNoSetIntervalOfItsOwn_When_Mounted | Unit | No per-row timers |

### NFR2 Scalability / bounded log

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| NFR2 | session/session_event_retention_test.go | TestRetention_Should_KeepAtMost200RowsPer30DaysPerSession_When_SweeperRunsOnFakeClock | Unit | Happy path |
| NFR2 | session/session_event_retention_test.go | TestRetention_Should_EnforceGlobalCapAndIncrementDropCounter_When_RowsExceed50k | Integration | Global cap |
| NFR2 | session/session_event_retention_test.go | TestRetention_Should_NotDeleteRowsInsideWindow_When_UnderCaps | Unit | Error/over-deletion |
| NFR2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_GroupWithCountsShowMoreAndStickyOldestWaiting_When_55ItemsRendered | Unit | UI at scale |
| NFR2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_KeepFocusedRowRendered_When_ListExceedsGroupCap | Unit | Focus not lost |

### NFR3 Security

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| NFR3 | session/session_event_scrub_test.go | TestScrub_Should_RedactApiKeyBearerGhpSkJwtAndUrlUserinfoAndCapAt160_When_SecretShapeCorpus | Unit | Happy path |
| NFR3 | session/session_event_scrub_test.go | TestScrub_Should_BeSameFunctionForEventLogAndNotificationBuilder_When_CallSitesInspected | Unit | One scrubber |
| NFR3 | session/session_event_scrub_test.go | TestScrub_Should_NotPanicOnEmptyOrBinaryInput_When_MalformedInput | Unit | Error path |
| NFR3 | session/transcript_tail_test.go | TestTranscriptTail_Should_NeverOpenFileOutsideProjectsDir_When_TraversalAttempted | Unit | (shared with R1) |
| NFR3 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | Row_should_Truncate160CharsAndShowNoRawTail_When_ExcerptLong | Unit | UI cap (UX-37) |

### OB1 Counters

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementCreatedSuppressedAndDismissed_When_ProseQuestionRaisedSuppressedDismissed | Unit | Happy path |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementClearedByLabel_When_ItemRemovedByAckVersusStateChange | Unit | Labels |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementTurnEndClassifiedByProgramKindSource_When_TurnEndRecorded | Unit | Classification counter |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementRetentionDroppedAndWritesByKind_When_RowsPrunedAndAppended | Unit | Event log counters |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementUndoAndNotDecrementDismissed_When_UnacknowledgeCalled | Unit | Monotonic dismissed counter |
| OB1 | server/review_queue_manager_test.go | TestReviewQueueManager_Should_LogNotificationSuppressedEventWithGateReason_When_HiddenSessionAwaitingDirectionGated | Unit | Gate result handler sole owner |
| OB1 | server/review_queue_manager_test.go | TestDeliveryGate_Should_ClassifyProseQuestionAsNeedsHumanAndDeliverToastTier2_When_PublishedAsInputRequired | Unit | Delivery class |
| OB1 | server/notifications/question_dedup_test.go | TestQuestionDedup_Should_KeyOnEpisodeAndDeliverOnce_When_SameEpisodePublishedTwice | Unit | Dedup by episode (replaces the 1-minute occurrence-count rule) |
| OB1 | session/review_queue_poller_test.go | TestSuppressionLogging_Should_NotRecordFromReconcileOrPollLoop_When_GocognitBeforeAfterCompared | Unit | Complexity guard (<= before score) |

### RC1 Flags and rollout (+ C4)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| RC1, C4 | server/services/feature_flags_attention_test.go | TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfSixFlags | Unit | Happy path |
| RC1, C4 | server/services/config_surface_proof_flags_test.go | TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run | Integration | CI gate |
| RC1, C4 | server/services/feature_flags_attention_test.go | TestAttentionFlags_Should_NotReadEnvVars_When_ResolverBuilt | Unit | No env vars |
| RC1, C4 | server/services/feature_flags_attention_test.go | TestFlagFlip_Should_TakeEffectNextPollCycleWithoutRestart_When_SetLive | Unit | Live-settable |
| RC1, C4 | session/review_queue_flag_matrix_test.go | TestFlagMatrix_Should_KeepOneItemPerSessionNoBuriedProseQuestionAndNeedsHumanBadge_When_FourOrdersAndEachFlagAlone | Integration | Flip-order matrix (Task 1.3.1c); name avoids "gate" |
| RC1 | session/ent_migration_test.go | TestMigration_ShouldBeReversible | Migration | Up, verify schema, "down" = flags off + legacy reader ignores additive columns/table |
| RC1 | session/ent_migration_test.go | TestSchemaCreate_Should_AddNullableColumnsAndTableIdempotently_When_RunOnLegacyDatabase | Integration | Additive migration |
| RC1 | session/ent_migration_test.go | TestLegacyRow_Should_LoadWithZeroValuesAndNoBackfillAtDbLevel_When_NewColumnsNull | Integration | Zero-downtime |

### C1 Lock-free reads

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| C1 | session/instance_snapshot_test.go | TestInstanceSnapshot_Should_ContainTurnEndedAndLastMeaningfulChangeAt_When_FieldsAdded | Unit | Happy path |
| C1 | session/instance_snapshot_test.go | TestRawFieldReads_Should_BeAbsentOutsideSetterAndSnapshotFiles_When_GrepTurnEndedAndUpdatedAt | Unit | Grep lint test (0 lines) |
| C1 | session/instance_race_test.go | TestInstanceConcurrentRecordTurnEndAndSnapshotReads_Should_PassRaceDetector_When_RunWithRace | Unit | `go test -race ./session ./server/...` |
| C1 | session/review_queue_determiner_test.go | TestDetermine_Should_UseSnapshotUpdatedAt_When_ReadingIdleClock | Unit | Collateral fix |

### C3 Nav facade and C5 cross-cutting

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| C3 | web-app/eslint (lint gate) | eslint_should_ReportZeroNextNavigationImportsInChangedFiles_When_Linted | Unit | Lint |
| C3 | web-app/src/app/review-queue/page.test.tsx | ReviewQueuePage_should_UseNavFacadeMockWithRequireActualMemoryImpl_When_Tested | Unit | Mock convention |
| C5 | server/services/ui_rpc_surface_test.go | TestUIRpcSurface_Should_PassSinkguardClassification_When_NewRpcAndDismissKindAdded | Unit | RPC gate |
| C5 | tests/e2e/conventions_check (CI) | e2eConventions_should_FindFeatureHeaderNoWaitForTimeoutNoCssSelectors_When_NewSpecsScanned | Unit | CI convention check |
| C5 | session/ent_generated_not_staged_test.go | TestGeneratedEnt_Should_NotBeStaged_When_GitStatusPorcelainSessionEnt | Integration | Hygiene |
| C5 | Makefile `ready` | makeReady_should_PassComplexityDuplicationAndJscpdGates_When_RunAfterEachSlice | Integration | gocognit/dupl new-from-rev; jscpd margin recorded |

### Triad repair iteration 1 additions (baselines, delivery policy, alert guarantee, accessibility, task splits)

Every new or re-scoped story, AC and task has rows below; plan Tasks in brackets.

#### Baselines and timer fix path (Stories 0.1.6, 0.1.2 AC; Tasks 0.1.6a-b, 0.1.2c, 4.1.1e, 4.2.2c)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM1 | scripts/soak/baseline_report_test.go | TestBaselineReport_Should_ComputeDetectedButMissedRateWithWindowAndSampleSize_When_FixtureHistoryReplayed | Unit | B1 happy path [0.1.6a] |
| SM1 | scripts/soak/baseline_report_test.go | TestBaselineReport_Should_StateActualCoveredWindowAndLimitations_When_HistoryShorterThan30Days | Unit | B1 error path: honest window [0.1.6a] |
| SM1 | scripts/soak/baseline_report_test.go | TestBaselineReport_Should_CountUndetectedQuestionSampleSeparately_When_LabelledTranscriptsProvided | Unit | B1 undetected estimate is a sample, not a census [0.1.6a] |
| SM1 | docs/reference/attention-inbox.md (baseline section) | baselineB1_should_RecordMethodWindowNSourcesAndLimitations_When_Story016Complete | Manual | Owner reads the recorded B1 [0.1.6a] |
| SM5 | docs/reference/attention-inbox.md (baseline section) | baselineU1_should_RecordPageViewSinkAndTrailing14DayOpensOrStartWindow_When_AnalyticsSinkTraced | Manual | U1 or 14-day collection window [0.1.6b] |
| R5, SM4 | session/instance_state_test.go | TestReloadWriter_Should_LeaveUpdatedAtAndLastMeaningfulChangeUnchanged_When_PlainReloadWithoutRestart | Unit | Failing characterization from Spike 0.1.2 now passes [4.1.1e; conditional] |
| R5, SM4 | session/instance_state_test.go | TestReloadWriter_Should_StillUpdateLastViewed_When_SessionOpened | Unit | Fix does not remove `last_viewed` writes [4.1.1e; conditional] |
| R5, SM4 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_NotDeriveLastActivityFromMountTime_When_ReRenderedAfterRemount | Unit | Client-side reset fix [4.2.2c; conditional] |

#### Task-split coverage (Tasks 1.1.3b-d, 3.1.4c/e/f, 6.2.1b-i)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R8 | session/review_queue_determiner_test.go | TestStatusDeterminerInterface_Should_BuildWithOptionsSignature_When_PollerCallSiteChanged | Integration | `go build ./... && go vet` after Task 1.1.3b only [1.1.3b] |
| R8 | session/review_queue_poller_test.go | TestFakesForStatusDeterminer_Should_CompileAndRunPollerTests_When_SignatureUpdated | Unit | All three fake sites updated [1.1.3c] |
| R3, SM2 | session/startup_scanner_test.go | TestStartupScanner_Should_ReadOptionsFromSamePerCycleSourceAsPoller_When_FirstCycleRuns | Unit | Scanner options [1.1.3d] |
| R3, SM2 | session/ent_repository_turn_end_test.go | TestUpdateAckedEpisodeAndTurnEnded_Should_IssueSingleUpdateWithoutSelect_When_QueryCountHookAttached | Integration | Writers are single-statement [3.1.4c] |
| R3, SM2 | server/services/review_queue_service_test.go | TestAcknowledgeSession_Should_SetMemoryThenPersistAfterLockRelease_When_BothBranches | Integration | Ack path ordering, `TryLock` succeeds in the fake writer [3.1.4e] |
| R3, SM2 | session/turn_end_test.go | TestTurnEndSinkAdapter_Should_PersistViaStorageAfterUnlockAndLogFailure_When_PersistErrors | Unit | Turn-end persist wiring [3.1.4f] |
| R6 | web-app/src/lib/nav/touchpoints.test.ts | navTouchpoints_should_RepointNavPathsRoutesCockpitShellAndBadge_When_FlagOn | Unit | [6.2.1b] |
| R6 | web-app/src/lib/features/features/review-queue.test.ts | featureCatalog_should_PointReviewQueueEntryAndSettingsLinksAtCanonicalUrl_When_FlagOn | Unit | [6.2.1e] |
| R6 | web-app/src/lib/omnibar/detectors/CommandDetector.test.ts | CommandDetector_should_ResolveReviewQueueCommandToCanonicalUrl_When_FlagOn | Unit | Omnibar registry [6.2.1f] |
| R6 | docs/registry (CI) | registryGenerate_should_ProduceCleanDiff_When_RunAfterOmnibarAndCatalogRepoint | Integration | [6.2.1f] |
| R6 | tests/e2e/review-queue.spec.ts, review-queue-severity.spec.ts, review-queue-priority-tiers.spec.ts | reviewQueueSpecs_should_PassUnderFlagOnAndFlagOff_When_RedirectAware | E2E | [6.2.1c] |
| R6 | tests/e2e/escalation-reasoning.spec.ts, enter-detection.spec.ts, session-detail-backlog-panel.spec.ts, smoke.spec.ts | secondarySpecs_should_PassUnderFlagOnAndFlagOff_When_RedirectAware | E2E | [6.2.1g] |
| R6 | tests/e2e/nav-navigation.spec.ts, route-titles.spec.ts, route-focus-and-announce.spec.ts, pathname-probe.spec.ts | navSpecs_should_PassUnderFlagOnAndFlagOff_When_RedirectAware | E2E | [6.2.1h] |
| R6 | tests/e2e/accessibility.spec.ts, viewport-meta.spec.ts, lighthouse.config.js, tools/parity-diff.ts | a11yAndToolingSpecs_should_PassUnderFlagOnAndFlagOff_When_RedirectAware | E2E | [6.2.1i] |

#### Delivery policy, mute and unconfirmed data (Story 3.2.6; ADR-007)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R2 | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_PushFirstSixThenOneDigestAndKeepAllTrayRecords_When_8DistinctSessionsFinishWithin10Minutes | Unit | Global cap + digest [3.2.6a] |
| R2 | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_ReplaceDigestByTagAndResetWindow_When_NinthArrivesThenFifteenMinutesPass | Unit | Digest tag and window reset (injected clock) [3.2.6a] |
| R2 | server/review_queue_manager_test.go | TestProseQuestionDelivery_Should_NotBeCountedOrHeldByGlobalCap_When_CapExhausted | Unit | Needs-human unaffected [3.2.6a] |
| R2 | server/services/notification_mute_service_test.go | TestSetNotificationMute_Should_PersistAndReturnUntilAndClampTo24h_When_SessionAndGlobalScopes | Unit | [3.2.6b] |
| R2 | server/services/notification_mute_service_test.go | TestNotificationMute_Should_SurviveReopenOfStore_When_ServerRestarts | Integration | Durable mute [3.2.6b] |
| R2 | server/review_queue_manager_test.go | TestMute_Should_SuppressPushAndSlackButKeepTrayRecordAndRowAndCountMutedReason_When_SessionMuted | Unit | [3.2.6c] |
| R2 | server/review_queue_manager_test.go | TestMute_Should_ExpireWithoutRestart_When_InjectedClockPassesUntil | Unit | Expiry [3.2.6c] |
| R2 | server/review_queue_manager_test.go | TestMute_Should_NotSuppressToastOrBadgeCount_When_MutedSessionRaisesProseQuestion | Unit | Mute never hides state [3.2.6c] |
| R2 | server/adapters/review_item_adapter_test.go | TestReviewItemAdapter_Should_MapTurnEndSourceUnconfirmedAndMutedUntil_When_ScreenAndHookItems | Unit | Both new fields round-trip [3.2.6d] |
| R2 | server/services/ui_rpc_surface_test.go | TestUIRpcSurface_Should_ClassifySetNotificationMuteAsWriteAndGetNotificationMutesAsRead_When_SinkguardTagRun | Unit | RPC gate [3.2.6b] |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementGlobalCapMutedAndDigestCounters_When_CapAndMuteApplied | Unit | New gate reasons and `awaiting_direction_digest_total` [3.2.6a/c] |

#### Alert guarantee (Story 7.1.2; SM6)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM6, R3 | server/review_queue_alert_guarantee_test.go | TestAlertGuarantee_Should_ProduceAtLeastOneChannelPerNeedsHumanItemAndAwaitingEpisode_When_40MixedEpisodesRunThroughRealPollerAndManager | Integration | Happy path across push available/unavailable, muted, capped [7.1.2a] |
| SM6 | server/review_queue_alert_guarantee_test.go | TestAlertGuarantee_Should_Fail_When_PushErrorsAndUnseenCueDisabled | Integration | Negative control: the test can fail [7.1.2a] |
| SM6 | server/review_queue_alert_guarantee_test.go | TestAlertGuarantee_Should_ProduceNoChannelByDesign_When_SessionIsHidden | Integration | Hidden sessions asserted silent [7.1.2a] |
| SM6 | tests/e2e/attention-alert-guarantee.spec.ts | alertGuarantee_should_ShowUnseenFinishedCueWithoutPush_When_NotificationPermissionDenied | E2E | Fallback channel [7.2.1c] |
| SM6 | tests/e2e/attention-alert-guarantee.spec.ts | alertGuarantee_should_DeliverOneLowUrgencyPushAndStillShowTheCue_When_PermissionGranted | E2E | Alerting channel plus the one cue rule [7.2.1c] |
| SM6, SM2 | tests/e2e/attention-alert-guarantee.spec.ts | alertGuarantee_should_ClearCueOnBothDevices_When_SessionOpenedOnOne | E2E | Server-derived Seen across two browser contexts [7.2.1c] |

#### Accessibility, shortcuts, toasts, vocabulary, coupling (Tasks 5.2.1d-m, 5.2.2c, 5.2.4a-d, 5.1.1b)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_NotMarkOldestWaitingCalloutAsLiveRegion_When_Rendered | Unit | Blocker 1 [5.2.1d] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_FindNoTimeElementOrTimeLikeTextInAnyLiveRegion_When_PageBannerToastAndRecentRegionsWalked | Unit | Extended UX-17 walk [5.2.1d, 7.2.1c] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_AnnounceOnlySessionNameWhenOldestWaitingChanges_When_OldestChanges | Unit | No age in speech [5.2.1d] |
| SM2 | web-app/src/lib/hooks/useAttentionListKeys.test.ts | useAttentionListKeys_should_ActOnlyWhenFocusInsideList_When_KeysPressedInInputOmnibarDialogAndBody | Unit | WCAG 2.1.4 scope [5.2.1i] |
| SM2 | web-app/src/lib/hooks/useAttentionListKeys.test.ts | useAttentionListKeys_should_BeInertAndKeepArrowsEnterTab_When_SettingOff | Unit | Disable setting [5.2.1i] |
| SM2 | web-app/src/lib/hooks/useAttentionListKeys.test.ts | useAttentionListKeys_should_NotRegisterQuestionMarkAndNotFireOnSessionDetailT_When_ConflictChecked | Unit | Conflicts with `shortcuts:open` and `SessionDetailView` [5.2.1i] |
| SM2 | web-app/src/app/settings/KeyboardShortcutsTab.test.tsx | KeyboardShortcutsTab_should_ToggleAttentionShortcutsAndPersistWithTryCatch_When_LocalStorageThrows | Unit | Setting persistence [5.2.1i] |
| SM2 | web-app/src/components/ui/KeyboardShortcutOverlay.test.tsx | KeyboardShortcutOverlay_should_ListAttentionListSection_When_Opened | Unit | `?` overlay section [5.2.1i] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_PauseToastCountdownOnHoverAndFocusAndStackThreeWithIndependentUndo_When_ActionsRepeated | Unit | WCAG 2.2.1 [5.2.1g] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_UndoLastActionWithCtrlZAndTabStopOnUndo_When_ListFocused | Unit | Keyboard undo [5.2.1g] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_OfferUndoForDoneNotAQuestionBulkAndMute_When_EachActionRun | Unit | Undo coverage [5.2.1g, 5.2.4b] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_MoveFocusToNextPreviousHeaderThenHeading_When_RowRemoved | Unit | Focus placement [5.2.1g] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowQuietRowsWithStatusTextAndHonorThresholdSetting_When_30MinBuildRunningVsIdle | Unit | Quiet status + 15/30/60/120 [5.2.1l] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowSameAllCaughtUpCopyAfterRestart_When_NoItemsAndNoHistory | Unit | Stable empty copy [5.2.1k] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RenderBannerAsNonLiveAndSpeakWithoutTimeAndSuffixNavBadge_When_StreamDropsFor10s | Unit | Disconnect banner + stale badge cue [5.2.1h] |
| SM2 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_AppendMayBeOutOfDateToAccessibleName_When_StreamStale | Unit | Stale cue [5.2.1h] |
| SM2 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_CollapseConsecutiveStatusChangesWithin2MinAndCapFiveRowsPerSession_When_12Flaps | Unit | Flapping cap [5.2.3b] |
| SM2 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_UseWindowedSectionTitlesAndRetiredWordsAbsent_When_Rendered | Unit | Vocabulary [5.2.3b] |
| SM2 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_HaveDialogAriaModalLabelledAndInertBackground_When_OpenedAt1280900And375 | Unit | Modal decision [5.1.1b] |
| SM2 | web-app/src/components/ui/NotificationItem.test.tsx | NotificationItem_should_ResolveTrayRecordAndDecrementBadge_When_AttentionItemAcknowledged | Unit | Two-way coupling [5.2.2c] |
| SM2 | web-app/src/components/ui/NotificationItem.test.tsx | NotificationItem_should_ShowTooltipThatDismissDoesNotRemoveFromAttention_When_Hovered | Unit | Asymmetry stated [5.2.2c] |
| SM2 | server/adapters/instance_adapter_test.go | TestInstanceAdapter_Should_MapLastViewedToProto_When_SnapshotHasValue | Unit | Seen source field [5.2.1j] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_CallMarkViewedOnSessionOpen_When_RowActivated | Unit | Opening a session attaches a terminal stream (the four `MarkViewed` callers); contingency `MarkSessionViewed` only if some open path attaches none [5.2.1j] |
| SM6, SM2 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_ShowSeparateFinishedUnseenTextAndAccessibleName_When_2NeedsMe4Awaiting3Unseen | Unit | Unseen cue separate from badge [5.2.4a] |
| SM6 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_NotUseAriaLiveOrAnimationForCue_When_CueUpdates | Unit | Cue never alarms [5.2.4a] |
| SM6 | web-app/src/lib/hooks/usePushCapability.test.ts | usePushCapability_should_ReportUnavailable_When_PermissionDeniedNoSubscriptionOrApiMissing | Unit | Three unavailable variants [5.2.4a] |
| SM6 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowPushOffLineOnceAndPersistDismissal_When_PushUnavailable | Unit | Header line [5.2.4a] |
| R2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowMutedUntilTextKeepRowAndUnmuteViaToast_When_SessionMuted | Unit | Mute UI [5.2.4b] |
| R2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowUnconfirmedTagHeaderCountAndNavSuffix_When_ScreenSourcedNeedsHumanItem | Unit | Unconfirmed signal [5.2.4c] |
| SM2 | web-app/src/components/sessions/AttentionFirstRunHelp.test.tsx | AttentionFirstRunHelp_should_ShowOnceListEveryNewLabelNeverMoveFocusAndReopenFromOverlay_When_FirstVisit | Unit | First-run help [5.2.4d] |

#### Operator usability script (Story 7.3.3)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM2, SM6 | docs/how-to/run-attention-usability-script.md | usability_A1toA5A7A9_should_PassOnPhoneAndDesktopWithNoTaskFailingTwiceInARow_When_RunBeforePRBFlagFlip | Manual | Phone + desktop self-test (wall-clock blocker: owner) [7.3.3a] |
| SM2 | docs/how-to/run-attention-usability-script.md | usability_A6andA8_should_PassKeyboardOnlyAndQuietRunningVsIdle_When_RunBeforePRCFlagFlip | Manual | Desktop keyboard and Quiet status [7.3.3a] |

#### Triad repair iteration 2 additions (one cue rule, single announcer, prose-question recourse, engineering corrections)

Every changed story, AC and task of iteration 2 has rows here or an edited row above (plan tasks in brackets).

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM6 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_ShowSameFinishedCount_When_PushGrantedDeniedNoSubscriptionOrBackgrounded | Unit | One cue rule: never depends on push [5.2.4a] |
| SM6 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_RenderSecondTextLineHideAtZeroAndCapAt9Plus_When_MobileBottomNav | Unit | Mobile width budget, hidden at 0 [5.2.4a] |
| SM6 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_ShowVisibleMutedWordAndAppendAlertsMutedToNameOnEveryPageAndFormFactor_When_GlobalMuteActive | Unit | Global mute marker: visible "muted" word on desktop and mobile plus accessible-name suffix [5.2.4b] |
| SM6 | session/metrics_test.go | TestMetrics_Should_CountUnseenFinishedGaugeIndependentOfPushOutcome_When_PushSucceedsAndFails | Unit | Server gauge uses the same rule [7.1.1b] |
| SM2 | web-app/src/components/sessions/AttentionAnnouncer.test.tsx | AttentionAnnouncer_should_SpeakHighestPriorityFirstCoalesceSameKindAndDropEntriesOlderThan10s_When_QueueOverfilled | Unit | Priority queue, max 3 entries [5.2.1n] |
| SM2 | web-app/src/components/sessions/AttentionAnnouncer.test.tsx | AttentionAnnouncer_should_SpeakNeedsYouArrival_When_TimelineDialogOpenAndBackgroundInert | Unit | Mounted outside the inert subtree [5.2.1n] |
| SM2 | web-app/src/components/sessions/AttentionAnnouncer.test.tsx | AttentionAnnouncer_should_SpeakInlineErrorOnceAndNeverAssertive_When_ActionRpcFailsAndRerenders | Unit | Inline error announce rule [5.2.1n] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_HaveExactlyOneLiveNodeAndNoLiveRoleOnToastsBannersCalloutOrDrawer_When_DomWalked | Unit | UX-17 topology [5.2.1n, 5.2.1d] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowMuteToastWithoutTimeAndStaticMutedUntilRowText_When_SessionMuted | Unit | No timestamp in a toast [5.2.4b] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowStaleTagClockIconAndDashedBorderWithoutLoweringOpacity_When_StreamDrops | Unit | Contrast-safe stale styling [5.2.1h] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_PromoteNewApprovalToStickyLineAndSpeakAtPriority1_When_HoldActiveUnderPointerOrFocus | Unit | A hold never buries an approval [5.2.1d] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_KeepCurrentRowButtonsAsTabStopsAndOthersTabindexMinus1_When_RovingFocusMoves | Unit | Roving access to sibling buttons [5.2.1i] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowCoverageCaveatAndNothingNeedsYouAffirmation_When_ProgramWithoutSignalOrOnlyFinishedRows | Unit | Honest empty state [5.2.1k] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_MoveFocusToBannerHeadingAndSpeakOnce_When_DeepLinkedItemAlreadyCleared | Unit | Deep-link banner focus and announce [5.2.2a] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_KeepNeedsYouRowsAboveFoldAt375x812_When_FirstRunHelpShownCollapsed | Unit | First-run help does not push rows down [5.2.4d] |
| SM2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_Size44UnderPointerCoarseAtAnyWidthAnd24UnderPointerFine_When_MediaEmulated | Unit | Target size keys on input type, tablet 768-1023 [5.2.1b] |
| SM2 | web-app/src/components/sessions/uiStrings.test.ts | uiStrings_should_ContainNoRetiredVocabulary_When_NewAttentionFilesGrepped | Unit | Needs me, Dismissed (Attention rows), snooze, Pause finished-session, Acknowledged, Just finished, Gone quiet absent [5.2.1m] |
| SM2 | tests/e2e/attention-inbox.spec.ts | attention_should_ReflowAt320pxAnd400PercentZoomKeepRoleSemanticsAndClipNothing_When_TextSpacingOverridesApplied | E2E | UX-49 [5.2.1b] |
| SM2 | tests/e2e/session-event-drawer.spec.ts | timeline_should_ReturnFocusToTheSameRowEachTime_When_ThreeTimelinesOpenedInARow | E2E | Automated half of usability step A10 [5.1.1b] |
| R4 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_CallGetSessionDiffOncePerOpenShowWorkingTreeNowAndOmitOnError_When_Opened | Unit | Lazy completion context [5.1.1d] |
| R4 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_GroupEntriesUnderDateHeadings_When_PageSpansTwoDays | Unit | Date separators [5.1.1d] |
| R4, R3 | web-app/src/components/sessions/RecentActivityView.test.tsx | RecentActivityView_should_ShowNowStateMarkerAndDateHeadings_When_FinishedRowsAcrossDays | Unit | Current-state marker [5.2.3b] |
| R1 | server/review_queue_manager_test.go | TestProseQuestionBurstGuard_Should_DeliverFirstThreeInFullAndLimitFourthToRowBadgeTray_When_SameSessionWithin30Min | Unit | Burst guard [3.2.6e] |
| R1 | server/review_queue_manager_test.go | TestProseQuestionBurstGuard_Should_NeverTouchApprovalInputRequiredOrError_When_BurstActive | Unit | Guard scope [3.2.6e] |
| R1 | session/review_queue_options_test.go | TestQuestionOptOut_Should_RecordQuestionAsFinishedSurviveRestartAndClear_When_SetForSession | Integration | Opt-out end to end [3.2.6e] |
| R1 | server/review_queue_manager_test.go | TestMuteScope_Should_HoldPushAndSlackForAwaitingDirectionAndProseQuestionOnly_When_SessionMuted | Unit | Session mute scope [3.2.6c] |
| R1 | server/review_queue_manager_test.go | TestMuteScope_Should_NotHoldProseQuestionOrApprovals_When_GlobalMuteActive | Unit | Global mute scope [3.2.6c] |
| R1 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_OfferStopFlaggingWithUndoAndFrequentQuestionsHint_When_ProseRowAnd4thEpisode | Unit | Recourse UI [5.2.4b] |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementProseQuestionBurstReason_When_GuardEngaged | Unit | New gate reason [3.2.6e] |
| NFR3 | server/guarded_procedures_test.go | TestGuardedProcedures_Should_ListMuteAndOptOutWithRebindingProfileAndExcludeReads_When_Inspected | Unit | Authorization of alert-silencing writes [3.2.6b] |
| NFR3 | server/services/review_queue_service_test.go | TestAcknowledgeSessions_Should_RejectMoreThan100Items_When_Called | Unit | Abuse limit [3.2.5b] |
| NFR3 | server/services/notification_mute_service_test.go | TestSetNotificationMute_Should_ReturnNotFoundAndPersistNothing_When_UnknownSession | Unit | Abuse limit [3.2.6b] |
| NFR3 | web-app/src/components/sessions/excerptRendering.test.ts | excerptRendering_should_UseTextNodesOnly_When_NewFilesGrepForDangerouslySetInnerHTMLAndMarkdown | Unit | No HTML in excerpts [5.2.1a] |
| SM2, SM6 | session/session_activity_persistence_test.go | TestLastViewed_Should_RevertToLastFullSaveValue_When_CrashWithoutFurtherSave_AcceptedLossWindow | Integration | Crash-window disclosure for Seen [5.2.1j] |
| SM2 | server/services/mark_viewed_callers_test.go | TestMarkViewedCallers_Should_BeExactlyTheFourStreamHandlers_When_ProductionGoGrepped | Unit | Audit that pins the caller set [5.2.1j] |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_FailCoverageMetric_When_GapZeroButRecallBelow80 | Unit | Gap 0 is necessary, not sufficient |
| SM1 | scripts/soak/baseline_report_test.go | TestBaselineReport_Should_SwitchToNoIncreasePlusRecallTarget_When_B1Below5Percent | Unit | Relative target when B1 < 5% [0.1.6a] |
| SM6 | docs/how-to/run-attention-usability-script.md | usability_A10_should_PassThreeTimelinesInARowOnKeyboardAndPhone_When_RunBeforePRCFlagFlip | Manual | Perception and modal-drawer flow (wall-clock blocker: owner) [7.3.3a] |

#### Triad round 3 additions (noise guardrail, post-ship rule, per-program scope, mute marker, opt-out reversal, row density, stale comment)

Every changed AC of round 3 has a row here or an edited row above (plan Story 5.2.4 new ACs, Story 5.2.1 row-density AC, Story 7.3.2 ACs, Story 1.2.3 comment AC, Task 3.1.4d gate).

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM6 | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_RenderNewFinishedVisibleTextDistinctFromFinishedChipCount_When_OneOfThreeOpened | Unit | Visible "new" text; cue 3 -> 2 while the chip number is unchanged [5.2.4a] |
| SM6 | web-app/src/components/sessions/MuteManagementList.test.tsx | MuteManagementList_should_ListMutesAndQuestionOptOutsWithSetDateAndReverseEachInOneStep_When_RowHasCleared | Unit | Opt-out reviewable and reversible after the row clears [5.2.4b] |
| SM6 | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_ShowQuestionsNotFlaggedTag_When_OptOutSet | Unit | Card indicator as text [5.2.4b] |
| SM6 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowFourteenDayReminderAndRedisplayAfterFourteenMoreDays_When_OptOutOlderThan14Days | Unit | Reminder decision, no auto-expiry [5.2.4b] |
| SM6 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_PlaceGlobalMuteControlInMoreButtonBesideHelpAndKeepNeedsYouRowsAboveFold_When_375x812 | Unit | Mobile placement of the global mute control [5.2.4b] |
| R2 | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_DropAwaitingDirectionRowOnNewTurnOrAfter24hAndKeepItUnderRecentFinished_When_AutoCleared | Unit | Auto-clear and recovery path [5.2.4] |
| SM2 | tests/e2e/attention-inbox.spec.ts | attention_should_MoveSecondaryActionsToOverflowBelow720ContainerAndKeepNotAQuestionVisible_When_768To1023320pxAnd400PercentZoom | E2E | Row action density threshold [5.2.1] |
| R1 | server/services/notification_mute_service_test.go | TestGetNotificationMutes_Should_ReturnQuestionOptOutsWithSetAt_When_OptOutSet | Unit | Management list data [3.2.6b/e] |
| OB1 | session/metrics_test.go | TestMetrics_Should_IncrementMuteSetAndQuestionOptOutSetCounters_When_Set | Unit | Guardrail inputs [3.2.6b/e] |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_FailNoiseGuardrail_When_DismissRateAtOrAbove10PercentOrMuteUsageOrCapHitsExceeded | Unit | Noise guardrail thresholds (a), (b), (c) [7.3.2a] |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_ReportCoveragePerProgramAndListNonClaudeAsFinishedOnlyForProseQuestions_When_FourProgramsPresent | Unit | Per-program Coverage scope [7.3.2a] |
| SM1 | scripts/soak/soak_report_test.go | TestSoakReport_Should_MapEachMissToExactlyOneAction_When_CoverageGuardrailAlertOrUsageMissed | Unit | Day-14 miss-to-action rule [7.3.2a] |
| C1 | session/review_state.go (grep step in Task 1.2.3b) | `grep -n "not holding Instance.mu" session/review_state.go` finds nothing and the comment states the caller holds Instance.mu | Manual | Stale doc comment corrected [1.2.3b] |

## UX Acceptance Tests

Stack is Playwright (`tests/e2e`, with axe) for criteria that need a real browser, jest + Testing Library for component-level criteria, and a short manual script for things no automated check can judge (screen reader announcements, contrast in dark mode on device, keyboard-covered-by-sheet). Every UX-NN below maps to exactly one primary test; Playwright specs run at 1280x800 and 375x812 where noted. Specs follow e2e conventions (`// @feature` header, `data-testid`/ARIA locators only, helpers in `tests/e2e/pages/AttentionInboxPage.ts`, no `waitForTimeout`).

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-01 Needs-me reachable in <= 1 click, visible without scrolling | tests/e2e/attention-inbox.spec.ts | attention_should_ShowNeedsMeRowsWithoutScrollAfterOneClickFromNavBadge_When_375x812And1280x800 | Playwright | From `/`, click nav badge, assert needs-me rows in viewport at both sizes |
| UX-02 Prose item shows label, icon, excerpt | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RenderAskedAQuestionIconAndExcerpt_When_ProseQuestionItem | Jest + RTL | Render item, assert label text, icon role/name, excerpt |
| UX-03 Not a question: 1 click, 10 s Undo, absent elsewhere; Stop flagging questions and burst guard | tests/e2e/attention-inbox.spec.ts | notAQuestion_should_RemoveInOneClickOfferUndoAndBeAbsentOnApprovalRows_When_Seeded | Playwright | Click button, assert toast Undo, click Undo restores; assert no button on approval/input/error rows; "Stop flagging questions for this session" present on prose rows with Undo (component half: `ReviewQueuePanel_should_OfferStopFlaggingWithUndoAndFrequentQuestionsHint_When_ProseRowAnd4thEpisode`; server half: the `TestProseQuestionBurstGuard_*` and `TestQuestionOptOut_*` rows) |
| UX-04 AwaitingDirection: one low-urgency tray + push per episode, no toast, not in badge, persists after open, Done removes | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowAwaitingDirectionWithoutToastKeepOnOpenRemoveOnDone_When_Interacted | Jest + RTL | Assert no toast mock call and badge unchanged, open session, row remains, Done removes; Done absent on needs-me row (server delivery half: R2 rows `TestAwaitingDirectionDelivery_*`) |
| UX-05 Tray -> list deep link with scroll, focus once, cleared-item banner announced and focused | tests/e2e/attention-inbox.spec.ts | trayRow_should_NavigateToReviewQueueSessionFocusRowOnceAndShowBannerWhenCleared_When_Clicked | Playwright | Click tray row, assert URL/focus; repeat with cleared item, assert banner with Open session and Open timeline, focus on the banner heading, one announcement (component half: `ReviewQueuePanel_should_MoveFocusToBannerHeadingAndSpeakOnce_When_DeepLinkedItemAlreadyCleared`) |
| UX-06 Timeline in <= 2 clicks from row and card, both form factors | tests/e2e/session-event-drawer.spec.ts | timeline_should_OpenInTwoClicksFromRowAndCard_When_DesktopAndMobile | Playwright | Click Timeline on row; on card; count clicks |
| UX-07 Mark all finished done never touches needs-me, undoable | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | MarkAllFinishedDone_should_AffectOnlyInformationalRowsAndUndoWithin10s_When_Clicked | Jest + RTL | Click bulk action "Mark all finished done"; assert needs-me rows remain; undo restores count; partial failure shows "3 of 4 ... Retry remaining" |
| UX-08 Paused collapsed group, no resume | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowCollapsedPausedGroupAndNeverResume_When_PausedViewed | Jest + RTL | Render with Paused session; assert collapsed, neutral class absent of needs-you, Open only; assert no resume RPC called |
| UX-09 55 items grouped, show more, sticky callout, <= 3 interactions | tests/e2e/attention-inbox.spec.ts | attention_should_ReachAny55thItemInThreeInteractions_When_GroupsCapped | Playwright | Seed 55; click "Show N more"; filter chip; reach item |
| UX-10 Sessions tab badge equals nav badge, default rule, flag-off unchanged | tests/e2e/unfinished-sessions-tab.spec.ts | sessionsTab_should_BeDefaultOnlyWhenFlagOnAndNeedsMeGreaterThanZero_When_Loaded | Playwright | Flag on with 2 items: first/selected; flag off: tabs identical to today; badge text equals nav badge |
| UX-11 Drawer/sheet `<ol>` of `<time>` newest first, absolute header, date headings, lazy working-tree line; Recent date headings and Now marker | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_ListNewestFirstOlOfTimeWithAbsoluteHeader_When_Opened | Jest + RTL | Open, assert order, `<time dateTime>`, header text; also at < 768 px render as sheet; plus `SessionEventDrawer_should_GroupEntriesUnderDateHeadings_When_PageSpansTwoDays`, `SessionEventDrawer_should_CallGetSessionDiffOncePerOpenShowWorkingTreeNowAndOmitOnError_When_Opened` and `RecentActivityView_should_ShowNowStateMarkerAndDateHeadings_When_FinishedRowsAcrossDays` |
| UX-12 Esc/Close/backdrop close; mobile Close button; focus returns | tests/e2e/session-event-drawer.spec.ts | drawer_should_CloseOnEscCloseAndBackdropAndReturnFocusToOpener_When_Desktop | Playwright | Open from card, press Esc, assert focus on opener; repeat Close/backdrop; at 375 px Close button present |
| UX-13 "Older events trimmed" last row | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_RenderOlderEventsTrimmedLastRow_When_TrimmedTrue | Jest + RTL | Mock `trimmed=true`; assert non-interactive last row |
| UX-14 Last activity/Created identical after reload; rename/pin no effect | tests/e2e/session-card-timer.spec.ts | cardTimer_should_BeIdenticalAfterReloadAndUnchangedByRenameOrPin_When_Performed | Playwright | Read label, reload, compare; rename then pin, compare |
| UX-15 Missing value text; legacy fallback no mixed labels | web-app/src/components/sessions/SessionCard.test.tsx | SessionCard_should_RenderNoActivityRecordedOrLegacyUpdatedWithoutMixedLabels_When_ValueMissingOrFieldAbsent | Jest + RTL | Two renders; assert exact strings, no "Last activity" with "Updated" |
| UX-16 Relative < 7 d, absolute after, touch-reachable | web-app/src/lib/unfinished/relativeTime.test.ts | formatRelativeTime_should_UseRelativeUnder7dAndAbsoluteAfterWithTouchPathToAbsolute_When_Cases | Jest | Boundary 6d23h vs 7d1h; absolute in drawer header on tap (card test) |
| UX-17 One interval; exactly one live region (the announcer) outside the inert subtree; no timestamp in anything spoken or in a toast | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_HaveExactlyOneLiveNodeAndNoLiveRoleOnToastsBannersCalloutOrDrawer_When_DomWalked | Jest + fake timers | Count `setInterval` calls; walk the DOM for `aria-live`, `role=status`, `role=alert` and assert exactly one (the announcer); assert it is outside any `inert` ancestor with the dialog open (`AttentionAnnouncer_should_SpeakNeedsYouArrival_When_TimelineDialogOpenAndBackgroundInert`); fail on `<time>` or time-like text in any message or toast (`FindNoTimeElementOrTimeLikeTextInAnyLiveRegion`, `ShowMuteToastWithoutTimeAndStaticMutedUntilRowText`) |
| UX-18 Every error state has a specific message and an exit action | web-app/src/components/sessions/errorStates.test.tsx | ErrorStates_should_ShowSpecificMessageAndAtLeastOneExitAction_When_EachS3S6S9ErrorTriggered | Jest + RTL | Table over: dismiss RPC fail, undo fail, drawer load fail, load-older fail, list load fail, 401/403, session deleted |
| UX-19 Disconnect > 10 s banner (plain text), stale tag without opacity change; reconnect refetch no new-item announcements | tests/e2e/attention-inbox.spec.ts | stream_should_ShowLiveUpdatesPausedBannerAndNotAnnounceKnownItemsOnReconnect_When_DroppedFor10s | Playwright | Block WatchReviewQueue route; wait for banner via expect; assert stale tag, clock icon, dashed border and computed opacity 1 on rows; restore; assert refetch and announcer text unchanged (component half: `ShowStaleTagClockIconAndDashedBorderWithoutLoweringOpacity_When_StreamDrops`) |
| UX-20 Empty states: one stable "All caught up" wording vs filtered, with last checked, coverage caveat, affirmation | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowStableAllCaughtUpAndDistinctFilteredEmptyWithLastChecked_When_Rendered | Jest + RTL | Two renders, assert copy and time; error card covers broken; plus `ShowCoverageCaveatAndNothingNeedsYouAffirmation_When_ProgramWithoutSignalOrOnlyFinishedRows` |
| UX-21 Failed Done/Not a question restores row with Retry | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RestoreRowShowCouldntAndRetry_When_ActionRpcFails | Jest + RTL | Reject RPC; row returns; Retry re-calls; Open still works |
| UX-22 Offline disables server actions with visible reason | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_DisableDoneAndNotAQuestionWithVisibleReason_When_Offline | Jest + RTL | Set `navigator.onLine=false`; assert disabled plus reason text |
| UX-23 Flag-off behavior (no new reasons/controls; timeline hidden or explained; legacy label) | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_RenderNoNewReasonsOrControlsAndLegacyCardLabel_When_AllFlagsOff | Jest + RTL | Render with flags off; snapshot absence; timeline button hidden or flag explainer |
| UX-24 Axe WCAG 2.2 AA in empty, populated, drawer-open, disconnected; desktop + 375 | tests/e2e/attention-inbox.spec.ts | attention_should_PassAxeAA_When_EmptyPopulatedDrawerOpenAndDisconnectedAtBothViewports | Playwright + axe | Four states x two viewports; zero violations |
| UX-25 No nested interactive controls | tests/e2e/attention-inbox.spec.ts | attentionRow_should_HaveNoNestedInteractiveViolation_When_AxeRunOnRows | Playwright + axe | Run rule `nested-interactive` on populated list |
| UX-26 Keyboard-only operation; focus not obscured | tests/e2e/attention-inbox.spec.ts | attention_should_BeOperableByKeyboardWithJkEnterTDEscAndVisibleFocus_When_NoPointer | Playwright | Drive keys with focus in the list; assert focus order, `d` rule, the global `?` overlay lists the Attention section, keys inert with focus outside the list, focus ring visible and not under sticky callout |
| UX-27 One debounced polite announcement; no focus move; no reorder; "N new - show"; a new approval is never held back | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_AnnounceOnceDebouncedKeepFocusAndInsertNNewButton_When_3ItemsArriveIn2s | Jest + fake timers | Burst events; assert single message, `document.activeElement` unchanged, button present, rows not reordered; plus `PromoteNewApprovalToStickyLineAndSpeakAtPriority1_When_HoldActiveUnderPointerOrFocus` |
| UX-28 Status never colour-only; contrast in every state including stale and offline | tests/e2e/attention-inbox.spec.ts | attention_should_ShowIconAndTextPerReasonAndPassContrastInLightAndDark_When_AxeColorContrastRun | Playwright + axe | Light/dark emulation; contrast rule run in populated AND disconnected states; assert every reason has icon + text and no row opacity < 1 |
| UX-29 Reduced motion: no pulse, static "New" tag | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowStaticNewTagAndNoAnimationClass_When_PrefersReducedMotion | Jest + matchMedia mock | Mock media query; new item; assert tag, no animation class |
| UX-30 Title "(N) Stapler Squad", nav badge accessible name | web-app/src/lib/hooks/useDocumentTitleCount.test.ts | useDocumentTitleCount_should_SetTitleWithCountAndBadgeAriaName_When_NeedsMeGreaterThanZero | Jest + RTL | Counts 0/3; title and `aria-label` "Attention, 3 need you" |
| UX-31 Targets >= 44x44 under `pointer: coarse` at any width, >= 24 px under `pointer: fine` | tests/e2e/attention-inbox.spec.ts | targets_should_Measure44x44OnCoarseAndAtLeast24OnFine_When_BoundingBoxesCheckedAt375And768To1023And1280 | Playwright | Measure rows, chips, tabs, Close, overflow, Load older with touch emulation at 375 and 1024 px and with a fine pointer at 800 and 1280 px (component half: `Size44UnderPointerCoarseAtAnyWidthAnd24UnderPointerFine_When_MediaEmulated`) |
| UX-32 Every swipe has button equivalent | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ExposeButtonEquivalentForEverySwipeAction_When_Mobile | Jest + RTL | Enumerate gesture handlers; each has overflow-menu action |
| UX-33 Sheet safe-area, keyboard, internal scroll | manual | bottomSheet_should_RespectSafeAreaAndNotCoverFocusedControlWithVirtualKeyboard_When_OnDevice | Manual | On real iOS/Android: open sheet, focus a control, open keyboard, check inset and scroll (jsdom cannot model `visualViewport`); automated half: SessionEventDrawer.test.tsx asserts `env(safe-area-inset-bottom)` style present |
| UX-34 Up Next tab strip horizontal scroll >= 44 px with edge cue at 375 | tests/e2e/unfinished-sessions-tab.spec.ts | tabStrip_should_ScrollHorizontallyWithTargets44AndEdgeCue_When_375px | Playwright | Viewport 375; measure tabs; scroll strip; assert cue element |
| UX-35 Redirect preserves params, replace nav, no flash, focus on row | tests/e2e/unfinished-sessions-tab.spec.ts | redirect_should_LandOnSessionsTabPreservingParamsWithReplaceAndFocusRow_When_FlagOn | Playwright | Visit `/review-queue?session=abc&filter=needs`; assert URL; `goBack` returns to previous page; assert no legacy content rendered; focus on linked row |
| UX-36 Flag off or fetch failure renders legacy | web-app/src/app/review-queue/page.test.tsx | ReviewQueuePage_should_RenderLegacyPage_When_FlagOffOrFlagFetchRejects | Jest + RTL | (also R6) two renders |
| UX-37 <= 160 chars agent text; no raw tail/unredacted secrets; text nodes only | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | Excerpt_should_NotExceed160CharsInRowTrayAndTimelineAndShowNoSecrets_When_LongSecretBearingTextProvided | Jest + RTL | Feed 400-char text with a token and an HTML tag to three components; assert truncated/redacted and rendered as literal text (server-side redaction covered by NFR3 Go tests; `excerptRendering_should_UseTextNodesOnly_...` greps the new files) |
| UX-38 Recent activity view in <= 2 clicks, filters, Done (24 h), hidden toggle, Load older | tests/e2e/attention-inbox.spec.ts | recent_should_BeReachableInTwoClicksFromNavBadgeAndListFinishedSessions_When_DesktopAnd375px | Playwright | Click nav badge then "Recent" at both sizes; assert "Finished (24 h)" lists a Done session and a never-raised session; toggle windows; Load older; hidden absent until toggled (component half: `RecentActivityView.test.tsx` rows above) |
| UX-39 Badge, tab badge and title share the needs-me count; AwaitingDirection never counts | web-app/src/components/sessions/ReviewQueueNavBadge.test.tsx | reviewQueueBadge_should_CountOnlyNeedsHumanReasonsAndShareOneSelectorWithTabAndTitle_When_AwaitingDirectionPresent | Jest + RTL | 2 needs-me + 3 awaiting_direction: badge, tab badge and title all read 2 |
| UX-40 Single-key shortcuts scoped, setting, no conflicts, row buttons reachable | web-app/src/lib/hooks/useAttentionListKeys.test.ts | useAttentionListKeys_should_ActOnlyWhenFocusInsideList_When_KeysPressedInInputOmnibarDialogAndBody | Jest + RTL | Press keys in each context; setting off makes them inert; Ctrl+T (`terminal.spawn-shell`) untouched (Triad rows for setting and conflicts; `KeepCurrentRowButtonsAsTabStopsAndOthersTabindexMinus1_When_RovingFocusMoves`) |
| UX-41 Cue counts every unseen finished session regardless of push; same on both devices; mobile second line; hidden at 0 | tests/e2e/attention-alert-guarantee.spec.ts | alertGuarantee_should_ShowUnseenFinishedCueWithoutPush_When_NotificationPermissionDenied | Playwright | Deny permission; assert text cue, no toast/sound; repeat with permission granted and assert the same count; open the session in a second context and assert the cue drops on both (component half: `reviewQueueBadge_should_ShowSameFinishedCount_When_PushGrantedDeniedNoSubscriptionOrBackgrounded`, `RenderSecondTextLineHideAtZeroAndCapAt9Plus_When_MobileBottomNav`) | Plus `reviewQueueBadge_should_RenderNewFinishedVisibleTextDistinctFromFinishedChipCount_When_OneOfThreeOpened`.
| UX-42 Volume bounds; mute scope table; never hides state; global marker | server/review_queue_manager_test.go | TestAwaitingDirectionDelivery_Should_PushFirstSixThenOneDigestAndKeepAllTrayRecords_When_8DistinctSessionsFinishWithin10Minutes | Go unit | Caps and digest; mute half: `TestMute_Should_NotSuppressToastOrBadgeCount_...`, `TestMuteScope_Should_HoldPushAndSlackForAwaitingDirectionAndProseQuestionOnly_When_SessionMuted`, `TestMuteScope_Should_NotHoldProseQuestionOrApprovals_When_GlobalMuteActive`, the UI test `ShowMutedUntilTextKeepRowAndUnmuteViaToast` and `ShowVisibleMutedWordAndAppendAlertsMutedToNameOnEveryPageAndFormFactor_When_GlobalMuteActive` |
| UX-43 Unconfirmed signal | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowUnconfirmedTagHeaderCountAndNavSuffix_When_ScreenSourcedNeedsHumanItem | Jest + RTL | Hook vs screen items; tag, header count, accessible name |
| UX-44 Toast accessibility, undo for every action, toasts are not live regions | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_PauseToastCountdownOnHoverAndFocusAndStackThreeWithIndependentUndo_When_ActionsRepeated | Jest + fake timers | Hover/focus pause, stack of 3, Ctrl/Cmd+Z, hotkey hint in the text, Undo for Done/Not a question/Stop flagging/bulk/Mute, no live role on the toast (Triad rows) |
| UX-45 Modal drawer semantics at 1280/900/375 | web-app/src/components/sessions/SessionEventDrawer.test.tsx | SessionEventDrawer_should_HaveDialogAriaModalLabelledAndInertBackground_When_OpenedAt1280900And375 | Jest + RTL | Assert role, aria-modal, labelledby, inert siblings; layout at 900 px is single column (Playwright half in `session-event-drawer.spec.ts`) |
| UX-46 Focus placement after removal | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_MoveFocusToNextPreviousHeaderThenHeading_When_RowRemoved | Jest + RTL | Remove last row of a group, last row overall, bulk; assert `document.activeElement` never `body` |
| UX-47 Quiet status/threshold; Changed set and flapping cap | web-app/src/components/sessions/ReviewQueuePanel.test.tsx | ReviewQueuePanel_should_ShowQuietRowsWithStatusTextAndHonorThresholdSetting_When_30MinBuildRunningVsIdle | Jest + RTL | Plus `RecentActivityView_should_CollapseConsecutiveStatusChangesWithin2MinAndCapFiveRowsPerSession_When_12Flaps` |
| UX-48 Tray coupling, stale cue, first-run help collapsed | web-app/src/components/ui/NotificationItem.test.tsx | NotificationItem_should_ResolveTrayRecordAndDecrementBadge_When_AttentionItemAcknowledged | Jest + RTL | Opt-out reviewability (triad round 3): `MuteManagementList_should_ListMutesAndQuestionOptOutsWithSetDateAndReverseEachInOneStep_When_RowHasCleared`, `SessionCard_should_ShowQuestionsNotFlaggedTag_When_OptOutSet`, `ReviewQueuePanel_should_ShowFourteenDayReminderAndRedisplayAfterFourteenMoreDays_When_OptOutOlderThan14Days`. Plus `reviewQueueBadge_should_AppendMayBeOutOfDateToAccessibleName_When_StreamStale`, `AttentionFirstRunHelp_should_ShowOnce...` and `KeepNeedsYouRowsAboveFoldAt375x812_When_FirstRunHelpShownCollapsed` |
| UX-49 Reflow at 320 px and 400% zoom, text spacing, role semantics | tests/e2e/attention-inbox.spec.ts | attention_should_ReflowAt320pxAnd400PercentZoomKeepRoleSemanticsAndClipNothing_When_TextSpacingOverridesApplied | Playwright | Viewport 320, zoom 400% and container widths either side of 720 px (768 and 1023 viewports), overflow menu holds Timeline/Stop flagging/Mute while "Not a question" stays visible; apply WCAG 1.4.12 overrides; assert no horizontal page scroll, no clipped row/drawer/toast, rows are `li` of `ul`, Needs you/Finished/Quiet chips are `aria-pressed` toggles in a `role=group` while the Recent chip navigates (no `aria-pressed`, `aria-current="page"`), the Up Next strip is a `tablist` |

UX criteria covered: 49/49 (UX-33 has a manual device half in addition to its automated assertion; UX-39..UX-42 delivery halves are the server rows in the mapping tables).

## Requirements coverage check

Every row of the Requirement ID table has at least one happy-path and one error-path test above, and each store-touching requirement has an Integration test:

| ID | Happy | Error | Integration |
|---|---|---|---|
| R1 | yes | yes | yes |
| R2 | yes | yes | yes (poller chain) |
| R3 | yes | yes | yes |
| R4 | yes | yes | yes |
| R5 | yes | yes | yes |
| R6 | yes | yes | yes (registry) / manual ADR gate |
| R7 | yes | yes | yes (analyzer) |
| R8 | yes | yes | yes |
| SM1 | yes | yes | yes (FN-rate, actuator) |
| SM2 | yes | yes | yes |
| SM3 | yes | yes | yes (e2e restart) |
| SM4 | yes | yes | yes (graceful/crash) |
| SM5 | yes | yes | N/A (no store; baseline U1 is a Manual doc row) |
| SM6 | yes | yes (negative control) | yes (real poller + manager) |
| NFR1 | yes | n/a | yes (query count) |
| NFR2 | yes | yes | yes |
| NFR3 | yes | yes | n/a (pure function) |
| OB1 | yes | yes | n/a (in-process counters; delivery covered in R1) |
| OB2 | yes | yes | n/a |
| OB3 | yes | yes | yes |
| RC1 | yes | yes | yes + Migration |
| C1-C5, OOS | yes | yes | yes |

Requirements covered: 27/27 IDs (R1-R8, SM1-SM6, NFR1-NFR3, OB1-OB3, RC1, C1-C5, OOS), with each Success Metric counted separately. Test totals in the mapping tables (recounted by script after triad round 3; was 253 Unit, 57 Integration, 16 E2E, 9 Manual, 1 Migration = 336 after iteration 2; before that 220 Unit, 55 Integration, 14 E2E, 8 Manual, 1 Migration = 298 after iteration 1, and 168/47/7/4/1 = 227 after the Phase 4 patch): 264 Unit, 57 Integration, 17 E2E, 10 Manual, 1 Migration (349), plus 49 UX acceptance rows (Playwright, jest, manual). UX-38 and UX-39 reuse tests already in the mapping tables; UX-40..UX-49 map to primary tests that are also listed in the Triad repair tables, so they add no new row to the 349 (UX-49 maps to the iteration 2 E2E row `attention_should_ReflowAt320pxAnd400PercentZoom...`).

## Test Stack
- **Unit**: Go `testing` + `testify` with table tests, fakes for `SessionEventRepository`, `SessionEventRecorder`, `turnEndSink`, counting flag resolver, injected clocks (`deterministic-fast-tests`); `-race` on `./session ./server/...`. Frontend: Jest + React Testing Library (`cd web-app && bunx jest --no-coverage`), fake timers, `@/lib/nav` mock via `jest.requireActual("@/lib/nav/impl.memory")`.
- **Integration**: in-memory SQLite ent client (`Schema.Create`, reopen to simulate restart), real `AcknowledgeSession`/`RecordTurnEnd` paths, ent query-count hook, bus/delivery-gate direct calls, `httptest` for hook and actuator handlers; no real tmux and no sleeps.
- **E2E / UX**: Playwright + axe via `tests/e2e` global-setup (isolated server on a free port, never `:8543`); specs `attention-inbox.spec.ts`, `session-event-drawer.spec.ts`, `unfinished-sessions-tab.spec.ts`, `session-card-timer.spec.ts`; manual checklist for UX-33, the soak run (isolated instance with `STAPLER_SQUAD_INSTANCE` + private tmux socket, never `make install-service`), Spike 0.1.2, and ADR ratification.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/... -coverprofile=coverage.out && go tool cover -func=coverage.out` (after `make build`) | >= 80% line on new/changed files; 100% of `Determine` reason branches via goldens |
| TypeScript/Jest | `cd web-app && bunx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | >= 80% line on new/changed files |
| Playwright | `cd tests/e2e && bun run test` | All listed specs green on desktop and 375 px projects |

- All public service methods (`ListSessionEvents`, `AcknowledgeSession` incl. `dismiss_kind`, `HandleStop`, `/actuator/attention-coverage`): happy path and error paths covered above.
- All external integrations (ent/SQLite, transcript file read, headless LLM tiebreak, delivery gate, bus): unit with fakes and at least one integration test.
- UX acceptance criteria: each of UX-01..UX-49 has a test or manual step above.
- Gate sweep (Story 7.3.1): `make ready`, `bun run lint:duplicates` (jscpd margin recorded), zero-warning web lint on changed files, `tsc`, `go test -tags sinkguard -short -run UIRpcSurface ./server/services`, `TestConfigSurfaceProof`, `route-meta.test.ts`, `make registry-generate` clean diff, no `waitForTimeout`, no test filename containing "gate" (BUG-087: the planned file `ui_rpc_surface_test.go` and the sink-guard test names comply; the Playwright/Go files above contain no "gate" in filenames).

## Notes and gaps

- Race and flake handling (triad iter 1; extended iter 2; facts verified in plan.md Verification Notes). Also known and open: BUG-077 (`docs/bugs/open/BUG-077-hibernation-resume-races-with-approve-deny-instance-status-write.md`), BUG-083 (`docs/bugs/open/BUG-083-server-services-flaky-under-full-suite-parallel-load.md`) and the four open BUG-099 files; a red test that matches one is re-run in isolation (`-run <name> -count=1`) and the result and bug file are cited, but a red test this plan added is investigated, not waved through. Original note: the `-race -count=20` rows (`TestTurnEndAndAckPersistence_Should_PassRaceDetectorCount20_When_RunWithRace`, Tasks 7.3.1b/c) run only on focused `./session` tests, never on `./server/services` or `./server/...`, because BUG-067 (open; its 150s/`-race` description is stale since `mcp-integration.yml` now uses `-timeout=1200s` and runs `server/services` without `-race`, while `build.yml` `test-rest` runs it under `-race -short -timeout=22m`) records load-dependent timing failures on that package. Full-package race runs use `-timeout 20m`. New `server/services` rows use fake clocks and sinks, no `captureLogs`/global `slog` swap under `t.Parallel()` (BUG-087, open), and no real `SessionService` plus `SetLifecycleContext` (BUG-088, open). A red `TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` (BUG-090, open) is re-run in isolation and cited; BUG-089 is fixed (`docs/bugs/fixed/`).
- SM6 (alert guarantee) is testable only against fakes for push and Slack; whether a real browser or phone actually displays the push, or the operator perceives a cue, is the Manual usability row (A1, A4, A10). It proves an alert or cue was produced, never that it was perceived.
- SM1 "gap = 0" is necessary but not sufficient: the actuator's ground truth cannot see a question the classifier never flagged, so recall on the owner-labelled corpus (>= 0.80) is the falsifier; `TestSoakReport_Should_FailCoverageMetric_When_GapZeroButRecallBelow80` makes the soak report fail on that combination.
- Task 1.1.3a-d is one L run (about 10 files, one compiling commit) in the plan's effort table; its rows are unchanged and are all produced by that run.
- `TestUpdateTimestamps_*` rows exercise the output path `Instance.UpdateTerminalTimestamps` (`session/instance_approval.go:107-115`) around the promoted `ReviewState.UpdateTimestamps`; the names are historical.
- B1 (missed-needs-operator baseline) measures detected-but-missed from existing history plus a sampled estimate for undetected prose questions; it cannot be a census (limitations are written into `docs/reference/attention-inbox.md`). The targets in `requirements.md` stay proposals until B1 and U1 are recorded.
- Conditional rows (`TestReloadWriter_*`, `SessionCard_should_NotDeriveLastActivityFromMountTime_*`) apply only if Spike 0.1.2 finds a reload-only writer outside the `SessionActivity` owners.
- Test IDs are planned names; plan Files lists are the only evidence for paths (INFERRED). Implementers may rename files but must keep one test per row.
- SM5 (usage increase vs baseline) is observational; only the instrumentation (`usePageView`) is testable, not the increase itself.
- SM1's "100%" and FN rate are measured by the fixture corpus plus the soak, not provable by CI alone; the soak row is Manual and gated on the owner window. The Story 0.1.5 exit gate is Manual and gated on the owner confirming the 80% threshold.
- Non-Claude programs are validated only for "Finished" and "cannot detect"; their `Question` coverage is untested by design until a labelled corpus exists (plan Story 3.1.2).
