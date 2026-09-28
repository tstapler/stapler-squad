# Validation Plan: backlog-diagnose-and-nudge

**Date**: 2026-09-27

## Derived Requirement List

`requirements.md`'s Scope → In Scope has no numbered `REQ-N` list; this plan derives
one from it, split to match `implementation/plan.md`'s Phase boundaries so each
requirement maps to a coherent set of files:

| ID | Requirement | Plan Phase(s) |
|----|-------------|----------------|
| REQ-1 | Diagnostic bundle assembly (item/AC/history/verdicts/snapshot/logs/diff/linked-transcript sections) | Phase 2, Epic 2.1 |
| REQ-2 | Bundle token/byte budgeting + compaction (per-section budget enforcement, `HandoffSummaryGenerator` reuse for the transcript section only) | Phase 1 (Epic 1.2.3), Phase 2 (Epic 2.1.3, 2.2) |
| REQ-3 | Diagnose UI action (button wiring, outcome display, history list) | Phase 8 |
| REQ-4 | Dispatch orchestration (`RequestDiagnosis`, concurrency guard, headless session dispatch, outcome persistence/notification wiring) | Phase 5 |
| REQ-5 | Nudge safety gates: flag → idle → identity → cap fail-closed pipeline, plus the dispatch-level write-attempt guard | Phase 3, Phase 4 |
| REQ-6 | Stale-session cleanup (idle-stale predicate, handoff-then-cleanup, `IsArchived()` consultation) | Phase 6 |
| REQ-7 | Structured logging / durable two-part notification (`notifyDiagnoseEvent`) | Phase 5, Epic 5.2 |
| REQ-8 | RPC + backend API surface (`DiagnoseBacklogItem`, `ListDiagnoseDispatches`) | Phase 7 |
| REQ-9 | Feature registry entries (RPC + component markers) | Phase 7.1.2, Phase 8.4 |

## Happy Path Scenario

Given backlog item `e6c2a88e` is flagged stuck (rework-cap hit) with one linked
session that has been continuously `detection.StatusIdle` (safe-steer context) for
longer than the idle-settle window, and `DiagnoseNudgeExecutionFeatureFlag` is on,
when Tyler clicks **Diagnose** on `StuckItemDetail`, then: a `DiagnoseDispatch` row is
persisted `Pending` before any session exists; a `headless-diagnose-e6c2a88e-<uuid>`
session is dispatched carrying the assembled, budget-capped `DiagnosticBundle`; the
diagnostic agent calls `steer_session`, which passes the `NudgeGate` pipeline (flag,
idle, identity, cap all green) and `verifyIdentityImmediatelyBeforeWrite`'s final
pre-write re-check, and the write succeeds; the dispatch is recorded `Completed` with
`DiagnoseOutcome{Kind: Nudged}`; and `DiagnoseOutcomeDisplay` renders "Diagnosed just
now — nudged the session" with a link into the diagnostic session, which still renders
identically after a page refresh (sourced from `ListDiagnoseDispatches`, never
client-only state).

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1 | `session/diagnose/bundle_test.go` | `TestDiagnosticBundleAssembler_AssembleHistory_ShouldIncludeAllSessions_WhenHistoryFitsSectionBudget` | Unit (happy) | 3 `ItemSessionSummary` rows, all fit `History`'s `SectionBudget.MaxBytes` → all 3 included, no drop note. |
| REQ-1 | `session/diagnose/bundle_test.go` | `TestDiagnosticBundleAssembler_AssembleHistory_ShouldDropOldestSessionsFirst_WhenHistoryExceedsSectionBudget` | Unit (error/edge) | 3 sessions totaling 40,000 bytes, `MaxBytes=30,000` → 2 most recent kept, one-line "1 older session dropped" note (Story 2.1.1 AC). |
| REQ-1 | `session/diagnose/bundle_test.go` | `TestDiagnosticBundleAssembler_AssembleDiff_ShouldReflectRealWorktreeChanges_WhenRunAgainstLiveGitRepo` | Integration | Real `go-git`-opened temp repo with uncommitted changes at `Workspace().ActiveDir`; asserts the `Diff` section contains the actual diff text — exercises the real filesystem/git dependency `enforceBudget`'s unit tests stub out. |
| REQ-2 | `session/diagnose/bundle_budget_test.go` | `TestEnforceBudget_ShouldPassthroughContent_WhenUnderSectionByteBudget` | Unit (happy) | Content `< MaxBytes` → returned unchanged, no truncation marker. |
| REQ-2 | `session/diagnose/bundle_budget_test.go` | `TestEnforceBudget_ShouldTruncateWithMarker_WhenContentExceedsSectionByteBudget` | Unit (error/edge) | 500,000-byte `Logs` content, `MaxBytes=240,000` → result `<= 240,000` bytes, ends with `"[... truncated, N bytes omitted for budget ...]"` (Story 2.1.3 AC — never a silent, unmarked cut). |
| REQ-2 | `session/diagnose/bundle_transcript_test.go` | `TestAssembleLinkedTranscript_ShouldPollUntilReadyAndSubstituteSummary_WhenTranscriptExceedsBudget` | Integration | Real `HandoffSummaryGenerator` + in-memory-sqlite-backed `HandoffSummary` row (not a fake): 300,000-byte transcript, 200,000-byte budget → `BeginGeneration`/`GenerateAndPersist` invoked, poll observes `ready`, section contains generated summary, not raw transcript (Story 2.2.1 AC). |
| REQ-3 | `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx` | `onDiagnose_should_TransitionIdleToPendingToIdle_When_DispatchResolves` | Unit (happy) | Click → `disabled`+`aria-busy="true"`+"Diagnosing…" immediately → reverts to idle "Diagnose" on resolve (Story 8.1.1 AC). |
| REQ-3 | `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx` | `onDiagnose_should_RenderRoleAlertError_When_DispatchRejects` | Unit (error) | `onDiagnose` rejects with `"already diagnosing"` → `role="alert"` renders the message, mirroring `overrideState === "error"`. |
| REQ-3 | `tests/e2e/diagnose-dispatch.spec.ts` | `diagnose_button_should_produce_persisted_nudged_outcome_end_to_end_via_real_rpc` | Integration (E2E) | Full stack against the isolated Playwright test server: click Diagnose → real `DiagnoseBacklogItem` RPC → outcome renders from a real `ListDiagnoseDispatches` response, not a mock. |
| REQ-4 | `server/services/diagnose_dispatcher_test.go` | `TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowThenDispatchSession_WhenNoInFlightDispatchExists` | Unit (happy) | Guard passes → `Pending` row recorded via `DiagnoseDispatchStore.Record` before `dispatch` (Story 5.1.2) is called. |
| REQ-4 | `server/services/diagnose_dispatcher_test.go` | `TestDiagnoseDispatcher_RequestDiagnosis_ShouldMarkDispatchFailedWithoutRetry_WhenSessionCreationReturnsConnectionError` | Unit (error) | Session-creation call returns `ECONNREFUSED`-shaped error → existing `Pending` row updated to `Completed`/`DispatchFailed`, no automatic retry, no second row (Story 5.1.3 AC). |
| REQ-4 | `server/services/diagnose_dispatcher_test.go` | `TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowQueryableBeforeSessionCompletes_WhenQueriedDuringDispatch` | Integration | In-memory-sqlite `DiagnoseDispatchStore` (real ent client, per this repo's test-isolation convention) + fake session manager: simulates a page-refresh race, asserting `ListByItem` returns the `Pending` row before the diagnostic session finishes (Task 5.1.1e / architecture-review Blocker 1). |
| REQ-5 | `session/diagnose/nudge_gate_test.go` | `TestNudgeGate_Evaluate_ShouldReturnTrue_WhenFlagIdleIdentityAndCapChecksAllPass` | Unit (happy) | All four checks pass → `(true, nil)`, caller proceeds to the final pre-write re-check. |
| REQ-5 | `session/diagnose/nudge_gate_test.go` | `TestNudgeGate_Evaluate_ShouldReturnNudgeExecutionDisabled_AndSkipIdleIdentityCap_When_FlagCheckFails` | Unit (error) | Flag off → returns `SafetyGateReasonNudgeExecutionDisabled`; a spy asserts zero calls to the idle/identity/cap checks (Story 3.4.1 AC). |
| REQ-5 | `server/mcp/tools_terminal_test.go` | `TestSteerSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenNudgeExecutionFlagDisabled` | Integration | Full `steerSession` MCP handler wired to the real `NudgeGate` + a fake tmux backend: flag off → MCP error result containing `"nudge_execution_disabled"`, no write call reaches the fake backend (Story 4.1.1 AC). |
| REQ-6 | `server/services/superseded_session_sweeper_test.go` | `TestFindIdleStaleSessions_ShouldReturnSession_WhenIdleSustainedWithNoNewerRound` | Unit (happy) | 2h-idle session, no newer round for item+role → returned (Story 6.1.1 AC). |
| REQ-6 | `server/services/diagnose_stale_session_cleanup_test.go` | `TestHandoffThenCleanup_ShouldLogWarnAndPostNoteThenArchiveAnyway_WhenHandoffSummaryGenerationErrorsOrTimesOut` | Unit (error) | `GenerateAndPersist` resolves `error` (or exceeds `handoffSummaryTimeout`) → `diagnose.cleanup.handoff_error_fallback` warning logged, diagnostic note posted, session still archived — never left running forever (Story 6.1.2 AC, MDD #3's resolved default). |
| REQ-6 | `server/services/diagnose_stale_session_cleanup_test.go` | `TestHandoffThenCleanup_ShouldArchiveOnlyAfterSummaryRowConfirmedDurable_WhenGenerationResolvesReady` | Integration | Real `HandoffSummaryGenerator` + in-memory-sqlite `HandoffSummary`/session store + fake tmux archive call: asserts `ArchiveSessionByUUID` is invoked strictly after the `ready` row is observed, never before — the exact ordering `requirements.md`'s Rabbit Holes section calls out as a failure-mode risk. |
| REQ-7 | `server/services/diagnose_dispatcher_test.go` | `TestNotifyDiagnoseEvent_ShouldMarkDispatchCompletedAndPublishLiveEvent_WhenOutcomeRecorded` | Unit (happy) | `MarkCompleted` called, then `EventBusNotifier.Publish` invoked with `itemID` as the coalescing key, mirroring `notifyReworkCapHit`. |
| REQ-7 | `server/services/diagnose_dispatcher_test.go` | `TestNotifyDiagnoseEvent_ShouldStillPersistOutcomeDurably_WhenLiveEventBusPublishFails` | Unit (error) | Live publish fails (event bus down) → durable row still updated to `Completed`/`Nudged` regardless (Story 5.2.2 AC — durability unconditional). |
| REQ-7 | `server/services/backlog_notifier_test.go` | `TestNotifyDiagnoseEvent_ShouldUpdateExistingPendingRowToCompleted_NotInsertSecondRow_WhenCalledAgainstRealStore` | Integration | Real ent-backed `DiagnoseDispatchStore` + real `EventBusNotifier`: same row ID/`CreatedAt` before and after, now `Completed` — never a duplicate insert. |
| REQ-8 | `server/services/diagnose_service_test.go` | `TestDiagnoseBacklogItem_ShouldReturnDispatchIDAndSessionID_WhenNoInFlightDispatchExists` | Unit (happy) | `{item_id: "e6c2a88e"}` → `{dispatch_id, diagnostic_session_id: "headless-diagnose-e6c2a88e-<uuid>"}`. |
| REQ-8 | `server/services/diagnose_service_test.go` | `TestDiagnoseBacklogItem_ShouldReturnAlreadyInFlightError_WhenDispatchAlreadyRunningForItem` | Unit (error) | A second call for the same item while one is in flight → gRPC error, no second dispatch started. |
| REQ-8 | `server/services/diagnose_service_test.go` | `TestListDiagnoseDispatches_ShouldReturnSamePendingRowAcrossRepeatedCalls_WhenDispatchStillInFlight` | Integration | Real ConnectRPC handler → real `DiagnoseDispatcher`/`DiagnoseDispatchStore`: two successive calls mid-dispatch return the identical `Pending` row — the concrete API-level fix for architecture-review Blocker 1 (Story 7.1.1 AC). |
| REQ-9 | `docs/registry/features/*.json` (generated) | `make registry-generate && make registry-diff` | Static/CI check (happy) | `// +api: backlog:diagnose` / `// +feature: backlog-diagnose-nudge` markers present → registry file created/updated, `registry-diff` reports no drift. |
| REQ-9 | `docs/registry/features/*.json` (generated) | `make registry-diff` (run *without* first running `registry-generate` after a marker is added) | Static/CI check (error) | A marker added but the generated file not regenerated/committed → `registry-diff` reports drift and fails CI, per this repo's existing registry-gate convention. |
| Migration | `session/ent/schema/migration_test.go` | `TestMigration_should_BeReversible_When_NudgeCapRecordAndDiagnoseDispatchTablesAreCreated` | Migration | See dedicated section below. |

Note on REQ-9: there is no meaningful integration test distinct from the CI check
itself — the registry mechanism has no runtime logic beyond marker-scanning and JSON
generation, so a third row would be redundant with the two above.

## Safety-Critical Path Tests

The plan's own review-repair passes (adversarial review, architecture review) added
four mechanisms explicitly because a defect in any one of them lets an autonomous
write land against the wrong session, twice, or with no way to stop it. Each gets
named test cases here, not just "covered by the general suite" — per the task's
explicit instruction.

### 1. `NudgeGate` fail-closed short-circuit (Story 3.4.1)

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `session/diagnose/nudge_gate_test.go` | `TestNudgeGate_Evaluate_ShouldAbortOnFirstFailingCheck_AndNeverEvaluateSubsequentChecks` | Unit | Table-driven across all four positions (flag/idle/identity/cap each failing in turn) — a spy asserts every check *after* the failing one records zero invocations (Task 3.4.1b). |
| `session/diagnose/nudge_gate_test.go` | `TestNudgeGate_Evaluate_ShouldTreatCheckErrorAsFailure_NeverDefaultAllow_WhenACheckReturnsAnAmbiguousError` | Unit | A check function returning an error (not just `false`) — e.g. the tmux marker read failing with a transport error — must abort the pipeline exactly like an explicit `false`, never fall through to `(true, nil)`. This is the literal "fail-closed" contract: an unhandled ambiguity in any one gate is not license to proceed. |

### 2. Dual identity-reverification facade (Story 3.1.2, ADR-002)

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `session/tmux/write_gate_ownership_test.go` | `TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnMatchedIdentity_WhenSnapshotAndTmuxMarkerBothMatchExpectedUUID` | Unit | Both checks agree with `expectedSessionUUID` → `(SessionIdentity{...}, nil)`. |
| `session/tmux/write_gate_ownership_test.go` | `TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnTmuxMarkerMismatchReason_WhenPaneMarkerDiffersFromSnapshot` | Unit | `Snapshot()` matches but `readSessionOwnerMarker` returns a different UUID → `SafetyGateReasonIdentityMismatchTmuxMarker` — the exact `ce71ad1a` cross-session-misdelivery class this facade exists to close. |
| `session/tmux/write_gate_ownership_test.go` | `TestVerifyIdentityImmediatelyBeforeWrite_ShouldReturnInstanceMismatchReason_WhenSnapshotUUIDDiffersFromExpected` | Unit | The `Snapshot()`-side check fails independently of the tmux-side check (covers the other half of the "which check failed" requirement). |
| `session/tmux/write_gate_ownership_test.go` | `TestVerifyIdentityImmediatelyBeforeWrite_ShouldBeIdempotent_WhenCalledTwiceInSuccession` | Unit | Calling the facade twice back-to-back (the pipeline's own check, then the caller's final pre-write re-check, per Story 3.4.1's documented "called twice is acceptable, it's idempotent and read-only" design) returns identical results both times with no side effects — regression-pins the double-call design as intentional. |

**Caveat, stated explicitly per this repo's evidence-and-claims discipline**: the
facade's core invariant — "no I/O between the `Snapshot()` check and the tmux-marker
check" — is, per the plan's own Task 4.1.1b note, *not* something Go or a runtime test
can assert ("verified by a code-level review checklist item... Go cannot enforce 'no
I/O between two lines' automatically"). No test above claims to verify that property;
it is enforced by the facade being a single, un-split function body, checked at code
review. The tests above verify the facade's *outputs* are correct, not that its
*internals* are race-free by construction — that guarantee is structural, not tested.

### 3. Dispatch-level write-attempt guard (Story 4.1.4, closes the adversarial re-review's residual CONCERNS finding)

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `server/services/diagnose_dispatch_store_test.go` | `TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnFalseAndSetTimestamp_WhenFirstAttemptForDispatch` | Unit | `WriteAttemptedAt` nil → sets to now, returns `(false, nil)`. |
| `server/services/diagnose_dispatch_store_test.go` | `TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnTrueWithoutModifying_WhenSecondAttemptForSameDispatch` | Unit | `WriteAttemptedAt` already set → returns `(true, nil)`, timestamp unchanged. |
| `server/services/diagnose_dispatch_store_test.go` | `TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldAllowExactlyOneOfTwoConcurrentCalls_WhenRaceTestedForSameDispatchID` | Unit (`-race`) | Two goroutines call `CheckAndSetWriteAttempted` for the same `dispatchID` simultaneously, run 100 times under `-race` — exactly one gets `(false, nil)` (mirrors Task 3.3.2b's pattern, reusing `diagnoseNudgeGuardMu`). |
| `server/mcp/tools_terminal_test.go` | `TestSteerSession_ShouldRejectSecondWriteAttempt_WhenDispatchAlreadyRecordedAWriteAttempt_EvenWithCapHeadroomRemaining` | Integration | The specific scenario the re-review flagged as the residual gap: `NudgeCount=1 < cap=2` (cap alone would allow a second write) but the dispatch's `WriteAttemptedAt` is already set → second call rejected with `SafetyGateReasonDuplicateWriteAttemptForDispatch`, and the cap is *not* further consumed (Task 4.1.4h AC). |
| `server/mcp/tools_terminal_test.go` | `TestSteerSession_ShouldNotApplyDuplicateWriteGuard_WhenCallerHasNoMatchingDiagnoseDispatchRow` | Integration | A manual (non-diagnose) `steer_session` call, with no `DiagnoseDispatch` row for the caller's session UUID → guard is skipped entirely, ordinary per-item `NudgeGate` checks still apply (Task 4.1.4g's explicit carve-out). |

### 4. Nudge-cap concurrency guard (Story 3.3.2)

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `server/services/nudge_cap_store_test.go` | `TestNudgeCapStore_CheckAndReserve_ShouldAllowExactlyOneSuccess_WhenTwoGoroutinesRaceForSameItemAtCapOne` | Unit (`-race`) | `cap=1`, two goroutines call `CheckAndReserve` concurrently for the same item, repeated 100 times under `-race` per Story 3.3.2's AC — exactly one `(true, nil)`, one `(false, nil)`, never two successes (closes the `JulesDispatchService` race class named in `research/pitfalls.md` §1). |
| `server/services/nudge_cap_store_test.go` | `TestNudgeCapStore_CheckAndReserve_ShouldReturnCapReachedWithoutIncrementing_WhenAlreadyAtCap` | Unit | `NudgeCount=2`, `cap=2` → `(false, nil)`, `NudgeCount` unchanged at 2 (Story 3.3.1 AC — cap check never silently over-increments). |
| `session/diagnose/nudge_gate_test.go` | `TestNudgeGate_CapCheck_ShouldAbortWrite_NeverLogOnly_WhenCheckAndReserveFails` | Unit | A failed reservation translates directly into `(false, SafetyGateReasonNudgeCapReached)` — the crash-loop-restart-storm's log-only-cap failure mode cannot recur here (Story 3.3.3 AC). |

## UX Acceptance Tests

Implemented as Playwright specs per the `ui-playwright` skill's conventions and this
repo's `e2e-test-conventions` skill (feature-annotation header, no `waitForTimeout`,
`data-testid`/ARIA-only locators, page helpers under `tests/e2e/pages/`). One test per
`design/ux.md` UX Acceptance Criterion (23 total), organized into three spec files by
concern.

`tests/e2e/pages/DiagnoseActionPage.ts` (new page helper) wraps: `diagnoseButton(itemId)`,
`outcomeDisplay(itemId)`, `historyList(itemId)`, `nudgingDisabledBanner(itemId)`.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Trigger diagnosis in 1 click, no confirmation dialog | `tests/e2e/diagnose-action.spec.ts` | `should_dispatch_diagnosis_with_a_single_click_and_no_confirmation_dialog` | Playwright | Open `StuckItemDetail`/`BacklogItemDetail` → click `Diagnose` once → assert no dialog role appears, button enters pending state immediately. |
| 2. Reach diagnostic session's full record in 1 click from the outcome chip | `tests/e2e/diagnose-action.spec.ts` | `should_navigate_to_diagnostic_session_record_via_view_diagnosis_link` | Playwright | Diagnose to a `Nudged` outcome → click "View diagnosis" → assert the `headless-diagnose-*` session detail is shown. |
| 3. Retry a failed dispatch in 1 click, no reload | `tests/e2e/diagnose-action.spec.ts` | `should_retry_failed_dispatch_via_retry_button_without_page_reload` | Playwright | Force a dispatch-creation failure (mock RPC error) → `role="alert"` + `Retry` renders → click `Retry` → assert a second RPC call fires with no `page.reload()`. |
| 4. View full diagnose history inline, 0 additional clicks beyond expanding the item | `tests/e2e/diagnose-history.spec.ts` | `should_render_diagnose_history_list_inline_without_additional_navigation` | Playwright | Expand a stuck item with 2 prior dispatches → assert `DiagnoseHistoryList` is visible with no further click. |
| 5. Tell whether nudging is enabled before clicking, 0 clicks | `tests/e2e/diagnose-action.spec.ts` | `should_show_nudging_disabled_banner_proactively_when_flag_is_off` | Playwright | Load page with `DiagnoseNudgeExecutionFeatureFlag` off → assert the banner text is visible with no interaction. |
| 6. Failed dispatch shows the literal required string + Retry | `tests/e2e/diagnose-action.spec.ts` | `should_show_exact_dispatch_failed_copy_and_retry_button` | Playwright | Mock dispatch RPC failure → assert exact text `"Couldn't start diagnosis — <reason>. Try again."` and a `Retry` button. |
| 7. Safety-gate skip names the specific gate, never generic "skipped" | `tests/e2e/diagnose-action.spec.ts` | `should_name_specific_safety_gate_reason_for_each_skip_variant` | Playwright | Parameterized over 4 mocked `SafetyGateReason` values → assert each renders its distinct required copy ("session wasn't idle" / "identity check failed" / "nudge cap reached: N/cap" / "cooldown active, next eligible <time>"). |
| 8. Bug-filed links to the bug; inconclusive links to the note | `tests/e2e/diagnose-action.spec.ts` | `should_link_bug_filed_outcome_to_filed_bug_and_inconclusive_outcome_to_note` | Playwright | Two mocked outcomes → assert each link's `href`/navigation target resolves to the correct record. |
| 9. Diagnostic-session-stalled copy is distinct from dispatch-failed, names the recovery lever | `tests/e2e/diagnose-action.spec.ts` | `should_show_distinct_stalled_session_copy_with_diagnose_again_lever` | Playwright | Mock a dispatch whose session ended with no completion signal → assert copy differs from Surface 9's and mentions "diagnose again". |
| 10. Failed history fetch shows its own inline error + Retry, never a silent empty list | `tests/e2e/diagnose-history.spec.ts` | `should_show_history_fetch_error_with_retry_never_a_silently_empty_list` | Playwright | Mock `ListDiagnoseDispatches` failure → assert "Couldn't load diagnosis history" + `Retry`, and assert this is visually/DOM-distinct from the empty-history state (Surface 12). |
| 11. Every degraded state (6, 9, 10) has a visible next action, no reload required | `tests/e2e/diagnose-action.spec.ts` | `should_provide_a_working_next_action_for_every_degraded_state_without_reload` | Playwright | Parameterized over the three degraded states → assert each exposes a focusable, functional control and no `page.reload()` is required to recover. |
| 12. Duplicate/racing dispatch never produces two contradictory outcomes | `tests/e2e/diagnose-action.spec.ts` | `should_reuse_in_flight_state_for_racing_dispatch_never_show_contradictory_outcomes` | Playwright | Fire two `Diagnose` triggers in quick succession (one via UI click, one via a second simulated tab/RPC call) → assert only one settled outcome ever renders. |
| 13. Nudging-disabled banner is not a dead end; button stays functional | `tests/e2e/diagnose-action.spec.ts` | `should_keep_diagnose_button_fully_functional_while_nudging_disabled_banner_shown` | Playwright | Flag off → click `Diagnose` → assert dispatch still proceeds (bug/note-only outcomes reachable). |
| 14. Diagnose button is a real `<button>`, Tab-reachable, correct `aria-label` | `tests/e2e/diagnose-accessibility.spec.ts` | `should_expose_diagnose_button_as_tab_reachable_button_with_correct_aria_label` | Playwright | `getByRole('button', {name: 'Diagnose this stuck item'})` on `StuckItemDetail` and `{name: 'Diagnose this item'}` on `BacklogItemDetail`; Tab order includes it. |
| 15. In-flight button exposes `aria-busy`, disabled, "Diagnosing…" label | `tests/e2e/diagnose-accessibility.spec.ts` | `should_expose_aria_busy_and_diagnosing_label_while_dispatch_in_flight` | Playwright | Click → assert `aria-busy="true"`, `disabled`, visible text "Diagnosing…" (no spinner-only state). |
| 16. Routine settles use `aria-live="polite"`; failure states use `role="alert"`/assertive | `tests/e2e/diagnose-accessibility.spec.ts` | `should_use_polite_live_region_for_routine_outcomes_and_assertive_alert_for_failures` | Playwright | Parameterized over all 7 states → assert the correct ARIA live-region attribute per state. |
| 17. Each of the 7 outcome states legible by text alone with color removed | `tests/e2e/diagnose-accessibility.spec.ts` | `should_remain_distinguishable_by_text_alone_when_color_is_removed` | Playwright | Force `prefers-contrast`/greyscale emulation (or strip color via a CSS override) → assert each state's `data-testid` + text content is still uniquely identifiable. |
| 18. `DiagnoseHistoryList` uses `role="list"`/`listitem`, keyboard-focusable | `tests/e2e/diagnose-accessibility.spec.ts` | `should_expose_history_list_as_role_list_with_keyboard_focusable_listitems` | Playwright | Assert `role="list"` container, `role="listitem"` children, each with a Tab-reachable focusable child when a link is present. |
| 19. New chip/badge color pairs meet WCAG AA 4.5:1 | `tests/e2e/diagnose-accessibility.spec.ts` | `should_pass_axe_core_contrast_check_for_all_diagnose_outcome_chips` | Playwright + Axe Core | Run the existing Axe Core CI gate scoped to the rendered `DiagnoseOutcomeDisplay`/`DiagnoseHistoryList` DOM for all 7 states; assert zero contrast violations. |
| 20. Full keyboard-only flow, no focus loss to `<body>` | `tests/e2e/diagnose-accessibility.spec.ts` | `should_complete_full_diagnose_flow_via_keyboard_only_with_no_focus_loss_to_body` | Playwright | Tab to `Diagnose` → Enter → assert focus remains on the button through the busy state → on settle, Tab continues into outcome display and history list, asserting `document.activeElement !== document.body` at each step. |
| 21. Page refresh after a nudge shows the identical persisted outcome | `tests/e2e/diagnose-history.spec.ts` | `should_show_identical_outcome_after_page_refresh_sourced_from_persisted_dispatch_state` | Playwright | Diagnose to `Nudged` → capture rendered outcome → `page.reload()` → assert byte-identical outcome text/link, sourced from a fresh `ListDiagnoseDispatches` call. |
| 22. No outcome state implies the underlying issue is resolved (no green "all good") | `tests/e2e/diagnose-accessibility.spec.ts` | `should_never_render_nudged_outcome_with_resolved_or_all_good_implication` | Playwright | Assert the `Nudged` state's copy is exactly "nudged the session" (never "fixed"/"resolved") and its color token is the repo's "acted" family, not its "success/resolved" token. |
| 23. Nudge-cap/cooldown counter visible on the item itself, not log-only | `tests/e2e/diagnose-history.spec.ts` | `should_show_visible_nudge_count_and_cap_on_cap_reached_outcome_and_in_history` | Playwright | Mock a `SkippedSafetyGate`/`NudgeCapReached` outcome → assert the "N/cap" numeric text is visible in both `DiagnoseOutcomeDisplay` and the corresponding `DiagnoseHistoryList` entry. |

## Migration Test Design (Step 5)

Per the Migration Plan (`plan.md` lines 139-171), both new ent schemas
(`NudgeCapRecord`, `DiagnoseDispatch`) are **additive-only** — brand-new tables, no
column changes to any existing table, no backfill. This repo's ent schema is
hand-written and generated output is never committed (`session/ent/generate.go`'s
policy), so there is no traditional SQL up/down migration file to execute — the
precedent for testing this shape is `session/ent/schema/migration_test.go`'s existing
`TestMigration_should_BeReversible_When_CreationFieldsAreAtZeroValues` (written for
the `async-session-creation` project's additive column change), adapted here for two
new tables instead of new columns on an existing one.

**Test**: `TestMigration_should_BeReversible_When_NudgeCapRecordAndDiagnoseDispatchTablesAreCreated`
**File**: `session/ent/schema/migration_test.go` (same file, new test function)
**Type**: Migration

- **Up**: using `session.NewTestEntRepository(t)`'s in-memory-sqlite ent client
  (already regenerated against the two new schema files by the time this test runs —
  `go build ./...` proves the schema compiles), create one `NudgeCapRecord` row and
  one `DiagnoseDispatch` row (`Status: Pending`), then re-fetch both by ID. Asserts:
  both tables exist and are queryable, `NudgeCapRecord.NudgeCount` round-trips its set
  value, `DiagnoseDispatch.Status` round-trips `Pending` with `OutcomeKind` nil until
  `MarkCompleted` is called — mirroring the existing test's "round-trip, not just
  exists" bar.
- **Down**: since both tables are additive-only, "down" is "the two new tables did
  not exist before this change and dropping them removes nothing else" — verified by
  asserting the existing `Session`/`BacklogItem` tables' pre-existing CRUD paths
  (exercised via `session.NewTestEntRepository(t)`'s existing helper methods, already
  covered by pre-existing tests) are unaffected by the schema addition, i.e. this test
  adds new assertions without modifying or weakening any existing migration test.
  Actually reverting (deleting the two schema files, regenerating, confirming
  `go build ./...`/`go test ./...` pass with the tables absent) is a process step for a
  human running `git revert` + `make ent-gen`, not something a test against the
  *current* tree can execute — same limitation the precedent test's own doc comment
  states for its column-level case.

## Test Stack

- **Unit**: Go `testing` + `testify` (assert/require), table-driven where the plan's
  ACs enumerate multiple cases (gate-check pipeline positions, budget clamp boundaries,
  identity match/mismatch combinations). React/TS unit tests via Jest +
  React Testing Library, mirroring `StuckItemDetail.test.tsx`'s existing patterns.
- **Integration**: Go tests using `session.NewTestEntRepository(t)`'s in-memory-sqlite
  ent client (this repo's existing test-isolation convention — no real Postgres/file
  DB) wired to real service-layer collaborators (`HandoffSummaryGenerator`,
  `DiagnoseDispatchStore`, `EventBusNotifier`) instead of fakes/mocks, plus real
  `go-git` operations against temp repos for the `Diff` bundle section. `-race` is
  mandatory for every concurrency-guard test (Story 3.3.2, Story 4.1.4's dispatch-level
  guard) — run via `go test ./... -race`, not just `make test`.
- **E2E / UX**: Playwright against the isolated `tests/e2e/global-setup.ts`-managed
  test server (dynamically assigned port, `STAPLER_SQUAD_TEST_DIR`-isolated state) —
  never the live `:8543` instance. New page helper: `tests/e2e/pages/DiagnoseActionPage.ts`.
  Axe Core gate extended to cover the new outcome/history components (criterion 19).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | >=80% line, with `session/diagnose/*`, `server/services/diagnose_*.go`, `server/services/nudge_cap_store.go`, and `session/tmux/write_gate_ownership.go` (the safety-critical packages) held to 100% branch coverage on every `NudgeGateCheck`/`SafetyGateReason` case — a gap in these specific files is a shipped nudge-safety hole, not ordinary test debt. |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | >=80% line, with `DiagnoseOutcomeDisplay.tsx`'s 7-state switch and the flag-off banner held to 100% branch coverage (every `DiagnoseOutcomeKind` case + the `Pending`/flag-off cases must each have an asserting test, not just be reachable). |

- All public service methods: happy path + error paths covered (see Requirement →
  Test Mapping table above).
- All external integrations: unit mocked + at least one integration test —
  `HandoffSummaryGenerator` (REQ-2, REQ-6), tmux pane read-back (REQ-5's identity
  facade), MCP session-creation (REQ-4), ConnectRPC (REQ-8) each have both.
- UX acceptance criteria: all 23 criteria in `design/ux.md` have a corresponding
  Playwright test above — none left to "manual step" given this repo's existing
  Playwright/Axe Core CI gate already covers this shape of assertion.
- Safety-critical paths (`NudgeGate` fail-closed short-circuit, identity
  reverification facade, dispatch-level write-attempt guard, nudge-cap concurrency
  guard): each has explicit, named test cases per the dedicated section above, run
  with `-race` where concurrency is the property under test — never covered only
  incidentally by a higher-level happy-path test.
