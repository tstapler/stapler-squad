# Validation Plan: stale-session-detection

**Date**: 2026-08-06

## Happy Path Scenario

Given a user is running 5–10 parallel `ACTIVE` agent sessions in the main session list
(the `requirements.md` Problem Statement baseline) and one of those sessions has produced
no output (`ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate`) for longer than the
configured `StaleSessionConfig.ThresholdMinutesOrDefault()` (default 30 minutes), when the
user opens the session list with no additional clicks or navigation, then that session's
card displays a `"Stale"` badge (and the session appears in the "Stale" grouping bucket if
selected) — surfacing the silently-dead agent without the user having to open each card
individually. All other stories (config plumbing, notification, approval-rule condition,
error/edge handling) are variations feeding or extending this one core scenario, not
independent priorities.

## Requirement → Test Mapping

Requirement IDs map 1:1 to `requirements.md`'s Scope → In Scope bullets (6 items, all
covered below — see Step 6 summary for the fraction).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: Config-driven threshold (Epic 1.1) | `config/types_test.go` | `StaleSessionConfig_should_ResolveDefaults_When_ZeroValue` | Unit (happy) | Zero-value `StaleSessionConfig{}` → `ThresholdMinutesOrDefault()==30`, `NotifyEnabledOrDefault()==true` (Task 1.1.1d) |
| REQ-1 | `config/types_test.go` | `ThresholdMinutesOrDefault_should_ReturnDefault_When_Negative` | Unit (error) | `StaleSessionConfig{ThresholdMinutes:-5}` → returns `30`, not `-5` or `0` (Task 1.1.1d) |
| REQ-1: RPC threading (Epic 1.2) | `server/services/defaults_service_test.go` | `sessionDefaultsToProto_should_ReturnResolvedThreshold_When_ConfigHasNoStaleSessionKey` | Unit (happy) | `config.json` with no `stale_session` key → response `staleSessionThresholdMinutes==30`, `staleSessionNotifyEnabled==true` |
| REQ-1 | `server/services/defaults_service_test.go` | `UpdateGlobalDefaults_should_FallBackToServerDefault_When_ThresholdIsZero` | Unit (error) | `UpdateGlobalDefaults({staleSessionThresholdMinutes:0})` → config left unset, response echoes resolved `30` |
| REQ-1 | `server/services/defaults_service_test.go` | `UpdateGlobalDefaults_should_PersistOverrideToConfigJSON_When_ExplicitValuesProvided` | Integration | `UpdateGlobalDefaults({45, false})` → `config.json`'s `stale_session.threshold_minutes==45` on disk, response echoes `45`/`false` (real file I/O, Task 1.2.1e) |
| REQ-1: Settings UI (Epic 1.3) | `web-app/src/components/settings/GlobalDefaultsForm.test.tsx` | `GlobalDefaultsForm_should_PrefillThresholdAndNotifyFlag_When_Mounted` | Unit (happy) | `getSessionDefaults` returns `{30,true}` → input shows `30`, checkbox checked |
| REQ-1 | `web-app/src/components/settings/GlobalDefaultsForm.test.tsx` | `GlobalDefaultsForm_should_CallUpdateGlobalDefaultsWithNewValues_When_SaveClicked` | Unit (error/edge) | User sets `45` + unchecks notify → Save calls `updateGlobalDefaults({45, false})` |
| REQ-2: Stale badge helper (Epic 2.1) | `web-app/src/lib/session-staleness.test.ts` | `isSessionStale_should_ReturnTrue_When_ActiveAndIdleExceedsThreshold` | Unit (happy) | `ACTIVE` session, idle 45min, threshold 30 → `true` |
| REQ-2 | `web-app/src/lib/session-staleness.test.ts` | `isSessionStale_should_ReturnFalse_When_StatusNotActive` | Unit (error) | `PAUSED` session, idle 5h → `false` regardless of timestamps |
| REQ-2 | `web-app/src/lib/session-staleness.test.ts` | `isSessionStale_should_ReturnFalse_When_NoRecordedActivity` | Unit (edge) | `ACTIVE` session, both timestamps unset (`seconds:0n`) → `false` (never flag a brand-new session) |
| REQ-2 | `web-app/src/lib/session-staleness.test.ts` | `getLastActivityTimestamp_should_ReturnMoreRecentTimestamp_When_BothSet` | Unit (happy) | `lastMeaningfulOutput=100`, `lastTerminalUpdate=200` → returns `200` (behavior-identical to replaced IIFE) |
| REQ-2: Badge UI (Epic 2.2) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_RenderStaleBadgeWithAccessibleLabel_When_ActiveSessionExceedsThreshold` | Unit (happy) | Stale `ACTIVE` session → visible "Stale" text + `role="img"` `aria-label` starting "Stale — no output for" |
| REQ-2 | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_NotRenderStaleBadge_When_PausedSessionWithOldTimestamp` | Unit (error) | `PAUSED` session, last output 6h ago → no "Stale" badge anywhere |
| REQ-2 | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_UseWarningColorTokens_When_BadgeRenders` | Unit (edge) | Badge CSS asserts `vars.color.warningBg`/`warningText`/`warning`, not a new hex value |
| REQ-2: Live re-render tick (Epic 2.3) | `web-app/src/lib/hooks/useStaleSessionConfig.test.ts` | `useStaleSessionConfig_should_ReturnFetchedConfig_When_FetchResolves` | Unit (happy) | `getSessionDefaults` returns `{45,true}` → hook returns `{thresholdMinutes:45, notifyEnabled:true}` |
| REQ-2 | `web-app/src/lib/hooks/useStaleSessionConfig.test.ts` | `useStaleSessionConfig_should_ReturnSafeDefault_When_FetchPending` | Unit (edge) | Before fetch resolves → hook returns `{30,true}` |
| REQ-2 | `web-app/src/components/sessions/SessionList.test.tsx` | `SessionList_should_ReclassifySessionAsStale_When_60SecondTickElapsesWithNoNewData` | Integration | Session fresh 29min ago (threshold 30) + 2 more minutes fake-timer elapse, no new `WatchSessions` event → badge/group reclassify within 60s (composes hook + tick + helper + card) |
| REQ-3: "Stale" grouping (Epic 3.1) | `web-app/src/lib/grouping/strategies.test.ts` | `groupSessions_should_BucketOnlyStaleActiveSessions_When_StaleStrategySelected` | Unit (happy) | 3 sessions (stale A, fresh B, paused-idle-6h C) → `"Stale"` group contains only A |
| REQ-3 | `web-app/src/lib/grouping/strategies.test.ts` | `groupSessions_should_ExcludePausedSessionFromStaleGroup_When_PausedSessionIdleSixHours` | Unit (error) | Paused session, 6h idle → never appears in `"Stale"` bucket (mirrors AC1.3 at grouping layer) |
| REQ-3 | `web-app/src/lib/grouping/strategies.test.ts` | `GroupingStrategyLabels_should_HaveStaleLabel_When_StaleEnumMemberAdded` | Unit (edge) | `GroupingStrategyLabels[GroupingStrategy.Stale] === "Stale"` — guards the not-compile-enforced labels map (X9) |
| REQ-3 | `web-app/src/lib/grouping/strategies.test.ts` | `groupSessions_should_ExcludeStaleFromSpecialGroupsSort_When_Bucketed` | Unit (edge) | `"Stale"` is not present in the `specialGroups` end-of-list sort array — sorts normally |
| REQ-4: Notifier core (Epic 4.1) | `server/services/stale_session_notifier_test.go` | `checkAll_should_NotifyExactlyOnce_When_SessionStaleAcrossMultipleTicks` | Unit (happy) | Session crosses threshold at tick N, still stale at N+1/N+2 → exactly one `NOTIFICATION_TYPE_WARNING` published |
| REQ-4 | `server/services/stale_session_notifier_test.go` | `checkAll_should_ReArmAndNotifyAgain_When_SessionRecoversThenGoesStaleAgain` | Unit (edge) | Recovers (idle drops under threshold) then goes stale again → second, new notification fires |
| REQ-4 | `server/services/stale_session_notifier_test.go` | `checkAll_should_NotNotify_When_SessionTransitionsToPausedSameTick` | Unit (error) | `ACTIVE→PAUSED` in the same tick idle crosses threshold → no notification (status checked at emission time) |
| REQ-4 | `server/services/stale_session_notifier_test.go` | `checkAll_should_NotPublish_When_NotifyEnabledIsFalse` | Unit (error) | `NotifyEnabledOrDefault()==false`, session past threshold → no event published |
| REQ-4 | `server/services/stale_session_notifier_test.go` | `checkAll_should_ObserveConfigChange_When_ConfigFileChangesBetweenTicks` | Integration | Write config, `checkAll()`, rewrite config with new threshold, `checkAll()` again → second call uses new value (real file I/O; proves live-reload without restart, closes adversarial-review BLOCKER) |
| REQ-4 | `server/services/stale_session_notifier_test.go` | `checkAll_should_ReNotify_When_SessionPausesThenResumesStillStale` | Integration | Stale→notified→`PAUSED` (dedup entry cleared)→back to `ACTIVE` still past threshold→`checkAll()` fires a *second* notification (multi-tick instance-state composition; closes adversarial-review Concern) |
| REQ-4: Server wiring (Epic 4.2) | `server/server_test.go` (or wherever `wireDepsIntoServer` is tested) | `wireDepsIntoServer_should_StartNotifierAndLogThreshold_When_ReviewQueuePollerAvailable` | Unit (happy) | `deps.ReviewQueuePoller != nil` → `StaleSessionNotifier` constructed, `Start(serverCtx)` called, startup log emitted |
| REQ-4 | manual (Task 4.2.1b, not automated) | `StaleSessionNotifier_manual_smoke_should_FireOnceAndClear_When_RealSessionIdlesPastLoweredThreshold` | Manual/E2E | Second manual instance, `threshold_minutes:1`, real idle session → exactly one NotificationPanel entry, no repeats, clears on next output |
| REQ-5: Proto + ent field (Epic 5.1) | `session/ent/schema/approvalrule_test.go` or `pkg/gen` round-trip test | `ApprovalRuleProto_should_RoundTripMinSessionIdleMinutes_When_FieldSet` | Unit (happy) | `MinSessionIdleMinutes:60` serialize→deserialize → `60` preserved |
| REQ-5 | same | `ApprovalRuleProto_should_DecodeToZero_When_FieldNotSet` | Unit (error) | No field set → `MinSessionIdleMinutes==0` (Go zero value, consistent with `require_ci_passing`'s off-idiom) |
| REQ-5: Conversion layers (Epic 5.2) | `server/services/rules_service_test.go` (or `rules_store_test.go`) | `UpsertApprovalRule_should_PersistAndReturnMinSessionIdleMinutes_When_RuleSaved` | Integration | `UpsertApprovalRule({min_session_idle_minutes:60})` → `ListApprovalRules` returns `60` (real ent-backed store round trip, Task 5.2.1e) |
| REQ-5 | `server/services/rules_store_test.go` | `RuleSpec_should_OmitEmptyMinSessionIdleMinutes_When_ZeroValue` | Unit (edge) | `RuleSpec{MinSessionIdleMinutes:0}` JSON-marshals without the key (`omitempty` contract) |
| REQ-5: Classifier match (Epic 5.3) | `pkg/classifier/classifier_test.go` | `TestClassify_MinSessionIdleMinutes_Matches_WhenIdleExceedsThreshold` | Unit (happy) | `Rule{MinSessionIdleMinutes:60}`, `ctx.SessionIdleMinutes:75` → condition passes |
| REQ-5 | `pkg/classifier/classifier_test.go` | `TestClassify_MinSessionIdleMinutes_DoesNotMatch_WhenIdleBelowThreshold` | Unit (error) | Same rule, `ctx.SessionIdleMinutes:10` → `matchesRule` returns `false` |
| REQ-5 | `pkg/classifier/classifier_test.go` | `TestClassify_MinSessionIdleMinutes_ANDsWithOtherConditions_WhenCombinedWithRequireCIPassing` | Unit (edge) | `RequireCIPassing:true` + `MinSessionIdleMinutes:60`, `CIStatus:"success"` but idle `5` → `false` (idle alone blocks) |
| REQ-5 | `pkg/classifier/classifier_test.go` | `TestClassify_MinSessionIdleMinutes_FailsClosed_WhenContextIdleUnset` | Unit (error, fail-closed) | `Rule{MinSessionIdleMinutes:60}`, `ClassificationContext{}` zero value → `false`, never accidentally matches |
| REQ-5: Context population (Epic 5.4) | `server/services/approval_handler_test.go` | `HandlePermissionRequest_should_PopulateSessionIdleMinutes_When_LiveInstanceFound` | Unit (happy) | Live instance `GetTimeSinceLastMeaningfulOutput()==75min` → `classCtx.SessionIdleMinutes==75` |
| REQ-5 | `server/services/approval_handler_test.go` | `HandlePermissionRequest_should_LeaveSessionIdleMinutesZero_When_NoLiveInstanceFound` | Unit (error, fail-closed) | `FindLiveInstance` returns `nil` → `classCtx.SessionIdleMinutes==0` (never a sentinel "unknown" value) |
| REQ-5: Rule builder UI (Epic 5.5) | `web-app/src/components/rules/RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_PrefillMinSessionIdleMinutes_When_EditingExistingRule` | Unit (happy) | `editRule.minSessionIdleMinutes===60` → input shows `60` |
| REQ-5 | `web-app/src/components/rules/RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_IncludeMinSessionIdleMinutesInPayload_When_Saved` | Unit (edge) | User sets `90`, submits → `upsertApprovalRule` payload includes `minSessionIdleMinutes:90` |
| REQ-5: End-to-end chain (Epic 5.6) | `server/services/approval_handler_test.go` | `HandlePermissionRequest_should_DenyWithMatchingRuleID_When_SessionIdleExceedsRuleThreshold` | Integration | `UpsertApprovalRule({Deny, MinSessionIdleMinutes:30, Bash})` upserted, live instance idle 45min → `Bash` permission request → `Deny` decision citing that rule's ID (full chain, satisfies requirements.md's "exercised by at least one test" success metric) |
| REQ-6: Feature registry (Epic 6.1) | N/A — process check, not a test file | `registry-generate_should_ProduceNoNewCoverageGaps_When_ThreeFeatureFilesAdded` | Process/CI | `make registry-generate` after adding the 3 per-feature JSON files → `docs/registry/coverage-gaps.json` count unchanged vs. `main` |

### Migration Risk Test (Step 5 — no reversibility test needed; both changes are additive)

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Migration: `config.json` decode | `config/types_test.go` | `StaleSessionConfig_should_DecodeToDefaults_When_LoadingPreExistingConfigJSON` | Unit (migration) | A `config.json` written before this feature existed (no `stale_session` key at all) decodes to `StaleSessionConfig{}` Go zero value, and `OrDefault()` accessors resolve to `(30, true)` — identical forward-compat behavior to `SessionRetentionConfig`'s existing rollout |
| Migration: ent `ApprovalRule` row decode | `session/ent_repository_test.go` (or wherever pre-existing-row backfill is tested) | `ApprovalRule_should_DefaultMinSessionIdleMinutesToZero_When_ReadingPreExistingRow` | Unit (migration) | An `ApprovalRule` row persisted before this feature existed (no `min_session_idle_minutes` column value) reads back as `MinSessionIdleMinutes==0` — "condition not applied," the correct behavior for every pre-existing rule, no `UPDATE` migration statement required |

This satisfies Step 5's instruction in place of a reversibility test: both schema-adjacent
changes (`config.json`'s `StaleSessionConfig`, ent's `min_session_idle_minutes` field) are
additive-only per the plan's own Migration Plan section — the real risk surface is
old-data-decodes-to-safe-defaults, not up/down migration, and both directions (config +
ent) are covered above.

## UX Acceptance Tests

**Note on source-count verification**: `design/ux.md`'s own Summary states "27 UX
acceptance criteria... 36 total," but a direct recount of the criteria actually enumerated
in the document (AC1.1–AC1.5=5, AC2.1–AC2.4=4, AC3.1–AC3.5=5, AC4.1–AC4.4=4,
AC5.1–AC5.4=4 → 22 surface-specific, plus X1–X9=9 cross-cutting) totals **31**, not 36.
This validation plan tests the 31 criteria actually present in the document (flagging the
discrepancy rather than silently reproducing it — see `.claude/CLAUDE.md`'s "recount every
number" discipline). All 31 are covered below.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1.1 (0-click discovery) | `SessionCard.test.tsx` | `SessionCard_should_ShowStaleBadgeWithNoUserAction_When_SessionIsStale` | Jest/RTL | Render card for a stale session with default props; assert badge visible with zero interaction |
| AC1.2 (badge never color-only) | `SessionCard.test.tsx` | `SessionCard_should_PairIconWithTextAndAriaLabel_When_BadgeRenders` | Jest/RTL | Assert visible text "Stale" AND `aria-label` starting "Stale — no output for" both present |
| AC1.3 (no badge on non-ACTIVE) | `SessionCard.test.tsx` | `SessionCard_should_NeverShowStaleBadge_When_StatusIsPausedStoppedHibernatedOrCreating` | Jest/RTL | Parameterized over 4 statuses, 6h-old timestamp each; assert badge absent in all 4 |
| AC1.4 (reused warning tokens) | `SessionCard.css.ts` review + `SessionCard.test.tsx` | `SessionCard_should_ApplyWarningTokenTriplet_When_BadgeRenders` | Jest/RTL + manual CSS diff review | Assert computed class references `warningBg`/`warningText`/`warning`; manual review confirms no new hex value in the diff |
| AC1.5 (no dead end / badge non-interactive) | `SessionCard.test.tsx` | `SessionCard_should_StillOpenSessionOnClick_When_StaleBadgeIsPresent` | Jest/RTL | Click card body (not badge) on a stale card → session-open handler still fires normally |
| AC2.1 (≤2 steps to view stale group) | Playwright / manual | `SessionList_manual_should_ShowStaleGroupIn2Clicks_When_UserSelectsStaleFromDropdown` | Manual / `ui-playwright` | Open "Group by" dropdown (1), select "Stale" (2) → grouped view renders, no third step |
| AC2.2 (keyboard-reachable label) | `SessionList.test.tsx` | `GroupBySelect_should_ExposeStaleAsKeyboardReachableOption_When_Rendered` | Jest/RTL | `Tab` into select, arrow-key to "Stale" option, `Enter` selects it — no custom widget/focus trap |
| AC2.3 (paused session excluded from Stale group) | `strategies.test.ts` | `groupSessions_should_ExcludePausedSessionFromStaleGroup_When_PausedSessionIdleSixHours` | Jest | (Same test as REQ-3 row above — this AC and that unit test are the same assertion, listed once) |
| AC2.4 (empty-state with exit path) | `SessionList.test.tsx` | `SessionList_should_ShowEmptyStaleStateWithExitPath_When_ZeroSessionsAreStale` | Jest/RTL | `groupSessions` returns empty "Stale" bucket → empty-state message renders, "Group by" selector remains visible/interactive |
| AC3.1 (exactly one toast+panel entry) | `stale_session_notifier_test.go` + `NotificationContext.test.tsx` | `checkAll_should_NotifyExactlyOnce_When_SessionStaleAcrossMultipleTicks` (backend) / `NotificationContext_should_AddOneToastAndOnePanelEntry_When_StaleEventReceived` (frontend) | Go test + Jest/RTL | Backend: covered above. Frontend: publish one stale event → assert `notifications` and `notificationHistory` each gain exactly one entry |
| AC3.2 (auto-minimize, not auto-close) | `NotificationToast.test.tsx` | `NotificationToast_should_MinimizeNotClose_When_5SecondsElapseForWarningType` | Jest/RTL + fake timers | Render warning toast, advance timers 5000ms → toast collapses to pill, `document.body` still contains it (not removed) |
| AC3.3 (every notification has a next action) | `NotificationToast.test.tsx`, `NotificationPanel.test.tsx` | `NotificationToast_should_RenderFocusAction_When_StaleNotificationShown` / `NotificationPanel_should_RenderFocusSessionAction_When_StaleEntryShown` | Jest/RTL | Assert "Focus"/"Focus session" button present and calls the session-navigate handler on click |
| AC3.4 (disabling notify → zero toasts, no restart) | `stale_session_notifier_test.go` | `checkAll_should_NotPublish_When_NotifyEnabledIsFalse` | Go test | (Same test as REQ-4 row above) |
| AC3.5 (paused session never notifies) | `stale_session_notifier_test.go` | `checkAll_should_NotNotify_When_SessionTransitionsToPausedSameTick` | Go test | (Same test as REQ-4 row above) |
| AC4.1 (≤3 steps to configure) | Manual / Playwright | `Settings_manual_should_ConfigureThresholdAndNotifyIn3Steps_When_UserEditsAndSaves` | Manual / `ui-playwright` | Navigate to Settings (1), edit field(s) (2), click Save (3) — no modal, no extra page |
| AC4.2 (resolved value shown before fetch resolves) | `GlobalDefaultsForm.test.tsx` | `GlobalDefaultsForm_should_ShowDefaultThirty_When_FetchNotYetResolved` | Jest/RTL | Mount with fetch promise unresolved → input shows `30`, not blank/`0` |
| AC4.3 (inline hint on `0` entry) | `GlobalDefaultsForm.test.tsx` | `GlobalDefaultsForm_should_ShowInlineHint_When_ThresholdEnteredAsZero` | Jest/RTL | Type `0` into threshold input → inline hint text "falls back to the default (30)" (or equivalent) renders near the field |
| AC4.4 (failed save keeps input, offers retry) | `GlobalDefaultsForm.test.tsx` | `GlobalDefaultsForm_should_PreserveInputAndOfferRetry_When_SaveRpcFails` | Jest/RTL | Mock `updateGlobalDefaults` to reject → typed values remain on screen, Save button still enabled/clickable |
| AC5.1 (≤1 extra step in rule editor) | `RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_RequireOnlyOneFieldFill_When_AddingIdleMinutesCondition` | Jest/RTL | Within an already-open rule edit, fill the one numeric field, no new modal/page opened |
| AC5.2 (helper text states 0=off and fail-closed) | `RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_RenderHelperTextWithZeroAndFailClosedContract_When_FieldRendered` | Jest/RTL | Assert helper text contains both "0" / "not applied" and the fail-closed sentence |
| AC5.3 (round-trips on re-edit) | `RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_ShowSavedValueOnReopen_When_EditingRuleAgain` | Jest/RTL | Save rule with `minSessionIdleMinutes:60`, reopen editor with that saved rule → input shows `60`, not `0` |
| AC5.4 (failed save keeps modal + input, offers retry) | `RuleBuilderForm.test.tsx` | `RuleBuilderForm_should_KeepModalOpenAndInputIntact_When_SaveRuleFails` | Jest/RTL | Mock `upsertApprovalRule` to reject → modal stays open, input values intact, "Save Rule" still actionable |
| X1 (0 or ≤2 clicks to find stale session) | Combined — see AC1.1, AC2.1 | (covered by AC1.1 + AC2.1 above) | Jest/RTL + Manual | No additional test — cross-cutting restatement of AC1.1/AC2.1 |
| X2 (≤3 / ≤1 config steps) | Combined — see AC4.1, AC5.1 | (covered by AC4.1 + AC5.1 above) | Manual + Jest/RTL | No additional test — cross-cutting restatement |
| X3 (every error state preserves input + retry) | Combined — see AC4.4, AC5.4 | (covered by AC4.4 + AC5.4 above) | Jest/RTL | No additional test — cross-cutting restatement |
| X4 (every notification has a next action) | Combined — see AC3.3 | (covered by AC3.3 above) | Jest/RTL | No additional test — cross-cutting restatement |
| X5 (color never sole signal) | Combined — see AC1.2, AC3.3 (icon+text pairing) | `NotificationToast_should_PairWarningIconWithTitleAndMessageText_When_StaleToastRenders` | Jest/RTL | Assert icon has `aria-hidden="true"` and adjacent title/message text nodes exist |
| X6 (`role="img"` + full-sentence `aria-label`) | Combined — see AC1.2 | (covered by AC1.2 above) | Jest/RTL | No additional test — cross-cutting restatement |
| X7 (keyboard navigation, no focus trap) | Combined — see AC2.2; extended to Settings/rule-builder inputs | `GlobalDefaultsForm_should_AssociateLabelsWithNativeInputs_When_Rendered` / `RuleBuilderForm_should_AssociateLabelsWithNativeInputs_When_Rendered` | Jest/RTL (`axe-core` or manual `htmlFor` assertion) | Assert `<label htmlFor="...">` wraps/references each new `<input>`; standard tab order (no `tabIndex` overrides) |
| X8 (contrast ≥4.5:1 via reused tokens) | Combined — see AC1.4 | (covered by AC1.4 above) | Jest/RTL + manual CSS review | No additional test — cross-cutting restatement |
| X9 (labels map not compile-enforced) | Combined — see AC2.2 / `GroupingStrategyLabels` test | (covered by REQ-3's `GroupingStrategyLabels_should_HaveStaleLabel_When_StaleEnumMemberAdded` above) | Jest | No additional test — cross-cutting restatement |

31 UX criteria total: 22 have a dedicated new test; the 9 cross-cutting (X1–X9) are, by
design, restatements that resolve to an already-listed AC-level test (X1→AC1.1/AC2.1,
X2→AC4.1/AC5.1, X3→AC4.4/AC5.4, X4→AC3.3, X6→AC1.2, X8→AC1.4, X9→the grouping-labels test)
except X5 and X7, which get one dedicated test each since they generalize the pairing/
labeling pattern to Surface 3 and Surfaces 4/5 respectively (not already exercised by an
existing AC test on those specific surfaces).

## Test Stack

- **Unit (Go)**: standard `testing` package + table-driven tests, matching the existing
  `pkg/classifier/classifier_test.go` (`TestClassify_RequireCIPassing_*`) and
  `session_retention_sweeper`-adjacent test shapes already in the codebase. Fake/minimal
  `*session.Instance` fixtures per Task 4.1.1e (reuse whatever factory
  `backlog_service_triage_test.go` already has); fake `events.EventBus` subscriber for
  notification-publish assertions.
- **Unit (TypeScript/Jest)**: Jest + React Testing Library for component tests
  (`SessionCard.test.tsx`, `GlobalDefaultsForm.test.tsx`, `RuleBuilderForm.test.tsx`,
  `NotificationToast.test.tsx`, `NotificationPanel.test.tsx`); plain Jest for pure-function
  modules (`session-staleness.test.ts`, `strategies.test.ts`, hook tests). `jest.useFakeTimers()`
  for the 60s tick and 5s toast-minimize assertions.
- **Integration (Go)**: real ent-backed SQLite store for the `ApprovalRule` round trip
  (Epic 5.2/5.6), real `config.LoadConfig()` file I/O for the notifier's live-reload test
  (Epic 4.1) and `UpdateGlobalDefaults`'s config.json persistence (Epic 1.2) — no mocks for
  the store/file layer in these specific tests, since the risk being tested *is* the
  persistence boundary itself.
- **Integration (Frontend)**: `SessionList.test.tsx` composing the real
  `useStaleSessionConfig` hook + `groupSessions` + `SessionCard` together under fake timers
  (Epic 2.3) — verifies the three modules agree, not just each in isolation.
- **E2E / UX**: manual smoke test for the notifier (Task 4.2.1b, explicitly not automated
  per the plan) and `ui-playwright`-driven manual verification for the ≤2-step /
  ≤3-step interaction-count acceptance criteria (AC2.1, AC4.1) where step-counting is the
  actual thing under test, not a DOM assertion.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this plan (`StaleSessionConfig`'s accessors,
  `StaleSessionNotifier.checkAll`/`notify`, `matchesRule`'s new condition,
  `HandlePermissionRequest`'s context population, the two RPC handler extensions): happy
  path + error/fail-closed paths covered per the table above.
- All external integrations (ent-backed `ApprovalRule` store, `config.json` file I/O, the
  notification event bus): unit-mocked in the classifier/handler-population tests, plus at
  least one integration test each (Epic 1.2, 4.1, 5.2, 5.6 rows above) exercising the real
  boundary.
- UX acceptance criteria: all 31 criteria in `design/ux.md` (see recount note above) have a
  corresponding automated test or an explicitly-labeled manual step — none silently
  unaccounted for.
- `make registry-generate` run with zero net-new entries in `docs/registry/coverage-gaps.json`
  relative to `main`, per REQ-6's row above and `.claude/rules/feature-registry.md`.
