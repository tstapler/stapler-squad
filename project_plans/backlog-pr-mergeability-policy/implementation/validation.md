# Validation Plan: backlog-pr-mergeability-policy

**Date**: 2026-07-17
**Author**: SDD Validation (Phase 4) — test design BEFORE code

Maps every requirement, behavior, and control in `requirements.md` + `plan.md` to concrete,
named test rows, grounded in the repo's real conventions (`TestX_should_Y_When_Z` /
`TestMethod_Scenario`, testify `require`/`assert`, table-driven; `TestBacklogIntegration_ITNNN_*`
for integration; `describe(...)/it(...)` Jest; `@feature`-annotated Playwright with `data-testid`
locators). The 8 mandatory regression guards from research + the adversarial review are called out
explicitly and mapped in the **Regression Guards** section.

Requirement IDs used below:
- **B1** — Behavior 1: auto-create PR on Complete (Phase 3, Epics 3.1/3.2)
- **B2** — Behavior 2: auto-fix loop on CI-fail / conflict (Phase 4)
- **B3** — Behavior 3: single "ready to merge" notify on genuine mergeability (Phase 5)
- **FLAG** — per-item `AutoMergePolicy` flag, 8-layer wiring + proto3-reset guard (Phase 1)
- **P0** — Phase-0 orphan-PR adoption / `review`|`BOUNCING`↔`pr_pending` desync fix (Phase 0, ADR-025)
- **KILL** — global kill-switch `backlog:auto-merge-policy` + `policyActive` predicate (Phase 2)
- **CITRI** — CI tri-state (failing/pending/passing), Blocker-1 correctness premise (Epic 3.0)

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| **FLAG**: persists & round-trips ent→proto→domain→repo | `session/backlog_integration_test.go` | `TestBacklogIntegration_IT012_AutoMergePolicyRoundTrips` | Integration | Happy path — Create with `AutoMergePolicy:true` reads back true through ent storage |
| **FLAG**: Create RPC copies flag | `server/services/backlog_service_test.go` | `TestCreateBacklogItem_should_PersistAutoMergePolicy_When_RequestSetsFlag` | Unit | Happy path — Create handler maps `req.Msg.AutoMergePolicy` into domain |
| **FLAG**: `backlogItemToProto` emits flag | `server/services/backlog_service_test.go` | `TestBacklogItemToProto_should_IncludeAutoMergePolicy_When_ItemFlagTrue` | Unit | Happy path — domain→proto emits the field |
| **FLAG**: Update handler unconditional-wrap (plain-bool, NOT presence-gated) | `server/services/backlog_service_test.go` | `TestUpdateBacklogItem_should_WrapAutoMergePolicyUnconditionally_When_Updating` | Unit | Edge — partial Update always sets the pointer (mirrors AutoSpawn, not PipelineMode) |
| **FLAG (REGRESSION #1)**: Go-side proto3 no-reset — partial Update leaving pointer nil preserves stored value | `server/services/backlog_service_test.go` | `TestUpdateBacklogItem_should_NotResetAutoMergePolicy_When_PartialUpdateOmitsPointer` | Unit | Error/edge — pointer-nil path leaves value intact |
| **FLAG (REGRESSION #1)**: frontend `currentFlags()` preserves flag on unrelated save | `web-app/src/components/backlog/BacklogItemDetail.regression.test.tsx` | `describe("BacklogItemDetail — proto3 flag preservation") › it("preserves autoMergePolicy=true through an unrelated notes save via currentFlags()")` | Jest/RTL | Error/edge — notes save payload includes `autoMergePolicy:true` (b28ace2f mirror) |
| **FLAG**: checkbox rendered, in submit payload + dep array | `web-app/src/components/backlog/BacklogItemForm.test.tsx` | `describe("BacklogItemForm — auto-merge policy toggle") › it("emits autoMergePolicy in the onSubmit payload and shows trust-boundary hint")` | Jest/RTL | Happy path — checkbox → payload, hint text present |
| **FLAG**: toggle durable end-to-end (UI proto3-reset trap) | `tests/e2e/backlog-auto-merge-policy.spec.ts` | `describe("backlog-auto-merge-policy") › test("toggle survives save + reload")` | e2e | Happy path — set checkbox, save, reload, still checked |
| **P0 (REGRESSION #3)**: Class-B orphan (`PrNumber>0`, status `review`) adopted → `pr_pending` w/ audit row | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_AdoptClassBReviewOrphan_When_PrNumberSetButStatusReview` | Unit | Error/edge (BLOCKER-2 guard) — asserts `BacklogStatusEvent` row written |
| **P0 (REGRESSION #3)**: Class-B `in_progress` orphan routed two-step `in_progress→review→pr_pending` | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_RouteInProgressClassBOrphanTwoStep_When_PrNumberSet` | Unit | Error/edge — asserts intermediate `review` hop leaves its own audit row; `in_progress→pr_pending` never used |
| **P0**: Class-A orphan (Instance PR URL only, `PrNumber==0`) `review` adopted + PR fields stamped | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_AdoptClassAReviewOrphan_When_InstanceCarriesPRURL` | Unit | Happy path — fake `instancePRLookup` supplies URL, PR number parsed & written |
| **P0**: Class-A `in_progress` orphan routed two-step | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_RouteInProgressClassAOrphanTwoStep_When_InstancePRURL` | Unit | Edge — two guarded transitions |
| **P0**: already-`pr_pending` item untouched; item with no PR anywhere untouched | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_LeaveItemUntouched_When_AlreadyPrPendingOrNoPRAnywhere` | Unit | Negative — no transition, no spawn |
| **P0**: adoption of a non-policy item does NOT spawn a fix | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_NotSpawnFix_When_AdoptedItemIsNonPolicy` | Unit | Edge — adoption is universal, fix is policy-gated |
| **P0**: detector panic-isolated; one bad item does not abort sweep | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_ContinueSweep_When_OneItemLookupPanicsOrErrors` | Unit | Error path — panic/err isolation |
| **P0**: non-hydrating Instance lookup (`FindInstanceDataByID`, no `Acquire`/`onConstruct`) | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedPRs_should_UseNonHydratingInstanceLookup_When_ResolvingClassAPR` | Integration | Edge — CONCERN guard: dead session not hydrated per sweep |
| **P0**: adopted item reconciled in same sweep (detector ordered before pr_ready step) | `session/backlog_integration_test.go` | `TestBacklogIntegration_IT013_OrphanAdoptedThenReconciledSameSweep` | Integration | Happy path — end-to-end adopt→poll in one `ReconcileStuck` |
| **KILL**: flag registered in `knownFeatureFlags`, default false | `server/services/feature_flag_service_test.go` | `TestKnownFeatureFlags_should_IncludeAutoMergePolicy_DefaultOff` | Unit | Happy path — appears in UI list, defaults off |
| **KILL**: `policyActive = globalOn AND item.AutoMergePolicy` truth table | `session/backlog_lifecycle_test.go` | `TestPolicyActive_should_ReturnTrue_OnlyWhen_GlobalOnAndItemFlagTrue` | Unit | Happy + negative — 4-row table (on/off × flag/no-flag) |
| **KILL (REGRESSION #7)**: global OFF halts arm + fix even for policy-enabled item | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_NotArmOrSpawnFix_When_GlobalKillSwitchOff_EvenForPolicyItem` | Integration | Error/edge — kill-switch overrides per-item opt-in |
| **KILL**: runtime-live (atomic `FeatureController`), takes effect without restart | `session/backlog_lifecycle_test.go` | `TestGlobalPolicyEnabled_should_ReflectRuntimeToggle_When_ControllerFlipped` | Unit | Edge — CONCERN guard: no stale `cfg` snapshot, atomic load observes toggle |
| **KILL**: nil-safe accessor defaults false when unwired | `session/backlog_lifecycle_test.go` | `TestIsGlobalPolicyEnabled_should_ReturnFalse_When_AccessorNil` | Unit | Error path — fail-safe default |
| **CITRI (REGRESSION #2)**: PENDING checks → `CIPending true`, not green | `session/git/worktree_git_test.go` | `TestGetPRStatus_should_SetCIPending_When_ChecksNotAllConcluded` | Unit | Edge (BLOCKER-1 guard) — non-terminal check ⇒ pending |
| **CITRI**: terminal failure still sets `CIFailing`, `CIPending` false | `session/git/worktree_git_test.go` | `TestGetPRStatus_should_SetCIFailingNotPending_When_TerminalFailure` | Unit | Happy path — failure classification unchanged |
| **CITRI**: all checks concluded & passing ⇒ neither bool set (green) | `session/git/worktree_git_test.go` | `TestGetPRStatus_should_LeaveBothFalse_When_AllChecksConcludedPassing` | Unit | Happy path — positive-passing |
| **CITRI**: empty rollup (no CI) leaves both false (accepted residual) | `session/git/worktree_git_test.go` | `TestGetPRStatus_should_LeaveBothFalse_When_RollupEmpty` | Unit | Edge — documents no-checks-yet residual |
| **CITRI (REGRESSION #2)**: tri-state maps pending→`CheckConclusion="pending"`; `prReadyToMergeSolo` returns false | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_MapPendingCI_And_BlockReadyAndArm_When_CIPending` | Integration | Edge (BLOCKER-1 guard) — pending blocks BOTH notify and arm |
| **B1**: `pushAndCreatePR` no longer arms auto-merge (arm relocated) | `session/backlog_lifecycle_test.go` | `TestPushAndCreatePR_should_NotEnableAutoMerge_When_CreatingPR` | Unit | Happy path — arm removed from PR-create |
| **B1 (REGRESSION #2)**: arm fires only when `policyActive AND ciPassing AND prReadyToMergeSolo` | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_ArmAutoMerge_OnlyWhen_PolicyActiveAndCIPassingAndReady` | Integration | Happy path — positive arm gate |
| **B1**: non-policy item NOT armed, still gets merged→done detection + notify | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_NotArm_ButStillDetectMergeAndNotify_When_NonPolicyItem` | Integration | Edge — detection universal, arm gated |
| **B1**: arm idempotent on repeated healthy ticks | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_ReArmIdempotently_When_HealthyTickRepeats` | Unit | Edge — `gh pr merge --auto` no-op when already enabled |
| **B1**: `EnablePRAutoMerge` on `prPendingChecker` interface; test fake stub satisfies it | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_CallCheckerEnablePRAutoMerge_When_ArmingViaReconciler` | Unit | Happy path — interface method wired |
| **B1**: arm-failure preserves existing WARNING notify (no error swallow) | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_NotifyWarning_When_EnablePRAutoMergeFails` | Unit | Error path — failure surfaced |
| **B1 / E7**: policy + `SkipReviewGate` item auto-creates PR instead of →done | `session/backlog_lifecycle_test.go` | `TestOnSessionExited_should_RoutePolicySkipReviewItemToPushAndCreatePR_When_WorkExits` | Unit | Happy path — E7 routing, `in_progress→review` then `pushAndCreatePR` |
| **B1 / E7**: non-policy `SkipReviewGate` item unchanged (→done) | `session/backlog_lifecycle_test.go` | `TestOnSessionExited_should_TransitionToDone_When_NonPolicySkipReviewItemExits` | Unit | Negative — default path unchanged |
| **B1 / E7**: policy item WITHOUT `SkipReviewGate` unchanged (→review gate) | `session/backlog_lifecycle_test.go` | `TestOnSessionExited_should_RouteToReviewGate_When_PolicyItemWithoutSkipReviewExits` | Unit | Edge — precedence policy>SkipReviewGate only when both true |
| **B1 / E7 (REGRESSION #8b)**: `pushAndCreatePR` entry idempotency guard prevents sequential-dup PR | `session/backlog_lifecycle_test.go` | `TestPushAndCreatePR_should_ShortCircuit_When_ItemAlreadyHasPRorIsPrPendingOrDone` | Unit | Error/edge — reload + early-return on `PrNumber>0`/terminal status |
| **B1 / E7 (REGRESSION #8b)**: concurrent TOCTOU documented as known gap (no full prevention) | `session/backlog_lifecycle_test.go` | `TestPushAndCreatePR_documents_ConcurrentDuplicatePRWindow_When_TwoExitsRaceGuard` | Unit | Edge — asserts sequential-dup prevented; comments the concurrent window as accepted gap |
| **B1**: end-to-end policy `SkipReviewGate` complete → PR created → armed → done (CI green) | `session/backlog_integration_test.go` | `TestBacklogIntegration_IT014_PolicyCompleteToMergedViaAutoMerge` | Integration | Happy path — full B1 flow with fake gh/CI |
| **B2**: both `AutoReopenForPRFix` call sites gated on `policyActive` | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_SpawnFix_OnlyWhen_PolicyActive_AtBothSpawnSites` | Unit | Happy path — closed-PR branch + unhealthy branch gated |
| **B2**: non-policy item — detection runs, fix NOT spawned | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_SkipFixButKeepDetection_When_NonPolicyItemUnhealthy` | Unit | Negative — "fix skipped by policy" logged, no spawn |
| **B2 (REGRESSION #4)**: auto-fix respects tombstone→hasActiveWorkSession→transition order; no churn when fix in flight | `server/services/backlog_service_triage_test.go` | `TestAutoReopenForPRFix_should_SkipWithoutStatusChurn_When_ActiveWorkSessionInFlight` | Unit | Error/edge (item-#10 guard) — active session ⇒ zero transition |
| **B2 (REGRESSION #5)**: rework cap terminal — hitting `maxAutoReworkIterations=3` escalates via `notifyReworkCapHit`/`MarkStuck`, loop stops | `server/services/backlog_service_triage_test.go` | `TestAutoReopenForPRFix_should_EscalateAndStopSpawning_When_ReworkCapHit` | Integration | Error/edge — durable `rework_cap` row, no further spawn |
| **B2 (REGRESSION #5)**: shared cap counts across both reopen paths (no new counter) | `server/services/backlog_service_triage_test.go` | `TestAutoReopenForPRFix_should_ShareReworkBudget_When_ReviewAndPRFixCombined` | Unit | Edge — shared lifetime work-session count |
| **B2**: closed-without-merge branch reopens fix for policy item | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_ClearPRFieldsAndReopenFix_When_PolicyPRClosedWithoutMerge` | Unit | Edge — closed-PR branch, policy-gated |
| **B2**: end-to-end CI-fail → fix session spawned → re-attempt (silent, no operator notify during loop) | `session/backlog_integration_test.go` | `TestBacklogIntegration_IT015_PolicyCIFailSpawnsSilentFixLoop` | Integration | Happy path — loop without ready/escalation notify mid-loop |
| **B3 (REGRESSION #6)**: ready-to-merge fires exactly once (notify-once dedup via `NotifiedAt`) | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_NotifyExactlyOnce_When_GreenMergeableAcrossSweeps` | Unit | Happy path — second sweep does not re-fire |
| **B3 (REGRESSION #6)**: notify-once survives restart (durable row) | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_NotDoubleNotify_When_StatePersistedAcrossRestart` | Integration | Error/edge — durable dedup survives reload |
| **B3**: re-arms after a fix cycle resolves then re-opens the ready episode | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_ReArm_When_FixCycleResolvesThenRepensReadyRow` | Unit | Edge — re-arm semantics |
| **B3 (REGRESSION #2)**: pending CI does NOT fire ready-notify AND does NOT arm | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_NotNotifyOrArm_When_GetPRStatusReportsCIPending` | Integration | Error/edge (BLOCKER-1 guard) — `CIPending:true, Mergeable:MERGEABLE` ⇒ no notify/arm |
| **B3**: notify copy branches policy vs non-policy ("will auto-merge" vs "merge on GitHub") | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_UsePolicyAwareCopy_When_ItemPolicyActive` | Unit | Happy path — message text branch |
| **B3**: fires only on genuine mergeability (green AND no conflict), not on mere PR existence | `session/backlog_lifecycle_test.go` | `TestMarkPRReadyUnmerged_should_NotNotify_When_HasConflictsTrue` | Unit | Negative — `HasConflicts` blocks notify |
| **REGRESSION #8a**: `SkipReviewGate` policy churn — E7 two-step hop feeds bounce counter; assert intended single/bounded escalation (`bouncing`+`rework_cap` documented) | `server/services/backlog_service_triage_test.go` | `TestSkipReviewPolicyChurn_should_TerminateAtSharedCap_When_E7HopsTripBounceCounter` | Integration | Error/edge (CONCERN 1) — churn is bounded (terminal state exists); asserts one escalation outcome, documents double-flag as accepted noise |
| **Registry**: backend + frontend per-feature JSON added, no coverage-gap growth | (CI) `make registry-generate` | `docs/registry/features/backend/backlog-auto-merge-policy.json` + `.../frontend/backlog-auto-merge-policy-toggle.json` | Registry | Gate — `coverage-gaps.json` count does not grow |

---

## Test Stack

- **Unit (Go)**: standard `testing` + `testify` (`require` for fatal preconditions, `assert` for
  soft checks), table-driven where the case set is small and orthogonal (e.g. `policyActive`
  truth table, CI tri-state classification). Naming follows the in-repo split:
  `TestMethod_should_Effect_When_Condition` (preferred for new behavior) and the older
  `TestMethod_Scenario` form for the orphan/reconcile family that matches neighbors like
  `TestReconcilePRPending_SpawnsFixSession_WhenHasConflictsTrue`.
- **Integration (Go)**: `createTestStorage(t)` (real ent, in-memory), `initGitRepoWithCommit` for
  repo fixtures, and **test doubles for every external gh/CI call**:
  - a fake `prPendingChecker` whose `GetPRStatus` / `IsPRMerged` / `EnablePRAutoMerge` return
    scripted values (mirrors the existing `TestReconcilePRPending_*` fakes) — this is how CI
    tri-state (`CIFailing`/`CIPending`/`Mergeable`/`HasConflicts`) and merge state are injected
    without hitting GitHub;
  - a fake `instancePRLookup func(sessionUUID string)(string,bool)` for Phase-0 Class-A adoption;
  - a fake global-policy accessor (`func() bool`) toggled in-test for kill-switch cases;
  - `notifyReworkCapHit` / notification capture via the existing notifier spy pattern in
    `backlog_service_triage_test.go`.
  Integration cases live in `session/backlog_integration_test.go` under the
  `TestBacklogIntegration_ITNNN_*` sequence (next free numbers IT012–IT015) and in
  `backlog_lifecycle_test.go` where the reconciler wiring is exercised directly.
- **Frontend (Jest/RTL)**: `@testing-library/react`, `describe("Component — topic")/it(...)`,
  `updateBacklogItem` mocked as `jest.fn()` (existing pattern in
  `BacklogItemDetail.regression.test.tsx` line ~67). The proto3-reset guard reuses the existing
  regression file; the checkbox-payload test extends `BacklogItemForm.test.tsx`
  (`data-testid="backlog-auto-merge-policy-checkbox"`, mirroring the `backlog-auto-spawn-session-checkbox`
  fieldset assertions at lines 153–154).
- **e2e (Playwright)**: `@feature backlog:auto-merge-policy` header, `test.describe('backlog-auto-merge-policy', ...)`,
  `data-testid` locators only, NO `waitForTimeout` (reload + `expect(locator).toBeChecked()`),
  run against `http://localhost:8544`. New helper method `setAutoMergePolicy(checked)` added to
  the existing `tests/e2e/pages/BacklogPage.ts` class.

---

## Regression Guards (mandatory — from research + adversarial review)

Each maps to a named row above; listed here for traceability.

1. **Proto3-bool silent-reset guard** (b28ace2f / AutoSpawnSession mirror) — Go:
   `TestUpdateBacklogItem_should_NotResetAutoMergePolicy_When_PartialUpdateOmitsPointer`;
   Frontend: `BacklogItemDetail.regression.test.tsx` currentFlags-preservation `it(...)`.
2. **CI tri-state / Blocker-1** — a PENDING-checks PR must not read as green: no ready-notify AND
   no auto-merge arm while pending. Guards:
   `TestGetPRStatus_should_SetCIPending_When_ChecksNotAllConcluded`,
   `TestReconcilePRPending_should_MapPendingCI_And_BlockReadyAndArm_When_CIPending`,
   `TestMarkPRReadyUnmerged_should_NotNotifyOrArm_When_GetPRStatusReportsCIPending`,
   `TestReconcilePRPending_should_ArmAutoMerge_OnlyWhen_PolicyActiveAndCIPassingAndReady`.
3. **Phase-0 Class-B orphan adoption / Blocker-2** — `PrNumber>0` item stuck in `review`/`in_progress`
   promoted to `pr_pending` via valid transition (`review→pr_pending`; `in_progress` two-step),
   leaving a `BacklogStatusEvent` audit row. Guards:
   `TestReconcileOrphanedPRs_should_AdoptClassBReviewOrphan_When_PrNumberSetButStatusReview`,
   `TestReconcileOrphanedPRs_should_RouteInProgressClassBOrphanTwoStep_When_PrNumberSet`.
4. **Auto-fix churn guard** (item-#10) — respects tombstone→hasActiveWorkSession→transition order,
   no churn when a fix is already in flight. Guard:
   `TestAutoReopenForPRFix_should_SkipWithoutStatusChurn_When_ActiveWorkSessionInFlight`
   (extends existing `TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn`).
5. **Shared rework-cap terminal** — `maxAutoReworkIterations=3` escalates via
   `notifyReworkCapHit`/`MarkStuck`, loop stops. Guards:
   `TestAutoReopenForPRFix_should_EscalateAndStopSpawning_When_ReworkCapHit`,
   `TestAutoReopenForPRFix_should_ShareReworkBudget_When_ReviewAndPRFixCombined`.
6. **Notify-once durability** — ready-to-merge fires exactly once and survives restart. Guards:
   `TestMarkPRReadyUnmerged_should_NotifyExactlyOnce_When_GreenMergeableAcrossSweeps`,
   `TestMarkPRReadyUnmerged_should_NotDoubleNotify_When_StatePersistedAcrossRestart`.
7. **Kill-switch** — global OFF halts reconciler auto-fix/arm even for policy-enabled items;
   `policyActive = globalOn AND item.AutoMergePolicy`. Guards:
   `TestReconcilePRPending_should_NotArmOrSpawnFix_When_GlobalKillSwitchOff_EvenForPolicyItem`,
   `TestPolicyActive_should_ReturnTrue_OnlyWhen_GlobalOnAndItemFlagTrue`.
8. **Two remaining adversarial CONCERNS**:
   - (a) SkipReviewGate churn double-notify (`bouncing` + `rework_cap`) —
     `TestSkipReviewPolicyChurn_should_TerminateAtSharedCap_When_E7HopsTripBounceCounter`
     asserts the churn is **bounded** (a terminal state exists via the shared cap) and documents
     the `bouncing`+`rework_cap` double-flag as accepted notification noise (per ADR-024 §b to be
     widened / Accepted Risks).
   - (b) Idempotency guard behavior — `TestPushAndCreatePR_should_ShortCircuit_When_ItemAlreadyHasPRorIsPrPendingOrDone`
     proves the sequential/retry duplicate is prevented; the companion
     `TestPushAndCreatePR_documents_ConcurrentDuplicatePRWindow_When_TwoExitsRaceGuard` documents
     the concurrent TOCTOU as a known, accepted gap (soften "prevents a duplicate PR" to "prevents
     the sequential-retry duplicate").

---

## Coverage Targets

- **Every requirement/behavior/control has ≥1 test**: 7 distinct requirement axes (B1, B2, B3,
  FLAG, P0, KILL, CITRI) — all covered, each with ≥1 unit happy-path, ≥1 unit error/edge, and
  (where a data store or gh/CI call is involved: B1, B2, B3, P0, KILL) ≥1 integration test.
- **All 8 mandatory regression guards** present and named (see section above).
- **Both remaining adversarial CONCERNS** each have a dedicated guarding/documenting test.
- **Line/branch focus**: the three gate points (arm, auto-PR-on-Complete, fix-spawn) each have a
  positive test (gate open) and a negative test (gate closed by policy/kill-switch/CI-pending) —
  no gate is asserted in only one direction.
- **Registry**: `make registry-generate` produces no net `coverage-gaps.json` growth; new
  per-feature JSONs carry `tested:true` with `testIds` matching the e2e `describe`.
- **Non-regression of manual paths**: default-false behavior tests
  (`TestOnSessionExited_should_TransitionToDone_When_NonPolicySkipReviewItemExits`,
  `TestReconcilePRPending_should_SkipFixButKeepDetection_When_NonPolicyItemUnhealthy`) assert the
  existing manual Review-Queue / SubmitManualReview flows are untouched.

### Verification gate (from plan)
`make build && make test` green; `make lint` green; the four `worktree_git`/`backlog_lifecycle`/
`backlog_service_triage`/`backlog_service` suites pass;
`cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItem"` green;
`npx playwright test backlog-auto-merge-policy.spec.ts` green against the test server;
ent regen confirmed with `--feature sql/upsert`.
