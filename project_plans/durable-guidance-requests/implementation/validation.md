# Validation Plan: durable-guidance-requests

**Date**: 2026-09-12

## Requirement Key

From `requirements.md`:

- **REQ-M1**: Automated triage asks a structured question and receives the answer on a later pass/resumed session, no human intervention beyond answering (Success Metrics #1).
- **REQ-M2**: A question from a paused/restarted/dead session is still answerable and durably retrievable by whichever process later picks it up (Success Metrics #2).
- **REQ-M3**: Renders as a structured form in backlog item detail, triage panel, and session view (Success Metrics #3).
- **REQ-M4**: No existing behavior regresses (Success Metrics #4).
- **REQ-S1**: Durable construct creatable by any subscribed LLM/session, scoped to item, session, or standalone (Scope bullet 1).
- **REQ-S2**: Yes/no, single-select multiple choice, short free-text answer types (Scope bullet 2).
- **REQ-S3**: Persistence survives restart/resume across sessions (Scope bullet 3, overlaps REQ-M2).
- **REQ-S4**: Durable notification path to the asker on answer, reusing the existing mechanism (Scope bullet 4).
- **REQ-S5**: Structured form rendering in the 3 named views (Scope bullet 5, overlaps REQ-M3).
- **REQ-S6**: Triage wired to use the construct for ≥1 real clarification scenario (Scope bullet 6).
- **REQ-S7**: New proto messages/RPCs (Scope bullet 7).
- **REQ-A1** *(adversarial-review-derived)*: Feature flag defaults enabled (`GetFeatureFlagWithDefault`, not `GetFeatureFlag`).
- **REQ-A2** *(adversarial)*: Answer-shape validation rejects invalid answers via direct RPC/MCP, bypassing the UI.
- **REQ-A3** *(adversarial)*: Notify-failure on answer must not surface as an `AnswerGuidanceRequest` RPC error.
- **REQ-A4** *(adversarial)*: Session-scoped AND item-scoped orphan reconciliation (symmetric, both directions).
- **REQ-A5** *(adversarial)*: `AwaitingGuidance` outcome path in `onAutonomousDriverComplete` — `MarkStuck`/`UpdateItemSessionEnded`/`AutoRespawnAutonomousWork` skipped when true, fire normally when false (Task 1.7.3c).
- **REQ-A6** *(adversarial, flagged as missing from plan)*: Ordinary DONE/WAIT/NEXT_MESSAGE-only autonomous-driver runs and ordinary (no-`blocking_question`) triage runs are completely unaffected — explicit regression coverage.
- **REQ-A7** *(adversarial)*: Cross-restart durability — session A creates a request, session A's process is fully torn down, a fresh session/process reads the answer back.
- **REQ-A8** *(adversarial)*: Dedup/abuse-cap on automated-triage-originated requests (cooldown + outstanding cap), bypassed for human-initiated requests.

## Test Stack

- **Backend unit/integration**: Go stdlib `testing` + `testify` (`assert`/`require`), table-driven where the plan's own precedent (`backlog_service_triage_test.go`, `autonomous_orchestration_service_test.go`) is table-driven. Run via `gotestsum` (`make test`). In-memory ent DB via `session.NewTestEntRepository(t)` (`session/testing.go`) for anything touching storage — no `t.TempDir()`, no real sleeps/`time.Sleep`/`require.Eventually` (`docs/explanation/test-io-storage-isolation.md`, `deterministic-fast-tests` skill). Streaming/event-bus tests use a synchronous/fake `*events.EventBus`, not real time.
- **Backend naming convention** (verified against `server/services/backlog_service_triage_test.go` and `autonomous_orchestration_service_test.go`): `Test<Subject>_<Behavior>_When_<Condition>` (equivalently `Test<Subject>_should_<behavior>_When_<Condition>` — both forms are live in this repo; this plan uses the former for new files).
- **Frontend unit**: Jest + React Testing Library, `describe("<Component>", () => { it("<plain-English behavior>", ...) })` — verified against `GoalPanel.test.tsx` and `RadioGroup.test.tsx`. Run via `cd web-app && npx jest --no-coverage`.
- **E2E**: Playwright + Allure (`tests/e2e/`), `data-testid`/ARIA locators only, no `waitForTimeout`, spec files annotated `// @feature ...` per `e2e-test-conventions`.

## Requirement → Test Mapping

### Epic 1.1 — Ent Schema + Storage Layer

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S1, REQ-S2 | `session/guidance_request_test.go` | `TestGuidanceRequest_CreateGetList_RoundTripsAllFields_When_AllThreeQuestionTypesCreated` | Unit | Happy path: create yes_no/multiple_choice/short_answer, `GetGuidanceRequest` round-trips every field (Task 1.1.3a) |
| REQ-S1 | `session/guidance_request_test.go` | `TestGuidanceRequest_ListGuidanceRequests_FiltersByItemSessionAndStatus_When_MultipleRequestsExist` | Unit | Happy path: `ListGuidanceRequests` filters correctly by `ItemID`/`SessionUUID`/`Status` (Task 1.1.3a) |
| REQ-S3, REQ-M2 | `session/guidance_request_test.go` | `TestGuidanceRequest_AnswerGuidanceRequest_SetsStatusAnsweredAndTimestamp_When_Pending` | Unit | Happy path: answer a pending request, assert `Status`/`AnsweredAt`/`Answer`/`AnsweredBy` set (Task 1.1.3b) |
| REQ-S3 | `session/guidance_request_test.go` | `TestGuidanceRequest_AnswerGuidanceRequest_RejectsSecondAnswer_When_AlreadyAnswered` | Unit | Error path: the compare-and-swap rejects a second answer instead of silently overwriting (Task 1.1.3b) |
| REQ-S1 | `session/guidance_request_test.go` | `TestGuidanceRequest_GetGuidanceRequest_ReturnsErrNotFound_When_IDDoesNotExist` | Unit | Error path: miss returns `session.ErrNotFound`, matching `GetSessionGoal`'s convention |
| REQ-S1, REQ-M2 | `session/guidance_request_test.go` | `TestGuidanceRequest_CreateGuidanceRequest_PersistsAndRoundTrips_When_ScopeIsStandalone` | Integration | `ItemID == nil && SessionUUID == ""` persists and round-trips via `session.NewTestEntRepository(t)` (in-memory ent) — proves backend is fully general regardless of scope (Task 1.1.3c) |

### Epic 1.2 — Proto + `GuidanceService` RPCs

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S7, REQ-S1 | `server/services/guidance_service_test.go` | `TestGuidanceService_CreateGuidanceRequest_PublishesEventAndPersists_When_ValidYesNoRequest` | Unit | Happy path: create succeeds, persists, publishes onto the fake `*events.EventBus` |
| REQ-S2 | `server/services/guidance_service_test.go` | `TestGuidanceService_CreateGuidanceRequest_RejectsInvalidArgument_When_MultipleChoiceHasEmptyChoices` | Unit | Error path: create-time validation (Task 1.2.2a) |
| REQ-S7 | `server/services/guidance_service_test.go` | `TestGuidanceService_GetGuidanceRequest_ReturnsNotFound_When_IDMissing` | Unit | Error path: `connect.CodeNotFound` mapping |
| **REQ-A2** | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_AcceptsYesOrNo_When_QuestionTypeYesNo` | Unit | Happy path: valid shape accepted (Task 1.2.2b) |
| **REQ-A2** | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_RejectsArbitraryString_When_QuestionTypeYesNoAndAnswerNotYesOrNo` | Unit | **Error path — direct RPC call bypassing the UI's `RadioGroup` constraint; asserts `connect.CodeInvalidArgument`.** This is the exact gap ADR-003's Consequences section calls out. |
| **REQ-A2** | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_RejectsAnswerNotInChoices_When_QuestionTypeMultipleChoice` | Unit | Error path: same bypass scenario for multiple-choice |
| **REQ-A2** | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_RejectsEmptyAnswer_When_QuestionTypeShortAnswer` | Unit | Error path: non-empty-string constraint for short_answer |
| REQ-S3 | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_ReturnsFailedPrecondition_When_AlreadyAnswered` | Unit | Error path: CAS failure surfaces as `connect.CodeFailedPrecondition` |
| REQ-S4, REQ-S7 | `server/services/guidance_service_test.go` | `TestGuidanceService_WatchGuidanceRequests_SendsSnapshotCompleteEvent_When_NoPendingRequestsMatchFilter` | Integration | Streaming RPC with fake sender interface (mirrors `backlogItemEventSender`) — the empty-match marker event (Task 1.2.2c) |
| REQ-S4 | `server/services/guidance_service_test.go` | `TestGuidanceService_WatchGuidanceRequests_ReplaysEventsSinceAfterSeq_When_Reconnecting` | Integration | Reconnect replay via `EventsSince(after_seq)`, `is_snapshot: true` forced on replayed events |
| **REQ-A1** | `config/config_test.go` | `TestConfig_GetFeatureFlagWithDefault_ReturnsTrue_When_GuidanceRequestsFlagNotSet` | Unit | **Happy path — the shipped-silently-disabled blocker.** Asserts `GetFeatureFlagWithDefault("guidance_requests", true)` returns `true` on a config with no explicit `guidance_requests` key, mirroring `config_test.go`'s existing "backlog feature flag should be false by default" test but for the opposite (default-true) case. |
| **REQ-A1** | `server/server_test.go` | `TestServer_GuidanceServiceHandler_RespondsSuccessfully_When_FeatureFlagNotExplicitlySet` | Integration | End-to-end wiring check: a fresh server (no persisted flag override) accepts a `CreateGuidanceRequest` call rather than 404/disabled — proves the interceptor actually uses the default-true call, not just that the helper function does |

### Epic 1.3 — Notification Integration

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S4, REQ-M2 | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_TriggersExactlyOneDurableNotification_When_AnsweredSuccessfully` | Unit | Happy path: fake `session.Notifier` records one call with `NOTIFICATION_TYPE_INPUT_REQUIRED` and a message referencing the question (Task 1.3.3a) |
| REQ-S4 | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_UsesDistinctCoalescingIdentity_When_TwoRequestsAnsweredForSameSession` | Unit | Edge case: two different pending requests for the same `session_uuid` produce two distinct coalescing identities, so `subscriber.go`'s `coalesceKey` doesn't merge them (Task 1.3.3b) |
| **REQ-A3** | `server/services/guidance_service_test.go` | `TestGuidanceService_AnswerGuidanceRequest_ReturnsSuccessResponse_When_NotifierFails` | Unit | **Error path — a fake `Notifier` that errors/panics-if-checked must not cause `AnswerGuidanceRequest` to return an error; the already-committed CAS write is authoritative.** Directly covers "notify-failure must not surface as RPC error." |

### Epic 1.4 — MCP Tool Surface

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S1 | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_CreateGuidanceRequest_CreatesAndRoundTripsViaGet_When_ValidParams` | Unit | Happy path: create → get round trip (Task 1.4.3b) |
| REQ-S2 | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_CreateGuidanceRequest_ReturnsToolError_When_QuestionTypeInvalid` | Unit | Error path: unknown `question_type` string rejected |
| **REQ-A2** | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_AnswerGuidanceRequest_RejectsInvalidAnswerShape_When_YesNoAnswerIsArbitraryString` | Unit | Error path: MCP path exercises the same validation as the RPC path (Task 1.4.2b delegates to `GuidanceService.AnswerGuidanceRequest`, doesn't duplicate logic — this test proves it actually delegates) |
| REQ-M1, REQ-S1 | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_WaitForGuidanceAnswer_ReturnsImmediately_When_AlreadyAnswered` | Unit | Happy path: precheck against persisted state, no bus subscription/blocking needed (Task 1.4.3b) |
| REQ-M1 | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_WaitForGuidanceAnswer_BlocksThenReturns_When_AnsweredEventFiresOnSyncBus` | Integration | Blocks on a synchronous test event bus, unblocks on the `answered` event — no real time (Task 1.4.3b) |
| REQ-M1 | `server/mcp/tools_guidance_test.go` | `TestGuidanceHandlers_WaitForGuidanceAnswer_TimesOut_When_NoAnswerWithinBound` | Unit | Error/edge path: the ≤60s bound is enforced, mirrors `waitForBacklogEvent`'s equivalent |

### Epic 1.5 — Abuse/Dedup Guard + StuckReason + Lifecycle Reconciler

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| **REQ-A8** | `server/services/guidance_dedup_test.go` | `TestGuidanceService_CreateGuidanceRequest_RejectsResourceExhausted_When_AutomatedOriginExceedsCapOfThree` | Unit | Error path: the 4th automated-origin create for the same scope is rejected with `connect.CodeResourceExhausted` (Task 1.5.4a) |
| **REQ-A8** | `server/services/guidance_dedup_test.go` | `TestGuidanceService_CreateGuidanceRequest_AllowsFourthRequest_When_HumanInitiated` | Unit | Control/happy path: a human-initiated 4th create is never capped — "the human user answers" side is never blocked by automation's own cap |
| **REQ-A8** | `server/services/guidance_dedup_test.go` | `TestGuidanceDedup_SuppressesDuplicateQuestion_When_AutomatedOriginWithinCooldownWindow` | Unit | Happy path: a near-identical automated question within the cooldown is suppressed, mirroring `session/nudge_dedup.go`'s shape (Task 1.5.4a) |
| **REQ-A8** | `server/services/guidance_dedup_test.go` | `TestGuidanceDedup_AllowsDuplicateQuestion_When_HumanInitiated` | Unit | Control path: human-initiated creates bypass the cooldown entirely |
| REQ-M3, REQ-S5 | `server/services/backlog_service_stuck_test.go` | `TestComputeStuckReasons_IncludesAwaitingGuidance_When_ItemHasPendingGuidanceRequest` | Unit | Happy path: `StuckReasonAwaitingGuidance` surfaces from a pending item-scoped `GuidanceRequest` (Task 1.5.2b), inserted into the shared priority list this file owns (PR #769) |
| REQ-M3 | `server/services/backlog_service_stuck_test.go` | `TestComputeStuckReasons_ClearsAwaitingGuidance_When_NoPendingRequestsRemain` | Unit | Happy path: auto-clears once no pending requests remain, matching the existing `ResolveStuck`-on-recovery convention |
| REQ-S3 | `session/backlog_lifecycle_guidance_test.go` | `TestExpireStaleGuidanceRequests_TransitionsToExpired_When_PendingRequestOlderThanMaxAge` | Unit | Happy path: age-based sweep (Task 1.5.4b) |
| REQ-S3 | `session/backlog_lifecycle_guidance_test.go` | `TestExpireStaleGuidanceRequests_LeavesAnsweredRequestUntouched_When_AlreadyAnswered` | Unit | Negative/error path: an already-answered request is not touched by the sweep |
| REQ-S3 | `session/backlog_lifecycle_guidance_test.go` | `TestExpireStaleGuidanceRequests_IsNoOp_When_RunTwiceConsecutively` | Unit | Idempotency: re-running the reconciler is a no-op the second time (Task 1.5.4b) |
| **REQ-A4** | `session/backlog_lifecycle_guidance_test.go` | `TestReconcileOrphanedGuidanceRequests_ExpiresRequest_When_ItemScopedAndItemIsDoneOrDeleted` | Integration | Happy path: item-scoped orphan reconciliation (Task 1.5.3b) |
| **REQ-A4** | `session/backlog_lifecycle_guidance_test.go` | `TestReconcileOrphanedGuidanceRequests_ExpiresRequest_When_SessionScopedAndSessionUUIDNoLongerResolves` | Integration | **Happy path — the symmetric session-scoped orphan check adversarial review flagged as a missing gap. Without this, a session-scoped orphan would sit pending for up to the full 14-day TTL instead of being reconciled immediately like an item-scoped orphan.** |
| **REQ-A4** | `session/backlog_lifecycle_guidance_test.go` | `TestReconcileOrphanedGuidanceRequests_DoesNotExpireAnything_When_ItemOrInstanceLookupErrors` | Unit | Error path: mirrors `PruneOrphaned`'s "nil/errored lookup means not-ready-to-judge, not everything-is-orphaned" guard — for both the item-lookup and instance-lookup branches |

### Epic 1.6 — Automated Triage Integration

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S6 | `session/backlog_triage_test.go` | `TestParseHeadlessTriageResult_RoundTripsBlockingQuestion_When_FieldPresent` | Unit | Happy path: `BlockingQuestion` round-trips through JSON extraction (Task 1.6.1b) |
| **REQ-A6** | `session/backlog_triage_test.go` | `TestParseHeadlessTriageResult_ParsesSuccessfully_When_BlockingQuestionFieldOmitted` | Unit | **Regression path — the common case (no blocking question) must keep parsing unchanged.** |
| REQ-M1, REQ-S6 | `server/services/backlog_service_triage_guidance_test.go` | `TestTriggerTriage_CreatesGuidanceRequestAndSkipsStatusAdvance_When_ResultHasBlockingQuestion` | Integration | Happy path: fake headless pool returns a `blocking_question` result; asserts a `GuidanceRequest` created with `AutomatedOrigin: true` scoped to the item, item status did not advance past `idea`, `StuckReasonAwaitingGuidance` present (Task 1.6.3a) |
| REQ-S6 | `server/services/backlog_service_triage_guidance_test.go` | `TestTriggerTriage_EndsItemSessionWithAwaitingGuidanceReason_When_BlockingQuestionCreated` | Integration | Happy path: `UpdateItemSessionEndedWithReason("awaiting_guidance")` called so orphan-detection doesn't misclassify the triage session as crashed (Task 1.6.2b) |
| **REQ-M4, REQ-A6** | `server/services/backlog_service_triage_guidance_test.go` | `TestTriggerTriage_CompletesNormally_When_ResultHasNoBlockingQuestion` | Integration | **Regression path explicitly required by requirements.md's "no existing behavior regresses" — an ordinary triage result is completely unaffected by this feature (Task 1.6.3b).** |

### Epic 1.7 — Autonomous Driver Integration

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S1, REQ-M1 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_ParseOrchestrationResponse_ReturnsDirectiveAskQuestion_When_ResponseContainsQuestionMarker` | Unit | Happy path: `QUESTION:` keyword parsed into `directiveAskQuestion` (Task 1.7.1a) |
| **REQ-M4** | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_ParseOrchestrationResponse_MatchesLastDirectiveCaseInsensitively_When_QuestionAppendedWithoutLeadingNewline` | Unit | **Regression path — guards the BUG-056 fix (anywhere-in-string, last-match-wins matching) for the new `QUESTION` alternative specifically, since Task 1.7.1a edits the same regex that bug was about.** |
| REQ-M1, REQ-S1 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_Run_CreatesGuidanceRequestAndContinuesWaiting_When_DirectiveAskQuestion` | Unit | Happy path: `directiveAskQuestion` creates a request scoped to the session and behaves like `directiveWait`, not a loop-exit (Task 1.7.3a) |
| REQ-S1 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_Run_SkipsGuidanceLogicWithoutPanicking_When_StorageIsNil` | Unit | Error/nil-guard path: `d.storage == nil` (no `WithGuidanceStorage` option) skips the guidance directive/per-turn-check logic entirely and logs a warning instead of panicking (Task 1.7.0a) |
| **REQ-A7, REQ-M2** | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_FreshInstance_InjectsAnswer_When_AnsweredByDifferentProcessDuringSimulatedRestart` | Integration | **The cross-restart durability test at the driver level: driver A asks a question (persisted `GuidanceRequest`); answer it directly via `storage.AnswerGuidanceRequest` (simulating a human answering while driver A's process is down); construct a brand-new driver B for the same session; assert driver B's first turn finds and injects the answer with no in-memory state carried over from driver A (Task 1.7.3b).** |
| REQ-S3 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_Run_MarksRequestDelivered_When_AnswerInjectedViaSubmitDriverContent` | Unit | Happy path: `MarkGuidanceRequestDelivered` called after injection (Task 1.7.2b) |
| REQ-S3 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_Run_DoesNotReinjectAnswer_When_AlreadyMarkedDelivered` | Unit | Error/edge path: a second turn/driver restart does not re-inject the same answer twice |
| REQ-S1 | `session/autonomous_driver_guidance_test.go` | `TestAutonomousDriver_Run_SetsAwaitingGuidanceOutcome_When_MaxTurnsExhaustedWithPendingSessionScopedRequest` | Unit | Happy path: `AutonomousDriverOutcome{Stuck: true, AwaitingGuidance: true, Reason: "awaiting_guidance"}` set on turn-budget exhaustion with a pending request (Task 1.7.1c) |
| **REQ-A5** | `server/services/autonomous_orchestration_service_guidance_test.go` | `TestOnAutonomousDriverComplete_SkipsMarkStuckEndSessionAndRespawn_When_AwaitingGuidanceTrue` | Unit | **The exact test specified as Task 1.7.3c. Invoke `onAutonomousDriverComplete` directly with `AwaitingGuidance: true`; assert `MarkStuck(StuckReasonAutonomousStuck, ...)`, `UpdateItemSessionEnded`, and `AutoRespawnAutonomousWork` are each NOT called (fakes/mocks recording invocation).** |
| **REQ-A5, REQ-A6** | `server/services/autonomous_orchestration_service_guidance_test.go` | `TestOnAutonomousDriverComplete_MarksStuckEndsSessionAndRespawns_When_AwaitingGuidanceFalseOrdinaryGiveUp` | Unit | **Control case in the same test, required by Task 1.7.3c: an otherwise-identical outcome with `AwaitingGuidance: false` DOES trigger all three — proves the branch, not just absence of a crash, is under test. This is also the regression guarantee that ordinary stuck/give-up behavior is unaffected.** |
| REQ-M3 | `server/services/autonomous_orchestration_service_guidance_test.go` | `TestOnAutonomousDriverComplete_LeavesAutonomousOutcomeAtPriorValue_When_AwaitingGuidanceTrue` | Unit | Happy path: the session-level `inst.AutonomousOutcome` badge is not set to `"stuck"` for `AwaitingGuidance: true` (round-3 review's Concern 1 — a second, earlier edit location than the pre-switch guard) |
| REQ-S4 | `server/services/autonomous_orchestration_service_guidance_test.go` | `TestOnAutonomousDriverComplete_SendsAwaitingGuidanceSpecificNotificationText_When_AwaitingGuidanceTrue` | Unit | Happy path: the generic push notification's `else if outcome.AwaitingGuidance` branch fires with "paused, waiting for an answer" text, not the misleading generic give-up text |
| **REQ-M4** | `server/services/autonomous_orchestration_service_guidance_test.go` | `TestOnAutonomousDriverComplete_StillFiresGenericPushNotification_When_AwaitingGuidanceTrue` | Unit | **Regression path: proves no blanket early `return` was used — the generic, role-independent push-notification block still executes for `SessionRoleWork` outcomes, same as before this feature.** |
| **REQ-A6, REQ-M4** | `session/autonomous_driver_test.go` (extend existing file) | `TestAutonomousDriver_Run_ProducesUnchangedOutcome_When_OnlyDoneWaitNextMessageDirectivesUsed` | Unit | **The regression test requirements.md's "no existing behavior regresses" constraint requires and that adversarial review flagged as missing from the plan's own task list. Runs a fake headless pool returning only ordinary `DONE:`/`WAIT:`/`NEXT_MESSAGE:` responses (no `QUESTION:`) end-to-end through `run()`; asserts `AwaitingGuidance` is always `false`, no `GuidanceRequest` is ever created, and the resulting `AutonomousDriverOutcome` is byte-for-byte identical in shape/values to what the pre-feature driver produced for the same fixture.** |

### Epic 2.1 — Frontend Data Layer

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S1 | `web-app/src/lib/redux/guidanceRequestsSlice.test.ts` | `describe("guidanceRequestsSlice") > it("upsertGuidanceRequest adds or replaces a single entry without a full re-snapshot")` | Unit | Happy path (Task 2.1.1a) |
| REQ-S1 | `web-app/src/lib/redux/guidanceRequestsSlice.test.ts` | `it("setGuidanceRequests replaces state with the bulk initial snapshot")` | Unit | Happy path (Task 2.1.1a) |
| REQ-S2 | `web-app/src/lib/redux/guidanceRequestsSlice.test.ts` | `it("mapGuidanceRequest maps an UNSPECIFIED question_type/status without throwing")` | Unit | Error/edge path: defensive proto→domain mapping (Task 2.1.1b) |
| REQ-M2, REQ-S4 | `web-app/src/lib/hooks/useWatchGuidanceRequests.test.ts` | `it("fetches the initial snapshot via listGuidanceRequests and populates state")` | Unit | Happy path (Task 2.1.3a) |
| REQ-S4 | `web-app/src/lib/hooks/useWatchGuidanceRequests.test.ts` | `it("upserts a request into state when a live answered event arrives")` | Unit | Happy path (Task 2.1.3a) |
| REQ-S4 | `web-app/src/lib/hooks/useWatchGuidanceRequests.test.ts` | `it("falls back to REST polling when the watch stream repeatedly fails to reconnect")` | Integration | Error path: exhausts `MAX_RETRIES`, falls back to `FALLBACK_POLL_INTERVAL_MS` polling (Task 2.1.2b) |
| REQ-S4 | `web-app/src/lib/hooks/useWatchGuidanceRequests.test.ts` | `it("requests after_seq on reconnect so events are not missed")` | Integration | Happy path: gap-detection reconnect behavior (Task 2.1.2b) |

### Epic 2.2 — Shared `GuidanceRequestForm` Component

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-S2, REQ-S5 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("renders Yes/No RadioGroup options and calls onAnswer with yes or no")` | Unit | Happy path (Task 2.2.2a) |
| REQ-S2 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("renders request.choices as RadioGroup options and calls onAnswer with the selected choice")` | Unit | Happy path (Task 2.2.2a) |
| REQ-S2 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("renders a textarea and calls onAnswer with the typed text for short_answer")` | Unit | Happy path (Task 2.2.2a) |
| REQ-S2 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("disables the submit button while the short_answer textarea is empty")` | Unit | Error/edge path: UI-layer non-empty constraint mirroring REQ-A2's server-side rule |
| REQ-S5 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("renders a read-only question/answer summary instead of interactive controls when status is answered")` | Unit | Happy path (Task 2.2.2a) |
| REQ-S5 | `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` | `it("does not show the answered state until the onAnswer promise resolves")` | Unit | Error/edge path: no optimistic-only UI (Task 2.2.1b) |

### Epic 2.3–2.5 — View Integrations

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-M3, REQ-S5 | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `it("renders a Guidance Needed section with a GuidanceRequestForm when a pending request exists for this item")` | Unit | Happy path (Task 2.3.1a) |
| REQ-M4 | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `it("does not render the Guidance Needed section when there are no pending requests for this item")` | Unit | Regression/negative control |
| REQ-S4 | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `it("calls answerGuidanceRequest with answered_by set to the UI-session marker when the form is submitted")` | Integration | Happy path — RPC call wiring (Task 2.3.2a) |
| REQ-M3, REQ-S6 | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` | `it("renders GuidanceRequestForm prominently when automated_origin is true and status is pending")` | Unit | Happy path (Task 2.4.1a) |
| REQ-M3 | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` | `it("keeps the structured GuidanceRequestForm visually distinct from an ordinary free-text triage suggestion")` | Unit | Happy path (Task 2.4.1b) |
| REQ-M3 | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` | `it("does not render a human-initiated (automated_origin false) pending request in the triage-blocking slot")` | Unit | Error/edge path |
| REQ-M3, REQ-M1 | `web-app/src/components/sessions/GuidancePanel.test.tsx` | `it("renders nothing when there are no pending guidance requests for the session")` | Unit | Happy path (Task 2.5.2a) |
| REQ-M3 | `web-app/src/components/sessions/GuidancePanel.test.tsx` | `it("renders a GuidanceRequestForm when one pending request exists for the session")` | Unit | Happy path (Task 2.5.2a) |
| REQ-S4 | `web-app/src/components/sessions/GuidancePanel.test.tsx` | `it("calls the answer RPC when the form is submitted")` | Integration | Happy path (Task 2.5.2a) |

### Epic 2.6 — Badge + Feature Registry

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-M3 | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `it("maps awaiting_guidance to a label and icon in the shared StuckReason mapping")` | Unit | Happy path (Task 2.6.1a) — this is the shared mapping file PR #769 introduced; confirmed via `grep -rl StuckReason web-app/src/` |

### Epic 2.7 — E2E

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-M1, REQ-M2, REQ-M3, REQ-S4 | `tests/e2e/guidance-requests.spec.ts` | `test("seeds a pending guidance request via the API, renders the form in BacklogItemDetail, and shows the answered state after submission")` | E2E | Happy path, full loop (Task 2.7.1a) |
| REQ-M3 | `tests/e2e/guidance-requests.spec.ts` | `test("StuckReasonAwaitingGuidance badge appears while pending and disappears after answering")` | E2E | Happy path (Task 2.7.1b) |
| **REQ-A2** | `tests/e2e/guidance-requests.spec.ts` | `test("rejects an invalid answer submitted directly against the API, bypassing the UI form's constraints")` | E2E | **Error path — defense-in-depth confirmation that REQ-A2's server-side validation holds even when the request is crafted outside the browser.** |

## Coverage Summary

- **Requirements coverage**: 19 of 19 explicit requirement IDs above (REQ-M1–M4, REQ-S1–S7, REQ-A1–A8) have at least one mapped test, verified by grepping each ID's occurrence count in the mapping tables → **19/19 (100%)**.
- **Adversarial-review-derived tests** (the ~8 the coordinator asked to confirm), each with its own row above:
  1. Feature flag defaults enabled → `TestConfig_GetFeatureFlagWithDefault_ReturnsTrue_When_GuidanceRequestsFlagNotSet` + `TestServer_GuidanceServiceHandler_RespondsSuccessfully_When_FeatureFlagNotExplicitlySet`
  2. Answer-shape validation via direct RPC/MCP → `TestGuidanceService_AnswerGuidanceRequest_RejectsArbitraryString_When_QuestionTypeYesNoAndAnswerNotYesOrNo` (+ multiple-choice/short-answer siblings) + `TestGuidanceHandlers_AnswerGuidanceRequest_RejectsInvalidAnswerShape_When_YesNoAnswerIsArbitraryString` + e2e `"rejects an invalid answer submitted directly against the API..."`
  3. Notify-failure must not surface as RPC error → `TestGuidanceService_AnswerGuidanceRequest_ReturnsSuccessResponse_When_NotifierFails`
  4. Session-scoped AND item-scoped orphan reconciliation → `TestReconcileOrphanedGuidanceRequests_ExpiresRequest_When_ItemScopedAndItemIsDoneOrDeleted` + `TestReconcileOrphanedGuidanceRequests_ExpiresRequest_When_SessionScopedAndSessionUUIDNoLongerResolves`
  5. `AwaitingGuidance` outcome skip/fire branch (Task 1.7.3c) → `TestOnAutonomousDriverComplete_SkipsMarkStuckEndSessionAndRespawn_When_AwaitingGuidanceTrue` + its control, `TestOnAutonomousDriverComplete_MarksStuckEndsSessionAndRespawns_When_AwaitingGuidanceFalseOrdinaryGiveUp`
  6. Ordinary autonomous-driver/triage regression (flagged missing from the plan) → `TestAutonomousDriver_Run_ProducesUnchangedOutcome_When_OnlyDoneWaitNextMessageDirectivesUsed` + `TestTriggerTriage_CompletesNormally_When_ResultHasNoBlockingQuestion`
  7. Cross-restart durability → `TestAutonomousDriver_FreshInstance_InjectsAnswer_When_AnsweredByDifferentProcessDuringSimulatedRestart` (driver level) + `TestGuidanceHandlers_WaitForGuidanceAnswer_ReturnsImmediately_When_AlreadyAnswered` (MCP level, "asker missed the event" case)
  8. Dedup/abuse-cap on automated-triage-originated requests → `TestGuidanceService_CreateGuidanceRequest_RejectsResourceExhausted_When_AutomatedOriginExceedsCapOfThree` + `TestGuidanceDedup_SuppressesDuplicateQuestion_When_AutomatedOriginWithinCooldownWindow` (+ human-initiated control siblings)

## Test Counts

Recounted directly from the table rows above (84 total mapped test cases):

- **Go unit**: 47
- **Go integration** (in-memory ent DB, synchronous/fake event bus, or multi-collaborator service wiring): 11
- **Frontend unit (Jest/RTL)**: 19
- **Frontend integration** (reconnect/backoff against a fake stream, or RPC-call wiring): 4
- **E2E (Playwright)**: 3

**Total: 84 test cases** across 21 test files (14 new/extended Go test files, 5 frontend test files touched — 4 new + `stuckReason.test.ts` extended, 1 existing Go file extended (`autonomous_driver_test.go`), 1 new E2E spec).

## Coverage Targets

- Unit test coverage: ≥80% line coverage on all new files (`session/guidance_*.go`, `server/services/guidance_service.go`, `server/services/guidance_dedup.go`, `server/mcp/tools_guidance.go`, `session/backlog_lifecycle_guidance.go`) — verify via `make test-coverage`.
- Every public `GuidanceService`/`Storage` guidance method has both a happy-path and an error-path test (see Epics 1.1–1.2 tables above).
- Every external integration point has at least one integration-level test: the in-memory ent DB (Epic 1.1), the ConnectRPC streaming path (Epic 1.2), the shared `EventBus`/`Notifier` (Epic 1.3), the MCP tool-call surface (Epic 1.4), the triage headless-pool fake (Epic 1.6), and the multi-collaborator `AutonomousOrchestrationService` wiring (Epic 1.7).
- `make ci`/`make ready` (backend) and `cd web-app && npx jest --no-coverage` (frontend) must both pass before Epic 2.7's e2e suite is considered the final gate, per root `CLAUDE.md`'s testing conventions.
