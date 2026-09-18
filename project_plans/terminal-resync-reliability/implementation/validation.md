# Validation Plan: terminal-resync-reliability

## Happy Path Scenario

*Given* a `SessionDetailView` with 3 mounted `TerminalOutput` instances (only `sess-2` has
`isVisible=true`) and all 7 `terminal:resync-*` feature flags enabled, *When* the browser tab
regains focus (`document.visibilitychange` fires), *Then* only `sess-2` issues a single
`resync_id`-tagged `CurrentPaneRequest` with `stale_dimensions=true`; the server skips the
dimension-mismatch slow path and services it through the exec-gate fast lane; the client matches
the reply by `resync_id` to clear `sess-2`'s pending state without ever showing the "Reconnecting
terminal…" banner; and `sess-1`/`sess-3` generate zero resync wire traffic and never arm a stall
watchdog — the multi-terminal "resync storm" is eliminated end-to-end.

## Requirement → Test Mapping

Rows are keyed to `plan.md`'s AC1–AC8 (the plan's own mapping of `requirements.md`'s 5 in-scope
fix categories + flags + observability onto testable acceptance criteria). AC6 is split into two
sub-rows because compression and batching are independently flagged and independently go/no-go'd
in the plan (Epics 5.1 / 5.2).

| Requirement | Unit (Happy) | Unit (Error) | Integration | UX Acceptance |
|---|---|---|---|---|
| AC1 — Visibility-scoped resync | `handleVisibilityOrFocusResyncInner_should_CallRequestFullResync_When_IsVisibleTrueAndFlagOn` | `handleVisibilityOrFocusResyncInner_should_BeNoOp_When_IsVisibleFalseAndFlagOn` | N/A — pure client hook logic, no store/external call (multi-instance behavior covered by UX test #1/#6 below) | See UX Acceptance Tests §1, #1, #6, #8 |
| AC2 — Correlation ID (`resync_id`) | `requestFullResync_should_GenerateResyncIdAndAttachToRequest_When_CorrelationFlagOn` | `notifyResyncOutputReceived_should_NotClearPendingResync_When_ResyncIdMismatch` | `streamViaTmuxCapturePane_should_EchoResyncIdOnTerminalOutput_When_RequestCarriesResyncId` | N/A |
| AC3 — Skip stale-dimension slow path (3a) | `handleCurrentPaneRequest_should_SkipResizeAndSigwinchLoop_When_StaleDimensionsTrueAndFlagOn` | `handleCurrentPaneRequest_should_RunFullSlowPath_When_StaleDimensionsFalseOrFlagOff` | `streamViaTmuxCapturePane_should_CaptureAtExistingPaneDimensions_When_StaleDimensionsTrueAndFlagOn` | N/A |
| AC4 — Exec-gate fast lane (3b) | `AcquireResyncExecSlot_should_AcquireFromSeparatePool_When_DefaultPoolSaturated` | `AcquireResyncExecSlot_should_BlockUntilSlotAvailable_When_FastLanePoolExhausted` | `CapturePaneContentPriority_should_UseFastLaneGate_When_ExecGateFastLaneFlagOn` | N/A |
| AC5 — Stagger/prioritize bursts | `staggerQueue_should_SpreadResyncCallsAcrossJitterWindow_When_ThreeInstancesBecomeVisibleWithin50ms` | `staggerQueue_should_PreemptQueuedEntries_When_NewInstanceBecomesVisibleWhileOthersQueued` | N/A — client-only scheduling; timing-sensitive multi-component preemption covered by UX test #7 | See UX Acceptance Tests §2, #6, #7, #8 |
| AC6a — Envelope compression | `parseResponseBody_should_DecompressGzipPayload_When_CompressedFlagSet` | `parseResponseBody_should_SurfaceDecodeError_When_CompressedFlagSetButPayloadIsTruncated` | `handleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_PayloadExceedsSizeThresholdAndCompressionFlagOn` | N/A |
| AC6b — Batching | `handleBatchedCurrentPaneRequest_should_DispatchNIndividuallyTaggedResponses_When_BatchingFlagOn` | `staggerCoordinator_should_SendNSeparateRequests_When_BatchingFlagOff` | `handleBatchedCurrentPaneRequest_should_PreserveCorrelationPerRequest_When_ThreeCoalescedRequestsHaveDistinctResyncIds` | N/A |
| AC7 — Per-fix feature flags | `GetFeatureFlags_should_ReturnAllSevenResyncFlagsDefaultingFalse_When_RegistryQueried` | `UpdateFeatureFlag_should_OnlyAffectTargetFlag_When_SingleFlagToggled` | `resyncFlow_should_BeByteForByteIdenticalToBaseline_When_AllSevenFlagsOff` | See UX Acceptance Tests §3, #9, #10, #11 |
| AC8 — Observability | `handleCurrentPaneRequest_should_LogSkippedSlowPathWithSessionIdAndElapsedMs_When_StaleDimensionSkipFires` | `notifyResyncOutputReceived_should_LogDebugOnMismatch_When_ResyncIdDoesNotMatchEitherPendingId` | `resyncStallWatchdog_should_TrackAnalyticsEventWithVisibilityState_When_WatchdogFiresWhileHidden` | N/A |

### Additional risk-driven tests (not 1:1 with a single AC row)

`plan.md`'s Risk Control and Rabbit Holes sections name two failure modes precise enough to need
their own named test, beyond the per-AC happy/error pair above:

| Risk (source) | Test Name | Method |
|---|---|---|
| Two independently-tracked resyncs outstanding at once; a non-matching response must still reset the stall watchdog without clearing the still-pending resync's banner (Story 3.1.2) | `TerminalOutput_should_ResetStallWatchdogWithoutClearingBanner_When_DifferentOutstandingResyncIdRespondsFirst` | Unit (client hook interaction, `useVisibilityResync` + `useTerminalFlowControl` composed) |
| Accidental modification of the 3 protected pre-existing full-resync triggers while editing `useVisibilityResync.ts`/`TerminalOutput.tsx` (Epic 8.2) | `protectedResyncTriggers_should_RemainByteForByteUnchanged_When_VisibilityAndStaggerFixesLand` | Regression / static check (pinned line-range diff, per Task 8.2.1.1–8.2.1.2) |
| `AcquireResyncExecSlot`'s pool must never borrow from or starve the shared `"default"` pool (Risk Control row 2) | `AcquireResyncExecSlot_should_NotConsumeDefaultPoolCapacity_When_FastLanePoolIsFull` | Integration (real `flock`-backed gate files, both pools driven concurrently) |
| Combined 8-slot default + 4-slot fast-lane saturation (12 simultaneous tmux subprocess calls on one `serverSocket`) is unvalidated as safe/performant — gating precondition for `terminal:resync-exec-gate-fast-lane` default-on (Task 4.2.1.10a-c; `pre-mortem.md` P2 #2) | `resyncExecGates_should_MeasureLatencyAndGateWaitTime_When_DefaultAndFastLanePoolsAreCombinedSaturated` | Load-characterization benchmark (Go, real `flock`-backed gate files, both pools driven to saturation concurrently; asserts on measured resync latency and gate-wait time, and — per Task 4.2.1.10b — asserts no tmux-server-level errors/garbled output on non-resync traffic sharing the socket while saturated). Must run and pass before `terminal:resync-exec-gate-fast-lane` ships default-on (Task 4.2.1.10c). |

## UX Acceptance Tests

One test per UX acceptance criterion in `design/ux.md` §6 (14 total across 3 interactive
surfaces + 3 cross-cutting). Method follows the `ui-playwright` skill's conventions
(`data-testid`/ARIA locators, no `waitForTimeout`, feature-annotation header) as the
implementation model, per `.claude/rules/e2e-test-conventions.md`; items the design doc itself
flags as screen-reader- or contrast-tool-dependent are marked Manual.

| Surface | UX Acceptance Criterion | Test Name | Method |
|---|---|---|---|
| 1 — Banner | #1 No banner on non-switched terminals after a tab refocus with 3+ mounted terminals | `terminalBanner_should_RemainHidden_When_BackgroundedTerminalWasNotSwitchedTo` | Playwright |
| 1 — Banner | #2 Soft banner reads "Reconnecting terminal…" and either self-clears or escalates — never persists indefinitely | `reconnectingBanner_should_SelfClearOrEscalateToHardFail_When_ResyncNeverStaysStuckPending` | Playwright |
| 1 — Banner | #3 Hard-fail banner always shows "Connection lost — Retry"; Retry is always clickable, no dead end | `hardFailBanner_should_ShowRetryAndReattemptConnection_When_Clicked` | Playwright |
| 1 — Banner | #4 Both banners announced via `role="status" aria-live="polite"` without moving focus | `terminalBanners_should_AnnounceViaAriaLiveWithoutMovingFocus_When_BannerAppears` | Manual (VoiceOver/NVDA, per `design/ux.md` §2 Accessibility) |
| 1 — Banner | #5 Text contrast ≥ 4.5:1 for both banner variants, light and dark theme | `bannerContrast_should_MeetWcagAA_When_RenderedInLightAndDarkTheme` | Automated (axe-core, via this repo's existing PR-triggered Axe/Lighthouse CI) + Manual spot-check against resolved theme tokens |
| 2 — Visibility/stagger | #6 No perceived added latency on a landed-on terminal vs. an equal number of single-terminal sessions | `terminalSwitch_should_ShowNoAddedLatencyVsSingleSessionBaseline_When_RapidlyCyclingFourPlusTabs` | Playwright (instrumented timing assertion) |
| 2 — Visibility/stagger | #7 A switched-to terminal's resync is never left stale behind a stagger slot; its request timestamp ≤ any resync it preempts | `resyncPreemption_should_FireBeforeAnyStillQueuedSiblingResync_When_UserSwitchesToNewTerminalMidStagger` | Playwright (network-log timestamp assertion) |
| 2 — Visibility/stagger | #8 With `terminal:resync-stagger` off, behavior is identical to pre-project (immediate, no jitter) | `staggerCoordinator_should_FireResyncImmediatelyWithNoJitter_When_StaggerFlagOff` | Playwright |
| 3 — Flags admin | #9 Operator finds and toggles any of the 7 new flags in ≤ 2 steps with a human-readable label | `featureFlagsPage_should_ToggleResyncFlagInTwoStepsWithReadableLabel_When_OperatorNavigatesAndClicks` | Playwright |
| 3 — Flags admin | #10 Failed toggle shows `{error}. Please refresh.` via `role="alert"`; refresh is a working exit | `featureFlagsPage_should_ShowActionableAlertAndAllowRefresh_When_SetFlagRpcFails` | Playwright (mocked RPC failure) |
| 3 — Flags admin | #11 Toggling never silently no-ops — badge flips within one round trip, or an error shows | `featureFlagsPage_should_ReflectToggleOrShowError_When_FlagToggled` | Playwright |
| Cross-cutting | #12 No dead ends — every cataloged error state has a named, working exit path | `errorStateCatalog_should_HaveWorkingExitPath_When_AnyRowFromDesignDocTablesOccurs` | Manual QA checklist, row-by-row against `design/ux.md` §2/§3/§4 tables |
| Cross-cutting | #13 Flag-off parity — indistinguishable from pre-project across all 3 surfaces | `resyncFlow_should_BeIndistinguishableFromPreProjectBehavior_When_AllSevenFlagsOff` | Manual QA (exploratory, run first before any flag flips on in prod) + Playwright regression suite (shared with AC7's integration test) |
| Cross-cutting | #14 No new visible state introduced beyond the A/B/C banner catalog | `terminalUI_should_ExposeOnlyCatalogedABCStates_When_AnyFixFlagCombinationIsEnabled` | Manual design-review checklist (reviewer sign-off against `design/ux.md` §2 state diagram; not automatable — it's a "did anyone add a 4th state" check) |

## Test Stack

- Unit (Go): standard `testing` package, `github.com/stretchr/testify` (already a project
  dependency, `go.mod:32`) for assertions — `go test ./server/services/... ./session/tmux/... ./config/...`.
- Unit (TypeScript/React): Jest + React Testing Library — `cd web-app && npx jest --no-coverage`.
- Integration (Go): `go test` against real tmux subprocesses / real `flock`-backed gate files
  (same style as this repo's existing `session/tmux` and `server/services` integration tests);
  slow/subprocess-heavy cases guarded by `-short` per this repo's recent flake-fix precedent
  (`2ab5dfe75`). Full-flag-matrix round trips (Epic 8.1) run against a real in-process
  `connectrpc_websocket.go` handler, not a mock.
- UX Acceptance: Playwright + Allure (`tests/e2e/`), following `.claude/rules/e2e-test-conventions.md`
  (`data-testid`/ARIA locators only, no `waitForTimeout`, `// @feature` header) and the
  `ui-playwright` skill's conventions as the implementation model; items marked Manual above use a
  short numbered script instead of test code (screen-reader announcement checks, contrast-token
  spot-checks, and the "no new visible state" design-review sign-off are not meaningfully
  automatable).

## Coverage Targets

| Language | Tool | Target |
|---|---|---|
| Go | `go tool cover` via `make test-coverage` | ≥ 80% line coverage on new/changed code in `session/tmux/exec_gate.go`, `server/services/connectrpc_websocket.go`'s new shared helper, and `config/types.go`'s new fields |
| TypeScript | Jest `--coverage` | ≥ 80% line coverage on `useVisibilityResync.ts`, `useTerminalFlowControl.ts`, and the new `SessionDetailView.tsx` stagger-queue module |

## Migration Test

N/A — `plan.md`'s Migration Plan section states this project makes no schema/data migration,
only new optional/defaulted proto fields (`resync_id`, `stale_dimensions`) and new default-off
feature flags, both proto3-wire-compatible with no version gate needed. Step 5 is skipped per
the explicit instruction accompanying this task.
