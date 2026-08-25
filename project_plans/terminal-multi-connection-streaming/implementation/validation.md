# Validation Plan: terminal-multi-connection-streaming

**Date**: 2026-08-20

## Happy Path Scenario

Given a tmux session whose `StreamPath` has resolved to `PathHubOwned` with one browser tab already attached to its `StreamHub`, when a second browser tab (or ssq-mux) attaches to the same session and later requests a different terminal size, then the `StreamHub` negotiates a single `NegotiatedSize` across both `Subscriber`s, performs resize + quiescence-wait + capture-pane exactly once, and broadcasts one shared, non-garbled `CatchUpSnapshot` to both — eliminating the resize/capture race by construction rather than by convention.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: Single owner performs resize+quiescence+capture exactly once regardless of subscriber count | `session/streamhub/hub_test.go` | `TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_MultipleSubscribersVoteForResize` | Unit | Happy path — 3 subscribers, one `SetWindowSize`/`CapturePaneContent` call, all 3 get the snapshot |
| REQ-1 | `session/streamhub/hub_test.go` | `TestStreamHub_should_BroadcastStreamEndedSentinelAndAttemptRestart_When_SessionControllerCallErrors` | Unit | Error path — `SetWindowSize`/`ResizePTY`/`CapturePaneContent` returns an error mid-resize |
| REQ-1 | `server/services/connectrpc_websocket_test.go` | `TestStreamTerminal_should_RouteThroughHubWithNoLegacyResizeCall_When_PathHubOwnedResolved` | Integration | Fake `SessionController` + real `HubRegistry`/`WebSocketTransport` wiring through `streamTerminal` |
| REQ-2: `Transport` interface decouples hub from `*websocket.Conn` | `session/streamhub/subscriber_test.go` | `TestMemoryTransport_should_SatisfyTransportInterface_When_SendAndCloseAreCalled` | Unit | Happy path |
| REQ-2 | `session/streamhub/subscriber_test.go` | `TestStreamHub_should_EvictSubscriberExactlyOnce_When_TransportSendReturnsError` | Unit | Error path — `MemoryTransport` configured with `WithErrorSend()` |
| REQ-2 | `server/services/websocket_transport_test.go` | `TestWebSocketTransport_should_WriteViaMutexGuardedWriteMessage_When_SendCalledOnLiveConnection` | Integration | Real `*connectWebSocketStream` (fake underlying conn) exercising the shared write-mutex path |
| REQ-3: ≥3 real `Transport` implementations (`WebSocketTransport`, `MuxTransport`, `MemoryTransport`) | `session/external_streamer_transport_test.go` | `TestMuxTransport_should_DeliverBroadcastBytesToConsumers_When_HubSendsFrame` | Unit | Happy path — delegates to `ExternalStreamer`'s existing consumer callback |
| REQ-3 | `session/streamhub/resize_test.go` | `TestNegotiatedSize_should_IgnoreVote_When_SubscriberCapabilityCanResizeIsFalse` | Unit | Error/edge path — `MuxTransport`'s forced `CanResize:false` vote has zero effect |
| REQ-3 | `session/streamhub/hub_test.go` | `TestStreamHub_should_BroadcastToAllTransportTypesUnmodified_When_WebSocketMemoryAndMuxSubscribersAreMixed` | Integration | One hub, one of each transport type attached simultaneously |
| REQ-4: Per-session connection registry (`SubscriberCount`) | `session/streamhub/hub_test.go` | `TestStreamHub_should_ReportSubscriberCount_When_SubscribersAttachAndDetach` | Unit | Happy path |
| REQ-4 | `server/services/connectrpc_websocket_test.go` | `TestActiveControlModeStreams_should_RemainUntouched_When_SessionIsPathHubOwned` | Unit | Error/edge path — legacy counter must stay `loaded=false` for hub-owned sessions |
| REQ-4 | `session/streamhub/hub_test.go` | `TestTmuxSession_should_HaveExactlyOneControlModeSubscriberEntry_When_NHubSubscribersAreAttached` | Integration | Real `TmuxSession.controlModeSubscribers` registry with a fake `SessionController` |
| REQ-5: Wire-protocol batching/coalescing (`BatchWindow`, eliminates Nx duplicated per-connection coalescing) | `session/streamhub/batch_test.go` | `TestBatchWindow_should_CoalesceOnceRegardlessOfSubscriberCount_When_ThreeSubscribersReceiveFiftyEventBurst` | Unit | Happy path — the concrete, corrected Success Metric target from Story 2.1.1 (hub's coalesce step runs once per flush vs. today's N independent per-connection coalesce loops; not a wire-message-count claim) |
| REQ-5 | `session/streamhub/batch_test.go` | `TestBatchWindow_should_NeverSplitAnEventMidBoundary_When_ConcatenatingOpaqueByteRanges` | Unit | Error/edge path — escape-sequence integrity under coalescing |
| REQ-5 | `session/streamhub/batch_test.go` | `TestBatchWindow_should_DeliverQuiescenceSignalImmediately_When_PendingBatchIsMidAccumulation` | Integration | Bypass path racing a real pending accumulation buffer + `HubSequenceNumber` ordering across 2+ subscribers |
| REQ-6: `OverlapInvariant` / retirement of the `420584566` WARN | `session/streamhub/failure_modes_test.go` | `TestOverlapInvariant_should_NeverBeViolated_When_1000ConcurrentAttachDetachCyclesRunUnderRace` | Unit | Happy path — the structural regression test for the whole project |
| REQ-6 | `session/streamhub/failure_modes_test.go` | `TestOverlapInvariant_should_PanicInDevOrLogErrorInProd_When_TwoOwnersAreForciblySimulated` | Unit | Error path — forced double-owner scenario in the test harness (harness intentionally bypasses the normal lock to prove the invariant check itself fires) |
| REQ-6 | `server/services/connectrpc_websocket_test.go` | `Test420584566WARN_should_ContinueFiringUnmodified_When_TwoLegacyPerConnectionStreamsOverlap` | Integration | Confirms `recordControlModeStreamStart` diff is zero and legacy WARN still fires for `PathLegacyPerConnection` |
| REQ-7: Sticky per-session `StreamPath` resolution | `session/streamhub/ownership_test.go` | `TestStreamOwnershipLock_should_ReturnCachedPath_When_FlagFlipsAfterFirstResolution` | Unit | Happy path |
| REQ-7 | `session/streamhub/failure_modes_test.go` | `TestStreamOwnershipLock_should_ResolveToSingleWinner_When_ConcurrentCallersRaceFirstResolution` | Unit | Error/edge path — 1000-iteration race, no split resolution |
| REQ-7 | `session/streamhub/ownership_test.go` | `TestStreamOwnershipLock_should_ResolveIndependently_When_TwoDifferentSessionNamesAreQueried` | Integration | Two independent sessions, one hub-backed |
| REQ-8: `StreamOwnershipLock` mutual exclusion (legacy `StartControlMode` vs. hub creation) | `session/streamhub/ownership_test.go` | `TestAcquireOwnershipLock_should_ReturnSameLockInstance_When_CalledWithSameSessionName` | Unit | Happy path |
| REQ-8 | `session/streamhub/ownership_test.go` | `TestHubRegistry_should_JoinLegacyPathExplicitly_When_LockAlreadyResolvedLegacyPerConnection` | Unit | Error/edge path — no silent success reinterpretation (Task 3.1.2b) |
| REQ-8 | `session/instance_tmux_test.go` | `TestInstanceStartControlMode_should_NeverProduceTwoOwners_When_100GoroutinesRaceHubRegistryConcurrently` | Integration | 100-goroutine `-race` test across the real `session`/`session/streamhub` package boundary, plus `go list -deps` cycle check |
| REQ-9: Resize negotiation (`ResizeVote`/`NegotiatedSize`, smallest-common-size) | `session/streamhub/resize_test.go` | `TestNegotiateSize_should_ReturnComponentWiseMinimum_When_TwoCanResizeSubscribersVoteDifferentSizes` | Unit | Happy path |
| REQ-9 | `session/streamhub/resize_test.go` | `TestNewTerminalSize_should_ReturnError_When_ColsOrRowsIsNonPositive` | Unit | Error path |
| REQ-9 | `session/streamhub/hub_test.go` | `TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_NegotiatedSizeChanges` | Integration | Fake `SessionController` call-counting through the full vote→negotiate→resize pipeline |
| REQ-10: `CatchUpSnapshot` delivery (new subscriber + post-quiescence broadcast) | `session/streamhub/subscriber_test.go` | `TestAttachSubscriber_should_SendCatchUpSnapshot_When_SubscriberJoinsActiveHub` | Unit | Happy path |
| REQ-10 | `session/streamhub/quiescence_test.go` | `TestWaitForQuiescence_should_LogHubScopedWarn_When_QuiescenceTimesOutAfter500ms` | Unit | Error path |
| REQ-10 | `session/streamhub/hub_test.go` | `TestStreamHub_should_CapturePaneExactlyOnceAndBroadcastToAllSubscribers_When_QuiescenceReachedAfterResize` | Integration | 3 subscribers, only 1 triggered the resize, all 3 must get the identical snapshot with exactly 1 `CapturePaneContent` call |
| REQ-11: Hub lifecycle (grace-period teardown, `ForceTeardown`, crash/restart) | `session/streamhub/lifecycle_test.go` | `TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches` | Unit | Happy path |
| REQ-11 | `session/streamhub/lifecycle_test.go` | `TestStreamHub_should_CancelPendingTeardown_When_SubscriberReattachesDuringGracePeriod` | Unit | Error/edge path |
| REQ-11 | `session/streamhub/failure_modes_test.go` | `TestStreamHub_should_BroadcastStreamEndedSentinelToAllSubscribersAndAttemptRestart_When_ControlModeSubprocessExitsUnexpectedly` | Integration | 2 `MemoryTransport` subscribers + simulated subprocess death |
| REQ-12: Observability (structured logs + 6 metrics) | `session/streamhub/observability_test.go` | `TestStreamHub_should_EmitOrderedLifecycleLogLines_When_SubscriberAttachesResizesAndDetaches` | Unit | Happy path |
| REQ-12 | `session/streamhub/failure_modes_test.go` | `TestStreamHub_should_IncrementSlowSubscriberDropsCounterExactlyOnce_When_EvictionOccursNotPerDroppedFrame` | Unit | Error/edge path — metric must count evictions, not frames |
| REQ-12 | `session/streamhub/observability_test.go` | `TestStreamHubMetrics_should_RegisterAndIncrement_When_HubIsCreatedAndOneSubscriberAttaches` | Integration | Real metrics registry (`connectrpc.com/otelconnect`) wiring |
| REQ-13: Rollout mechanics (per-session override + rollback rehearsal) | `session/streamhub/ownership_test.go` | `TestStreamOwnershipLock_should_ForcePathHubOwnedForOverriddenSession_When_GlobalDefaultIsFalse` | Unit | Happy path |
| REQ-13 | `session/streamhub/ownership_test.go` | `TestStreamOwnershipLock_should_ResolvePathLegacyPerConnection_When_NoOverrideAndGlobalDefaultIsFalse` | Unit | Error/edge path — sibling session without override must not inherit the canary's path |
| REQ-13 | Manual (recorded in this file, §Rollback Rehearsal below) | *Story 3.3.2's rehearsal* | Manual | No automated integration test — plan.md explicitly requires one real, executed rehearsal against a disposable session before wider rollout; outcome logged here per Task 3.3.2c |
| REQ-14: Frontend `connection_count` exposure | `server/services/connectrpc_websocket_test.go` | `TestStreamTerminal_should_PopulateConnectionCount_When_SessionIsPathHubOwned` | Unit | Happy path |
| REQ-14 | `server/services/connectrpc_websocket_test.go` | `TestStreamTerminal_should_OmitConnectionCount_When_SessionIsPathLegacyPerConnection` | Unit | Error/edge path — must never fabricate from `activeControlModeStreams` |
| REQ-14 | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `should return undefined connection_count when the field is absent from the stream message` | Integration | Jest test against a fake ConnectRPC stream carrying/omitting the field, verifying the hook↔wire contract end to end |

**Requirements covered: 14/14 (100%).** *(All Scope → In Scope bullets from requirements.md, cross-referenced against plan.md's Epics 1.1–4.2, map to at least one row above.)*

## UX Acceptance Tests

*(19 criteria from `design/ux.md`, across its 3 surfaces.)*

### Surface 1 — Connection-count indicator (11 criteria)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: Indicator does not render when `connection_count <= 1`/absent | `tests/e2e/connection-count-indicator.spec.ts` | `should not render the connection-count indicator when only one connection is attached` | Playwright | Open one tab on a `PathHubOwned` session; assert `getByTestId('connection-count-indicator')` has no matching element |
| AC2: Operator sees "another connection attached" in 0 extra steps | `tests/e2e/connection-count-indicator.spec.ts` | `should show the connection-count indicator immediately when a second tab attaches, with no navigation required` | Playwright | Open tab A, then tab B on the same session; assert tab A's indicator becomes visible via `expect(locator).toBeVisible()` with no click/nav in between |
| AC3: Resize-mismatch explanation reachable in 1 step (hover/tap) | `tests/e2e/connection-count-indicator.spec.ts` | `should reveal the resize-mismatch tooltip after a single hover on the indicator` | Playwright | With tab B negotiating a smaller `NegotiatedSize`, hover tab A's indicator; assert tooltip text appears without a second interaction |
| AC4: `role="status"` + `aria-live="polite"`, never `role="alert"` | `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx` | `should render with role=status and aria-live=polite, never role=alert` | Jest (RTL) | Render with `connectionCount=2`; assert DOM attributes directly (matches AC's own "verified by DOM inspection / component test" wording) |
| AC5: Announces a count change exactly once; no announcement on mount if already `>1` | `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx` | `should announce a count change exactly once and not announce on initial mount when already above one` | Jest (RTL) | Mount with `connectionCount=2` → assert 0 announcements; then rerender `2→3→2` → assert exactly 2 announcement events |
| AC6: Icon is `aria-hidden="true"`; visible/announced text carries all meaning | `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx` | `should mark the glyph icon aria-hidden and carry all meaning in the text label` | Jest (RTL) | Assert icon element has `aria-hidden="true"` and `aria-label` text matches `"N connections active"` |
| AC7: Text/icon contrast ≥ 4.5:1 in both light and dark themes | `tests/e2e/connection-count-indicator.spec.ts` (Axe Core, via this repo's existing UX-analysis CI) | `should pass Axe Core color-contrast checks for the connection-count indicator in light and dark themes` | Playwright + Axe Core | Render indicator under `data-theme="light"` and `data-theme="dark"`; run the repo's existing Axe Core integration scoped to the indicator's container |
| AC8: Keyboard-navigable (Tab reaches it; tooltip revealable via focus, not hover-only) | `tests/e2e/connection-count-indicator.spec.ts` | `should reveal the tooltip on keyboard focus without requiring a mouse hover` | Playwright | `page.keyboard.press('Tab')` through the terminal chrome to the indicator; assert tooltip visible on `:focus`, using `getByRole` locators only |
| AC9: No dead ends — every visible state has a plain-language explanation | `tests/e2e/connection-count-indicator.spec.ts` | `should describe every visible indicator state in plain language, never raw internals like hub or transport` | Playwright | Assert rendered/announced text never matches `/hub|transport|subscriber/i` in either the bare-count or mismatch-tooltip state |
| AC10: Mismatch sentence appears only when this tab's vote actually lost negotiation | `tests/e2e/connection-count-indicator.spec.ts` | `should show the mismatch sentence only when this tabs resize vote lost the negotiation, never speculatively` | Playwright | Two tabs request the *same* size; hover the indicator; assert only the first line ("N connections active") renders, no mismatch sentence |
| AC11: Rapid flapping is coalesced/debounced to one announcement per user-perceptible state | `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx` | `should coalesce a rapid sequence of count changes into at most one announcement per perceptible state` | Jest (RTL) | Fire a burst of `1→2→1→2→1` prop updates within one debounce window; assert exactly one (or zero, per debounce policy) announcement fires, matching `InputDropBadge.tsx`'s episode-coalescing precedent |

### Surface 2 — Hub-can't-start error banner (4 criteria, condensed)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Hub-start failure renders existing `hardFailedBanner` with `role="alert"` and unchanged copy | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `should render the existing hardFailedBanner with role=alert when hub start fails for a PathHubOwned session` | Jest (RTL) | Simulate a hub-start failure trigger (in addition to existing triggers); assert banner renders with the exact existing "Connection lost — Retry" copy |
| Retry button re-triggers the same reconnect path as today's hard-failure case | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `should re-trigger the existing reconnect handler when Retry is clicked after a hub-start failure` | Jest (RTL) | Click the Retry button; assert `handleHookReconnect` (or its test double) is called, identical to the pre-existing hard-failure path |
| No new copy requiring i18n/localization review | Manual | *string-diff check* | Manual | Diff the banner's rendered string against the pre-existing `hardFailedBanner` string; confirm byte-identical |
| Existing `hardFailedBanner` component tests extended (not replaced) to cover the hub-start-failure trigger | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `should extend existing hardFailedBanner render-condition tests to include the hub-start-failure trigger` | Jest (RTL) | Confirm the new trigger is a new `it()` block alongside pre-existing ones, not a replacement (reviewed at PR time) |

### Surface 3 — Feature-flag / path state in logs (4 criteria, condensed)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Every session-stream start logs `stream_path` as one of the two `StreamPath` enum values, never omitted | `server/services/connectrpc_websocket_test.go` | `TestStreamTerminal_should_LogStreamPathEnumValue_When_SessionStreamStarts` | Go test (`slog` capture) | Start a stream under each `StreamPath`; assert the captured log line's `stream_path` field is always `PathLegacyPerConnection` or `PathHubOwned`, never blank |
| `subscriber_count` present at hub creation and each attach/detach | `session/streamhub/observability_test.go` | `TestStreamHub_should_LogSubscriberCountOnEveryAttachDetachAndCreation_When_LifecycleEventsOccur` | Go test (`slog` capture) | Assert every `hub_created`/`subscriber_attached`/`subscriber_detached` log line carries a `subscriber_count` field |
| A log line states which `420584566` outcome applies (structurally impossible vs. hard-invariant conversion) per session | `session/streamhub/observability_test.go` | `TestStreamHub_should_LogWhichOverlapOutcomeApplies_When_SessionStreamStarts` | Go test (`slog` capture) | Assert a log line names the applicable outcome so an operator can audit from logs alone |
| No PII or session content in these log lines — identifiers and counts only | `session/streamhub/observability_test.go` | `TestStreamHub_should_NeverLogSessionContentOrPII_When_EmittingLifecycleLogLines` | Go test | Assert log line field set is exactly `{tmux_session, stream_path, subscriber_count, transport_type, capability, ...}`-shaped, never raw pane bytes |

**UX acceptance tests: 19/19 covered (11 + 4 + 4).**

## Rollback Rehearsal (Story 3.3.2 — manual, tracked here per plan.md)

- [x] Executed 2026-08-24 against the live operator instance — **PASS**. `config.RollbackRehearsalCompletedAt` persisted at `2026-08-24T02:29:59.571400044-07:00` via `CompleteStreamHubRollbackRehearsal`.
  - Created a disposable session (`streamhub-rehearsal-2`, `SESSION_TYPE_ONE_OFF`) via the real `CreateSession` RPC (not the MCP `create_session` tool — see below), forced `PathHubOwned` via `SetStreamHubSessionOverride`, and attached a real `StreamTerminal` WebSocket client. Confirmed `streamhub hub created` / `resolved_path=hub_owned`, a real subscriber attach with resize negotiation (`streamhub resize negotiated`), data flowing (`streamhub batch flushed`), and clean detach on disconnect (`subscriber_count` back to 0, no errors, no `OverlapInvariant` firings).
  - Created a second disposable session (`streamhub-rehearsal-3-legacy`) with **no** override and attached the same way — confirmed `routing managed session to control mode streaming` (legacy path), with real terminal output streamed and no errors. This is the practical stand-in for "remove the override and confirm clean legacy reconnect": `StreamOwnershipLock.Resolve` is deliberately sticky for a session's entire process lifetime (by design — see `session/streamhub/ownership.go`'s `resolveLocked`), so removing an override on the *same* already-hub-owned session cannot flip it back to legacy without a process restart. A fresh, unoverridden session resolving cleanly to legacy is the correct same-process equivalent, and it was exercised against the same real registration/attach path as the hub-owned case, not a fake.
  - Both disposable sessions and their overrides were deleted/cleared after the exercise; `stelekit`'s pre-existing override (set by the operator before this rehearsal) was left untouched.
  - **Side finding, not part of this rehearsal's pass/fail**: sessions created via the MCP `create_session` tool (`server/mcp/tools_lifecycle.go`) are never registered with `reviewQueuePoller`/`historyLinker` (unlike `session_service.go`'s `CreateSession` RPC, which does), so `resolveSession` reports `session not found` for them and they never appear in `WatchSessions` or session search. Worth its own bug report; did not block this rehearsal once a session was created through the real RPC instead.

## Test Stack

- **Unit**: Go `testing` package + `testify` (`stretchr/testify`, already a repo dependency) for assertions, `go.uber.org/goleak` for goroutine-leak verification (Story 1.2.1/1.4.2's explicit AC), table-driven tests per this repo's existing convention.
- **Integration**: Same Go `testing` stack, but exercising real cross-package wiring — fake `SessionController` doubles (Story 1.3.2's `SessionController` interface makes this possible without a real tmux process), a real `*connectWebSocketStream`/`TmuxSession`/`ExternalStreamer` where the AC specifically calls for it, and `-race` for every concurrency-sensitive test (Stories 1.4.2, 3.1.2).
- **E2E / UX**: Playwright (`tests/e2e/`) for multi-tab/multi-connection browser scenarios, following `.claude/rules/e2e-test-conventions.md` (feature annotation header `// @feature terminal:multi-connection-streaming`, no `waitForTimeout`, `data-testid`/ARIA-role locators only, a new `tests/e2e/pages/TerminalPage.ts` helper for shared attach/resize/hover interactions). Jest + React Testing Library for component-level accessibility assertions (`role`, `aria-live`, `aria-hidden`, announcement-debounce timing) where the AC itself calls for "DOM inspection / component test" rather than a live two-tab scenario. Axe Core via this repo's existing UX-analysis CI for contrast (AC7).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/streamhub/... ./session/... ./server/services/... -race -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, `session/streamhub` specifically ≥90% given it's the new core (Testability NFR) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public `StreamHub`/`HubRegistry`/`StreamOwnershipLock` methods: happy path + error path covered, per requirement mapping above.
- All 3 `Transport` implementations: unit-mocked (`MemoryTransport`) plus at least one integration test each (`WebSocketTransport` against a real stream, `MuxTransport` against a real `ExternalStreamer`).
- Every concurrency-sensitive test (attach/detach races, flag-flip races, ownership-lock contention) runs under `go test -race`, per plan.md's own AC language ("verified under `-race`").
- All 19 UX acceptance criteria from `design/ux.md` have a corresponding automated test or an explicit manual step above — none silently dropped.
- Migration test: **N/A** — plan.md's Migration Plan states no schema or persisted-data changes; the equivalent "safe transition" concern is covered by REQ-7/REQ-8/REQ-13's sticky-resolution, ownership-lock, and rollback-rehearsal tests instead.
