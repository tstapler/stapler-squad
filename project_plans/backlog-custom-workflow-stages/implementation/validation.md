# Validation Plan: backlog-custom-workflow-stages

**Date**: 2026-09-03

## Happy Path Scenario

Given an sdd-mode backlog item sitting in the `idea` stage with an active headless triage
call and no per-mode liveness override configured (today's Baseline), when the operator
creates a `("idea","sdd")` `LivenessDefinition` override via `CreateLivenessDefinition`
(`ExpectedDuration=45m, StalenessMargin=10m`), then `reconcileOrphanedTriageItems`'s next
sweep — and `TriggerTriage`'s own call-budget timeout — both resolve the new 55m
`StalenessThreshold`/45m `ExpectedDuration` instead of the old flat 35m/30m constants, so
the item is no longer marked `STUCK_REASON_ORPHANED_TRIAGE` at a point where it previously
would have been. *(This is Milestone 1's ship-defining scenario — Success Metrics' "12
parked items recover" claim, and the anchor every other test below is a variation of:
Phase 2's gate/custom-stage tests are the same "config change takes effect without a
redeploy" shape, applied to transitions instead of liveness.)*

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.1.1: `LivenessDefinition`/`LivenessKind` tagged union | `session/liveness_definition_test.go` | `TestLivenessDefinition_should_ComputeStalenessThreshold_When_KindIsDurationBudget` | Unit | Happy path — `ExpectedDuration=30m, StalenessMargin=5m` → `StalenessThreshold()==35m` |
| Story 1.1.1: `LivenessDefinition`/`LivenessKind` tagged union | `session/liveness_definition_test.go` | `TestNewLivenessDefinition_should_RejectExpectedDuration_When_KindIsHeartbeat` | Unit | Error path — Shape-B kind constructed with a Shape-A field returns a naming error, not silent accept |
| Story 1.1.2: `stage_liveness_definitions` ent schema | `session/ent_liveness_repository_test.go` | `TestEntLivenessRepository_should_RejectDuplicate_When_StageSlugAndPipelineModePairAlreadyExists` | Integration | ent/DB — `UNIQUE(stage_slug, pipeline_mode)` constraint violation on duplicate `Create` |
| Story 1.2.1: `DefaultLivenessEngine` reproduces hardcoded constants | `session/liveness_engine_test.go` | `TestDefaultLivenessEngine_should_ReturnThirtyFiveMinuteThreshold_When_StageIsIdeaDefaultMode` | Unit | Happy path — `LivenessFor(BacklogStatusIdea, PipelineModeDefault).StalenessThreshold()==35m` |
| Story 1.2.1: `DefaultLivenessEngine` reproduces hardcoded constants | `session/liveness_engine_test.go` | `TestDefaultLivenessEngine_should_ReturnNoTimeoutSentinel_When_StageHasNoTimeoutConcept` | Unit | Error/edge path — `plan_not_approved`/`blocked_by_dependency`-shaped stages return a sentinel, not an error |
| Story 1.2.2 / **Risk Control (Milestone 1, required)**: characterization test — sweep decisions bit-for-bit identical pre/post | `session/liveness_characterization_test.go` (new); regresses `session/backlog_lifecycle_stuck_test.go` (existing, must not change golden behavior) | `TestLivenessCharacterization_should_ProduceIdenticalStuckDecisionAndReasonDetail_When_ComparingHardcodedConstantPathToDefaultLivenessEngine` | Integration | Fixture corpus (1 item per liveness shape A/B/C) — decision + `reasonDetail` string equality between the pre-migration hardcoded-constant path and the `DefaultLivenessEngine`-backed path. **Must not regress**: `TestBackfillStuckStates_should_seedDBDerivableRowsWithNotifiedAt_When_ItemsParked`, `TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved`, `TestReconcileStuckReviewItems_should_markAbandoned_When_OnlyActiveSessionIsDeadZombie` (all in `session/backlog_lifecycle_stuck_test.go`), and `server/services/backlog_service_triage_test.go`'s existing BUG-055-invariant test. |
| Story 1.3.1: `LivenessRepository` + `livenessCache` | `session/liveness_cache_test.go` | `TestLivenessCache_should_ReturnLockFreeMiss_When_NoRowMatchesStageAndMode` | Unit | Happy path — `Get("idea","sdd")` on an empty cache returns a miss without touching `writeMu` |
| Story 1.3.1: `LivenessRepository` + `livenessCache` | `session/liveness_cache_test.go` | `TestCachingLivenessEngine_should_FallBackToModeLessRowWithoutWarnLog_When_StageModeRowAbsentButStageOnlyRowExists` | Unit | Error/fallback path — `("idea","sdd")` absent, `("idea", nil)` present with `ExpectedDuration=40m`: returns 40m, logs no Warn (mode-less fallback isn't a failure) |
| Story 1.3.1: `LivenessRepository` + `livenessCache` | `session/liveness_cache_test.go` | `TestCachingLivenessEngine_should_FallBackToDefaultEngineWithWarnLog_When_NeitherStageModeNorStageOnlyRowExists` | Integration | ent/DB-backed cache load + fallback-to-`DefaultLivenessEngine` + exactly-one-Warn-line assertion |
| Story 1.3.2: `LivenessDefinition` CRUD RPCs | `server/services/backlog_service_liveness_test.go` | `TestCreateLivenessDefinition_should_ReturnLivenessDefinition_When_StageSlugAndPipelineModePairIsNew` | Unit | Happy path — successful create returns the persisted row |
| Story 1.3.2: `LivenessDefinition` CRUD RPCs | `server/services/backlog_service_liveness_test.go` | `TestCreateLivenessDefinition_should_ReturnAlreadyExists_When_StageSlugAndPipelineModePairHasEnabledRow` | Unit | Error path — duplicate enabled `(stage_slug, pipeline_mode)` → `connect.CodeAlreadyExists` |
| Story 1.3.2: cache invalidation on update | `server/services/backlog_service_liveness_test.go` | `TestUpdateLivenessDefinition_should_InvalidateCache_When_UpdateSucceeds` | Integration | RPC handler + `livenessCache` + repo — next `LivenessFor` call reflects the new value with no server restart |
| Story 1.4.1: `reconcileOrphanedTriageItems` consults `LivenessEngine` | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileOrphanedTriageItems_should_NotMarkStuck_When_NoOverrideConfiguredAndSessionAgeUnder35Min` | Unit | Happy path — zero-regression: unchanged 35m behavior with no override row |
| Story 1.4.1: `reconcileOrphanedTriageItems` consults `LivenessEngine` | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileOrphanedTriageItems_should_NotMarkStuck_When_SddModeOverrideRaisesThresholdPast40Min` | Integration | DB-backed override row — the concrete Milestone 1 fix: 40m-old sdd-mode session, past the old 35m constant, under the new 55m derived threshold, is not marked stuck |
| Story 1.4.2: `TriggerTriage` call budget consults `LivenessEngine` | `server/services/backlog_service_triage_test.go` | `TestTriggerTriage_should_UseFlatThirtyMinuteConstant_When_LivenessEngineIsNil` | Unit | Error/fallback path — construction-failed engine falls back byte-for-byte to the `triageCallBudget` constant |
| Story 1.4.2: `TriggerTriage` call budget consults `LivenessEngine` | `server/services/backlog_service_triage_test.go` | `TestTriggerTriage_should_UseResolvedFortyFiveMinuteTimeout_When_SddModeOverrideConfigured` | Integration | DB-backed override — the headless call's actual `context.WithTimeout` reflects the resolved `ExpectedDuration`, not the flat constant |
| Story 1.4.2: BUG-055 invariant ported to the derived-threshold relationship | `session/liveness_definition_test.go` | `TestLivenessDefinition_should_HaveStalenessThresholdGreaterThanExpectedDuration_When_KindIsDurationBudget` | Unit | Structural/property test — `StalenessThreshold() > ExpectedDuration` holds for every configured Shape-A row, not just the two old hardcoded constants |
| Story 1.4.3: `reconcileStaleWorkSessions` consults `LivenessEngine` (Shape B) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStaleWorkSessions_should_NotMarkStuck_When_NoOverrideConfiguredAndProgressAgeUnder2Hours` | Unit | Happy path — zero-regression: unchanged 2h behavior with no override |
| Story 1.4.3: `reconcileStaleWorkSessions` consults `LivenessEngine` (Shape B) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStaleWorkSessions_should_NotMarkStuckAndInterpolateResolvedDuration_When_SddModeOverrideRaisesThresholdPast2h1m` | Integration | DB-backed override — 2h1m-old sdd-mode session not marked stale under a 3h override; notify body interpolates the resolved `3h` |
| Story 1.4.3: `reconcileBouncingItems` consults `LivenessEngine` (Shape C) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileBouncingItems_should_MarkStuck_When_NoOverrideConfiguredAndThreeCyclesInDefaultLookback` | Unit | Happy path — zero-regression: unchanged `bounceThreshold=3`/`bounceLookback=24h` behavior with no override |
| Story 1.4.3: `reconcileBouncingItems` consults `LivenessEngine` (Shape C) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileBouncingItems_should_NotMarkStuckForSddModeItemButStillMarkDefaultModeSibling_When_PerItemOverrideRaisesCycleThreshold` | Integration | DB-backed per-item override — proves per-mode resolution inside a single sweep tick, not a package-level override (two items, same 4-cycles-in-30h shape, different pipeline modes) |
| Epic 1.5 (Story 1.5.1): parked-row recovery via existing `RemediationDue` | `session/backlog_remediation_test.go` | `TestRemediationDue_should_ReturnTrue_When_ParkedOrphanedTriageRowGetsLivenessOverrideWithZeroCodeChange` | Integration | DB-backed — a parked `BacklogStuckState{remediation_attempts:5}` row becomes remediation-due once the sdd-mode override is created, with `session/backlog_remediation.go` untouched |
| Story 1.6.1: `[LivenessEngine]` log lines | `session/liveness_engine_test.go` | `TestLivenessFor_should_EmitExactlyOneWarnLine_When_FallingBackToDefaultEngine` | Unit | Happy path (observability) — a resolvable-via-fallback stage/mode logs exactly one `[LivenessEngine]` Warn line |
| Story 1.6.1: `[LivenessEngine]` log lines | `session/liveness_engine_test.go` | `TestLivenessFor_should_NotEmitDuplicateWarnLines_When_CalledRepeatedlyForSameUnresolvedStageAndMode` | Unit | Error path — Warn fires once per call, not once per internal retry |
| Story 2.1.1: `OverrideVerdict` re-routed through injected engine | `server/services/backlog_service_lifecycle_test.go` | `TestOverrideVerdict_should_RefuseTransition_When_ConfiguredWorkflowEngineHasDisabledTheEdge` | Unit | Happy path — engine-backed refusal even though the static map would allow it |
| Story 2.1.1: `OverrideVerdict` re-routed through injected engine | `server/services/backlog_service_lifecycle_test.go` | `TestOverrideVerdict_should_AllowTransition_When_StaticMapWouldRefuseButEngineAllows` | Unit | Error/inverse path — confirms the call site reads the engine, not `domain.CanTransitionBacklog`, in either direction |
| Story 2.1.2: `AttachSessionToItem` re-routed through injected engine | `server/services/backlog_service_sync_test.go` | `TestAttachSessionToItem_should_NotCallTransitionBacklogItemStatus_When_ConfiguredWorkflowEngineHasDisabledTheEdge` | Unit | Mirrors 2.1.1 for the `ready -> in_progress` call site |
| Story 2.2.1: 4 new ent schemas, uniqueness constraints | `session/ent_stage_config_repository_test.go` | `TestStageTransitionRepository_should_RejectDuplicate_When_FromStageAndToStagePairAlreadyExists` | Integration | ent/DB — `UNIQUE(from_stage_id, to_stage_id)` |
| Story 2.2.1: `gate_satisfaction_records` uniqueness | `session/gate_satisfaction_repository_test.go` | `TestGateSatisfactionRepository_should_RejectDuplicate_When_ItemAndGatePairAlreadyExists` | Integration | ent/DB — `UNIQUE(item_id, gate_id)` |
| Story 2.2.2: seed migration for the built-in 9-stage graph | `session/configured_workflow_engine_test.go` | `TestConfiguredWorkflowEngine_should_MatchDomainValidTransitions_When_DatabaseIsFreshlySeeded` | Integration | Seeded DB — `AllowedTransitions("idea")` matches `DefaultWorkflowEngine.AllowedTransitions("idea")` exactly, for every stage |
| Story 2.2.2: seed migration error path | `session/configured_workflow_engine_test.go` | `TestConfiguredWorkflowEngine_should_ReturnEmptyGraph_When_SeedMigrationHasNotRun` | Unit | Error/edge path — unseeded table produces a defined empty result, not a panic |
| Story 2.3.1: `CanTransition`/`AllowedTransitions` against DB-loaded graph | `session/configured_workflow_engine_test.go` | `TestConfiguredWorkflowEngine_should_MatchDefaultWorkflowEngineByteForByte_When_NoCustomStagesAdded` | Integration | **Risk Control regression gate** — every `domain.ValidTransitions()` `(from,to)` pair agrees between `ConfiguredWorkflowEngine` and `DefaultWorkflowEngine` |
| Story 2.3.1: newly-added custom transition legality | `session/configured_workflow_engine_test.go` | `TestConfiguredWorkflowEngine_should_AllowNewCustomTransitionImmediately_When_CreateStageTransitionRPCJustSucceeded` | Integration | DB write + cache invalidation — `idea -> "design-review"` legal immediately after creation, no redeploy |
| Story 2.3.2: `PendingGates`/`ValidateGates` thin wrapper | `session/configured_workflow_engine_test.go` | `TestValidateGates_should_ReturnError_When_PendingGatesReportsAnyUnsatisfiedEntry` | Unit | Happy path — `ValidateGates` derives directly from `PendingGates`'s unsatisfied count |
| Story 2.3.2: structural gate freshness | `session/configured_workflow_engine_test.go` | `TestPendingGates_should_ReportUnsatisfied_When_PreviouslySatisfiedStructuralGateHasSinceRegressed` | Unit | Error/edge path — no stale cached "satisfied" result reused across calls |
| Story 2.4.1: human-approval gate | `session/gate_approval_test.go` | `TestRecordGateApproval_should_PersistAndNotReAsk_When_ApprovalIsRecorded` | Unit | Happy path — recorded, one-shot, survives unrelated item field changes |
| Story 2.4.1: human-approval gate RPC | `server/services/backlog_service_gates_test.go` | `TestRecordGateApproval_should_RejectDuplicate_When_ItemAndGatePairAlreadySatisfied` | Integration | DB `UNIQUE(item_id, gate_id)` enforced at the RPC layer (defense in depth per ux.md) |
| Story 2.4.2: structural/mechanical check gate | `session/gate_structural_test.go` | `TestStructuralGate_should_ReportSatisfied_When_AcCompleteAllCriteriaChecked` | Unit | Happy path — `"ac_complete"` fresh evaluation, all 3/3 criteria done |
| Story 2.4.2: structural/mechanical check gate | `session/gate_structural_test.go` | `TestStructuralGate_should_ReportUnsatisfiedWithCountDescription_When_AcCompleteHasIncompleteCriteria` | Unit | Error path — `"1 of 3 acceptance criteria incomplete"` description, no session spawn |
| Story 2.4.3: automated-review gate generalizes `ReviewGateRunner` | `session/review_gate_test.go` | `TestReviewGateRunner_should_SkipDiffWorktreeBranchDriftChecks_When_GateContextRequiresDiffIsFalse` | Unit | Happy path — a no-diff custom transition (`idea -> ready`) proceeds straight to the review-prompt call |
| Story 2.4.3: existing behavior unchanged | `session/backlog_lifecycle_review_test.go` | `TestReviewGateSpawn_should_FireForReviewToPrPending_When_AutomatedReviewGateAttachedMatchingTodaysBehavior` | Integration | Regression — the generalized "has an automated-review gate attached" condition still fires identically to the old `toStatus == BacklogStatusReview` literal for the built-in transition |
| Story 2.4.4: custom/pluggable check gate, allowlist | `session/gate_custom_check_test.go` | `TestInvokeCustomGateCheck_should_BlockTransitionFailClosed_When_SkillNotInPreRegisteredAllowlist` | Unit | Error path — unregistered skill name rejected, transition blocked (never silently passes) |
| Story 2.4.4: custom check bounded by `LivenessEngine`, reuses the existing sweep | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileCustomGateChecks_should_MarkStuckViaLivenessEngine_When_CustomCheckExceedsExpectedDurationPlusMargin` | Integration | DB-backed — a 16m-old custom-check invocation bounded by a 10m/5m `LivenessDefinition` is caught by the same `reconcile*` sweep code path Epic 1.4 built, not a new detector |
| Story 2.5.1: `StageConfigSnapshot` at transition-write time | `session/backlog_status_event_test.go` | `TestBacklogStatusEvent_should_RenderOriginalStageName_When_ReferencedCustomStageIsLaterDeleted` | Integration | DB-backed — history renders "Design Review" after the stage row is deleted |
| Story 2.5.2: frozen-snapshot fallback for in-flight item on deleted stage | `session/configured_workflow_engine_test.go` | `TestAllowedTransitions_should_ReturnSnapshottedTransitionsWithWarnLog_When_ItemsCurrentStageWasSinceDeleted` | Unit | Error/edge path — defined answer instead of empty slice or panic |
| Story 2.6.1: DFS reachability + trap validator, happy path | `session/graph_validator_test.go` | `TestGraphValidator_should_AcceptValidGraph_When_EveryStageIsReachableAndEveryNonTerminalHasAnOutgoingEdge` | Unit | Happy path |
| Story 2.6.1: DFS reachability + trap validator, error path | `session/graph_validator_test.go` | `TestGraphValidator_should_RejectDeadEndStage_When_NonTerminalStageHasZeroOutgoingTransitions` | Unit | Error path |
| Story 2.6.1: adversarial coverage (required per plan Task 2.6.1d) | `session/graph_validator_test.go` | `TestGraphValidator_should_RejectUnreachableStage_When_SelfLoopMultiCycleAndDisconnectedComponentFixturesAreEachSubmitted` | Unit | Adversarial table test — self-loop, multi-node cycle, disconnected component |
| Story 2.6.2: cycle-with-no-escape lint warning | `session/graph_validator_test.go` | `TestGraphValidator_should_ReturnNonBlockingWarning_When_ThreeStageCycleHasNoGateOnAnyEdge` | Unit | Happy path (soft-warning, non-blocking) |
| Story 2.6.2: gated cycle produces no warning | `session/graph_validator_test.go` | `TestGraphValidator_should_ReturnNoWarning_When_CycleHasAtLeastOneGatedEdge` | Unit | Error/negative path |
| Story 2.7.1: Stage CRUD RPCs, delete guard | `server/services/backlog_service_stages_test.go` | `TestDeleteStage_should_ReturnFailedPrecondition_When_StageHasLiveItems` | Unit | Error path — item-count-named rejection |
| Story 2.7.1: Stage CRUD RPCs, happy path + cache invalidation | `server/services/backlog_service_stages_test.go` | `TestUpdateStage_should_InvalidateStageConfigCache_When_UpdateSucceeds` | Integration | DB + cache — mirrors Story 1.3.2's cache-invalidation AC for stages |
| Story 2.7.2: transition + gate CRUD RPCs, validator invoked | `server/services/backlog_service_transitions_test.go` | `TestCreateStageTransition_should_InvokeGraphValidatorBeforeCommitting_When_RequestWouldCreateUnreachableStage` | Integration | DB — validation error, nothing persisted (verified via a follow-up `ListStages`) |
| Story 2.7.2: transition + gate CRUD RPCs, happy path | `server/services/backlog_service_transitions_test.go` | `TestCreateStageTransition_should_PersistAndReturnTransition_When_GraphRemainsValid` | Unit | Happy path |

## Migration Tests

| Migration | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Phase 1: `stage_liveness_definitions` (additive-only, new table) | `session/ent_liveness_repository_test.go` | `migration_should_be_reversible` (rendered as `TestStageLivenessDefinitionMigration_should_BeReversible_When_TableIsDroppedAfterRowsExist`) | Migration | Given rows exist in `stage_liveness_definitions`, when the table is dropped (rollback procedure per plan.md's Migration Plan), then `CachingLivenessEngine.LivenessFor` falls back to `DefaultLivenessEngine`'s identical pre-migration constants with zero code change to the four call sites required — proving the rollback path plan.md specifies actually reproduces today's behavior, not just that the DDL is drop-able. |
| Phase 2: `backlog_stages`/`stage_transitions`/`transition_gates`/`gate_satisfaction_records` (additive-only + seed) | `session/configured_workflow_engine_test.go` | `migration_should_be_reversible` (rendered as `TestBacklogStageSchemaMigration_should_BeReversible_When_EngineConstructionRevertsToDefaultWorkflowEngine`) | Migration | Given the four Phase 2 tables are seeded and `ConfiguredWorkflowEngine` is wired into `server/dependencies.go`, when the rollback procedure runs (revert engine construction to `NewDefaultWorkflowEngine()`, tables left in place unused), then transition-legality behavior for every existing item is byte-for-byte identical to `DefaultWorkflowEngine`'s pre-Phase-2 output — no data loss, no behavior change, confirming the plan's "no code change to any existing table" reversibility claim is not just additive-DDL trivia but an actually-safe operational rollback. |

Both migrations are additive-only (new tables, no altered columns on existing tables), so
"reversible" here means *behaviorally* reversible — dropping the table or reverting the
one-line engine-construction change reproduces pre-migration behavior exactly — not a
down-migration script, matching plan.md's own stated Rollback Procedure for both phases.

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Task efficiency #1: create a stage with zero transitions in ≤4 interactions | `tests/e2e/backlog-stages-crud.spec.ts` | `operator can create a minimal custom stage in four interactions` | Playwright | Navigate to `/settings/backlog-stages` → click New Stage → fill slug → fill name → Save; assert the new stage row appears in the list with no page reload, and count exactly 4 form interactions before the Save click resolves. |
| Task efficiency #2: add one gated transition in ≤6 interactions | `tests/e2e/backlog-stages-crud.spec.ts` | `operator can add a gated transition to an existing stage in six interactions` | Playwright | Edit an existing stage → Add transition → select target stage → check "Human approval" → (no sub-field required for this gate kind) → Save; assert `role="alert"`/success and the transition appears in the read-only graph's `sr-only` table. |
| Task efficiency #3: identify which gate blocks a transition in ≤1 click from the board | `tests/e2e/backlog-item-gate-checklist.spec.ts` | `operator identifies the blocking gate for an item in one click from the board` | Playwright | From `/backlog` board, click a card whose item has a pending human-approval gate → assert the item-detail page's gate checklist is already rendered (no extra expand/tab click) and the human-approval row is visible without further navigation. |
| Error handling #4/#5: every error names the offending entity and offers a next action (no dead ends) | `tests/e2e/backlog-stages-crud.spec.ts` | `stage-load failure shows a named retry banner with no dead end` | Playwright | Force `listStages()` to fail (route intercept) → assert `role="alert"` banner reads "Couldn't load stages — <message>" and a `[Retry]` button re-invokes the RPC successfully once the intercept is cleared. |
| Error handling #4/#5: graph-validator rejection is fixable in place | `tests/e2e/backlog-stages-crud.spec.ts` | `unreachable-stage rejection preserves form state and names the stage` | Playwright | Submit a stage-form save that the Epic 2.6 validator would reject (a non-terminal stage with no outgoing transition) → assert inline `role="alert"` names the specific stage slug, form fields retain entered values, and adding a transition + re-saving succeeds without navigating away. |
| Error handling #6: soft-warning cycle commits but is acknowledged, not silently absorbed | `tests/e2e/backlog-stages-crud.spec.ts` | `gate-free cycle commits with an acknowledged warning banner` | Playwright | Configure a 3-stage cycle with no gates on any edge → Save → assert an amber `role="status"` warning banner names the cycle, the save succeeds (row appears in the list), and clicking `[Got it]` dismisses the banner and closes the overlay. |
| Accessibility #7/#8: every control keyboard-operable, no drag, labeled | `tests/e2e/backlog-stages-crud.spec.ts` | `stage form is fully keyboard-operable with no drag interaction` | Playwright | Tab through the New Stage form and the Outgoing-transitions sub-list using only keyboard (`Tab`/`Space`/`Enter`) — add a transition, check a gate checkbox, fill its progressive-disclosure sub-field, and Save — all via keyboard; assert every input has a resolvable accessible name (`getByLabel`/`getByRole` succeed, no placeholder-only labeling). |
| Accessibility #10: graph diagram has row-for-row `sr-only` text equivalent | `tests/e2e/backlog-stages-crud.spec.ts` | `graph preview sr-only table matches every rendered edge` | Playwright | Configure ≥3 transitions with varying gate counts → assert the `sr-only` table's row count equals the SVG's edge count and each row's "Gates" cell matches that edge's `🔒N` badge value. |
| Accessibility #11: gate-checklist `aria-live` fires once per action, at list level not per row | `tests/e2e/backlog-item-gate-checklist.spec.ts` | `approving a gate announces once at the list level, not per row` | Playwright | Click Approve on a human-approval gate row → assert exactly one `aria-live="polite"` announcement fires on the `<ul>` container (not one per `<li>`), reading a summary like "Human approval satisfied. 1 gate remaining." |
| Surface 5 AC: custom stage appears as its own board column within one cache-refresh cycle | `tests/e2e/backlog-board-dynamic-stages.spec.ts` | `new custom stage appears as its own board column without a page reload` | Playwright | Create a custom stage via the settings UI → assign an item to it (via existing RPC/fixture) → on the already-open `/backlog` board, wait for the cache-refresh interval → assert a new column matching the stage's `Name` renders with that item's card, no `page.reload()` called. |
| Surface 5 AC (hard regression gate): unrecognized-stage item is never dropped | `tests/e2e/backlog-board-dynamic-stages.spec.ts` | `item on a deleted or unrecognized stage always renders in the overflow column` | Playwright | Seed an item with `status="some-deleted-stage"` matching no fetched stage → assert it renders in the "Unrecognized stage" column in 100% of 3 repeated page loads (not a "usually visible" flake — this regresses BUG-037). |
| No dead ends — every error path offers an exit | manual | Full click-through of every error row in `implementation/design/ux.md` Surfaces 1/2/4's Error tables | Manual | Trigger each documented error case (load failure, toggle-off rejection, checked-gate-missing-subfield, save timeout, config-error gate row) in a running dev instance and confirm each has a Retry/Cancel/Never-mind/Fix-in-Stages-settings affordance — no state that requires a page reload to escape. |
| Keyboard navigable end-to-end (cross-check for automated a11y test gaps) | manual | Tab-order walkthrough of Surfaces 1, 2, 4, 5 | Manual | With a mouse physically unplugged, complete "create a custom stage with a human-approval-gated transition, then approve that gate on a live item" using only keyboard — confirms the automated Playwright keyboard tests above didn't miss a focus-trap or unreachable control the assertions happened not to probe. |

## Test Stack

- **Unit**: Go `testing` + `testify` (`assert`/`require`), table-driven per this repo's
  `golang-testing` convention; TypeScript/React via Jest + React Testing Library
  (`@testing-library/react`), matching existing `*.test.tsx` files under
  `web-app/src/components/backlog/`.
- **Integration**: Go tests against a real (test-mode) ent/SQLite-or-Postgres-backed
  repository — no mocked DB layer, per this repo's existing `*_test.go` convention for
  `session/ent_*_repository_test.go`-shaped files; RPC handler tests use the existing
  ConnectRPC in-process test-server harness (see `server/services/backlog_service_triage_test.go`
  for the established pattern).
- **E2E / UX**: Playwright, per `tests/e2e/`'s existing conventions (`@feature` annotation
  header, `data-testid`/ARIA-role locators only, no `waitForTimeout`, isolated per-run
  server via `global-setup.ts`) — implementation should follow the `ui-playwright` skill
  and this repo's `e2e-test-conventions` skill. Manual checklist items are for scenarios
  Playwright cannot economically assert (full unplugged-mouse keyboard walkthroughs,
  cross-error-table completeness sweeps) rather than a substitute for automation.

## Test Stack Notes — Go convention confirmed against this repo

`session/backlog_lifecycle_stuck_test.go` already uses the `TestFoo_should_X_When_Y`
naming convention (e.g. `TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved`)
and testify assertions — every new test name above matches that exact shape. Web-app test
files sit alongside their component (`BacklogBoard.test.tsx` pattern, confirmed against
`web-app/src/components/backlog/*.test.tsx`) and must use `pnpm`, never `npm`/`yarn`, per
`docs/how-to/use-pnpm-in-web-app.md`.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of `LivenessEngine`/`ConfiguredWorkflowEngine`/`GraphValidator` branches covered given their fail-closed/fail-open asymmetry is safety-critical |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}' --testPathPatterns="backlog-stages|GateChecklist|BacklogBoard|StageTracker"` | ≥80% line |
| Milestone 1 ship gate (plan.md Story 1.6.2, required before merge) | `go test ./session/... ./server/services/... -run 'Liveness\|OrphanedTriage\|BUG055\|BUG083'` then `make quick-check` | Zero failures; `make quick-check` exits 0 on the Milestone 1 branch with no Phase 2 code present |
| E2E | `cd tests/e2e && npx playwright test backlog-stages-crud.spec.ts backlog-item-gate-checklist.spec.ts backlog-board-dynamic-stages.spec.ts` | All UX acceptance criteria above pass; `make e2e-lighthouse` warns (not blocks) below 70 |

- All public service methods (`LivenessEngine`, `LivenessRepository`, `WorkflowEngine`/
  `ConfiguredWorkflowEngine`, `GraphValidator`, gate evaluators): happy path + error paths
  covered per the mapping table above.
- All external integrations (ent/DB reads and writes, the review-gate headless-call
  spawn, the custom-check skill invocation): unit-mocked where the plan calls for a fast
  unit test, plus at least one integration test against a real test-mode DB for every
  repository/cache pair.
- Every UX acceptance criterion in `design/ux.md`'s "UX acceptance criteria (cross-surface)"
  section (11 numbered criteria) has a corresponding automated or manual test above —
  criteria 1–3 (task efficiency), 4–6 (error handling), 7–11 (accessibility) are each
  covered by name in the UX Acceptance Tests table.
