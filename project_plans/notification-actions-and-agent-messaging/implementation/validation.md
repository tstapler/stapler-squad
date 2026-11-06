# Validation Plan: notification-actions-and-agent-messaging

**Date**: 2026-10-17
**Complexity**: 3
**Inputs**: `requirements.md`, `implementation/plan.md` (49 stories, 196 tasks after triad round 3), `design/ux.md` (88 UX-ACs). Code-state checks were run against the worktree on 2026-10-17 (section "Repo readiness check"). Updated in the Phase 4 repair for every new or changed acceptance criterion (Stories 1.7 and 4.11 are new; see the "Phase 4 repair additions" section) and in triad round 1 (Stories 1.8 and 2.10 are new; see "Triad round 1 additions" at the end). The six deviations below are fixed in the plan text.

## Happy Path Scenario

Given the live baseline (11 stopped hidden sessions, zero history rows, an empty Background tab, and a pending `AskUserQuestion` "Which cache?" with options Redis/Memcached on a visible Claude session), when the operator opens the notification pane, sees 11 Background rows (rows whose server-computed `background_outcome` is `completed` read "Finished", the rest "Stopped, outcome unknown") with the roll-up "11 background sessions, no failures reported", and taps "Redis" once on the question row, then the fake pane receives exactly the byte `1` (no Enter), the durable `GuidanceRequest` goes `pending -> sent -> acknowledged` with `answer_latency_ms` logged, the row reads "Answered: Redis", and no session navigation occurred.

Every error path below is a variation on this flow: stale dialog, answered elsewhere, archived or stopped session, remote listener, flag off, restart between claim and delivery.

## Test conventions applied (repo-enforced)

- Go: names follow `TestSubject_ShouldX_WhenY`; `t.Parallel()` by default; no `time.Sleep` (`notimesleeptest` analyzer); time enters through injected `Clock`/fake clocks; no `t.Setenv` fixtures; scale cases are `Benchmark*`, correctness is proven at small N (`deterministic-fast-tests`). Concurrency, cap, claim and dispatcher tests run with `-race`; goroutine joins asserted with `go.uber.org/goleak` (already in `go.mod`, precedent `server/services/superseded_session_sweeper_test.go`).
- Jest (web-app, pnpm only): `it('should ... when ...')`, fake timers, shared fixtures in `web-app/src/components/ui/__fixtures__/notificationFixtures.ts` (Task 1.1b) to stay under the jscpd 0.14% gate.
- Playwright (`tests/e2e/`): first line `// @feature <ids>`, no `waitForTimeout`, `data-testid`/ARIA role locators only, page helpers in `tests/e2e/pages/`, phone case via `profiles` in `tests/e2e/helpers/viewport-profiles.ts` (390x844, `hasTouch`).

## Repo readiness check (test locations and helpers, verified by `ls`/`grep` on 2026-10-17)

| Item the plan relies on | State today | Action recorded in this plan |
|---|---|---|
| `web-app/src/components/ui/NotificationPanel.test.tsx`, `BackgroundActivity.test.tsx`, `guidance/GuidanceRequestPanel.test.tsx` | exist | Extend; Story 1.1 requires them to pass unchanged |
| `web-app/src/lib/utils/backgroundActivity.test.ts` (Task 1.2b) | **does not exist** (only `backgroundActivity.ts`) | Create it; existing selector coverage, if any, is inside `BackgroundActivity.test.tsx` and must keep passing |
| `tests/e2e/background-activity.spec.ts`, `tests/e2e/pages/NotificationPanel.ts` | **exist** (plan Task 1.4b says "write a spec") | Extend the existing spec and page helper, do not create a parallel file |
| Hidden sessions in e2e | The shared test-mode instance cannot create `Hidden: true` sessions (header of `background-activity.spec.ts`); it fulfils `ListSessions{hiddenOnly}` by route interception | Plan AC "test server seeded with 11 hidden sessions" is implemented with the same route-fulfilment technique plus real `SendNotification` for failures |
| Remote listener in e2e | `tests/e2e/helpers/test-server.ts` and `global-setup.ts` have no remote-port support | Tier B on `:8444` is proven by a Go integration test through the remote handler chain (S6) and by an e2e that fulfils descriptors with `available=false, reason=desktop_only`; no Playwright run against a real `:8444` |
| Phone profile | `tests/e2e/helpers/viewport-profiles.ts` has `phone` (390x844, `hasTouch`), `tests/e2e/touch-targets.spec.ts` exists | Reuse for 44px assertions |
| Fake clocks | Package-local fakes only (`server/deliverygate/helpers_test.go`, `server/services/pi_extension_health_test.go`, `session/host_registry.go` `Clock`) | `server/wakes` ships its own `Clock` plus `FakeClock` in Task 4.2a; not shared across packages |
| `goleak` | in `go.mod` v1.3.0, used in `superseded_session_sweeper_test.go` | Use for Tasks 4.2c and the reaper/sampler |
| Pane fixtures dir | plan said `session/testdata/panes/`; repo has `session/testdata/reply_dialogs/` (`question_single_select_3opts.txt`, `question_wrapped_2opts.txt`, `permission_*.txt`) | New Claude AskUserQuestion captures go next to the existing ones in `session/testdata/reply_dialogs/`; other harnesses under `session/testdata/panes/<harness>/`. **Plan text amended** (Stories 0.1, 3.6) |
| Fault-injection submitter for pane writes | Task 3.5 named `paneSubmitter`; no such type found under `server/` (the `hidden_question_reply_test.go` suite has the fault cases `NOT_SENT` vs `SEND_INDETERMINATE` as `TestReply_Should...` tests) | Extract the existing test double from `hidden_question_reply_test.go` into a shared `_test` helper in Task 2.4a and reuse it (also for the wake-write safety tests). **Plan text amended** (Stories 2.4, 3.5, 4.3) |
| `Session.background_outcome` and `ItemSession` lookups (Story 1.7) | `ItemSession` with `review_verdict`/`triage_result`/`end_reason` and `GetItemSessionBySessionUUID` exist (`proto/session/v1/backlog.proto`, `session/storage_backlog.go:386`); the proto field does not | Created by Task 1.7a; whether the 11 live sessions have recorded verdicts is Spike S7 item (8) (UNVERIFIED) |
| Generic "is a dialog showing" check (wake writes, Task 4.3c; sampler, Task 3.3f; acknowledgement, Task 3.4e) | `session.DialogMatch(capture, q)` needs a specific question (`session/pane_reply.go:74`); `dialogFooterPrefix` exists (`pane_reply.go:31`) | `session.PaneShowsDialog` is new in Task 2.4c (W1, before all three consumers), tested against the existing `reply_dialogs/` captures |
| `server/wakes/`, `server/mcp/caller_identity_gate.go`, `notification_action*.go`, `question_answer_router.go` | do not exist yet (new work) | Test files below are created by the owning tasks |
| `GetFeatureFlagScoped` | exists at `config/config.go:1925`; `config/feature_flag_scopes_test.go` exists | Extend the table test |
| `tools/lint/noarchivedrevival/{analyzer.go,analyzer_test.go,testdata/}` | exist | Add fixtures per Task 4.3f |
| Flag-env guard precedent | `server/deliverygate/flag_env_guard_test.go` | Mirror it for the four new flags (REQ-24) |

## Requirement → Test Mapping

Requirement IDs (derived from `requirements.md`; recounted below, 33 total):

| ID | Requirement (source section) |
|---|---|
| REQ-1 | Background tab shows non-empty rows for the 11 live hidden sessions (Success metric: Background) |
| REQ-2 | Background shows a meaningful never-blank state when nothing failed; failure/needs-human rows pinned first; explanatory empty state (Success metric + Scope 1) |
| REQ-3 | Approval resolvable from the pane in one interaction (Success metric: Inline actions) |
| REQ-4 | Pending question with options resolvable from the pane in one interaction |
| REQ-5 | Backlog decision resolvable from the pane in one interaction |
| REQ-6 | Structured question renders question text and option list in the pane (Success metric: AskUserQuestion) |
| REQ-7 | Answer reaches the originating live session (keystroke or harness-native channel) |
| REQ-8 | Answer for a dead/stopped/archived session lands in the durable record, nothing revived |
| REQ-9 | created_at to answered_at latency logged on the guidance entity |
| REQ-10 | Session can request help from the human |
| REQ-11 | Session can request help from a named peer session |
| REQ-12 | Schedule a re-prompt by delay/cron |
| REQ-13 | Schedule a re-prompt by backlog/PR/CI event |
| REQ-14 | Schedule a re-prompt by another session's state |
| REQ-15 | Schedule a re-prompt when a human answer arrives |
| REQ-16 | Wakes survive a service restart (integration incl. restart) |
| REQ-17 | Zero new `ScheduleWakeup` poll loops |
| REQ-18 | NFR: pane renders <= 200 ms from cached state |
| REQ-19 | NFR: action round-trip p95 < 1 s on localhost |
| REQ-20 | NFR: scalability, tens of sessions and hundreds of notification records |
| REQ-21 | NFR/Security: approval-gate and caller-identity checks preserved; MCP tools gated server-side |
| REQ-22 | Constraint: no live `:8543` restart; manual isolated instance procedure |
| REQ-23 | Constraint: reuse `GuidanceRequest`, delivery gate and registry conventions; no parallel question store; gate unchanged |
| REQ-24 | Constraint: rollout flags live-settable (RPC/panel), never env vars |
| REQ-25 | Constraint: mobile and desktop UX for every UI feature |
| REQ-26 | Constraint: `noarchivedrevival` applies to every wake path |
| REQ-27 | Constraint: generated ent/proto output untracked; pnpm; registry current |
| REQ-28 | Observability: metrics and log fields (inline action, latency, wake lifecycle, Background row counts) |
| REQ-29 | Observability: alerts (missed wakes after restart > 0; actions failing delivery) |
| REQ-30 | Risk control: four flags, additive-only rollback, staged rollout order |
| REQ-31 | Scope 1 / Open Q1: diagnose why hidden sessions produce no history rows (gate vs producers vs prune) |
| REQ-32 | Scope 2-4 / Open Q2-4: inventory, harness survey, vision and ADRs; per-harness answer-channel verdicts |
| REQ-33 | Delivery gate semantics unchanged; gate stats remain authoritative (ADR-001) |

Type legend: Unit, Integration (real SQLite/on-disk DB, real bus, fakes at the pane/network edge), E2E (Playwright), Benchmark (opt-in, not in default `go test`), Migration, Doc-check (scripted grep/ls assertion run in CI or the Task 5.3 checklist).

### Epic 0 and cross-cutting (flags, spikes, constraints)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-24 | `config/feature_flag_scopes_test.go` | `TestGetFeatureFlagScoped_ShouldReturnFalse_WhenScopedFalseOverridesGlobalTrue` (`action:DENY` false, `action:APPROVE` true) | Unit | Happy path (Story 0.8) |
| REQ-24 | `config/feature_flag_scopes_test.go` | `TestGetFeatureFlagScoped_ShouldInheritGlobal_WhenScopeAbsentForAllFourFlags` | Unit | Error/edge: absent scope |
| REQ-24 | `server/services/feature_flags_notification_actions_test.go` | `TestFeatureFlagRegistry_ShouldListFourFlagsWithDefaultsAndScopes_WhenGetFeatureFlagsCalled` (`background_activity_feed` ON, others OFF, `status_detail` present) | Unit | Happy path |
| REQ-24 | `server/services/feature_flags_notification_actions_test.go` | `TestUpdateFeatureFlag_ShouldRefuseLooseningFlip_WhenAuditSinkDown` (`background_activity_feed` explicit OFF to ON) | Unit | Error path |
| REQ-24, REQ-30 | `server/services/feature_flags_notification_actions_test.go` | `TestUpdateFeatureFlag_ShouldReflectWithoutRestartInDispatcherStub_WhenAgentWakeTriggersFlipped` | Integration | Live flip, readback via `GetFeatureFlags` |
| REQ-24 | `server/services/feature_flag_env_guard_test.go` (mirror of `server/deliverygate/flag_env_guard_test.go`) | `TestNewFlags_ShouldNotReadOsEnv_WhenControllersConstructed` (AST scan: no `os.Getenv` in new controllers/decision paths) | Unit | Guard |
| REQ-24 | `web-app/src/lib/hooks/useFeatureFlagValue.test.ts` | `useFeatureFlagValue_should_returnScopedValueAndCache_When_flagReadTwice` | Unit | Happy path |
| REQ-24 | `web-app/src/lib/hooks/useFeatureFlagValue.test.ts` | `useFeatureFlagValue_should_fallBackToDefaultOff_When_getFeatureFlagsRejects` | Unit | Error path |
| REQ-30 | `server/services/feature_flags_notification_actions_test.go` | `TestFlagOff_ShouldRestorePriorBehaviorForAllFourFlags_WhenFlippedLive` (table: Background legacy selector, descriptors reduce to `OPEN`, no new guidance writes, dispatcher stops) | Integration | Rollback |
| REQ-30 | `server/services/feature_flags_notification_actions_test.go` | `TestAskUserQuestionStructured_ShouldGateStagesInOrder_WhenRenderAnswerHookReplyScopesSet` | Unit | Staged rollout ladder (`stage:render` < `stage:answer` < `stage:hook_reply`) |
| REQ-27 | CI shell step (`make build` then `git ls-files session/ent \| grep -v schema \| grep -v generate.go`) | `generated_ent_output_should_not_be_tracked` | Doc-check | Empty output required (Story 5.4) |
| REQ-27 | `make registry-diff` | `registry_should_be_clean_after_registry_generate` | Doc-check | New RPCs have `// +api:` markers; feature files committed |
| REQ-27 | `proto/` via `make proto-gen && make lint` | `proto_should_pass_buf_standard_lint_with_unspecified_zero_enums` | Doc-check | Story 2.1 |
| REQ-27 | `cd web-app && pnpm run lint:duplicates` | `jscpd_should_stay_under_0_14_percent_after_each_epic` | Doc-check | Each epic's final branch |
| REQ-22 | `docs/how-to/verify-notification-actions-rollout.md` checklist, step 0 | `manual_instance_should_run_on_ports_62871_62872_with_private_socket` (`ss -ltnp \| grep -E '62871\|62872'` empty before start; log line `Using private tmux socket`) | Doc-check | Story 5.3 |
| REQ-22 | `tests/e2e/global-setup.ts` (existing) | `e2e_should_use_dynamic_port_and_never_touch_8543` | E2E | Existing guarantee, asserted by a one-line check in the new specs' fixture (`TEST_SERVER_URL` not `:8543`) |
| REQ-31 | `research/spike-S7.md` + `research/features.md` | `spike_S7_should_record_root_cause_of_empty_history` (doc-check: file contains the three-way verdict gate/producer/prune and pasted command output) | Doc-check | Open Q1 |
| REQ-32 | `research/spike-S1.md`, `S2`, `S3`, `S4`, `S5`, `S6`, `S7` | `each_spike_file_should_contain_PASS_or_FAIL_line_and_fallback_decision` (script greps `**Result**: PASS\|FAIL` and an ADR status-line update) | Doc-check | Seven spikes |
| REQ-32 | `decisions/ADR-001..008` | `adr_status_lines_should_reflect_spike_results_before_dependent_stories_start` | Doc-check | S1 to ADR-005, S2 to ADR-005, S3 to ADR-006, S6 to ADR-004 |
| REQ-32 | `docs/` index test (`CLAUDE.md` reference table) | `claude_md_should_index_three_new_docs` | Doc-check | Story 5.2 |

### Epic 1: Background tab (flag `background_activity_feed`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1 | `web-app/src/lib/utils/backgroundActivity.test.ts` (**new**; only `backgroundActivity.ts` exists today) | `buildActivityRows_should_return10FinishedAnd1StoppedUnknownAndNoFailuresRollup_When_10CompletedReviewsAnd1UnknownDiagnoseAndEmptyHistory` | Unit | Happy path, production-realistic: no history record that production cannot produce |
| REQ-1, REQ-2 | same | `buildActivityRows_should_return11StoppedUnknownRows_When_allOutcomesUnknownAndEmptyHistory` | Unit | Degraded evidence: rollup still `no_failures_reported`, never healthy |
| REQ-2 | same | `buildActivityRows_should_markExceptionFailureWithoutAnyHistory_When_outcomeFailed` | Unit | Failure from the session list, pinned first |
| REQ-2 | same | `buildActivityRows_should_notMarkFinished_When_outcomeUnknownEvenIfRoutineHistoryRecordPresent` | Unit | Guard: history is never the evidence for `finished` |
| REQ-1 | `session/background_outcome_test.go` (new) | `ComputeBackgroundOutcome_should_returnCompleted_When_reviewVerdictRecorded` / `_Failed_When_statusCrashed` / `_Unknown_When_endReasonShutdown` / `_Unknown_When_diagnoseRole` / `_Unknown_When_noItemSession` (table) | Unit | Story 1.7 rules |
| REQ-1 | `server/services/session_service_crud_test.go` | `ListSessions_should_populateBackgroundOutcome_When_hiddenOnly_And_leaveEmptyWhenNot` (seeded in-memory ent: verdict, crashed, no ItemSession) | Integration | Server-side evidence |
| REQ-1 | `web-app/src/lib/hooks/useBackgroundSessions.test.ts` | `useBackgroundSessions_should_mapBackgroundOutcomeAndDefaultToUnknown_When_oldServerOmitsField` | Unit | Compatibility |
| REQ-23 | `session/ent_repository_guidance_roundtrip_test.go` style proto test | `SessionProto_should_roundTripBackgroundOutcome_When_converted` | Unit | Proto field |
| REQ-2 | same | `buildActivityRows_should_pinExceptionFirstAndNotDuplicate_When_unreadErrorRecordForHiddenSession` (still 11 rows, row 0 `exception_failure`) | Unit | Pinned exceptions |
| REQ-2 | same | `buildActivityRows_should_giveExceptionPrecedence_When_sessionHasFailureAndPendingDecision` | Unit | Edge: one row, exception wins |
| REQ-2 | same | `buildActivityRows_should_keepStableOrder_When_sameInputPolledTwice` | Unit | Edge: no reorder between polls |
| REQ-1 | same | `buildActivityRows_should_emitDepartedOnceThenDrop_When_sessionMissingFromNextList` | Unit | Edge: reaped session |
| REQ-2 | same | `buildActivityRows_should_labelOtherRole_When_tagsUnknown` | Unit | Error path: unknown role renders "other" |
| REQ-2 | same | `buildRollup_should_returnNoneVariantWithExactCopy_When_zeroHiddenSessions` | Unit | Empty state |
| REQ-2 | same | `buildRollup_should_neverEmitTheWordHealthy_When_anyVariant` (property over all variants) | Unit | Overclaim guard, UX-AC1 |
| REQ-2 | same | `selectBackgroundRows_should_keepLegacyBehavior_When_flagOff` | Unit | Flag-off rollback |
| REQ-1 | `web-app/src/components/ui/BackgroundActivity.test.tsx` | `BackgroundActivity_should_renderRollupTestIdWithNoFailuresCopy_When_11StoppedSessions` | Unit | Happy path |
| REQ-2 | same | `BackgroundActivity_should_renderDistinctStoppedUnknownChip_When_noRecordedOutcome` | Unit | Chip text "Stopped, outcome unknown" |
| REQ-2 | same | `BackgroundActivity_should_renderNoBackgroundWorkCopyWithoutSpinner_When_zeroSessions` | Unit | Empty state |
| REQ-2 | same | `BackgroundActivity_should_showNNewButtonAndKeepRowPositions_When_pollAddsSession` | Unit | No reorder under cursor |
| REQ-2 | same | `BackgroundActivity_should_keepLastKnownRowsWithRetryBanner_When_pollFails` ("Couldn't refresh background sessions. Showing data from 10:03.") | Unit | Error path |
| REQ-2 | same | `BackgroundActivity_should_clearBannerAndRepoll_When_retryClickedAndSucceeds` | Unit | Error exit |
| REQ-2 | same | `BackgroundActivity_should_showSessionNoLongerExistsAndNotNavigate_When_openOnDepartedRow` | Unit | Error path |
| REQ-20 | same | `BackgroundActivity_should_capAt100RowsWithShowAll_When_150Sessions` | Unit | Scale cap at small N via injected cap |
| REQ-18 | same | `BackgroundActivity_should_renderRowsOnFirstCommit_When_cachedPollStateProvided` (no loading state, no awaited fetch before first paint) | Unit | Deterministic proxy for 200 ms budget |
| REQ-25 | `web-app/src/components/ui/NotificationPanel.test.tsx` | `NotificationPanel_should_exposeTablistWithRovingTabindexAndExceptionCountInName_When_rendered` ("Background, 1 needs attention" with one unread failure, plain "Background" at 0 and unchanged when informational rows are added; ArrowRight moves focus and `aria-selected`) | Unit | Tab semantics |
| REQ-1 | same (existing suite) | `NotificationPanel_existing_suite_should_passUnchanged_After_useBackgroundActivityExtraction` + `wc -l` lower than before | Unit | Seam story 1.1 regression |
| REQ-28 | `web-app/src/lib/hooks/useBackgroundActivity.test.ts` | `useBackgroundActivity_should_postStateCountsOncePerPoll_When_tabRenders` (`{stopped_unknown:10, exception_failure:1}`) | Unit | Happy path |
| REQ-18, REQ-28 | same | `useBackgroundActivity_should_postPaneOpenMsSample_When_firstRowCommits` | Unit | `notification_pane_open_ms` |
| REQ-29 | `web-app/src/lib/hooks/useBackgroundActivity.test.ts` or the server alert evaluator | `Alert_should_raiseOnceBackgroundOutcomeEvidenceEmpty_When_finishedZeroAndStoppedUnknownPositiveFor24hFakeClock` (and none when one `finished` appears) | Unit | Story 1.5 alert |
| REQ-28 | same | `useBackgroundActivity_should_notPostOrThrow_When_clientEventRpcFails` | Unit | Error path (telemetry never breaks UI) |
| REQ-28 | server client-event handler test (file located by Task 0.7a) | `ClientEvent_should_incrementBackgroundActivityRowsByState_When_eventReceived` | Unit | Server counter |
| REQ-28 | same | `ClientEvent_should_rejectUnknownStateLabel_When_cardinalityGuardHit` | Unit | Error path: label cardinality |
| REQ-1 | `tests/e2e/background-activity.spec.ts` (extend; both it and `pages/NotificationPanel.ts` exist) | `backgroundTab_should_show11RowsAndNoFailuresRollup_When_11HiddenStoppedSessionsAndNoHistory` (route-fulfilled `ListSessions{hiddenOnly}`; `getByRole('row')` count 11) | E2E | Happy path, success metric 0 to 11 |
| REQ-1 | same | `backgroundTab_should_showAtLeastOneFinishedChip_When_noHistoryRowsAndOutcomesCompleted` (seed has zero history rows; `Finished` comes from `backgroundOutcome`) | E2E | Pre-mortem P1-a: finished is satisfiable in production-shaped data |
| REQ-2 | same | `backgroundTab_should_pinFailureRowFirst_When_realErrorNotificationSentForHiddenSession` | E2E | Real `SendNotification` |
| REQ-2 | same | `backgroundTab_should_showNoBackgroundWorkCopy_When_noHiddenSessions` | E2E | Empty state |
| REQ-2 | same | `backgroundTab_should_keepRowsAndOfferRetry_When_listSessionsFails` | E2E | Error path |
| REQ-30 | same | `backgroundTab_should_showLegacyEmptyState_When_flagOffViaUpdateFeatureFlag` | E2E | Rollback live, no reload |
| REQ-25 | same (phone profile) | `backgroundTab_should_stackRowsWith44pxTargetsAndSingleScroll_When_390pxTouch` | E2E | Mobile |

### Epic 2: Inline notification actions (flag `notification_inline_actions`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-3 | `server/services/notification_actions_test.go` | `BuildDescriptors_should_returnApproveDenyOpen_When_approvalPending` | Unit | Happy path |
| REQ-4 | same | `BuildDescriptors_should_returnAnswerQuestionWithOptionLabelsTierBThenOpen_When_pendingQuestionReplyAvailable` (labels Redis and Memcached) | Unit | Happy path |
| REQ-4 | same | `BuildDescriptors_should_returnOnlyOpen_When_questionAlreadyAnswered` | Unit | Error path: resolved |
| REQ-4 | same | `BuildDescriptors_should_returnOnlyOpen_When_replyUnavailableTrue` | Unit | Error path: non-replyable shape |
| REQ-5 | same | `BuildDescriptors_should_mapBacklogChoicesToExistingRpcs_When_itemAwaitsDecision` (accept/rework/merge); a choice with no RPC emitted as `OPEN` | Unit | Happy + gap path |
| REQ-3 | same | `BuildDescriptors_should_degradeUnknownActionKindToOpen_When_kindUnspecified` | Unit | Error path |
| REQ-21, REQ-25 | same | `BuildDescriptors_should_setTierBUnavailableDesktopOnly_When_listenerRemote_and_available_When_local` | Unit | ListenerClass (Task 2.2d) |
| REQ-33 | `server/deliverygate/policy_test.go` | `ShouldDeliver_should_deliverEveryNewEmission_When_hiddenSession` (table: `INPUT_REQUIRED` question, `CONFIRMATION_NEEDED` decision, wake missed/skipped/rate-limited as failure class with `HintFailure`, peer-help escalation as `INPUT_REQUIRED`, the `notify_operator` reminder as a system-level notification with no session binding (`VisibilityNotASession`); fails if an emission is missing from the table) | Unit | Gate classes |
| REQ-33 | `server/deliverygate/matrix_test.go` / `stats_test.go` (existing) | existing suites should pass unchanged | Unit | Gate untouched |
| REQ-5 | `server/services/backlog_notifier_test.go` | `Notifier_should_emitDecisionSchemaMetadata_When_itemInReviewAwaitingMerge` | Unit | `decision_schema` choices |
| REQ-3 | `server/services/notification_action_service_test.go` | `Invoke_should_returnAppliedBothTimesAndResolveOnce_When_sameKeySamePayloadTwice` | Unit | Idempotency happy path |
| REQ-3 | same | `Invoke_should_returnInvalid_When_sameKeyDifferentPayloadHash` | Unit | Error path |
| REQ-3 | same | `Invoke_should_returnFlagOffWithZeroWrites_When_flagOff` | Unit | Error path |
| REQ-4 | same | `Invoke_should_returnFlagOffWithZeroWrites_When_answerStageOffOnVisibleSession` | Unit | Staged flag |
| REQ-3 | same | `Invoke_should_returnAlreadyResolvedOrStaleNotConnectError_When_targetResolvedOrUnknown` | Unit | Error path |
| REQ-21 | same | `Invoke_should_returnInvalidAndLeaveApprovalUnresolved_When_ciBlockedApproveWithoutConfirm` | Unit | Delegate checks preserved |
| REQ-21 | same | `Invoke_should_refuse_When_secretScanHit` | Unit | Delegate checks preserved |
| REQ-21 | same | `Invoke_should_returnForbiddenWithZeroPaneWrites_When_tierBOnRemoteChain` | Unit | Tier policy |
| REQ-3 | same | `Invoke_should_returnApplied_When_tierAOnRemoteChain` | Unit | Tier A works remote |
| REQ-3 | same | `Invoke_should_returnRateLimitedWithRetryAfter_When_secondReplyWithinWindow` | Unit | UX-AC17 server side |
| REQ-21 | same | `Invoke_should_returnAppliedAndWriteNoPane_When_answerGuidanceOrCancelWakeOrCancelPeerHelpOnRemoteChain` | Unit | ADR-004 Tier A kinds (Story 2.8) |
| REQ-5 | same | `Invoke_should_returnAlreadyResolvedAndMakeNoSecondMergeCall_When_mergeAppliedThenDispatcherRestarted` | Unit | Post-restart idempotency, empty cache |
| REQ-5 | same | `Invoke_should_callEachBacklogDelegateOnce_When_decisionInvokedPerChoice` (`TransitionBacklogItemStatus`, `SubmitManualReview`, `TriggerReReview`, `MergePR`) | Unit | Delegate mapping |
| REQ-3 | same (`-race`) | `Invoke_should_applyOnce_When_32GoroutinesUseSameIdempotencyKey` | Unit | Race |
| REQ-21 | `server/services/notification_action_service_test.go` | `Dispatcher_should_dependOnlyOnConsumerSideInterfaces_When_importsInspected` (no `*SessionService`) | Unit | Structure guard |
| REQ-20, REQ-18 | `server/services/notification_actions_test.go` | `BuildDescriptors_should_callEachStoreOncePerPage_And_skipResolvedRecords_When_300RecordsWith40Sessions` (counting fakes) | Unit | Task 2.2e batching |
| REQ-19 | `server/services/notification_action_service_bench_test.go` | `BenchmarkInvokeNotificationAction_Approve100Sequential` (reports p95; asserts < 1 s in `make benchmark`, not in default run) | Benchmark | Plan AC "p95 < 1 s" lives here per `deterministic-fast-tests` |
| REQ-19 | `server/services/notification_action_service_test.go` | `Invoke_should_completeWithinFakeClockBudget_When_delegatesAreFakes` (no real timers; asserts no blocking call on the hot path) | Unit | Deterministic proxy |
| REQ-3, REQ-4, REQ-5 | `server/services/notification_action_service_integration_test.go` | `InvokeNotificationAction_should_resolveApprovalQuestionAndBacklogDecision_When_realStoresAndInMemoryEntDB` | Integration | Full path, real ent |
| REQ-7 | `server/services/pane_reply_core_test.go` | `PaneReplyCore_should_claimAuditLeaseAndWriteOneDigit_When_visibleTarget` | Unit | Happy path |
| REQ-7 | `server/services/hidden_question_reply_test.go` (existing) | existing `TestReply_*` suite incl. fault injection `NOT_SENT` / `SEND_INDETERMINATE` should pass unchanged, `-race` | Unit | Regression (Story 2.4) |
| REQ-7 | `server/services/pane_reply_core_test.go` | `ReplyToPendingQuestion_should_returnNotHidden_When_visibleSessionAndAnswerStageOff` | Unit | Error path |
| REQ-7 | same | `PaneReplyCore_should_keepLimiterConstantsInOnePlace_When_hiddenAndVisibleCallersUsed` (per-session 1/5 s, global 10/min) | Unit | Structure |
| REQ-7 | `session/pane_reply_sendkeys_guard_test.go` (sendkeysguard lint run: `make lint-custom`) | `sendkeysguard_should_pass_When_paneReplyExtracted` | Doc-check | Task 2.4b |
| REQ-4 | `server/notifications/resolved_item_reaper_test.go` | `Store_should_retireHistoryRowKeyedOnQuestionID_When_dispatcherPublishesResolveEvent` | Unit | New event-driven path |
| REQ-4 | same | `Store_should_keepBothRecordsActionable_When_twoDistinctQuestionsFromOneSession` (`q1`,`q2` within 5 s) | Unit | Dedup by `question_id` |
| REQ-4 | same | `Store_should_collapseOnly_When_textAndLabelsIdentical` | Unit | Edge |
| REQ-3 | `server/services/notification_action_integration_test.go` | `ResolvePath_should_notReapUnrelatedItems_When_resolveEventHasUnknownTargetKey` | Integration | Error path |
| REQ-23 | `docs/` proto test | `ActionKind_should_haveUnspecifiedZeroAndUnknownDegradesToOpen_When_protoRoundTripped` | Unit | Contract |
| REQ-3, REQ-4, REQ-5 | `web-app/src/components/ui/notification-actions/ActionRow.test.tsx` | `ActionRow_should_renderButtonsFromDescriptorsWithKindTestIds_When_approveDenyOpen` | Unit | Happy path |
| REQ-3 | same | `ActionRow_should_showRefreshAndRefetchDescriptors_When_outcomeInvalid` | Unit | UX-S2 INVALID copy (Task 2.5d) |
| REQ-3 | same | `ActionRow_should_showHandledAutomaticallyByRuleNameWithCreateRule_When_staleOnAutoApprovedApproval` | Unit | STALE variant (Task 2.5d) |
| REQ-3 | same | `ActionRow_should_countDownRateLimitedTextAndReenableAtZero_When_fakeTimersAdvance` | Unit | UX-AC17 |
| REQ-3 | same | `ActionRow_should_renderFirstRowsOnFirstCommit_And_postPaneOpenMs_When_cachedHistoryOf40` | Unit | REQ-18 deterministic proxy (Task 2.5d) |
| REQ-3 | same | `ActionRow_should_mapEveryActionOutcomeToExpectedUi_When_parametrizedOver9Outcomes` (APPLIED, ALREADY_RESOLVED, STALE, NOT_SENT, SEND_INDETERMINATE, RATE_LIMITED, FORBIDDEN, FLAG_OFF, INVALID) | Unit | Error table |
| REQ-3 | same | `ActionRow_should_disableButtonAndShowBusyLabelWithin100ms_And_sendNothingOnSecondPress` | Unit | Double-tap guard |
| REQ-3 | same | `ActionRow_should_showAlreadyAnsweredValueWithoutRoleAlert_When_alreadyResolvedWithValue` | Unit | No toast |
| REQ-3 | same | `ActionRow_should_reuseIdempotencyKey_When_retryAfterTransientFailure` | Unit | Retry |
| REQ-3 | same | `ActionRow_should_openInlineDenyReasonField_And_restoreRowWithDraft_When_cancel` | Unit | Deny flow |
| REQ-5 | same | `ActionRow_should_showInlineConfirmWithFocusOnConfirm_When_mergeRequiresConfirm` | Unit | Two-step |
| REQ-25 | same | `ActionRow_should_renderAriaDisabledWithDesktopOnlyDescription_When_availableFalse` | Unit | Tier B remote |
| REQ-25 | same | `ActionRow_should_renderOnlyOpen_When_unknownActionKind` | Unit | Fallback |
| REQ-25 | same | `ActionRow_should_ariaDisableWithOfflineReason_When_navigatorOffline` | Unit | Offline |
| REQ-3 | `web-app/src/lib/hooks/useInvokeNotificationAction.test.ts` | `useInvokeNotificationAction_should_generateNewKeyPerAttemptButReuseOnRetry` | Unit | Idempotency client |
| REQ-3 | same | `useInvokeNotificationAction_should_rollBackOptimisticState_When_outcomeNotApplied` | Unit | Error path |
| REQ-25 | `web-app/src/components/ui/notification-actions/ActionRow.keyboard.test.tsx` | `ActionRow_should_moveFocusToNextItemAndAnnounceApproved_When_itemResolves` | Unit | Focus management |
| REQ-25 | same | `ActionRow_should_selectNthOptionOnlyWhenCardFocused_When_digitPressed` | Unit | UX-AC49 |
| REQ-25 | same | `ActionRow_should_collapseAndPreserveDraft_When_escapePressed` | Unit | Draft |
| REQ-25 | `web-app/src/lib/hooks/useNotificationSync.test.ts` | `useNotificationSync_should_postIdSetNotClear_When_answeredInTabA` | Unit | BroadcastChannel |
| REQ-25 | same | `useNotificationSync_should_ignoreMalformedMessage_When_payloadNotIdSet` | Unit | Error path |
| REQ-25 | `web-app/src/lib/hooks/useOnlineStatus.test.ts` | `useOnlineStatus_should_flipWithinTwoSecondsAndRecover_When_onlineOfflineEvents` | Unit | Offline banner source |
| REQ-3 | `tests/e2e/notification-inline-actions.spec.ts` (new) | `approve_should_resolveWithOneClickAndEmptyApprovalList_When_pendingApprovalSeeded` | E2E | Happy path, success metric |
| REQ-3 | same | `deny_should_resolveWithReasonInTwoInteractions_When_denyWithReason` | E2E | Documented 2-step |
| REQ-5 | same | `backlogDecision_should_resolveWithOneClick_When_sendBackChosen` | E2E | Happy path |
| REQ-5 | same | `backlogDecision_should_requireInlineConfirmWithoutModal_When_mergeChosen` | E2E | Confirm |
| REQ-4 | same | `guidanceAnswer_should_resolveWithOneClick_When_openGuidanceRequestWithOptions` | E2E | Happy path |
| REQ-4 | same | `question_should_showAlreadyHandledWithoutAlert_When_answeredViaRpcBeforeClick` | E2E | Stale path |
| REQ-4 | same | `question_should_showAnsweredElsewhereInSecondTab_When_firstTabAnswers` | E2E | Cross-tab |
| REQ-25 | same (phone profile, `hasTouch`) | `actions_should_stackFullWidthWith44pxTargets_When_390pxTouch` | E2E | Mobile |
| REQ-30 | same | `actions_should_degradeToLegacyApproveDenyWithoutReload_When_flagFlippedOffLive` | E2E | Rollback |
| REQ-25 | same | `tierBAction_should_renderDesktopOnlyAndOpenSession_When_descriptorsFulfilledAvailableFalse` | E2E | Remote (fulfilled descriptors; real `:8444` covered by Go integration below) |
| REQ-21 | `server/remote_listener_actions_integration_test.go` (locate chain via S6) | `RemoteChain_should_return401Or403AndRunNoHandler_When_unauthenticatedInvokeNotificationAction` | Integration | S6 happy-security |
| REQ-21 | same | `RemoteChain_should_rejectForeignHost_And_noStateChangingGet_When_chainWalked` | Integration | S6 |
| REQ-21 | same | `RemoteChain_should_returnForbiddenTierBAndAppliedTierA_When_authenticated` | Integration | ADR-004 |
| REQ-19 | `tests/e2e/notification-inline-actions.spec.ts` | `actionRoundTrip_should_haveP95UnderOneSecond_When_20ApprovalsAnd20BacklogDecisionsOnIsolatedServer` (Task 2.9c; soft on CI) | E2E | Real round trip next to the Go benchmark |
| REQ-30 | same | `actions_should_completeOrFallBackToOpenWithoutReload_When_flagFlippedOffWhileActionInFlight` | E2E | UX-AC57 |
| REQ-18 | `tests/e2e/notification-inline-actions.spec.ts` | `pane_should_renderActionRowsWithin200ms_When_openedFromCachedState` (`performance.measure` between open click and first row; 200 ms budget, annotated soft on CI) | E2E | NFR |
| REQ-20 | `server/services/notification_actions_bench_test.go` | `BenchmarkBuildDescriptors_300RecordsWith40Sessions` | Benchmark | Scalability |
| REQ-20 | `server/services/notification_actions_test.go` | `BuildDescriptors_should_bePureFunctionOfRecord_When_300RecordsViaInjectedPageSize` (small-N proof of linear behavior) | Unit | Scalability |

### Epic 3: Structured AskUserQuestion (flag `askuserquestion_structured`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-23 | `session/ent_repository_guidance_test.go` | `Migration_should_returnLegacyRowWithDefaults_When_schemaAutoMigrated` (`source=agent_mcp`, `answerer_kind=human`, `multi_select=false`) | Migration | See migration section |
| REQ-6 | `session/domain/guidance_request_source_test.go` | `QuestionPayload_should_accept1To4QuestionsWithOptions_When_valid` | Unit | Happy path |
| REQ-6 | same | `QuestionPayload_should_reject_When_zeroOrFiveQuestionsOrLabelTooLongOrTooManyOptions` | Unit | Error path |
| REQ-23 | same | `NewGuidanceRequest_should_returnTypedErrorAndWriteNothing_When_peerSourceWithHumanAnswerer` | Unit | Cross-field rule |
| REQ-23 | same | `NewGuidanceRequest_should_returnTypedError_When_readOnlyRouteWithSentState` | Unit | Cross-field rule |
| REQ-23 | same | `DeliveryState_should_allowOnlyGuardedTransitions_When_allFromToPairsEnumerated` | Unit | Table over all pairs |
| REQ-23 | `session/ent_repository_guidance_test.go` | `CreateHookSource_should_yieldExactlyOneRow_When_8GoroutinesSameAskerAndHookCallID` (`-race`) | Unit | Dedup happy path |
| REQ-23 | same | `CreateHookSource_should_yieldTwoRows_When_sameTextDifferentHookCallID` | Unit | Coalesce bypass |
| REQ-23 | same | `CreateHookSource_should_coalesceByPayloadHash_When_hookCallIDAbsent` | Unit | Documents accepted limitation |
| REQ-23 | same | `PendingCap_should_createHookRow_When_4OpenAgentMcpRows` | Unit | Per-class cap |
| REQ-23 | same | `PendingCap_should_createAgentMcpRow_When_4OpenHookRowsAndSeparately4OpenPeerRows` | Unit | Reverse direction |
| REQ-23 | same | `PendingCap_should_stillRaisePendingCapExceeded_When_fifthAgentMcpRow` | Unit | Legacy behavior preserved |
| REQ-23 | same | `PendingCap_should_returnExistingRowWithoutCapError_When_hookClassAtCapAndSameQuestionReregistered` | Unit | Dedup hit |
| REQ-11 | same | `CreatePeerHelp_should_setScopeSessionScopeKeyAskerSourcePeerAnswererAnd24hExpiry_When_requestHelpAtoB` | Unit | Peer row shape |
| REQ-23 | `session/ent_repository_guidance_roundtrip_test.go` | `GuidanceRequestProto_should_roundTripNewFields_When_converted` | Unit | Proto |
| REQ-6 | `server/services/askuserquestion_parser_test.go` | `ParseAskUserQuestion_should_returnReplyableSingleSelect_When_fixtureSingleSelect` | Unit | Happy path (fixtures from S1) |
| REQ-6 | same | `ParseAskUserQuestion_should_return2QuestionsAndMultiSelectNonReplyable_When_multiTwoQuestionsFixture` | Unit | Happy path |
| REQ-6 | same | `ParseAskUserQuestion_should_returnUnknownShapeKeepQuestionText_When_labelOver80Runes` | Unit | Error path |
| REQ-6 | same | `ParseAskUserQuestion_should_returnUnknownShape_When_moreThan7Options` | Unit | Error path |
| REQ-21 | same | `ParseAskUserQuestion_should_sanitizeHostileText_When_ansiOrControlCharsInLabels` | Unit | Security |
| REQ-6 | same | `ParseAskUserQuestion_should_returnError_When_malformedJSONOrMissingQuestions` | Unit | Error path |
| REQ-6 | `server/services/pending_question_store_test.go` (existing) | existing tests should pass after parser extraction | Unit | Regression |
| REQ-6, REQ-8 | `server/services/guidance_request_hook_mirror_test.go` | `Mirror_should_createSessionScopeHookRowAndMetadataIds_When_verifiedProofRegistersQuestion` | Unit | Happy path |
| REQ-21 | same | `Mirror_should_createReadOnlyRow_When_attributionFromCwdPrefix` | Unit | Error path |
| REQ-8 | same | `Mirror_should_rehydratePendingStoreAndReturnStaleWithZeroBytes_When_restartAndDifferentDialogOnPane` | Integration | Restart (on-disk DB, fake pane) |
| REQ-8 | same | `Mirror_should_beIdempotent_When_registeredTwiceWithSameHookCallID` | Unit | Idempotency |
| REQ-8 | `server/services/guidance_session_reaper_test.go` | `Reaper_should_cancelHookRowWithEndedReason_When_archiveEventPublished` | Unit | Happy path |
| REQ-8 | same | `Reaper_should_cancelRows_When_startupPassFindsDeletedSession` | Integration | Downtime delete |
| REQ-8 | same | `Reaper_should_notCancelAgentMcpSessionGuidance_When_sessionStoppedOrArchived` | Unit | Scope boundary |
| REQ-8 | same | `Reaper_should_resolveTitleToUUIDAndCancelOnlyThatSessionsHookRows_When_eventSessionIdIsTitle` | Unit | Resolve |
| REQ-8 | same (`goleak`) | `Reaper_should_joinSubscriptionGoroutine_When_stopped` | Unit | Lifecycle |
| REQ-8 | `server/services/guidance_dialog_gone_sampler_test.go` | `Sampler_should_cancelWithDialogGone_When_twoConsecutiveNonMatchingSamples3sApart` (fake clock) | Unit | Happy path |
| REQ-8 | same | `Sampler_should_keepRowOpen_When_singleNonMatchThenMatch` | Unit | Edge |
| REQ-8 | same | `Sampler_should_stopPollingButKeepRowOpenAndAnswerable_When_10MinutesDialogStillMatching` (row's `delivery_state` unchanged, descriptors still include `ANSWER_QUESTION`) | Unit | Pre-mortem P1-b: no `stale` by timer |
| REQ-8 | same | `Sampler_should_sampleEvery3sForFirstMinuteThenEvery30s_When_fakeClockAdvances` | Unit | Backoff |
| REQ-8 | same | `Sampler_should_sampleAtMost20RowsPerPass_When_30OpenRows` | Unit | Bound |
| REQ-8 | `server/services/guidance_request_hook_mirror_test.go` | `Mirror_should_reverifyOpenHookRowsOnDemandAndCancelGoneOnes_When_createWouldHitHookClassCap` (4 open rows older than 10 min, 3 dialogs gone: 5th created, no `PENDING_CAP_EXCEEDED`); `Mirror_should_refuseAsToday_When_allFourDialogsStillMatch` | Integration | Cap starvation without a poll |
| REQ-7, REQ-8 | `server/services/question_answer_router_test.go` | `Route_should_writeOneByte1AndReturnApplied_When_rowOpen45MinutesRehydratedAndDialogStillMatches` (UX-AC59); `Route_should_returnStaleWithZeroBytes_When_45MinRowAnsweredInTerminal` | Unit | Late answer from the phone |
| REQ-8 | same | `Mirror_should_mirror6thQuestionAndClearFirst5WithReasons_When_5TerminalAnsweredThen6th` (`guidance_mirror_cleared_total` incremented) | Integration | Cap starvation regression |
| REQ-8 | `server/services/guidance_request_service_test.go` | `Answer_should_createZeroNotifyAnsweredNotifications_When_hookSourceRowAnswered` | Unit | Per-source side effects |
| REQ-8 | same | `Answer_should_notResumeTriage_When_sessionScopeRow` | Unit | Error path |
| REQ-7 | `server/services/question_answer_router_test.go` | `Route_should_writeSingleByte1WithoutEnterAndMoveToSentThenAcknowledged_When_visibleSessionDialogMatches` | Unit | Happy path |
| REQ-8 | same | `Route_should_saveForLater_When_sessionStoppedNotArchived` | Unit | Dead session |
| REQ-8, REQ-26 | same | `Route_should_storeAnswerAndNotCallStartOrRecoverFromStopped_When_sessionArchived` (spy) | Unit | `noarchivedrevival` |
| REQ-7 | same | `Route_should_setUnacknowledgedFlag_When_61sFakeClockWithoutAck` | Unit | UX-AC24 server side |
| REQ-7, REQ-21 | same | `Route_should_returnNotSentHumanActiveWithZeroBytes_When_paneInputWithin5s` | Unit | Guard |
| REQ-7 | same | `Route_should_returnStaleWithZeroBytes_When_dialogMatchFails` | Unit | Error path |
| REQ-7 | same | `Route_should_returnNotSentVsSendIndeterminate_When_faultInjectedBeforeVsAfterWrite` | Unit | Fault injection |
| REQ-7 | same | `Route_should_chooseRouteFromClosedSwitch_When_tableOfSessionStateHarnessStage` | Unit | Route table |
| REQ-9 | same | `Answer_should_logAnswerLatencyMs7500_When_createdAt100000AnsweredAt100007_5` | Unit | Success metric |
| REQ-9, REQ-28 | same | `Answer_should_observeLatencyHistogramWithSourceAndRouteLabels_When_answered` | Unit | Metric |
| REQ-7 | `server/services/hook_injector_test.go` | `HookInjector_should_includeAskUserQuestionPostToolUseMatcher_When_S7ProbeFoundItMissing` | Unit | Task 3.4e (conditional) |
| REQ-7 | `cmd/ssq-hooks/main_test.go` | `PostToolUseAskUserQuestion_should_postReceiptToServer_When_hookFires` | Unit | Ack path |
| REQ-7 | `cmd/ssq-hooks/hook_reply_test.go` (conditional on S1 PASS) | `HookReply_should_returnAllowWithUpdatedInputAnswers_When_preAnsweredRowExists` | Unit | Rung 1 |
| REQ-7 | same | `HookReply_should_exitEmptyAndLeaveNativeDialog_When_noAnswerOrServerUnreachable` | Unit | Fail-open |
| REQ-7 | same | `HookReply_should_notBlockBeyondHookBlockMsCeiling_When_ownerApprovedBoundedWait` (conditional on S2 and owner decision) | Unit | Bound |
| REQ-7 | `server/services/pane_reply_sequence_test.go` (conditional on S1c PASS) | `Sequence_should_replayExactFixtureKeySequenceAndReturnBothAnswers_When_twoQuestionDialogWithMultiSelect` | Unit | Happy path |
| REQ-7 | same | `Sequence_should_abortWithStaleAndNoFurtherWrites_When_dialogMatchFailsAfterStep1` | Unit | Error path |
| REQ-7 | same | `Sequence_should_reportNotSentVsSendIndeterminatePerStage_When_faultInjectedAtEachStep` | Unit | Fault matrix |
| REQ-7 | `session/detection/question_profile_test.go` | `ClaudeProfile_should_wrapExistingDialogMatch_When_realCaptureFixture` | Unit | Strategy |
| REQ-7 | same | `OpenCodeProfile_should_extractOptionsAndRejectForgedFooter_When_fixtureAndForgedVariant` (conditional on S5) | Unit | Forged-text negative |
| REQ-7 | same | `Registry_should_containOnlyMeasuredHarnesses_When_listed` (Aider/pi/Gemini/agy absent unless S5 marks replyable) | Unit | Guard |
| REQ-7 | same | `Detection_should_emitOpenOnlyDescriptor_When_aiderYesNoPromptWithoutFixture` | Unit | Degrade |
| REQ-7 | same | `ReplyableProfile_should_createRowWithSourceAskUserQuestionHookAndProfileName_When_openCodeFixtureDetected` (no `terminal_detect` source exists) (conditional on S5) | Unit | ADR-003 source vocabulary |
| REQ-10 | `server/mcp/tools_guidance_test.go` | `CreateGuidanceRequest_should_storeQuestionsJSONAndQuestionText_When_questionsArgProvided` | Unit | Happy path |
| REQ-10 | same | `CreateGuidanceRequest_should_behaveAsToday_When_newArgsOmitted` | Unit | Backward compat |
| REQ-10 | same | `CreateGuidanceRequest_should_rejectInvalidPayloadAndKeepOwnershipCheck_When_questionsExceedLimits` | Unit | Error path |
| REQ-6, REQ-7 | `web-app/src/components/guidance/GuidanceQuestionForm.test.tsx` | `GuidanceQuestionForm_should_renderRadiogroupAndCheckboxesAndOneSubmit_When_singleAndMultiSelectPayload` | Unit | Happy path |
| REQ-6 | same | `GuidanceQuestionForm_should_sendOnOptionTapWithoutSubmit_When_singleQuestionSingleSelect` | Unit | UX-AC18 |
| REQ-6 | same | `GuidanceQuestionForm_should_disableSubmitWithHelperText_When_secondQuestionUnanswered` | Unit | UX-AC20 |
| REQ-6 | same | `GuidanceQuestionForm_should_renderNativeSelect_When_moreThan8Options` | Unit | UX-AC21 |
| REQ-6 | same | `GuidanceQuestionForm_should_showInlineErrorSetAriaInvalidAndFocusFirstInvalid_When_submitEmpty` | Unit | UX-AC22 |
| REQ-6 | same | `GuidanceQuestionForm_should_renderReadOnlyTextWithOpenSession_When_unmeasuredOrUnrenderable` | Unit | UX-AC28 |
| REQ-8 | same | `DeliveryStateLine_should_renderExactCopyForEachState_When_tableOfEightStates` | Unit | UX-AC23 |
| REQ-6 | `web-app/src/components/guidance/GuidanceRequestPanel.test.tsx` (existing) | existing suite should pass unchanged after extraction | Unit | Regression (Task 3.7a) |
| REQ-6, REQ-7 | `tests/e2e/askuserquestion-structured.spec.ts` (new) | `structuredQuestion_should_sendByte1AndShowAnsweredRedis_When_optionClickedOnFixtureDialogSession` (real private-socket session, fake dialog fixture) | E2E | Happy path (anchor) |
| REQ-6 | same | `structuredQuestion_should_showAllOptionsAndMultiSelectGroups_When_multiQuestionPayload` (conditional on S1c) | E2E | Render |
| REQ-8 | same | `structuredQuestion_should_showSavedAnswerCopyAndNoRevive_When_sessionArchived` | E2E | Dead session, UX-AC27 |
| REQ-7 | same | `structuredQuestion_should_flipToHandledWithPoliteAnnouncement_When_answeredInTerminal` | E2E | Race, UX-AC26 |
| REQ-25 | same (phone, `hasTouch`) | `structuredQuestion_should_stackOptionsAndPinSubmit_When_390pxTouch` | E2E | Mobile |
| REQ-7 | `session/dialog_fixture_integration_test.go` | `DialogMatch_should_matchRealCapturesAndRejectForgedFooter_When_fixturesFromS1c` (captures in `session/testdata/reply_dialogs/`) | Integration | Fixture-driven |

### Epic 4: Agent messaging and durable wake triggers (flag `agent_wake_triggers`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-16 | `session/ent_repository_wake_test.go` | `Create_should_returnFirstRow_When_sameDedupKeyTwice` | Unit | Happy path |
| REQ-16 | same (`-race`) | `Claim_should_applyExactlyOnceAndLeaveDeliveredAtNull_When_8GoroutinesClaimSameID` | Unit | Race |
| REQ-16 | same | `MarkDelivered_should_setDeliveredAtOnceAndFireCount1_When_calledTwice` | Unit | Idempotent |
| REQ-16 | same | `MarkMissed_should_setMissedWithNullClaimAndDelivered_When_expiredPending` | Unit | Error path |
| REQ-16 | same | `Transitions_should_acceptOnlyTable_When_allStatusPairsEnumerated` (rejects `pending->delivered`, any move out of terminal, `delivered->claimed`) | Unit | Table test |
| REQ-16 | same | `Cancel_should_applyFromPendingClaimedNeedsRevive_And_noopOnDelivered` | Unit | Table |
| REQ-16 | same | `MarkSkipped_should_applyFromClaimedAndNeedsRevive` | Unit | Guard |
| REQ-12 | same | `CronRow_should_rearmClaimedToPendingWithAdvancedNextFire_And_endDeliveredAfterMaxFires3` | Unit | Cron lifecycle |
| REQ-20 | same | `Create_should_enforceOwnerCountInSameTransaction_When_concurrentInserts` (`-race`) | Unit | Cap atomicity |
| REQ-12 | `server/wakes/timer_test.go` | `Timer_should_claimOnce_When_fakeClockAdvances2sForAtTrigger` | Unit | Happy path |
| REQ-12 | same | `Timer_should_rearmEarlier_When_triggerCreatedBeforeCurrentMin_And_returnToLaterOnCancel` | Unit | Re-arm |
| REQ-12 | same | `NextCronTime_should_return1005AndRearm1010_When_spec5MinAndFakeTime1002` | Unit | Cron |
| REQ-12 | same | `NextCronTime_should_returnError_When_specInvalid` | Unit | Error path |
| REQ-12 | same (`goleak`) | `Timer_should_exitAllGoroutinesBeforeStopReturns_When_shutdown` | Unit | Lifecycle |
| REQ-12 | same | `Timer_should_claimDueTrigger_When_workflowSchedulerNotConstructed` | Unit | No coupling |
| REQ-23 | `server/workflows/scheduler_test.go` (existing) | existing suite should pass unchanged | Unit | `workflows.Scheduler` untouched |
| REQ-16 | `server/wakes/dispatcher_test.go` | `Dispatcher_should_deliverOnce_When_eventPublishedBetweenSubscribeAndInitialRead` | Unit | Subscribe-before-read |
| REQ-16 | same | `Dispatcher_should_keepRowClaimedAndWriteNothing_When_ownerMidTurn_And_deliverOnceOnIdle` | Unit | Busy owner |
| REQ-16 | same | `Reconcile_should_notRequeueLiveHeldClaim_When_periodicPassAndOwnerBusy3MinOld` | Unit | Lease startup-only |
| REQ-16 | same | `Reconcile_should_requeueStaleClaimOnce_When_startupAfterRestart` | Unit | Restart |
| REQ-16 | same | `Reconcile_should_deliver3MissOneAndChangeNothingOnSecondRun_When_fourRowRestartScenario` (`requeued_claimed=1`) | Integration | Restart reconcile (on-disk DB) |
| REQ-26 | same | `Guards_should_skipWithArchivedReasonAndCallNoRevive_When_ownerArchived` (spy) | Unit | `noarchivedrevival` |
| REQ-26 | same | `Guards_should_skip_When_ownerOneShotOrBacklogItemTerminal` | Unit | Guards (`steer_text`) |
| REQ-26 | same | `Guards_should_skipOwnerStoppedAndNeverEnterNeedsRevive_When_allowReviveFalseDefault` (spy: no `Start`/`RecoverFromStopped`; failure-class notification) | Unit | B3: single decision table |
| REQ-26 | same | `NotifyOperator_should_deliverAndSkipEveryGuard_When_ownerArchivedOneShotItemTerminalStoppedOrDeleted` (spy: no revive call; deleted owner text "Session no longer exists") | Unit | B4: guards scoped by action kind |
| REQ-33 | `server/deliverygate/policy_test.go` | `ShouldDeliver_should_deliverReminderNotification_When_noSessionBindingAndTargetInMetadata` (`VisibilityNotASession`); `ShouldDeliver_should_suppressInfoNotification_When_boundToHiddenSession` (documents why) | Unit | B4: gate behaviour |
| REQ-21, REQ-26 | `server/wakes/delivery_test.go` | `SteerDelivery_should_writeZeroBytesAndStayClaimed_When_ownerStatusIdleButPaneShowsAskUserQuestionDialog` (fixture `reply_dialogs/question_single_select_3opts.txt`, shared fault double from Task 2.4a) | Unit | Pre-mortem P1-c |
| REQ-21 | same | `SteerDelivery_should_writeZeroBytes_When_captureErrorUnrecognizedLayoutOrPermissionDialog` | Unit | Fail closed |
| REQ-16 | same | `SteerDelivery_should_deliverOnceAndBeginPayloadWithWakeMarker_When_dialogClearsAndFreshCaptureShowsComposer` | Unit | Release path |
| REQ-21 | `server/wakes/pane_dialog_test.go` | `PaneShowsDialog_should_detectFooterPermissionAndUnknownLayout_And_notFlagIdleComposer_When_tableOfCaptures` (existing `reply_dialogs/` captures) | Unit | Generic negative check |
| REQ-26 | `server/wakes/revive_test.go` | `ReviveStoppedOwner_should_returnWithoutRecoverOrStart_When_archivedBetweenClaimAndRevive` | Unit | Own `IsArchived()` check |
| REQ-26 | same | `ReviveStoppedOwner_should_mark_needs_revive_When_wipCapFull_And_reviveOnceWhenSlotFrees` | Unit | Admission |
| REQ-26 | same (`-race`) | `ReviveStoppedOwner_should_reviveExactlyOne_When_3StoppedOwners1FreeSlotRacing` | Unit | Lock spans `Admit` |
| REQ-26 | same | `ReviveStoppedOwner_should_notRevive_When_allowReviveFalseDefault` | Unit | ADR-007 default |
| REQ-26 | `tools/lint/noarchivedrevival/analyzer_test.go` + `testdata/` | `Analyzer_should_reportUnguardedReviveInServerWakesRevive_And_notReportGuarded` | Unit | Task 4.3f |
| REQ-26 | `make lint-custom` | `noarchivedrevival_should_report_zero_findings_on_server_wakes` | Doc-check | CI |
| REQ-20 | `server/wakes/caps_test.go` (`-race`) | `Caps_should_admitExactly5Of20_When_perSessionCap5And20ConcurrentSchedules` | Unit | Boundary `>` vs `>=` |
| REQ-20 | `server/wakes/limits_test.go` | `Limits_should_enforceEveryRowOfTheWakeLimitsTableAtItsPoint_When_tableDriven` (12 rows: default accepted, one past the ceiling refused naming the limit; config cannot raise a ceiling) | Unit | Pre-mortem P1-d |
| REQ-20 | same | `Limits_should_refuseEveryMinuteAndFiveMinuteCron_AtDefaultFloor_And_acceptFiveMinuteWhenConfigLowered_And_maxFires49_And_expiry8Days_And_generation6` | Unit | Create-time limits |
| REQ-20 | same | `Limits_should_deliver6Defer7thRaiseOneDigestAndCountMetric_When_oneOwnerSevenWakesInAnHour` | Unit | Fire-time per-owner limit |
| REQ-20 | same | `Limits_should_deferGlobal61st_And_skipRateLimitedAfter3ConsecutiveDeferralsForOneShot_And_skipOnlyTheOccurrenceForCron` | Unit | Fire-time global limit and bound |
| REQ-28 | same | `Limits_should_incrementWakeLimitHitTotalWithLimitAndPhaseLabels_And_setWakeDeliveredPerOwnerHour_When_limitsHit` | Unit | Metric |
| REQ-20 | same | `Caps_should_returnCapExceededVisibleResult_When_overCap` | Unit | Error path |
| REQ-20 | same | `Caps_should_enforceMaxFiresGlobalBudgetAndGenerationLimit_When_chainedWakes` | Unit | Chain bound |
| REQ-13 | `server/wakes/debounce_test.go` | `Debounce_should_deliverOne_When_10VerdictRecordedEventsIn2sWindow5s` | Unit | Coalescing |
| REQ-23 | `server/wakes/imports_test.go` | `Wakes_should_notDependOnServicesOrMcp_When_goListDepsRun` | Unit | Import direction |
| REQ-13 | `server/wakes/sources_backlog_test.go` | `BacklogSource_should_fire_When_verdictRecordedEventForItem` | Unit | Happy path |
| REQ-13 | `pkg/events/types_test.go` | `TestEveryBacklogKindMapped` (fails when `mapBacklogChangeKind` lacks a kind used by a trigger) | Unit | Completeness |
| REQ-13 | `server/wakes/predicates_test.go` | `ScheduleWake_should_returnNoReconcilePredicateAndCreateNoRow_When_bareEdgeKind` | Unit | Error path |
| REQ-13 | same | `ScheduleWake_should_acceptStatusReachedAndRejectBareStatusTransitionItemUpdatedTriageProgressActivityNote_When_tableRun` | Unit | Story 4.4 `(type, kind, predicate)` table |
| REQ-13, REQ-16 | same | `Reconcile_should_fireOnce_When_verdictConditionBecameTrueWhileDown` | Integration | Restart + predicate |
| REQ-13 | same | `TestEveryKindHasReconcilePredicateOrIsRejected` | Unit | Table completeness |
| REQ-13 | `server/wakes/sources_pr_test.go` | `PrSource_should_fireWithConclusionSuccess_When_checksComplete` (poller-derived or bus per S4) | Unit | Happy path |
| REQ-13 | same | `ScheduleWake_should_returnUnavailable_When_S4FoundNoPrCiSource` | Unit | Fallback |
| REQ-14 | `server/wakes/sources_session_test.go` | `SessionStateSource_should_fireOnce_When_peerBecomesIdle` | Unit | Happy path |
| REQ-14 | same | `SessionStateSource_should_fireOnce_When_peerAlreadyIdleAtSubscribe` | Unit | Edge |
| REQ-14 | same | `SessionStateSource_should_reevaluateAtReconcile_When_restart` | Integration | Restart |
| REQ-15 | `server/wakes/sources_guidance_test.go` | `GuidanceAnswered_should_claimOnce_When_answerListenerCallbackRuns` | Unit | Happy path |
| REQ-15, REQ-16 | same | `GuidanceAnswered_should_deliverOnceViaReconcile_When_restartBeforeDelivery` | Integration | Restart |
| REQ-15 | same | `GuidanceAnswered_should_fireImmediately_When_rowAlreadyAnsweredAtSchedule` | Unit | Edge |
| REQ-21 | `server/mcp/caller_identity_gate_test.go` | `Registration_should_wrapAllFiveToolsInGate_And_failWhenUnwrappedToolAdded` | Unit | Registration guard |
| REQ-21 | same | `Gate_should_returnIdentityUnverifiedWithZeroWrites_When_forgedUuidHeaderNoProof` (per tool x5) | Unit | Forgery |
| REQ-21 | same | `Gate_should_returnIdentityUnverified_When_proofOfAWithUuidOfB` | Unit | Cross-session |
| REQ-21 | same | `Gate_should_recordVerifiedCallerAsOwner_When_sessionIdArgPassed` | Unit | Argument ignored |
| REQ-21 | same | `Gate_should_denyPerRoleAllowlist_When_roleNotAllowed` | Unit | Allowlist (owner table pending) |
| REQ-12 | `server/mcp/tools_wake_test.go` | `ScheduleWake_should_createDelayTrigger_When_validSpecAndVerifiedCaller` | Unit | Happy path |
| REQ-12 | same | `ScheduleWake_should_returnTypedFlagOffCapExceededUnavailable_When_conditions` | Unit | Typed errors |
| REQ-12 | same | `ScheduleWake_should_returnEventStreamUnavailable_When_eventBusNil` | Unit | Error path |
| REQ-12 | same | `ListWakes_and_CancelWake_should_onlyAffectCallersOwnTriggers_When_otherOwnersExist` | Unit | Ownership |
| REQ-11 | `server/mcp/tools_peer_help_test.go` | `RequestHelp_should_createPeerRowAndWatchForAnswer_When_validPeer` | Unit | Happy path |
| REQ-11 | same | `RequestHelp_should_returnTypedRejectedAndCreateNoRow_When_selfTargetArchivedOneShotOrCrossWorkspace` (4 subtests; `peer_help_total{state="rejected"}`) | Unit | Error path (Story 4.11, Task 4.11c) |
| REQ-11 | same | `AnswerHelp_should_returnForbiddenAndLeaveRowUnchanged_When_nonAnswererCalls` | Unit | Authorization |
| REQ-11 | same | `AnswerHelp_should_answerWithProvenanceAndFireGuidanceAnsweredWakeWithoutNotifyAnswered_When_answererCalls` | Unit | Happy path |
| REQ-10 | same | `RequestHelp_should_aliasCreateGuidanceRequestWithNoNewStore_When_peerHuman` | Unit | Human help |
| REQ-10 | `server/mcp/tools_guidance_test.go` | `CreateGuidanceRequest_should_raiseOperatorCardAndNotification_When_agentAsksHuman` | Integration | Human help end to end |
| REQ-11, REQ-15 | `server/wakes/peer_help_integration_test.go` | `PeerHelp_should_deliverAnswerToAskerExactlyOnce_When_peerAnswersThenServiceRestarts` | Integration | Restart |
| REQ-21 | `server/services/mcp_proof_test.go` | `McpProofs_should_verifyForOwnUuidOnly_And_beDomainSeparatedFromHookProofs` | Unit | Happy + error |
| REQ-21 | same | `McpProofs_should_failVerification_When_rotated` | Unit | Rotation revokes |
| REQ-21 | `session/instance_tmux_test.go` (existing, update) | `ClaudeMCPConfigArgs_should_returnFilePathWith0600FileAnd0700Dir_When_requireLocalAuthOff` | Unit | Always-file route |
| REQ-21 | same | `ClaudeMCPConfigArgs_should_notContainProofInArgvOrExtraEnv_When_commandBuilt` | Unit | Non-leak |
| REQ-21 | same | `ClaudeMCPConfigArgs_should_failClosedWithNoProofInline_When_mcpDirUnwritable` | Unit | Fail closed (forced write failure) |
| REQ-21 | same | `Instance_should_emitNoProofHeaderAndNotFail_When_providerNil` | Unit | Boot window |
| REQ-21 | `session/imports_test.go` | `Session_should_notDependOnServerServices_When_goListDepsRun` | Unit | Import direction |
| REQ-21 | `server/server_mcp_identity_test.go` | `McpWithUUID_should_setVerifiedIdentityOnlyWhenProofVerifies_And_keepLegacyPathUnchanged` | Unit | Middleware |
| REQ-21 | same | `NewTools_should_returnIdentityUnverifiedWithRelaunchCopy_When_sessionPredatesUpgrade` | Unit | Degraded |
| REQ-21 | `server/mcp/proxy_test.go` | `ProxyHeaders_should_returnVerifiableProof_When_proofFilePresent_And_noHeaderWhenRemoved` | Unit | Stdio path |
| REQ-21 | `server/mcp/spawn_path_matrix_test.go` | `ProofMinting_should_skipS5FailedNonClaudeAndSsqMuxSessions_When_matrixTableRun` | Unit | Matrix fail-closed rows |
| REQ-21 | same | `ProofMinting_should_mintVerifiableProofWithNoArgvOrEnvLeak_When_harnessS5MarkedProofCapable` (conditional on S5, `t.Skip("blocked on S5: <harness>")`; Task 4.9g) | Unit | B2 |
| REQ-21 | same | `RunStdio_should_returnIdentityUnverified_When_inProcess` | Unit | Matrix |
| REQ-21 | `server/services/mcp_proof_lifecycle_test.go` | `ProofFiles_should_beDeletedAndLaterProofRejected_When_sessionDeleted` | Unit | Lifetime |
| REQ-21 | same | `ProofSweep_should_removeStaleFiles_When_startupFindsUnknownUuid` | Integration | Startup sweep |
| REQ-21 | `server/mcp/tools_terminal_test.go` | `ReadSessionOutput_should_replaceProofValueWithRedacted_When_paneContainsHeader` (also `wait_for_output`) | Unit | Mitigation |
| REQ-21 | `session/instance_tmux_spawn_test.go` | `SpawnedClaudeSession_should_carryVerifiableProofReachingMcpWithUuid_When_workSessionBuilt` | Integration | Spawned-session verify |
| REQ-12, REQ-13 | `server/services/wake_trigger_service_test.go` | `ListWakeTriggers_should_returnOwnerConditionSinceNextFire_When_twoPending` | Unit | Operator visibility |
| REQ-12 | same | `CancelWakeTrigger_should_setCancelled_When_pending` / `CancelWakeTrigger_should_returnAlreadyFiredAtTime_When_firedMeanwhile` | Unit | Cancel race |
| REQ-11 | `server/services/notification_actions_test.go` | `CancelPeerHelp_should_cancelRowAndFireCancelledGuidanceAnsweredWake_When_invoked` | Unit | `CANCEL_PEER_HELP` |
| REQ-11 | same | `AnswerForThem_should_showAlreadyAnsweredBySessionValue_When_peerAnsweredFirst` | Unit | Race |
| REQ-29 | `server/wakes/notify_test.go` | `Missed_should_raiseFailureClassOperatorNotificationDeliveredForHiddenOwner_When_expiredTriggerAtRestart` | Unit | Alert, gate class |
| REQ-30 | `server/wakes/kill_switch_test.go` | `Dispatcher_should_stopFiringAndKeepRows_When_flagTurnedOff_And_resumeWithReconcilePolicy` | Unit | Rollback |
| REQ-17 | `server/mcp/schedulewakeup_guard_test.go` | `ScheduleWakeupReferences_should_matchPerFileMap_When_gitGrepCounted` (exact map of 8 files) | Unit | Zero new poll loops |
| REQ-17 | `session/backlog_context_test.go` | `BacklogPrompt_should_referenceScheduleWakeAndForbidScheduleWakeup_When_flagOn` | Unit | Prompt |
| REQ-17 | same | `BacklogPrompt_should_equalStoredGolden_When_flagOff` | Unit | Rollback |
| REQ-16 | `server/wakes/integration_test.go` | `Restart_should_deliverEachOfThreePendingTriggersOnceAndMatchReconcileCounters_When_serverRestartsOnSameDB` | Integration | Success metric |
| REQ-16 | `server/wakes/restart_gap_test.go` | `RestartGap_should_deliverOnce_When_killedAfterClaimBeforePaneWrite` | Integration | Claim/delivery split |
| REQ-16 | same | `RestartGap_should_notResend_When_rowAlreadyDelivered` | Integration | Exactly-once |
| REQ-16 | same | `RestartGap_should_deliverOnceOnIdle_When_busyOwnerAtRestart` | Integration | Busy |
| REQ-20, REQ-26 | `server/wakes/storm_test.go` (`-race`) | `Storm_should_reviveAtMost2PerCycleAndLoseNone_When_50AllowReviveTriggersOnItemX` | Integration | Storm at small N via injected cap + fake owners |
| REQ-20 | same | `Storm_should_neverExceed6DeliveriesPerOwnerHourAnd60Global_When_cronChainReschedulingFor3SimulatedHours` (fake clock; `generation` never above 5; `wake_limit_hit_total` non-zero) | Integration | Task 4.8d numeric ceiling |
| REQ-20 | same | `Storm_should_wakeLiveOwnersWithoutRevive_When_mixedLiveAndStopped` | Integration | Mixed |
| REQ-20 | `server/wakes/wakes_bench_test.go` | `BenchmarkDispatcherReconcile_1000Rows` | Benchmark | Scale kept out of `Test` |
| REQ-12..REQ-16 | `web-app/src/components/ui/ScheduledPrompts.test.tsx` | `ScheduledPrompts_should_listOwnerPlainLanguageConditionSinceNextFireAndCancel_When_twoPending` | Unit | UX-AC35 |
| REQ-12 | same | `ScheduledPrompts_should_showCancelConfirmThenPromptCancelledAnnouncement_When_cancelConfirmed` | Unit | UX-AC36 |
| REQ-12 | same | `ScheduledPrompts_should_showAlreadyFiredAtTimeWithoutToast_When_cancelRaces` | Unit | UX-AC37 |
| REQ-20 | same | `ScheduledPrompts_should_render20RowsWithShowAllN_When_25Pending` | Unit | Task 4.6d cap |
| REQ-12 | `server/services/wake_trigger_service_test.go` | `RemindMe_should_emitSystemLevelNotificationWithTargetSessionMetadata_When_ownerArchivedOrStopped` (last wave; Remind me ships) | Unit | B4 end to end |
| REQ-12 | same | `ScheduledPrompts_should_showCouldntLoadWithRetry_When_listFails` | Unit | Error path |
| REQ-12 | same | `ScheduledPrompts_should_showMissedReasonExceptionRowWithCancelAndOpenOwner_When_missed` | Unit | UX-AC39 |
| REQ-30 | same | `ScheduledPrompts_should_showScheduledPromptsPausedBannerResumeConfirmAndFlagLink_When_flagOff` | Unit | UX-AC38 |
| REQ-11 | `web-app/src/components/ui/PeerHelpRow.test.tsx` | `PeerHelpRow_should_showAskerAnswererQuestionAgeState_And_notCountInBadge_When_waiting` | Unit | UX-AC31 |
| REQ-11 | same | `PeerHelpRow_should_offerAnswerForPeerCancelRequestOpen_When_escalated` | Unit | UX-AC32 |
| REQ-11 | same | `PeerHelpRow_should_showNotSentPlainReason_When_acceptedThenAnswererDeletedOrArchived` | Unit | UX-AC33 (call-time rejections create no row) |
| REQ-11 | `server/wakes/escalator_test.go` (`goleak`) | `Escalator_should_raiseExactlyOneInputRequiredWithThreeDescriptors_When_10MinutesWithoutAnswerOnFakeClock` | Unit | Story 4.11 emitter |
| REQ-11 | same | `Escalator_should_escalateImmediately_When_answererStoppedArchivedOrDeleted` / `_notEscalateAndDropDeadline_When_answeredAtMinute4` | Unit | Event path |
| REQ-11 | same | `Escalator_should_createNoDuplicateNotification_When_restartAndSecondPass` (dedup key = request id) | Integration | Restart |
| REQ-11 | `server/deliverygate/policy_test.go` | `ShouldDeliver_should_deliverPeerHelpEscalation_When_hiddenAsker` | Unit | Gate-class table row |
| REQ-12 | `web-app/src/components/ui/RemindMeMenu.test.tsx` (last wave, default presets) | `RemindMeMenu_should_createNotifyOperatorTriggerAndShowUndo10s_When_presetPicked` | Unit | UX-AC40 |
| REQ-12 | same | `RemindMeMenu_should_rejectPastTimeInline_And_showCapCopy_And_createSingleTrigger_When_duplicateTap` | Unit | UX-AC41 |
| REQ-12 | same | `RemindMeMenu_should_beAbsent_When_rowIsBlockingQuestionOrApproval` | Unit | Scope guard |
| REQ-12..REQ-16 | `tests/e2e/agent-wake-triggers.spec.ts` (new) | `waitingOn_should_listAndCancelPendingWake_When_triggerSeededViaRpc` | E2E | Happy path |
| REQ-30 | same | `scheduledPrompts_should_showPausedAndResume_When_flagOffThenResumeScheduledPrompts` | E2E | Kill switch |
| REQ-11 | same | `peerHelp_should_escalateToNeedsYouWithAnswerForPeerAndCancelRequest_When_stalled` | E2E | Exit paths |
| REQ-25 | same (phone) | `waitingOn_should_use44pxTargetsAndSingleScroll_When_390pxTouch` | E2E | Mobile |

### Epic 5: Observability, docs, rollout

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-28 | `server/services/notification_action_metrics_test.go` | `Metrics_should_incrementKindApproveOutcomeApplied_And_kindAnswerQuestionOutcomeStale_When_scraped` | Unit | Happy path |
| REQ-28 | same | `Metrics_should_registerEveryNameInObservabilityPlan_When_registryListed` (table over the 27 names in the plan's Observability Plan Metrics bullet, recounted by grep after the triad round 2 repair) | Unit | Completeness |
| REQ-28 | `server/wakes/metrics_test.go` | `WakeMetrics_should_countScheduledClaimedDeliveredNeedsReviveMissedCancelledSkipped_And_lagHistogram` | Unit | Lifecycle |
| REQ-28 | same | `ReconcileMetrics_should_countRequeuedClaimed_When_restart` | Integration | Restart |
| REQ-28 | `server/services/logging_fields_test.go` | `Logs_should_carryQuestionIdTriggerIdActionKindAndTransitionFields_When_oneLinePerTransition` (log-capture helper pattern from `server/deliverygate/log_assertions_guard_test.go`) | Unit | Log fields |
| REQ-29 | `server/wakes/alerts_test.go` | `Alert_should_raiseOperatorNotificationReferencingTriggerID_When_restartMissesExpiredTrigger` | Integration | Missed wake alert |
| REQ-29 | same | `Alert_should_countStaleUnrequeuedClaimAsMissed_When_reconcileCannotRequeue` | Unit | No silent loss |
| REQ-29 | `server/services/notification_action_alerts_test.go` | `Alert_should_raiseOperatorNotification_When_moreThan3NotSentOrSendIndeterminateIn10Min` (fake clock, boundary 3 vs 4) | Unit | Delivery failure alert |
| REQ-29 | same | `Alert_should_surfaceDeliveryFailedOnCard_When_answerDeliveryFailed` | Unit | Visible failure |
| REQ-32 | `docs` check | `three_docs_should_exist_and_be_indexed_in_claude_md` | Doc-check | Story 5.2 |
| REQ-22, REQ-30 | `docs/how-to/verify-notification-actions-rollout.md` | `rollout_checklist_should_record_output_for_each_flag_flip_and_rollback` (manual, output pasted to PR) | Doc-check | Story 5.3 |

### Migration (Step 5)

The plan's migration is additive-only (ent auto-migrate, no hand-written SQL, no `DROP`), so "down" means: the pre-migration reader still works on the migrated database, and flags-off makes the new columns/table inert. `migration_should_be_reversible` is therefore defined as up, verify, then old-reader/flag-off verify (there is no schema-down step by design, per the plan's rollback procedure).

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-23, REQ-30 | `session/ent_repository_guidance_migration_test.go` | `migration_should_be_reversible` (1. create DB with legacy schema and one `scope=backlog-item, question_type=yes-no` row; 2. run `client.Schema.Create` under `EntSchemaCreateMu`; 3. assert new columns exist with defaults `source='agent_mcp'`, `answerer_kind='human'`, `multi_select=false`, `delivery_attempts=0`, nullable others, and `wake_triggers` table with unique `dedup_key` index and `(status,next_fire_at)` index; 4. "down": query using only the legacy column list and assert the original row is byte-identical; flip all flags off and assert no new rows written; 5. re-run `Schema.Create` and assert idempotent) | Migration | Up, verify, legacy-reader and flag-off verify |
| REQ-23 | same | `migration_should_leaveLegacyRowsReadable_When_runTwiceOnPopulatedDatabase` | Migration | Idempotence on populated DB |

## UX Acceptance Tests

All 88 `design/ux.md` section 10 criteria are mapped (UX-AC1 to UX-AC59 below, UX-AC60 to UX-AC88 in "Triad round 1 additions"), one row each. "Playwright" rows use the phone profile where the criterion is mobile-specific. Component-level Jest rows are used where the behavior is pure DOM/ARIA and the Playwright server cannot reproduce the state (remote listener, offline, 60 s timers); each of those also has a Playwright row in the requirement tables where a real-page check adds value. Items that cannot be proven by automation alone carry a Manual step.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-AC1: 11 hidden sessions, no history: 11 rows + roll-up, never "healthy" | `tests/e2e/background-activity.spec.ts` | `ux_ac1_should_show11RowsAndRollupWithoutHealthy_When_11HiddenNoHistory` | Playwright | Fulfil `ListSessions{hiddenOnly}` with 11 stopped; open Background; assert row count 11, `background-rollup` contains "no failures reported", text lacks "healthy", a "Finished" chip exists only for rows whose outcome is `completed`, and rows with unknown outcome show "Stopped, outcome unknown" |
| UX-AC2: zero sessions copy, no spinner | same | `ux_ac2_should_showNoBackgroundWorkCopyAndNoSpinner_When_zeroSessions` | Playwright | Empty list; assert copy and no `role=progressbar` |
| UX-AC3: "Needs attention" group, once per session | same | `ux_ac3_should_pinFailedSessionAboveRollupOnce_When_errorRecordExists` | Playwright | Send real ERROR notification for one hidden id; assert group heading precedes roll-up and id appears once |
| UX-AC4: "N new" no reorder, announce | same | `ux_ac4_should_keepRowsAndAnnounceNNew_When_pollAddsSessions` | Playwright | Capture row order, add session to next poll, assert unchanged order + button; click; assert merged and status text "1 new background session" |
| UX-AC5: poll failure banner + working Retry | same | `ux_ac5_should_showCouldntRefreshBannerAndRetryRecovers_When_listFails` | Playwright | Fail then succeed ListSessions; assert banner text with time, click Retry, banner gone |
| UX-AC6: departed Open copy | same | `ux_ac6_should_showSessionNoLongerExists_When_openOnDepartedRow` | Playwright | Remove session between polls; click Open; assert text, URL unchanged |
| UX-AC7: tab name carries exception count only; arrows | `NotificationPanel.test.tsx` + `background-activity.spec.ts` | `ux_ac7_should_nameTabWithExceptionCountOnlyAndMoveSelectionOnArrowRight` | Jest + Playwright | `getByRole('tab', {name:'Background, 1 needs attention'})`; add an informational row and assert the name is unchanged; ArrowRight/Left/Home/End; assert `aria-selected` |
| UX-AC8: approval, question, decision in one click | `tests/e2e/notification-inline-actions.spec.ts` | `ux_ac8_should_resolveApprovalQuestionAndDecisionWithOneClick_Deny_and_Merge_two` | Playwright | Seed each; open pane; count clicks via action log (1; Deny reason and Merge confirm = 2) |
| UX-AC9: disabled + busy label within 100 ms, second press no-op | `ActionRow.test.tsx` + e2e | `ux_ac9_should_disableAndShowBusyWithin100ms_And_secondPressSendsNothing` | Jest + Playwright | Delay RPC route; click twice; assert `aria-busy`, one request |
| UX-AC10: already handled, no role=alert | `ActionRow.test.tsx` + e2e | `ux_ac10_should_showAlreadyHandledAndCreateNoRoleAlert_When_alreadyResolvedOrStale` | Jest + Playwright | Answer via RPC first; click; `locator('[role=alert]')` count 0 |
| UX-AC11: distinct NOT_SENT and SEND_INDETERMINATE copy | `ActionRow.test.tsx` | `ux_ac11_should_showNotSentWithRetry_And_showIndeterminateWithOpenAndConfirmedRetry_And_notShowAnswered` | Jest | Return each outcome; assert the two strings differ, `NOT_SENT` offers Retry + Open, `SEND_INDETERMINATE` offers Open and a Retry behind "Retry anyway?", no "Answered" |
| UX-AC12: transient failure Retry reuses key | `ActionRow.test.tsx` + e2e | `ux_ac12_should_retryWithSameKeyAndProduceOneWrite_When_transientFailure` | Jest + Playwright | Abort first request; click Retry; assert same `idempotency_key`, single side effect |
| UX-AC13: Tier B aria-disabled "Answer from desktop"; Tier A works | `ActionRow.test.tsx`, e2e (fulfilled descriptors), Go `RemoteChain_*` | `ux_ac13_should_renderTierBAriaDisabledWithDescription_And_tierAWorks` | Jest + Playwright + Go | Descriptor `available=false`; assert `aria-disabled`, `aria-describedby` text, focusable, Open enabled; Tier A button enabled |
| UX-AC14: Open always; unknown kind only Open | `ActionRow.test.tsx` | `ux_ac14_should_alwaysRenderOpen_And_unknownKindRendersOnlyOpen` | Jest | Table of descriptor lists |
| UX-AC15: inline confirm, focus to Confirm merge, Back restores | `ActionRow.test.tsx` + e2e | `ux_ac15_should_confirmInlineWithoutModalAndFocusConfirmMerge_And_backRestores` | Jest + Playwright | Click Merge; assert no `role=dialog`, focus on "Confirm merge"; Back restores row and draft; no button named "Cancel" |
| UX-AC16: no dismiss on actionable unread; Mark all read leaves them | `NotificationPanel.test.tsx` | `ux_ac16_should_hideDismissAndKeepUnread_When_actionableAndMarkAllRead` | Jest | Seed actionable unread; assert no dismiss control; mark all read; still unread |
| UX-AC17: RATE_LIMITED countdown text, re-enable at zero | `ActionRow.test.tsx` | `ux_ac17_should_showCountdownTextAndReenableAtZero_When_rateLimited` | Jest (fake timers) | Advance timers; assert text sequence and `aria-disabled` removed |
| UX-AC18: single-select option buttons, activation sends, browsing never sends | `GuidanceQuestionForm.test.tsx` + `askuserquestion-structured.spec.ts` | `ux_ac18_should_renderButtonGroupAndSendOnlyOnActivation_And_sendNothingWhenBrowsingByKeyboard` | Jest + Playwright | 2-4 options; no `radio`/`radiogroup` role; Tab/ArrowDown/ArrowUp/digits across all options = zero RPCs; Enter on option 2 = one RPC and card locked |
| UX-AC19: multi-select checkboxes + "Submit answers" + Clear + visible count | same | `ux_ac19_should_renderCheckboxGroupSubmitClearAndVisibleNonLiveCount` | Jest + Playwright | Toggle 3 boxes; text "3 selected" is present and not in a live region |
| UX-AC20: two-question payload, Submit disabled with helper | same | `ux_ac20_should_disableSubmitWithHelperUntilAllGroupsAnswered` | Jest | Answer Q1 only; assert helper "Answer question 2 to continue" |
| UX-AC21: >8 options select; free text 16px; Cmd/Ctrl+Enter | same | `ux_ac21_should_renderNativeSelect_And_freeTextFont16pxAndCtrlEnterSends_OnlyOnTextCarryingRoutes` | Jest + Playwright | Computed `font-size`>=16px; keyboard send; no free-text field on a `live_keystroke` or hook-mirrored card |
| UX-AC22: validation error aria-describedby, aria-invalid, focus | same | `ux_ac22_should_showLinkedErrorAndFocusFirstInvalid_When_submitEmpty` | Jest | Submit empty; assert attributes and `document.activeElement` |
| UX-AC23: exactly one of 17 settled delivery-state lines (transient/reason strings are a separate set of 6), not a live region | `GuidanceQuestionForm.test.tsx` | `ux_ac23_should_renderExactlyOneDeliveryLineWithExactCopy_ForEachOf17States_And_haveNoLiveRole` | Jest | Parametrize 17 states (rows 16 and 17 included); the line element has no `aria-live`/`role=status`; copy matches `design/ux.md` 3.1 |
| UX-AC24: unacknowledged at 60 s: failed / acknowledged / Unconfirmed, stays in Needs you | `GuidanceQuestionForm.test.tsx` + Go `Router_should_decideAt60sByFreshCapture_*` | `ux_ac24_should_stayInNeedsYouAsUnconfirmed_When_captureIndeterminate_And_showRow16WhenDialogStillShows_And_ackWhenGone` | Jest (fake timers) + Go | Advance 61 s with each capture result and for both a row-2 `sent` and a row-6 `SEND_INDETERMINATE` start; assert state, line (row 16 never contains "Nothing was typed"), chip, Needs-you membership, badge count (a `sent` row at 30 s is not counted), and that a late PostToolUse clears the chip |
| UX-AC25: other tab shows "Answered elsewhere: {value}" no focus move | `notification-inline-actions.spec.ts` | `ux_ac25_should_showAnsweredElsewhereInSecondTab_WithoutFocusChange` | Playwright | Two pages one context; answer in A; assert B text and `document.activeElement` unchanged |
| UX-AC26: terminal answer flips card, polite, no focus | `askuserquestion-structured.spec.ts` | `ux_ac26_should_flipToHandledWithPoliteAnnouncement_When_answeredInTerminal` | Playwright | Remove fixture dialog; assert read-only copy and focus unchanged |
| UX-AC27: archived copy visible before Submit | same | `ux_ac27_should_showArchivedCopyBeforeSubmit_And_neverRevive` | Playwright | Archived session card; copy visible without click; submit; session still archived |
| UX-AC28: unrenderable shape text + View full prompt | `GuidanceQuestionForm.test.tsx` | `ux_ac28_should_renderTextOptionsWithViewFullPromptAndOpenSession` | Jest | Unknown shape payload |
| UX-AC29: aborted multi-question state | `GuidanceQuestionForm.test.tsx` + Go `Sequence_should_abort...` | `ux_ac29_should_showPartOfAnswerSentCopyWithSubmitDisabledAndOpenEnabled` | Jest | Aborted-partial state; Dismiss requires inline confirm |
| UX-AC30: Esc collapses, draft restored | `ActionRow.keyboard.test.tsx` | `ux_ac30_should_collapseOnEscapeWithoutSubmit_And_restoreDraftOnReexpand` | Jest | Type draft; Esc; re-expand |
| UX-AC31: peer request fields, no badge before escalation | `PeerHelpRow.test.tsx` | `ux_ac31_should_showFieldsAndNotIncrementBadge_When_waiting` | Jest | Badge count unchanged |
| UX-AC32: escalated row actions work or Retry | `PeerHelpRow.test.tsx` + `agent-wake-triggers.spec.ts` | `ux_ac32_should_offerAnswerForPeerCancelRequestOpen_AndEachShowsRetryOnError` | Jest + Playwright | Stall 10 min fake clock; click each; inject failure |
| UX-AC33: accepted-then-undeliverable request shows "Not sent: {reason}"; call-time rejections create no row | `PeerHelpRow.test.tsx` + Go `RequestHelp_should_returnTypedRejectedAndCreateNoRow_*` | `ux_ac33_should_showNotSentReason_When_answererArchivedOrDeletedAfterCreation` | Jest + Go | Reasons: archived, deleted, cross-workspace after creation; the 4 call-time reasons are asserted server-side to create no row |
| UX-AC34: answered by {session} to Recent | `PeerHelpRow.test.tsx` | `ux_ac34_should_showAnsweredByWithProvenanceAndMoveToRecent` | Jest | Answer fires; no operator action |
| UX-AC35: Scheduled prompts lists fields; empty copy | `ScheduledPrompts.test.tsx` + e2e | `ux_ac35_should_listFieldsWithCancelPrompt_And_showNoScheduledPromptsWhenEmpty` | Jest + Playwright | Seed 2 triggers and 1 peer request; then none; the empty copy is "No scheduled prompts. Agents that ask to be prompted again will appear here." |
| UX-AC36: Cancel inline confirm and announcement | same | `ux_ac36_should_confirmInlineThenAnnouncePromptCancelled` | Jest + Playwright | "Yes, cancel"/"Keep" buttons; polite text |
| UX-AC37: "Already fired at {time}." no toast | same | `ux_ac37_should_showAlreadyFiredWithoutToast_When_cancelRaces` | Jest | Race outcome |
| UX-AC38: Scheduled prompts paused + Resume + flag link; Cancel works | same | `ux_ac38_should_showPausedBannerResumeConfirmAndFlagLink` | Jest + Playwright | Flag off; Resume with inline confirm calls `UpdateFeatureFlag` |
| UX-AC39: missed scheduled-prompt exception row | same | `ux_ac39_should_showMissedReasonWithWarningIconCancelAndOpenOwner` | Jest | Missed state |
| UX-AC40: Remind me scope, Undo 10 s, failure copy | `RemindMeMenu.test.tsx` | `ux_ac40_should_appearOnlyOnInformationalRowsAndOfferUndo10s` | Jest | Enabled (Remind me is in scope; last wave) |
| UX-AC41: past time rejected inline | same | `ux_ac41_should_rejectPastTimeWithChooseFutureCopy` | Jest | Same condition |
| UX-AC42: 44px at 390, desktop >= 900 | `tests/e2e/notification-inline-actions.spec.ts` + `touch-targets.spec.ts` | `ux_ac42_should_haveComputedHeightAtLeast44px_At390_AndDesktopLayoutAt900Plus` | Playwright (phone + desktop profiles) | Iterate buttons, option rows, checkbox rows, tabs, disclosures; `boundingBox().height >= 44` |
| UX-AC43: stacked, full-width Submit above keyboard | `askuserquestion-structured.spec.ts` | `ux_ac43_should_stackAndKeepSubmitVisible_When_textFieldFocusedAtPhone` | Playwright + Manual | Emulated viewport shrink via `visualViewport`; Manual: real iOS Safari and Android Chrome, soft keyboard open, Submit visible |
| UX-AC44: input font >= 16px | same | `ux_ac44_should_useFont16pxOnAllTextInputs` | Playwright | Computed style on every `input`/`textarea` in panel |
| UX-AC45: no hover-only control | `notification-inline-actions.spec.ts` (`hasTouch`, no hover) | `ux_ac45_should_operateEveryActionByTapAlone` | Playwright | Use `tap()`, never `hover()`; list all controls reachable |
| UX-AC46: mobile panel one scroll region | `background-activity.spec.ts` (phone) | `ux_ac46_should_haveSingleScrollRegion_NoNestedScrollTrap` | Playwright + Manual | Assert only one element with `overflow-y:auto` scrollable; Manual swipe test on device |
| UX-AC47: action toasts pause on hover/focus, item stays | `notification-inline-actions.spec.ts` | `ux_ac47_should_pauseToastOnHoverAndFocus_And_keepItemAfterDismiss` | Playwright | Hover; wait for toast TTL via fake timers (`page.clock`); item still listed |
| UX-AC48: keyboard-only flow, no trap, Esc restores focus to bell | `notification-inline-actions.spec.ts` | `ux_ac48_should_completeWholeFlowByKeyboard_And_escReturnsFocusToBell` | Playwright | Tab/arrow/Enter/digit/Esc only |
| UX-AC49: digits only move focus, only in a digit-armed card | `ActionRow.keyboard.test.tsx` | `ux_ac49_should_onlyMoveFocusWithDigits_When_cardDigitArmed_And_ignoreOtherwise` | Jest | Dispatch `keydown` on body, terminal element, an unarmed card and a card focused programmatically; assert no RPC and no focus move; on a digit-armed card (opened with Enter, 750 ms elapsed) digits 1, 2, 3 pressed 50 ms apart focus options 1, 2, 3 in order, the card stays engaged, still no RPC |
| UX-AC50: focus to next item (not engaged), else heading, short result announced | `ActionRow.keyboard.test.tsx` + e2e | `ux_ac50_should_focusNextItemElseHeadingElseEmptyState_And_announceShortResult_And_notEngageNext` | Jest + Playwright | Resolve last item; focus the `h3` (`tabindex=-1`); status "Approved"; with items left, focus the next but a digit within 750 ms does nothing |
| UX-AC51: single polite region from first paint; no role=alert for expected | `NotificationPanel.test.tsx` | `ux_ac51_should_haveSinglePoliteStatusAtFirstPaint_And_noAssertiveForExpectedOutcomes` | Jest | Query before any interaction; run all outcomes; no `role=alert`/`aria-live=assertive`; `querySelectorAll('[aria-live],[role=status],[role=alert]')` length 1 |
| UX-AC52: chips icon+text+aria-label; contrast 4.5/3 in both themes | `tests/e2e/accessibility.spec.ts` (existing, extend) | `ux_ac52_should_passAxeColorContrast_InLightAndDark_And_chipsHaveIconTextLabel` | Playwright (axe) + Manual | Axe `color-contrast` on panel in both themes; Manual: measure icon/border 3:1 (axe does not check non-text contrast) |
| UX-AC53: aria-disabled + aria-describedby, focusable | `ActionRow.test.tsx` | `ux_ac53_should_useAriaDisabledDescribedByAndStayFocusable` | Jest | Offline, Reconnecting, desktop-only and Resume-from-desktop variants |
| UX-AC54: focus indicator not obscured by sticky bar | `askuserquestion-structured.spec.ts` | `ux_ac54_should_notObscureFocusedControlBehindStickyActionBar` | Playwright + Manual | Tab to last option on phone; compare `boundingBox` to sticky bar; Manual visual check of focus ring |
| UX-AC55: every error/terminal state has an exit control | `tests/e2e/ux-error-exits.spec.ts` (new, `// @feature notification:error-exits`) | `ux_ac55_should_offerAtLeastOneExitControl_ForEveryErrorState` | Playwright | Parametrized over states in ux.md 1.3, 2.3, 3.3, 4.3, 5.3, 6.3 (fulfilled outcomes); assert a Retry/Open/Dismiss/Cancel/link exists |
| UX-AC56: offline / reconnecting banner, aria-disabled, drafts, recovery | `tests/e2e/ux-offline.spec.ts` (new) | `ux_ac56_should_showOfflineBannerPreserveDraftAndRecoverWithoutReload` | Playwright | `context.setOffline(true/false)`; banner within 2 s via `expect` poll; draft unchanged; focus unchanged; failing the poll while online shows Reconnecting then "Can't reach the server." with Retry (fake clock) |
| UX-AC57: flag off at runtime degrades, in-flight completes or Open fallback | `notification-inline-actions.spec.ts` | `ux_ac57_should_degradeWithoutReload_And_inFlightActionCompletesOrFallsBackToOpen` | Playwright | Flip each of 4 flags; delayed RPC straddles flip |
| UX-AC58: no copy claims operator authenticated locally or delivery to archived | `web-app/src/__tests__/copyGuard.test.ts` | `ux_ac58_should_notContainAuthenticatedOrWillBeDeliveredCopyForArchived` | Jest (grep over component strings) + Manual | Pattern ban list; Manual copy review during Task 5.3 |
| UX-AC59: a still-displayed dialog stays answerable however long it has been open; "Already handled" only after a confirmed mismatch | `server/services/question_answer_router_test.go` + `askuserquestion-structured.spec.ts` | `ux_ac59_should_answerWithApplied_When_rowOpen45MinutesAndDialogStillOnScreen` and `ux_ac59_should_showAlreadyHandledOnlyAfterConfirmedMismatch` | Go + Playwright | Fake clock 45 min; fixture dialog still on screen: one `1` written; remove the dialog: `STALE`, copy "Already handled in the session"; assert the copy is never shown merely because time passed |

## Story acceptance-criteria trace

Every plan story maps to the tests above. IDs refer to the table sections; "Req tbl" is the Epic's requirement table.

| Story | Covered by |
|---|---|
| 0.1 S1 | Doc-check spike files; fixtures `server/services/testdata/askuserquestion/*.json` loaded by `ParseAskUserQuestion_*` tests; `HookReply_*` tests exist only if PASS (otherwise Task closed with result recorded) |
| 0.2 S2 | Doc-check; `HookReply_should_notBlockBeyondHookBlockMsCeiling` (conditional) |
| 0.3 S3 | Doc-check; Channels transport test only if PASS (`Delivery_should_useChannelWhenSpikePassed`, conditional, Epic 4 delivery file) |
| 0.4 S4 | Doc-check; `PrSource_*` / `ScheduleWake_should_returnUnavailable_*` |
| 0.5 S5 | Doc-check; `OpenCodeProfile_*`, `Registry_should_containOnlyMeasuredHarnesses` |
| 0.6 S6 | Doc-check; `RemoteChain_*` integration tests |
| 0.7 S7 | Doc-check (Tasks 0.7a-g: checks (1)-(6) and (8)-(11) + the 0.7c header-echo probe pasted); `HookInjector_*` conditional |
| 0.8 | Flag rows in the Epic 0 table |
| 1.1-1.8 | Epic 1 table (incl. `ComputeBackgroundOutcome_*`, `ListSessions_should_populateBackgroundOutcome_*`, the `finished`-without-history e2e) and UX-AC1..7, 46, 56, 66-73 (Story 1.8 rows in "Triad round 1 additions") |
| 2.1 | Doc-check proto/registry rows; `ActionKind_*` |
| 2.2 | `BuildDescriptors_*` (incl. the batching call-count test), `ShouldDeliver_*` gate table (incl. reminder, escalation, rate-limited rows), `Notifier_should_emitDecisionSchemaMetadata` |
| 2.3 | `Invoke_*` rows incl. restart idempotency; benchmark for p95; e2e round-trip p95 (Task 2.9c) |
| 2.4 | `PaneReplyCore_*`, existing `TestReply_*` regression, `PaneShowsDialog_*` (Task 2.4c) |
| 2.5-2.6, 2.10 | `ActionRow*.test.tsx`, `useEngagedCard`/`useFrozenList`/`useCommitDelay` tests, UX-AC8-17, 48-54, 60-65, 74-77, 81-88 |
| 2.7 | `Store_should_retire...`, dedup rows, `useNotificationSync` rows, UX-AC25 |
| 2.8 | `Invoke_should_returnForbidden...tierB`, `RemoteChain_*`, UX-AC13 |
| 2.9 | `notification-inline-actions.spec.ts` rows |
| 3.1 | Epic 3 table (domain, repo, caps, migration) |
| 3.2 | `ParseAskUserQuestion_*` |
| 3.3 | Mirror, Reaper, Sampler (bound without `stale`, backoff), on-demand cap re-verification, 45-minute answer, per-source side-effect rows; UX-AC26, 59 |
| 3.4 | `Route_*`, `Answer_should_logAnswerLatencyMs7500`, `Router_should_decideAt60sByFreshCapture_*`, `HumanActivity_*`, hook injector row, bounded-hook fail-open row |
| 3.5 | `Sequence_*` (conditional S1c), UX-AC29 |
| 3.6 | `question_profile_test.go` rows (conditional S5) |
| 3.7 | `GuidanceQuestionForm.test.tsx`, `askuserquestion-structured.spec.ts`, saved-for-later list, UX-AC18-30, 79 |
| 3.8 | `tools_guidance_test.go` rows |
| 4.1-4.2 | Wake repo and timer rows |
| 4.3 | Dispatcher, revive, decision-table guards (`allow_revive=false`, `notify_operator` exemption), wake-write safety, Wake limits, caps, debounce, imports, analyzer rows |
| 4.4 | Source rows, completeness tests |
| 4.5, 4.9 | Gate, tools, proof, spawn-matrix, lifetime, redaction rows |
| 4.6, 4.10, 4.11 | `wake_trigger_service_test.go`, ScheduledPrompts/PeerHelpRow/RemindMeMenu tests, `escalator_test.go`, UX-AC31-41, 80 |
| 4.7 | Prompt goldens, ScheduleWakeup per-file map |
| 4.8 | `integration_test.go`, `restart_gap_test.go`, `storm_test.go` (incl. the numeric-ceiling test) |
| 5.1-5.4 | Epic 5 table, Doc-check rows |

## Test Stack
- **Unit (Go)**: `go test` with `testify` (v1.12.1), table-driven, `t.Parallel()`, fake `Clock`/pane/approval fakes, `go.uber.org/goleak` for goroutine joins; `-race` on: `session/ent_repository_*`, `server/wakes/...`, `server/services` (idempotency, hidden reply, router), `server/mcp` gate tests.
- **Unit (web)**: Jest + React Testing Library + `jest-axe`-free ARIA assertions; fake timers; run with `cd web-app && npx jest --no-coverage` (pnpm for installs).
- **Integration (Go)**: in-memory ent SQLite (`test-io-storage-isolation` pattern) for repository tests; on-disk temp DB only for restart/restart-gap tests; real `EventBus`; fakes at the tmux pane edge.
- **Benchmarks**: `go test -bench` via `make benchmark-tier1`, run with `&` per repo rule; used for p95, scale and reconcile volume.
- **E2E / UX**: Playwright + Allure in `tests/e2e/` (isolated server, dynamic port, route fulfilment for hidden sessions and remote descriptors, `page.clock` for time), `@axe-core/playwright` for contrast, phone and desktop profiles from `viewport-profiles.ts`.
- **Manual**: real-device soft keyboard (UX-AC43, 46, 54), icon/border contrast measurement (UX-AC52), copy review (UX-AC58), spike live-session experiments (S1-S3, S5), and the Task 5.3 rollout checklist on the isolated instance (`PORT=62871`, `--remote-port 62872`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (new packages `server/wakes`, `server/services/notification_action*`, `question_answer_router`, `guidance_*` additions, `server/mcp` gate/tools) | `go test ./server/wakes/... ./server/services/... ./server/mcp/... ./session/... -race -coverprofile=coverage.out && go tool cover -func=coverage.out` | >= 80% line on new files; 100% of transition-table pairs and of `ActionOutcome`/`DeliveryState`/`WakeStatus` values via enumerating tests |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}' --testPathPatterns="notification-actions\|BackgroundActivity\|GuidanceQuestionForm\|ScheduledPrompts\|PeerHelpRow\|useBackgroundActivity\|backgroundActivity"` | >= 80% line on new files |
| Playwright | `cd tests/e2e && npm test` (conventions enforced by CI: `@feature` header, no `waitForTimeout`, ARIA/testid locators) | Every spec in this plan green; each UX-AC has a row above |
| Duplication | `pnpm run lint:duplicates`, `make ready-complexity-gate` | jscpd < 0.14%; `dupl` clean on diff |

- All public service methods: happy path and error paths covered (Epic tables).
- All external integrations (tmux pane writer, bus, GitHub poller, hooks): unit with fakes plus at least one integration (restart, mirror, remote chain).
- UX acceptance criteria: each criterion in `design/ux.md` section 10 has a row above.

## Deviations from the plan text surfaced by this validation (all six now fixed in `plan.md`, Phase 4 repair)

1. Story 2.3's "p95 < 1 s over 100 invocations" is a `Benchmark`, not a `Test` (`notimesleeptest` and `deterministic-fast-tests`); a deterministic fake-clock test guards the hot path instead. **Fixed**: Story 2.3 states both; Story 2.9 (Task 2.9c) adds the real-server round-trip measurement.
2. Story 1.4b said to write `tests/e2e/background-activity.spec.ts`; the file and `pages/NotificationPanel.ts` already exist and are extended. Hidden sessions in e2e are route-fulfilled because test mode cannot create them. **Fixed**: Task 1.4b says extend; Story 1.2 and Task 1.2b say `backgroundActivity.test.ts` is created (it does not exist today).
3. Remote-listener behavior (Story 2.8, UX-AC13) cannot be run through Playwright because the e2e server has no `--remote-port`; it is proven in Go through the remote handler chain plus a descriptor-fulfilled e2e. **Fixed**: Stories 0.6 and 2.8 say so.
4. Fixtures: existing captures live in `session/testdata/reply_dialogs/`, not `session/testdata/panes/`; new Claude AskUserQuestion captures go beside them. **Fixed**: Stories 0.1 and 3.6 and Task 0.1c.
5. Task 3.5's `paneSubmitter` fault-injection double does not exist by that name; extract the existing double in `hidden_question_reply_test.go` (Task 2.4a) so 3.5, the router, the wake-write safety tests and the visible-session tests share it. **Fixed**: Task 2.4a and Story 3.5.
6. Conditional stories (3.4c on S1, 3.5 on S1c, 3.6 on S5, Channels on S3, per-harness proof 4.9g on S5) keep their tests written as `describe.skip` / `t.Skip("blocked on S1: <link>")` with the spike or owner decision named, never deleted, so a PASS result enables them without redesign. **Fixed**: the Conditional-story rule in the Epic 0 goal plus an AC in each affected story (0.3, 3.4, 3.5, 3.6, 4.6, 4.9).

## Phase 4 repair additions (new or changed acceptance criteria mapped to tests)

| Plan change | Tests (rows above) |
|---|---|
| P1-a: Story 1.7 and the rewritten Story 1.2 fixtures; ADR-001 decision 6; Task 1.5b alert | `ComputeBackgroundOutcome_*`, `ListSessions_should_populateBackgroundOutcome_*`, `useBackgroundSessions_should_mapBackgroundOutcome*`, the four `buildActivityRows_*` rows for finished/unknown/failed/never-from-history, `backgroundTab_should_showAtLeastOneFinishedChip_*`, the 24 h alert row |
| P1-b: sampler bound without `stale`, backoff, on-demand cap re-verification, 45-minute answer | `Sampler_should_stopPollingButKeepRowOpenAndAnswerable_*`, `Sampler_should_sampleEvery3s*`, `Mirror_should_reverifyOpenHookRows*`, `Route_should_writeOneByte1AndReturnApplied_When_rowOpen45Minutes*`, UX-AC59 |
| P1-c: wake-write safety | `SteerDelivery_should_writeZeroBytes*` (three rows), `PaneShowsDialog_*` |
| P1-d: Wake limits table | `Limits_*` (five rows), `Storm_should_neverExceed6DeliveriesPerOwnerHourAnd60Global_*` |
| B2: per-harness proof (conditional on S5) | `ProofMinting_should_skipS5Failed*`, `ProofMinting_should_mintVerifiableProof*` (skipped until S5) |
| B3 / B4: decision table and action-kind-scoped guards | `Guards_should_skipOwnerStoppedAndNeverEnterNeedsRevive_*`, `NotifyOperator_should_deliverAndSkipEveryGuard_*`, the two `ShouldDeliver_*` reminder rows, `RemindMe_should_emitSystemLevel*` |
| C7: Story 4.11 | `Escalator_*` (four rows), `PeerHelpRow_*`, `RequestHelp_should_returnTypedRejectedAndCreateNoRow_*`, gate-class row |
| C9: `(type, kind, predicate)` table | `ScheduleWake_should_acceptStatusReachedAndRejectBare*`, `Debounce_*VerdictRecorded*` |
| C13 and REQ-18/REQ-19 | `ActionRow_*` rows for Invalid/Refresh, auto-handled STALE, countdown and first-commit; `BuildDescriptors_should_callEachStoreOncePerPage*`; `actionRoundTrip_should_haveP95UnderOneSecond*`; `ScheduledPrompts_should_render20RowsWithShowAllN*`; flag-flip-in-flight e2e |
| Counts (as of the Phase 4 repair; **superseded**, see "Counts" at the end of this file) | 47 stories, 154 tasks, 8 ADRs, 59 UX-ACs, 44 glossary terms, 16 metric names |

## Triad round 1 additions (2026-10-18; every new or changed AC mapped to tests)

Counts here were produced by grep after the last edit and are restated in "Counts" below. Go tests are named `TestSubject_ShouldX_WhenY` in the style above; names in backticks are the test function (Go) or `it()` title stem (Jest/Playwright).

### Go and flag tests

| Requirement | Test file | Test name | Type | Scenario |
|---|---|---|---|---|
| REQ-24, REQ-30 | `config/feature_flag_scopes_test.go` | `TestFlagMatrix_ShouldResolveEveryCell_WhenGlobalAndScopesPermuted` (every row of the plan's "Flag precedence and dependency matrix": `stage:answer` without `stage:render`, `stage:hook_reply` without `stage:answer`, `channel:remote:tierB` without `channel:remote`, `tool:*`/`trigger:*` without the dispatcher, scoped true under global false, scoped false under global true) | Unit | Pre-mortem #10 |
| REQ-24 | `server/services/feature_flags_notification_actions_test.go` | `TestInvoke_ShouldReturnFlagOffAndWriteNothing_WhenAnswerStageOnButRenderStageOff` | Unit | Dependency: no invisible card with live buttons |
| REQ-24 | same | `TestScheduleWake_ShouldSucceedForAtAndRefuseCron_WhenOnlyTriggerAtScopeIsTrue_And_PendingRowsStayPendingWhenScopeTurnsFalse` | Unit | Scoped true under global false; rows neither fired nor missed |
| REQ-3 | `web-app/src/components/ui/notification-actions/ActionRow.test.tsx` | `ActionRow_should_showTurnedOffCopyWithOpen_When_outcomeFlagOff` ("This action is turned off. Open session.") | Unit | `FLAG_OFF` copy |
| REQ-21 | `server/services/notification_action_service_test.go` | `Invoke_should_writeAuditChannelAndCallerKind_And_countUnknownChannel_When_postedWithoutSpaHeader` | Unit | Pre-mortem #17 |
| REQ-21 | `server/services/notification_actions_test.go` | `TestEveryActionKindHasTierAndRemoteAvailability` (fails for a fixture kind with no row); `BuildDescriptors_should_disableResumeScheduledPromptsAndLiveKeystroke_When_listenerRemote` | Unit | Story 2.8 / Task 2.8c, UX-AC77 |
| REQ-7 | `server/services/question_answer_router_test.go` | `Router_should_decideAt60sByFreshCapture_Failed_Acknowledged_Unconfirmed_When_dialogStillShowsGoneOrCaptureErrors` (fake clock; three captures) | Unit | B3, UX-AC24 |
| REQ-7 | same | `Router_should_acknowledgeOnPostToolUseReceipt_And_onTwoConsecutiveCleanCaptures_And_clearUnconfirmedWhenLateReceiptArrives` | Unit | B3 signal |
| REQ-7 | same | `Router_should_neverSetSavedForLaterOnHookOrPeerRows_And_setItForAgentMcpRowsOnStoppedSession` | Unit | Scope of `saved_for_later` |
| REQ-7 | `server/services/terminal_access_test.go` | `HumanActivity_should_recordLastInputPerSession_When_paneWriterSendInputCalled` (fake clock; two sessions) | Unit | Task 3.4f signal (a) |
| REQ-7 | `server/services/human_activity_test.go` | `HumanActivity_should_refuseWithHumanActive_When_inputWithin5s`; `_ShouldProceed_When_lastInput6sAgo`; `_ShouldIgnoreControlModeClient_And_RefuseOnNonControlClientActivity2sAgo` (fake `list-clients` output); `_ShouldUseInputSignalOnly_When_listClientsFails` | Unit | Pre-mortem #16 |
| REQ-7 | `cmd/ssq-hooks/main_test.go` | `HookReply_should_exitEmptyAtCap_When_serverHangsAndBlockMsNonZero`; `_ShouldNeverBlock_When_defaultBlockMsZero` (conditional on S1/S2; skipped with the spike linked until then) | Unit | Pre-mortem #19 |
| REQ-8 | `server/services/guidance_request_hook_mirror_test.go` | `Mirror_should_returnDeferUnder300ms_When_repositoryBlocks5s`; `_ShouldReturnDeferAndCountFailure_When_repositoryErrors` | Unit | Pre-mortem #7: fail open |
| REQ-8 | `server/services/question_dialog_canary_test.go` | `Canary_should_raiseOneNotificationAndDegradeHookRowsToReadOnly_And_leaveAgentRowsAndHookReplyUnchanged_When_4Of6AttemptsMismatchInAnHour`; `_ShouldRecover_When_shareDropsBelow50Percent` | Unit | Pre-mortem #5; degraded = `read_only` (no durable-record route exists for hook rows) |
| REQ-8 | `server/services/guidance_request_hook_mirror_test.go` | `Mirror_should_redactSecretInQuestionsJsonAndSnippet_When_scannerMatches`; `_ShouldPurgeTerminalQuestionsJsonAfter30Days` | Unit | Pre-mortem #18 |
| REQ-12 | `server/wakes/timer_test.go` | `Cron_should_skipNonexistentLocalTimeAndFireOnNextDay_When_springForwardChicago`; `_ShouldFireOnce_When_fallBackAmbiguousHour`; `_ShouldNotRefire_When_clockJumpsBack1h`; `_ShouldComputeInUTC_When_noZoneInSpec`; `_ShouldRejectUnknownZone` | Unit | Pre-mortem #20, Task 4.2d |
| REQ-16 | `server/wakes/reconcile_test.go` | `Reconcile_should_raiseExactlyOneDigest_When_12ExpiredRows4OwnersAnd3ArchivedSkips`; `_ShouldDeliverAtMostPerOwnerLimitFirstHour_When_flagReenabledWith30Overdue` | Unit | Pre-mortem #6, Task 4.3i |
| REQ-16 | `server/wakes/delivery_test.go` | `Payload_should_containIdAndStandingSentence`; `Delivery_should_countDuplicate_When_crashAfterWriteBeforeMarkDelivered`; `Delivery_should_markMissedOwnerBusyTimeout_When_busyOwner31Min` | Unit | Pre-mortem #9, Task 4.3j |
| REQ-26 | `server/wakes/revive_test.go` | `NeedsRevive_should_stayAndAlertAt1h_When_admitRefusesInProgressOwner`; `_ShouldBecomeMissed_When_expiresAtPasses` | Unit | Pre-mortem #14, Task 4.3k |
| REQ-26 | `tools/lint/noarchivedrevival/analyzer_test.go` | `Analyzer_should_reportUnguardedRevival_InQuestionAnswerRouterResumeFixture`; `_ShouldNotReport_GuardedFixture` | Unit | `question_answer_router.go` in `lifecycleFiles` |
| REQ-21 | `server/mcp/spawn_path_matrix_test.go` | `CredentialSelfCheck_should_logOkAndSetProofPresent_When_roundTripSucceeds`; `GetSession_should_exposeProofPresentBooleanNeverValue` | Unit | Pre-mortem #13, Task 4.9h |
| REQ-28 | `server/services/notification_action_metrics_test.go` | `QualityMetrics_should_exportDeliveryFailureRateUndoAndReopen_When_10AnswersOneNotSentOneUndoOneReopen` | Unit | Task 5.1d |
| REQ-11 | `server/wakes/escalator_test.go` | `Escalator_should_deliverOnBehalfOfProvenanceToAsker_When_operatorAnswersForPeer` | Unit | Task 4.11d |

### Jest and Playwright tests (Stories 1.6d-e, 1.8, 2.5e-f, 2.6c-e, 2.10, 3.7e-f, 4.11d)

| Requirement | Test file | Test name | Type | Scenario |
|---|---|---|---|---|
| REQ-2 | `web-app/src/components/ui/OfflineBanner.test.tsx` | `OfflineBanner_should_showLoadingOnlyAfter150ms_And_StillLoadingWithRetryAfter8s`; `_ShouldShowReconnectingThenCantReachWithRetryAfter30s_When_onlineButPollFails`; `_ShouldShowStillSendingAt10sAndNoResponseCopyAt30s`; `_ShouldBeNonLiveText_And_PanelHasOneLiveRegion` | Unit | Task 1.6d, UX-AC73 |
| REQ-2 | `web-app/src/components/ui/BackgroundActivity.test.tsx` | `BackgroundActivity_should_nameNewFailureInStickyStripAndNotMoveRows_When_cardEngaged`; `_ShouldShowChipInPlace_When_existingRowBecomesFailure`; `_ShouldMoveFocusToRowOnlyOnShow` | Unit | UX-AC66 |
| REQ-2 | `web-app/src/lib/utils/badgeCount.test.ts` | `badgeCount_should_equal6AndTabBadge2_When_3Actionable1Unconfirmed2Failures10Finished1PeerWaiting` (+ table over every kind) | Unit | UX-AC67 |
| REQ-2 | `web-app/src/lib/hooks/useReviewedMarks.test.ts` | `useReviewedMarks_should_collapseAndPersist_When_markAllReviewed`; `_ShouldFallBackToSessionAndShowNote_When_localStorageThrows` | Unit | UX-AC68, UX-AC70 (first-run note) |
| REQ-20, REQ-18 | `BackgroundActivity.test.tsx` | `BackgroundActivity_should_commitAtMost120RowsAndCollapseOlder_When_500Sessions480Old` (Profiler < 100 ms, soft on CI) | Unit | UX-AC69, pre-mortem #8 |
| REQ-2 | same | `BackgroundActivity_should_toggleDetailsOnRowTextClick_And_neverNavigate`; `_ShouldNotRenderControlsForSessionWithPendingQuestion_And_ShowLinkChip` | Unit | UX-AC71, UX-AC72 |
| REQ-3 | `ActionRow.test.tsx` | `ActionRow_should_renderFullCommandWithScrollRegionCountAndCopy_When_1204Chars`; `_ShouldReplaceApproveWithOpen_When_inputUnrenderable`; `_ShouldSeparateDenyAndApproveBy24pxDenyFirst_When_390px` | Unit | UX-AC64, UX-AC65 |
| REQ-25 | `web-app/src/lib/copy/copyGuard.test.ts` | `copyGuard_should_rejectWakeWordAndWaitingOnPhraseAndBareCancel_When_anyExportedStringOrRenderedPanelText`; `copyGuard_should_allowFlagNameInFlagSettingsLinkOnly` | Unit | UX-AC76 (UI strings only; identifiers such as `wake_trigger` are out of scope) |
| REQ-3 | `web-app/src/lib/hooks/useFrozenList.test.ts` | `useFrozenList_should_holdOrderAndQueueArrivals_When_cardEngaged`; `_ShouldMergeAndAnnounceOnce_When_engagementEnds` | Unit | UX-AC60 |
| REQ-3 | `web-app/src/lib/hooks/useEngagedCard.test.ts` | `Digits_should_beInertUntilEngaged_And_for750msAfterProgrammaticFocus_And_neverSend`; `Digits_should_visitOptions1To3InSequence_When_pressed50msApart_And_notRestartGrace_And_keepCardEngaged`; `SettledLayout_should_ignoreActivationFor300ms` | Unit | UX-AC61, UX-AC62 |
| REQ-3 | `web-app/src/lib/hooks/useCommitDelay.test.ts` | `CommitDelay_should_dispatchAfter3s_And_notDispatchOnUndo_And_cancelWithoutDispatchOnPagehideVisibilitychangeCloseAndBack_And_restoreCardWithNote_And_notApplyToDenyOrAnswers` | Unit | UX-AC63 |
| REQ-25 | `web-app/src/components/ui/AlertsPage.test.tsx` | `AlertsPage_should_pushOneHistoryEntryAndCloseOnBack_And_restoreDraft`; `_ShouldHaveNoDialogRoleOrAriaModal`; `_ShouldNotMoveFocusOnEventArrival` | Unit | UX-AC74, UX-AC75 |
| REQ-25 | `web-app/src/components/ui/NotificationPanel.a11y.test.tsx` | `Panel_should_haveExactlyOneLiveRegion`; `_ShouldHaveH2H3AndTabindexMinus1Fallbacks`; `_ShouldNotScrollHorizontally_At320px`; `_ShouldNotClip_WithTextSpacingOverrides`; `_ShouldMeet24pxInlineTargets`; `_ShouldDropTransitions_WithReducedMotion`; `_ShouldKeepBordersAndOutlineFocus_WithForcedColors` (emulated media) | Unit | UX-AC81 to UX-AC86 |
| REQ-25 | `web-app/src/components/ui/notification-actions/ActionRow.keyboard.test.tsx` | `Shortcuts_should_beInert_When_targetIsInputTextareaSelectContenteditableTerminalOrModifierOrIme` | Unit | UX-AC88 |
| REQ-6 | `web-app/src/components/guidance/GuidanceQuestionForm.test.tsx` | `SingleSelect_should_exposeButtonsNotRadios`; `_ShouldLockCardAfterOneActivation`; `SavedForLaterList_should_showOnlyAgentMcpRowsReadOnlyWithOpenAndNoBadge` | Unit | UX-AC18, UX-AC79 |
| REQ-11 | `web-app/src/components/ui/PeerHelpRow.test.tsx` | `PeerHelpEscalation_should_raiseBadgeAnnounceOnceAndKeepFocus_And_waitBehindNNew_When_cardEngaged` | Unit | UX-AC80 |
| REQ-25 | `tests/e2e/notification-inline-actions.spec.ts` | `ux_ac60_should_keepOrderWhileCardEngaged_And_ac61_ignoreDigitAfterFocusAdvance_And_ac63_undoApprove` (page.clock; `// @feature notification:interaction-safety`) | Playwright | B2, B3-adjacent, consequential taps |
| REQ-25 | `tests/e2e/ux-mobile.spec.ts` (new) | `ux_ac74_should_closeAlertsPageOnBack_And_ac87_keepStickySubmitAboveKeyboard_On390pxTouch` (soft keyboard simulated with `visualViewport` resize; real-device pass is manual) | Playwright | Mobile |

### UX acceptance tests for UX-AC60 to UX-AC88

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-AC60 | `useFrozenList.test.ts` + `notification-inline-actions.spec.ts` | `ux_ac60_*` (rows above) | Jest + Playwright | Engage card A, poll adds B and resolves C; order unchanged; "1 new"; merge after disengage |
| UX-AC61 | `useEngagedCard.test.ts` + e2e | `ux_ac61_*` | Jest + Playwright | Resolve, press `1` at 100 ms: zero RPC; engage then `2`: focus only |
| UX-AC62 | `useEngagedCard.test.ts` | `ux_ac62_*` | Jest | Activate at +100 ms and +400 ms |
| UX-AC63 | `useCommitDelay.test.ts` + e2e | `ux_ac63_*` | Jest + Playwright | Approve then Undo at 2 s; Approve then close panel, `visibilitychange` hidden and `pagehide` at 1 s: no call, card restored with the cancellation line; Deny immediate; focus on Undo during the hold |
| UX-AC64 | `ActionRow.test.tsx` | `ux_ac64_*` | Jest | 14-line command; region role/label; count; Copy; unrenderable input |
| UX-AC65 | `ActionRow.test.tsx` + `touch-targets.spec.ts` (extend) | `ux_ac65_*` | Jest + Playwright | 390px; gap >= 24px; Deny precedes Approve |
| UX-AC66 | `BackgroundActivity.test.tsx` | `ux_ac66_*` | Jest | Failure while engaged; strip text; no row moved; Show focuses row |
| UX-AC67 | `badgeCount.test.ts` | `ux_ac67_*` | Jest | Composition table; a `SEND_INDETERMINATE` row at 5 s counts, a plain `sent` row at 30 s does not |
| UX-AC68 | `useReviewedMarks.test.ts` | `ux_ac68_*` | Jest | Mark all, reload, storage failure |
| UX-AC69 | `BackgroundActivity.test.tsx` | `ux_ac69_*` | Jest | 500 rows, Older (480), Show all, no windowing (all rows present after Show all) |
| UX-AC70 | `useReviewedMarks.test.ts` | `ux_ac70_*` | Jest | First-run note once; Got it persists |
| UX-AC71 | `BackgroundActivity.test.tsx` | `ux_ac71_*` | Jest | Row text click toggles; control click does not; no anchor wraps a row |
| UX-AC72 | `BackgroundActivity.test.tsx` + `background-activity.spec.ts` | `ux_ac72_*` | Jest + Playwright | Hidden session with pending question: controls only in Needs you; no notification id in both tabs |
| UX-AC73 | `OfflineBanner.test.tsx` + `ux-offline.spec.ts` | `ux_ac73_*` | Jest + Playwright | Timers per Story 1.6 AC |
| UX-AC74 | `AlertsPage.test.tsx` + `ux-mobile.spec.ts` | `ux_ac74_*` | Jest + Playwright | `history.back()`; one entry; draft restored |
| UX-AC75 | `AlertsPage.test.tsx` | `ux_ac75_*` | Jest | No `[role=dialog],[aria-modal=true]`; focus unchanged on arrival |
| UX-AC76 | `copyGuard.test.ts` | `ux_ac76_*` | Jest | All exported strings |
| UX-AC77 | `notification_actions_test.go` + `ActionRow.test.tsx` | `ux_ac77_*` | Go + Jest | Tier table on the remote listener |
| UX-AC78 | doc-check on `docs/how-to/use-inline-notification-actions.md` | `ux_ac78_should_containAwayOperatorDependencyParagraph` | Doc-check | grep |
| UX-AC79 | `GuidanceQuestionForm.test.tsx` | `ux_ac79_*` | Jest | Saved list contents and exclusions |
| UX-AC80 | `PeerHelpRow.test.tsx` | `ux_ac80_*` | Jest | Escalation with and without an engaged card |
| UX-AC81 | `NotificationPanel.a11y.test.tsx` + `ux-mobile.spec.ts` | `ux_ac81_*` | Jest + Playwright | 320px `scrollWidth` |
| UX-AC82 | `NotificationPanel.a11y.test.tsx` | `ux_ac82_*` | Jest | Spacing overrides, no clipped overflow |
| UX-AC83 | `NotificationPanel.a11y.test.tsx` | `ux_ac83_*` | Jest | Inline control boxes >= 24px, stand-alone >= 44px |
| UX-AC84 | `NotificationPanel.a11y.test.tsx` | `ux_ac84_*` | Jest | Live-region count 1; countdowns announce start/end only |
| UX-AC85 | `NotificationPanel.a11y.test.tsx` | `ux_ac85_*` | Jest | Heading levels; fallbacks focusable |
| UX-AC86 | `NotificationPanel.a11y.test.tsx` + Playwright `emulateMedia` | `ux_ac86_*` | Jest + Playwright | Reduced motion, forced colors |
| UX-AC87 | Task 5.3 checklist | `ux_ac87_should_holdMobileKeyboardMatrixOnIosSafariAndAndroidChrome` | Manual | Real devices; matrix in `design/ux.md` 11.4 |
| UX-AC88 | `ActionRow.keyboard.test.tsx` | `ux_ac88_*` | Jest | Targets, modifiers, IME |

### Known flaky suites and CI isolation (Story 5.4)

| Flaky area (cited by file) | Where it can bite this project | Isolation |
|---|---|---|
| `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md` and `docs/bugs/open/BUG-099-server-services-package-slow-enough-to-blow-ci-job-budgets.md` (`server/services` runtime) | Every new `server/services/*_test.go` | New wake code in `server/wakes`; new `server/services` tests run as a separate required step with the name-prefix `-run` filter in plan Story 5.4 and `-timeout=20m` |
| `docs/bugs/open/BUG-099-retrysession-permanentlyfailed-instance-status-race-under-full-suite.md` (`server/services/session_service_retry_test.go`) | Tests that spawn `Instance`s | New tests use the fakes in Task 2.4a, not real `Instance` status |
| `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md` (`server/services/connectrpc_websocket_test.go`) | `HumanActivity` recorder is added next to the websocket writer path | Recorder tested in `terminal_access_test.go` with a fake sender, never through the hub |
| `docs/bugs/open/BUG-079-jest-full-suite-only-failures-stuckitem-websocket-terminal-backlogdetail.md` (Jest full-suite-only) | New Jest suites near the terminal, StuckItem and BacklogDetail components | Run via `--testPathPatterns` for the new components; no assertion depends on the full-suite run |
| `docs/bugs/open/BUG-079-autonomousdriver-idle-timer-race-under-full-suite.md` (`session`) | Reaper and sampler tests that share goroutine-leak checks | `goleak` options scoped to the new goroutines |

A red known-flaky test is re-run in isolation (`-count=3 -race`) before attribution; this project neither edits nor skips them.

### Triad round 2 additions (2026-10-18; every new or changed AC mapped to tests)

| Plan change | Test file | Test name | Type | Scenario |
|---|---|---|---|---|
| UX-B1: lifecycle table (Stories 2.6, 2.10; `design/ux.md` 7.1) | `useEngagedCard.test.ts`, `useFrozenList.test.ts` | `Lifecycle_should_keepResolvedRowUntilNothingEngaged750ms_And_moveFocusOnlyWhenFocusWasInsideLeavingRow_And_notEngageNextCard`; `Freeze_should_notReapplyAfterProgrammaticFocusAdvance`; `Esc_should_closeInnermostLayerFirst` | Unit | Resolve with pointer elsewhere vs. pointer over; keyboard resolve; next card focused but not engaged and not digit-armed; a poll then inserts at once |
| UX-B2: Create rule removed from the primary row (Story 2.5, Task 2.5f; UX-AC65, UX-AC77) | `ActionRow.test.tsx` | `ActionRow_should_putCreateRuleInMoreMenuAtDesktopOnly_And_confirmInline_And_omitMoreOnPhoneAndRemote`; `_ShouldRenderApproveDenyMoreOpenInThatOrder` | Unit | 1280px local, 390px, remote listener |
| UX-B3: NOT_SENT vs SEND_INDETERMINATE vs STALE reasons (Story 2.5; UX-AC11, UX-AC23) | `ActionRow.test.tsx` | `ActionRow_should_showDistinctNotSentAndIndeterminateStrings_WithSafeRetryVsRetryAnywayConfirm`; `_ShouldShowAlreadyHandledOnlyForDialogGoneAndSomethingElseForDialogReplaced` | Unit | Outcome mapping table; `STALE.reason` |
| ENG-B1: `pending -> skipped(rate_limited)` and cron occurrence skip (ADR-006 rule 2; Tasks 4.1c, 4.3h) | `session/ent_repository_wake_test.go`, `server/wakes/limits_test.go` | `Transitions_should_acceptEveryPairInTheTableIncludingPendingToSkippedForOneShot_And_rejectEveryOther`; `Repo_should_rejectRateLimitedSkipOnCronRow_And_SkipOccurrenceAdvancesNextFire`; `Limits_should_skipOnlyTheOccurrenceForCron_And_makeOneShotSkippedAfter3Deferrals`; `Limits_should_deriveCronFloorFromPerOwnerLimit` (10 min default = 6/h, 5 min floor = 12/h ceiling) | Unit | All-pairs test now includes the new row |
| Revive lock bounded and separate (ADR-007 rule 5; Task 4.3l, split from 4.3d) | `server/wakes/revive_test.go` | `Revive_should_releaseLockAt30sAndKeepNeedsRevive_When_startBlocks`; `CapChecks_should_completeWithin50msWhileReviveBlocked`; `Revive_should_treatSessionAsLive_When_nextCycleFindsItRunning` | Unit | Fake clock; fake `Start` blocks |
| Terminal-row tombstone and retention (ADR-006 rule 12; Task 4.1d) | `session/ent_repository_wake_test.go` | `Repo_should_recreateTriggerAfterTerminal_And_returnFirstWhileLive`; `Repo_should_purgeTerminalRowsOlderThan30DaysOnly_AtStartupAndReconcileCadence` | Unit | 29 and 31 day rows |
| Flag carve-out: `expires_at` wins (ADR-008, Story 0.8) | `server/services/feature_flags_notification_actions_test.go` | `TestScheduleWake_ShouldMarkPendingAtRowMissedAtExpiry_WhenTriggerScopeFalse` | Unit | Scope false; row neither fired nor missed until expiry, then missed |
| `BackgroundOutcome` failure allowlist (ADR-001 decision 6; Task 1.7a) | `session/background_outcome_test.go` | `ComputeBackgroundOutcome_should_treatTimeoutAsFailed_And_JulesCompletedAndUnlistedReasonAsUnknown_And_VerdictBeatsFailureReason`; `TestEndReasonClassification_ShouldCoverEveryKnownProducer` | Unit | Table over every producer literal; unknown defaults to non-failure |
| Batched `ItemSession` lookup (Story 1.7, Task 1.7b) | `server/services/session_service_crud_test.go` | `ListSessions_should_callBatchedItemSessionLookupOnce_And_neverPerSession_When_40HiddenSessions` | Unit | Counting fake repository |
| `ANSWER_QUESTION` stub seam (Story 2.3) | `server/services/notification_action_service_test.go` | `Invoke_should_returnFlagOffForVisibleSession_And_serveHiddenSession_When_hiddenReplyAdapterWired` | Unit | Seam until Story 3.4 |
| `resume` route never revives; archived copy before click (Story 3.4; UX-AC27) | `server/services/question_answer_router_test.go`, `GuidanceQuestionForm.test.tsx` | `Route_should_storeOnlyAndCallNoLifecycleFunction_ForStoppedAndArchived`; `ArchivedCard_should_showArchivedCopyBeforeAnyClick` | Unit | Spy on `Start`, `RecoverFromStopped`, `restartForRetry`, `transitionToLocked` |
| Terminal answer flips an open card (Story 3.3; UX-AC26) and `dialog_replaced` | `guidance_dialog_gone_sampler_test.go`, `GuidanceQuestionForm.test.tsx` | `Sampler_should_cancelWithDialogReplaced_When_otherDialogShowing`; `OpenCard_should_flipToHandledWithOnePoliteAnnouncementAndNoFocusChange_When_clearedAnsweredInTerminal` | Unit | UX-AC26 plan assertion |
| One announcement per async delivery transition (Story 3.7, `design/ux.md` 11.1) | `GuidanceQuestionForm.test.tsx` | `Announce_should_sayAnswerNotDeliveredForRow5Only_AnswerSentQuestionStillShowingForRow16_NotConfirmedPartlySentOncePerTransition_And_silentOnLateAck_And_sentSaysAnswerSent` | Unit | Fire each transition twice |
| Hold cancels, never flushes (Story 2.10; UX-AC63) | `useCommitDelay.test.ts` | see the updated `CommitDelay_*` name above | Unit | Panel close, `visibilitychange`, `pagehide`, route change |
| Layout stability and fling guard (Task 2.10d; UX-AC62) | `useScrollSettle.test.ts`, `notification-inline-actions.spec.ts` | `Fling_should_ignoreTouchWithin120msOfScroll_And_swallowClick`; `StatusSlot_should_moveNoRow_When_bannerPillAndStripAppear` | Unit + Playwright | `getBoundingClientRect().top` unchanged |
| Disabled until fresh, "As of", tier copy, `sessionStorage` drafts (Task 1.6e; UX-AC56, UX-AC73, UX-AC30) | `OfflineBanner.test.tsx`, `NotificationPanel.test.tsx` | `Recovery_should_keepControlsDisabledUpdatingUntilFirstRefresh_And_showAsOf`; `Tiers_should_showEmptyLoadingAndFetchFailedCopy`; `Drafts_should_surviveCloseAndReload_And_fallBackToMemory_When_sessionStorageThrows` | Unit | Fake timers and throwing storage |
| Plan assertion for UX-AC13 (Story 2.8) | `ActionRow.test.tsx` | `ux_ac13_should_keepOpenSessionEnabledBesideAriaDisabledTierB_And_tierAWorks` | Unit | Remote descriptors |
| Plan assertion for UX-AC47 (Story 2.6) | `notification-inline-actions.spec.ts` | `ux_ac47_should_pauseToastOnHoverAndFocus_And_keepItemListedAfterDismiss` | Playwright | `page.clock` |
| G6: reviewed-mark metric (Task 1.8c) | `useReviewedMarks.test.ts` | `useReviewedMarks_should_emitMarkCounterWithStorageLabel` | Unit | `local` vs `session` |
| Epic 4 cost outcome (Task 5.1e) | `server/services/notification_action_metrics_test.go` | `CostMetrics_should_sampleWakeFiresPerSessionAndCountScheduleWakeupReceipts_When_sessionEnds` | Unit | 3 wake deliveries, 2 receipts |
| Task 4.9b/4.9d split (Tasks 4.9i, 4.9j) | `server/mcp/proxy_test.go`, `spawn_path_matrix_test.go`, `session/instance_tmux_nonleak_test.go` | rows already listed for Story 4.9, now owned by 4.9i (proxy read) and 4.9j (verify, non-leak, copy) | Unit | No test removed |
| Hostile usability trials T8 to T10 (Task 5.3c; `design/ux.md` 12) | `docs/how-to/verify-notification-actions-rollout.md` | `usability_trials_T8_T9_T10_should_record_pass_or_defect_per_flag` (manual) | Manual | 10 trials each |

UX acceptance tests whose steps changed in round 2 (no new UX-AC rows): UX-AC11, 13, 23 (15 strings), 26, 27, 30 (`sessionStorage`), 47, 49 (digit-armed), 50 (focus moves when the lifecycle table removes the row), 56 and 73 (disabled until fresh, tier copy), 60 to 63, 65 (More menu), 74, 77, 84 (one announcement per transition), 88 (WCAG 2.1.4).

### Triad round 3 additions (2026-10-18; every new or changed AC mapped to tests, no new UX-AC)

| Plan change | Test file | Test name | Type | Scenario |
|---|---|---|---|---|
| `PaneShowsDialog` tri-state check (Task 2.4c, W1) | `session/dialog_presence_test.go` | `PaneShowsDialog_should_returnShowingAbsentIndeterminate_ForEachFixture_And_failWhenFixtureHasNoExpectation` | Unit | `reply_dialogs/` captures, permission prompt, idle composer, empty string, forged numbered row |
| Wake-write and sampler consume it (Tasks 3.3f, 4.3c) | `guidance_dialog_gone_sampler_test.go`, `server/wakes/delivery_test.go` | `Sampler_should_treatIndeterminateAsNeitherGoneNorReplaced`; `Delivery_should_writeOnlyWhen_Absent` | Unit | Fake pane double |
| Canary degrades hook rows to `read_only` (Task 3.3h) | `question_dialog_canary_test.go` | see the updated `Canary_*` name in Epic 3 | Unit | Hook row, agent row, `hook_reply` enabled |
| `ANSWER_QUESTION` three-flag composition (Task 0.8d, ADR-008) | `server/services/feature_flags_matrix_test.go` | `TestFlagMatrix_ShouldResolveAnswerQuestionComposition_ForActionRouteClassHiddenRenderAnswerPermutations` | Unit | Hidden live, visible live, durable record, `hook_reply` |
| `rate_limit_deferrals` storage (Tasks 4.1a, 4.1d) | `session/ent_repository_wake_test.go` | `Defer_should_incrementPersistedCounter_And_surviveReopen_And_resetOnClaimAndSkipOccurrence` | Unit | Two deferrals, reopen, third skips a one-shot row |
| Cron floor derived from the per-owner limit (Task 4.3h) | `server/wakes/limits_test.go` | `TestEffectiveCronFloor_ShouldEqualMaxOf5AndCeil60OverLimit_For_limits1To12`; `Schedule_should_refuseStarFiveCron_When_floorConfigured5ButOwnerLimit6` | Unit | Limit 1 to 12; floor 5 with limit 6 |
| Saved answer is read back by the agent (Task 3.4d, UX-AC79) | `server/mcp/tools_guidance_test.go` | `GetGuidanceRequest_should_returnAnsweredWithStoredText_When_savedForLaterThenSessionResumed` | Unit | Replaces the unbacked "delivered on resume" copy |
| Row 16 and row 17 strings; no "Nothing was typed" on the 60 s path (UX-AC23, 24) | `GuidanceQuestionForm.test.tsx` | `DeliveryLine_should_showRow16WithConfirmedRetry_When_dialogStillShowsAt60s`; `_ShouldShowRow17_When_sessionEndsAfterSend`; `_ShouldNeverShowRow10_ForSentRows` | Unit | Fake timers; all start rows |
| Free-text "Other" only on text-carrying routes (UX-AC21) | `GuidanceQuestionForm.test.tsx` | `OtherField_should_renderOnAgentMcpRow_And_beAbsentOnLiveKeystrokeAndHookMirroredRows` | Unit | Same payload, three routes |
| Operator-expanded card stays engaged; pre-expanded first item inert (7.1 (f)) | `useEngagedCard.test.ts`, `useFrozenList.test.ts` | `Engaged_should_holdUntilCollapsed_When_expandedByTapWithoutFocus`; `PreExpandedFirstItem_should_ignoreDigitsUntilTabbedOrClicked` | Unit | Touch device, no focus on tap |
| Async-resolve announcement (Story 2.10, 7.1) | `useFrozenList.test.ts` | `LateAck_should_announceMoveOnlyWhenFocusInsideLeavingRow_And_otherwiseStaySilent` | Unit | Focus inside vs outside |
| Cancelled hold stays in the badge; outside click and offline cancel (Task 2.10c, UX-AC63, T10) | `useCommitDelay.test.ts`, `badgeCount.test.ts` | `CommitDelay_should_cancelOnOutsideClickAndOffline`; `Badge_should_countRestoredCardAfterCancelledHold` | Unit | Panel hidden then shown |
| Vocabulary guard over rendered states (Task 2.5e, UX-AC76) | `copyGuard.test.ts` | see the updated `copyGuard_*` names in Epic 2 | Unit | Every state of sections 1.3, 2.3, 3.1, 3.3, 5.3, 6.3 |
| Task 5.1f baseline row precedes the first `agent_wake_triggers` flip | `docs/reference/notification-actions-observability.md` | `baseline_row_should_exist_before_first_wake_flag_flip` (PR template check, manual) | Manual | Window of at least 7 days, session count |
| Task 4.3d split (4.3d guards, 4.3l revive path) | `server/wakes/guards_test.go`, `server/wakes/revive_test.go` | rows already listed for Story 4.3 | Unit | No test removed |

UX acceptance tests whose steps changed in round 3 (no new UX-AC rows): UX-AC21, 23 (17 strings), 24 (row 16), 35, 36, 37, 39 (Scheduled prompts vocabulary), 49 and 61 (digit grace), 63 (outside click, offline, badge), 67 (plain `sent` under 60 s not counted; `SEND_INDETERMINATE` counted at 5 s), 72 (chip "Needs you (N)"), 76 (rendered-state scan).

### Counts (recounted by grep after the last edit)

- Round 3 totals: 6 epics, 49 stories, 196 tasks, 8 ADRs, 48 glossary terms, 88 UX acceptance criteria, 27 metric names, 17 delivery-state strings. The line below is the round 2 recount and is superseded by this one.
- (Round 2) 6 epics, 49 stories, 193 tasks (`grep -c '^# Epic'`, `'^### Story'`, `'^##### Task'` on `implementation/plan.md`); 8 ADRs (`ls decisions/ADR-*.md | wc -l`); 48 glossary terms; 88 UX acceptance criteria (`grep -c '^- UX-AC' design/ux.md`); 27 metric names in the Observability Plan.
- 439 requirement-to-test rows (`grep -c '^| REQ-'`, 33 requirement ids REQ-1 to REQ-33 unchanged) and 88 UX acceptance-test rows (`grep -c '^| UX-AC'`, one per UX-AC1 to UX-AC88). A row can name several test functions; the row count is the test-plan size, not a function count. The triad round 2 table above adds 22 mapped rows without a `REQ-n`/`UX-ACn` first cell, so the two grep counts are unchanged.
