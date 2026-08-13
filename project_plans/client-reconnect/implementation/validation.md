# Validation Plan: client-reconnect

**Date**: 2026-06-23

---

## Happy Path Scenario

Given the session-watch stream and terminal stream are both connected, when the user's laptop wakes from sleep and the browser tab fires `visibilitychange` to `"visible"`, then both streams reconnect within 200 ms via browser lifecycle events (no page reload), the session list updates with current data, and the terminal pane appends a dim `--- reconnected ---` separator to the existing scroll buffer.

---

## Migration Tests

N/A — this feature has no database migration and no protocol change.

---

## Requirement → Test Mapping

### Phase 1: Backoff Utility (`backoff.ts`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Full jitter formula `Math.random() * Math.min(cap, base * 2^attempt)` | `web-app/src/lib/utils/backoff.test.ts` | `jitteredDelay_should_returnValueBetweenZeroAndCap_When_attemptIsZero` | Unit (happy) | attempt=0, base=1000, cap=30000 → result in [0, 1000] |
| REQ: Full jitter formula | `web-app/src/lib/utils/backoff.test.ts` | `jitteredDelay_should_neverExceedCapMs_When_attemptIsVeryLarge` | Unit (error) | attempt=50 → result ≤ 30000 |
| REQ: Backoff reset | `web-app/src/lib/utils/backoff.test.ts` | `BackoffState_should_returnBaseRangeDelay_When_resetCalledBeforeNext` | Unit (happy) | reset() → attempt back to 0, next() ≤ baseMs |
| REQ: Mean delay ≈ cap/2 over large N | `web-app/src/lib/utils/backoff.test.ts` | `jitteredDelay_should_haveMeanApproximatelyHalfCap_When_calledThousandTimes` | Unit (happy) | 1000 calls at attempt=10 → mean within 10% of cap/2 |
| REQ: `NON_RETRIABLE_WS_CODES` — code 4001 non-retriable | `web-app/src/lib/utils/backoff.test.ts` | `isRetriableCloseCode_should_returnFalse_When_codeIs4001` | Unit (error) | `isRetriableCloseCode(4001)` → false |
| REQ: `NON_RETRIABLE_WS_CODES` — code 4004 non-retriable | `web-app/src/lib/utils/backoff.test.ts` | `isRetriableCloseCode_should_returnFalse_When_codeIs4004` | Unit (error) | `isRetriableCloseCode(4004)` → false |
| REQ: Code 1006 (abnormal) IS retriable | `web-app/src/lib/utils/backoff.test.ts` | `isRetriableCloseCode_should_returnTrue_When_codeIs1006` | Unit (happy) | `isRetriableCloseCode(1006)` → true |
| REQ: Code 1000 (clean close) is retriable | `web-app/src/lib/utils/backoff.test.ts` | `isRetriableCloseCode_should_returnTrue_When_codeIs1000` | Unit (happy) | `isRetriableCloseCode(1000)` → true |
| REQ: `getWsCloseCode` extracts code from ConnectError metadata | `web-app/src/lib/utils/backoff.test.ts` | `getWsCloseCode_should_returnCode_When_connectErrorHasWsCloseCodeHeader` | Unit (happy) | ConnectError with header `ws-close-code: "4001"` → 4001 |
| REQ: `getWsCloseCode` returns null for non-ConnectError | `web-app/src/lib/utils/backoff.test.ts` | `getWsCloseCode_should_returnNull_When_errorIsPlainError` | Unit (error) | `new Error("boom")` → null |

### Phase 1: WS Close Code Propagation (Transport Layer)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Non-clean WS close → `ConnectError` with `ws-close-code` header | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `fromWebSocket_should_pushConnectError_When_wsClosesWithNonCleanCode` | Unit (happy) | ws closes with code=4001 → stream receives `ConnectError` with `ws-close-code: "4001"` |
| REQ: AbortSignal fire → `push(null)` (no error) | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `fromWebSocket_should_pushNull_When_abortSignalFires` | Unit (error) | abortController.abort() → push(null) called, no ConnectError emitted |
| REQ: `wasClean=true` WS close → `push(null)` | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `fromWebSocket_should_pushNull_When_wsClosesCleanly` | Unit (happy) | ws close event with wasClean=true → push(null) |
| REQ: Terminal transport propagates code 4004 | `web-app/src/lib/transport/websocket-transport.test.ts` | `fromWebSocket_should_throwConnectErrorWithCode4004_When_terminalWsClosesWithCode4004` | Integration | terminal ws closes 4004 → ConnectError header `ws-close-code: "4004"` propagates to hook |

### Phase 2: Watch Stream — `useSessionService`

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Stream reconnects after clean close | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_reconnectAfterJitteredDelay_When_streamClosesCleanly` | Unit (happy) | mock stream closes → jest fake timers advance → startStream called again |
| REQ: `stopWatching()` during backoff sleep halts reconnect | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_notReconnect_When_stopWatchingCalledDuringBackoffSleep` | Unit (error) | stream closes → stopWatching() called before timer fires → startStream NOT called second time |
| REQ: StreamGeneration counter prevents dual-stream | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_abortFirstStream_When_watchSessionsCalledConcurrently` | Unit (error) | two rapid watchSessions() calls → exactly one active stream; first generation abandoned |
| REQ: watchOptionsRef preserves stale options on reconnect | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_useStoredWatchOptions_When_reconnectFiresWithoutNewWatchSessionsCall` | Unit (happy) | watchSessions({categoryFilter:"work"}) → stream drops → reconnect → new stream uses categoryFilter:"work" |
| REQ: `visibilitychange` fires reconnect within 200ms | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleVisibilityOrOnline_should_callWatchSessions_When_documentBecomesVisible` | Unit (happy) | mock `document.visibilityState="visible"`, fire `visibilitychange` → watchSessions invoked within 200ms (jest fake timers) |
| REQ: `online` event fires reconnect | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleVisibilityOrOnline_should_callWatchSessions_When_windowOnlineEventFires` | Unit (happy) | fire `online` event → watchSessions invoked after 200ms debounce |
| REQ: Handler suppressed when `shouldReconnectRef=false` | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleVisibilityOrOnline_should_notReconnect_When_shouldReconnectRefIsFalse` | Unit (error) | stopWatching() then tab becomes visible → NO watchSessions call |
| REQ: Debounce — 3 rapid events produce 1 reconnect | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleVisibilityOrOnline_should_fireOnlyOnce_When_eventsFlapThreeTimesInTwoSeconds` | Unit (happy) | 3 visibilitychange events in 200ms → exactly 1 watchSessions call |
| REQ: Feature flag absent → no event listeners registered | `web-app/src/lib/hooks/useSessionService.test.ts` | `useSessionService_should_notRegisterVisibilityListener_When_featureFlagAbsent` | Unit (error) | NEXT_PUBLIC_RECONNECT_V2 unset → document.addEventListener not called for visibilitychange |
| REQ: `setConnectionState("connected")` after first event, not WS open | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_setConnectedAfterFirstEvent_When_wsOpens` | Unit (happy) | ws opens → state still NOT "connected" → first event arrives → state becomes "connected" |
| REQ: Non-retriable WS code 4001 → no reconnect | `web-app/src/lib/hooks/useSessionService.test.ts` | `startStream_should_notReconnect_When_streamClosesWithCode4001` | Unit (error) | stream throws ConnectError with `ws-close-code: "4001"` → no startStream retry |
| REQ: `listSessions` dispatch guarded after unmount (Pitfall #3) | `web-app/src/lib/hooks/useSessionService.test.ts` | `listSessions_should_notDispatch_When_stopWatchingCalledBeforeFetchCompletes` | Unit (error) | stopWatching() called while listSessions in-flight → dispatch(setSessions) NOT called |
| REQ: Seq backwards-jump → afterSeq reset to 0 | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleSessionEvent_should_resetAfterSeqToZero_When_seqBackwardsJumpDetected` | Unit (error) | lastSeqRef=5000, event.seq=1 → needsFullResyncRef=true, lastSeqRef=0 |
| REQ: No backwards-jump action on monotone seq | `web-app/src/lib/hooks/useSessionService.test.ts` | `handleSessionEvent_should_notResetAfterSeq_When_seqIncreasesMonotonically` | Unit (happy) | lastSeqRef=5000, event.seq=5001 → no reset, needsFullResyncRef stays false |
| REQ: `reconnectAttemptCount` exposed via context | `web-app/src/lib/hooks/useSessionService.test.ts` | `useSessionService_should_exposeReconnectAttemptCount_When_backoffStateAdvances` | Unit (happy) | after 3 reconnect attempts → `reconnectAttemptCount` in hook return value = 3 |
| REQ: StrictMode double-mount — single visibilitychange listener | `web-app/src/lib/hooks/useSessionService.test.ts` | `useSessionService_should_registerExactlyOneVisibilityListener_When_strictModeRemountsComponent` | Unit (happy) | mount → unmount → remount (StrictMode simulation) → `document.addEventListener("visibilitychange")` called net once |

### Phase 2: ConnectionIndicator

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Stale state → `watchSessions()` on click, not reload | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_callWatchSessions_When_clickedInStaleState` | Unit (happy) | render with connectionState="stale" → click → watchSessions mock called; window.location.reload NOT called |
| REQ: Disconnected state → `watchSessions()` on click | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_callWatchSessions_When_clickedInDisconnectedState` | Unit (happy) | render with connectionState="disconnected" → click → watchSessions called |
| REQ: Connected state → button shows "Live", no action on click | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_renderLiveLabel_When_connectionStateIsConnected` | Unit (happy) | connectionState="connected" → button text="Live"; click → watchSessions NOT called |
| REQ: `reconnectAttemptCount` in tooltip | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_showAttemptCountInTooltip_When_reconnectAttemptCountIsThree` | Unit (happy) | reconnectAttemptCount=3, hover → tooltip contains "attempt 3" |
| REQ: "Stale" and "Offline" labels replaced by "Reconnecting…" | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_showReconnectingLabel_When_stateIsStaleOrDisconnected` | Unit (happy) | both "stale" and "disconnected" states → button text = "Reconnecting…" (no "Stale", no "Offline") |
| REQ: aria-live on separate div, not on button | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_haveAriaLiveOnSeparateDiv_When_rendered` | Unit (happy) | DOM query: `button[aria-live]` → not found; `div[aria-live="polite"]` → found |
| REQ: aria-live announces "Reconnecting…" on state drop | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_announceReconnecting_When_connectionStateChangesToStale` | Unit (happy) | connectionState transitions connected→stale → aria-live div text = "Reconnecting…" |
| REQ: aria-live announces "Connection restored" on reconnect | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_announceConnectionRestored_When_connectionStateChangesToConnected` | Unit (happy) | stale→connected transition → aria-live div text = "Connection restored" |
| REQ: Tooltip contains hard-reload escape hatch | `web-app/src/components/layout/ConnectionIndicator.test.tsx` | `ConnectionIndicator_should_showReloadEscapeHatch_When_tooltipIsOpen` | Unit (happy) | hover → tooltip contains "Reload page (resets state)" link |
| REQ: `reconnectAttemptCount` field added to `SessionServiceContextValue` | `web-app/src/lib/contexts/SessionServiceContext.test.ts` | `SessionServiceContext_should_exposeReconnectAttemptCount_When_contextValueProvided` | Integration | context value with reconnectAttemptCount=5 → consumer reads 5 |

### Phase 3: Terminal Stream — `useTerminalStream`

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Auto-reconnect after clean close when flag enabled | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_reconnectAfterJitteredDelay_When_streamClosesCleanlyAndShouldReconnectTrue` | Unit (happy) | stream finally block runs, shouldReconnectRef=true → connect() scheduled via setTimeout |
| REQ: `disconnect()` during backoff → no reconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_notReconnect_When_disconnectCalledDuringBackoffSleep` | Unit (error) | stream closes → disconnect() called → setTimeout fires → connect() NOT called |
| REQ: Code 4004 → shouldReconnectRef=false, no reconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_setReconnectFalseAndNotRetry_When_wsClosesWithCode4004` | Unit (error) | stream throws ConnectError with `ws-close-code: "4004"` → shouldReconnectRef=false → no connect() call |
| REQ: Code 4001 → no reconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_notReconnect_When_wsClosesWithCode4001` | Unit (error) | ConnectError header `ws-close-code: "4001"` → no reconnect |
| REQ: Feature flag absent → no auto-reconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_notScheduleReconnect_When_featureFlagAbsent` | Unit (error) | NEXT_PUBLIC_RECONNECT_V2 unset → stream closes → connect() NOT called (existing TerminalOutput.tsx logic runs) |
| REQ: `shouldReconnectRef=true` after `connect()` | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_setShouldReconnectTrue_When_called` | Unit (happy) | call connect() → shouldReconnectRef.current = true |
| REQ: `shouldReconnectRef=false` after `disconnect()` | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `disconnect_should_setShouldReconnectFalse_When_called` | Unit (happy) | call disconnect() → shouldReconnectRef.current = false |
| REQ: Cleanup sets `shouldReconnectRef=false` before disconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `cleanup_should_setShouldReconnectFalseBeforeDisconnect_When_hookUnmounts` | Unit (happy) | unmount hook → shouldReconnectRef=false set before disconnect() called |
| REQ: `visibilitychange` fires connect() on terminal | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `handleVisibilityOrOnline_should_callConnect_When_tabBecomesVisibleAndStreamIsDisconnected` | Unit (happy) | shouldReconnectRef=true, isConnected=false → fire visibilitychange "visible" → connect() called immediately (backoff reset) |
| REQ: No terminal reconnect when shouldReconnectRef=false | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `handleVisibilityOrOnline_should_notCallConnect_When_shouldReconnectRefIsFalse` | Unit (error) | shouldReconnectRef=false → tab becomes visible → connect() NOT called |
| REQ: `online` event triggers terminal reconnect | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `handleVisibilityOrOnline_should_callConnect_When_onlineEventFires` | Unit (happy) | fire `online` event → connect() called |
| REQ: finally NOT catch schedules reconnect (single reconnect per attempt) | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `connect_should_scheduleReconnectOnlyOnce_When_retriableErrorThrown` | Unit (error) | retriable error thrown → exactly one setTimeout scheduled (not two) |
| REQ: StrictMode double-mount — single listener on terminal | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `useTerminalStream_should_registerExactlyOneVisibilityListener_When_strictModeRemounts` | Unit (happy) | mount → unmount → remount → `document.addEventListener("visibilitychange")` net once |

### Phase 3: Terminal Banner UX — `TerminalOutput`

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Banner hidden before 2s | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_notShowBanner_When_disconnectedForLessThanTwoSeconds` | Unit (happy) | isConnected=false, advance time 1999ms → no banner in DOM |
| REQ: Banner shown after 2s | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_showReconnectingBanner_When_disconnectedForTwoOrMoreSeconds` | Unit (happy) | isConnected=false, advance time 2000ms → banner with "Reconnecting terminal…" rendered |
| REQ: Banner hidden after reconnect | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_hideBanner_When_streamReconnects` | Unit (happy) | banner showing → isConnected=true → banner removed from DOM |
| REQ: `--- reconnected ---` separator written on reconnect | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_appendReconnectedSeparator_When_streamReconnectsAfterDisconnect` | Unit (happy) | banner was showing → reconnect → ANSI dim separator written to terminal |
| REQ: No banner on initial mount (never connected) | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_notShowBanner_When_initialMountWithNoConnection` | Unit (error) | mount with isConnected=false, hasEverConnected=false → 2000ms advance → no banner |
| REQ: 2s timer cleared on unmount | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_clearBannerTimer_When_componentUnmounts` | Unit (error) | isConnected=false → unmount at 1500ms → no setState-on-unmount warnings |
| REQ: Separator NOT written on first connect | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_notAppendSeparator_When_firstConnect` | Unit (happy) | initial connect (no prior disconnect) → no dim separator written |
| REQ: Hard failure state shown after 5 attempts | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_showHardFailureBanner_When_fiveConsecutiveReconnectsFail` | Unit (error) | connectionAttempts=5 → banner text="Connection lost", Retry button present |
| REQ: Retry button resets counter | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_resetAttemptCounter_When_retryButtonClicked` | Unit (happy) | hard failure state → click Retry → attempt counter resets, banner reverts to spinner state |
| REQ: Banner hidden on invisible pane | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_notShowBanner_When_paneIsNotVisible` | Unit (happy) | isVisible=false, isConnected=false, 2000ms → no banner rendered |

### Phase 4: Feature Flag + Integration

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ: Feature flag absent → existing behaviour unchanged | `web-app/src/lib/hooks/useSessionService.test.ts` | `useSessionService_should_useExistingBehaviour_When_featureFlagAbsent` | Unit (error) | NEXT_PUBLIC_RECONNECT_V2 unset → no visibilitychange listener, no backoff import called for new paths |
| REQ: Feature flag present → jitter + listeners active | `web-app/src/lib/hooks/useSessionService.test.ts` | `useSessionService_should_enableJitterAndListeners_When_featureFlagIsTrue` | Unit (happy) | NEXT_PUBLIC_RECONNECT_V2="true" → addEventListener called for visibilitychange and online |

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-1: Connected → "Live", disabled button, green dot | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_showLiveWithGreenDotAndBeDisabled_When_streamIsConnected` | Playwright | 1. Load app with stable connection. 2. Assert button text = "Live". 3. Assert button has `disabled` attr or `aria-disabled="true"`. 4. Assert a green dot/indicator is visible. 5. Click button — no watchSessions RPC fires (check network). |
| AC-2: Disconnected → "Reconnecting…" with spinner, amber | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_showReconnectingSpinnerAmber_When_streamDrops` | Playwright | 1. Load app. 2. Kill WS connection (intercept or force close). 3. Assert button text = "Reconnecting…". 4. Assert an animated spinner SVG/CSS element is visible (no dot). 5. Assert computed color is amber (not green, not red). 6. Assert button is enabled (cursor: pointer). |
| AC-3: Click triggers soft reconnect, no page reload | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_triggerSoftReconnect_When_clickedInReconnectingState` | Playwright | 1. Drop the WS connection. 2. Wait for "Reconnecting…" state. 3. Monitor page reload via `page.on('framenavigated')` — should NOT fire. 4. Monitor WatchSessions RPC call. 5. Click the indicator. 6. Assert WatchSessions RPC was called. 7. Assert page did NOT reload (Redux session list still has same items). |
| AC-4: Stale state shows "Reconnecting…" not "Stale" | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_neverShowStaleOrOfflineLabel_When_connectionIsStaleOrDisconnected` | Playwright | 1. Trigger stale state (suppress events 15+ s). 2. Assert button text ≠ "Stale". 3. Assert button text ≠ "Offline". 4. Assert button text = "Reconnecting…". |
| AC-5: Tooltip shows attempt count and reload link | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_showAttemptCountAndReloadLink_When_tooltipIsOpenDuringReconnect` | Playwright | 1. Drop WS. 2. Wait for attempt ≥ 1. 3. Hover "Reconnecting…" button (hold 300ms). 4. Assert tooltip visible. 5. Assert tooltip text matches /Reconnecting… attempt \d+/. 6. Assert tooltip contains "Reload page (resets state)" link. |
| AC-6: Hard reload only via tooltip escape hatch, never automatic | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_neverReloadAutomatically_When_reconnectingForThirtySeconds` | Playwright | 1. Drop WS. 2. Monitor `page.on('framenavigated')`. 3. Wait 30 seconds. 4. Assert NO framenavigated fired. 5. Assert "Reconnecting…" button still present. 6. Open tooltip — assert "Reload page" link exists as the only reload trigger. |
| AC-7: Screen reader aria-live announces on drop and restore | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_announceReconnectingAndRestoredViaAriaLive_When_connectionDropsAndRecovers` | Playwright + axe | 1. Run axe on page — 0 WCAG AA violations. 2. Drop WS → query `div[aria-live="polite"]` → assert text = "Reconnecting…". 3. Restore WS → assert text = "Connection restored". 4. Trigger another retry (same stale state) → assert text did NOT change on retry (no re-announcement). |
| AC-8: aria-live on separate div, not on button | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_notHaveAriaLiveOnButton_When_rendered` | Playwright | 1. Load any state. 2. Query `button[aria-live]` → assert 0 results. 3. Query `div[aria-live="polite"][aria-atomic="true"]` → assert 1 result. |
| AC-9: Terminal banner hidden < 2s after drop | `tests/e2e/client-reconnect.spec.ts` | `TerminalBanner_should_notBeVisible_When_disconnectedForLessThanTwoSeconds` | Playwright | 1. Navigate to session with terminal. 2. Drop terminal WS. 3. Within 1.9 s, assert banner element is not in DOM or not visible. |
| AC-10: Terminal banner appears after 2s | `tests/e2e/client-reconnect.spec.ts` | `TerminalBanner_should_appearWithSpinner_When_disconnectedForTwoSeconds` | Playwright | 1. Navigate to session with terminal. 2. Drop terminal WS. 3. Wait 2.1 s. 4. Assert banner with text "Reconnecting terminal…" is visible. 5. Assert terminal output area is still scrollable (user can scroll). 6. Assert terminal text is selectable. |
| AC-11: Terminal buffer preserved through reconnect | `tests/e2e/client-reconnect.spec.ts` | `TerminalOutput_should_preserveScrollBuffer_When_streamDropsAndReconnects` | Playwright | 1. Navigate to active session. 2. Note current terminal text (screenshot or text read). 3. Drop terminal WS. 4. Restore WS (reconnect). 5. Scroll to top of terminal. 6. Assert previous output is still present above the separator. |
| AC-12: `--- reconnected ---` separator visible after reconnect | `tests/e2e/client-reconnect.spec.ts` | `TerminalOutput_should_showReconnectedSeparator_When_streamReconnectsAfterDisconnect` | Playwright | 1. Drop terminal WS. 2. Wait for reconnect. 3. Assert terminal output contains `--- reconnected ---` line. 4. Assert that line appears dimmer than adjacent lines (check ANSI or CSS). |
| AC-13: Hard failure after 5 attempts → Retry button | `tests/e2e/client-reconnect.spec.ts` | `TerminalBanner_should_showHardFailureWithRetryButton_When_fiveReconnectAttemptsFail` | Playwright | 1. Drop terminal WS and block all reconnect attempts (mock or block all WS). 2. Wait for 5 failed attempts. 3. Assert banner text = "Connection lost". 4. Assert spinner replaced by warning icon. 5. Assert "Retry" button visible inside banner. 6. Click Retry — assert new WS connection attempted (network monitor). 7. Assert page did NOT reload. |
| AC-14: Terminal banner not shown on initial load | `tests/e2e/client-reconnect.spec.ts` | `TerminalBanner_should_notAppear_When_terminalMountsForFirstTime` | Playwright | 1. Navigate to a session terminal page from scratch. 2. For the first 5 seconds, assert no "Reconnecting terminal…" banner exists. 3. Assert loading overlay ("Loading terminal content…") is shown if applicable. |
| AC-15: Banner not shown on inactive/hidden pane | `tests/e2e/client-reconnect.spec.ts` | `TerminalBanner_should_notAppear_When_terminalPaneIsNotActiveTab` | Playwright | 1. Open session with terminal. 2. Switch to a different session or hide the terminal pane. 3. Drop the first terminal's WS. 4. Wait 3 s. 5. Switch back to the first terminal. 6. Assert no banner is shown (pane was invisible when drop occurred). |
| AC-16: Keyboard focus preserved after soft reconnect | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_retainFocus_When_keyboardUserTriggersReconnect` | Playwright | 1. Drop WS. 2. Tab to "Reconnecting…" button. 3. Press Enter to trigger reconnect. 4. Wait for reconnect or failure. 5. Assert `document.activeElement` is still the ConnectionIndicator button (not the terminal or session list). |
| AC-17: Separator not shown on first connect | `tests/e2e/client-reconnect.spec.ts` | `TerminalOutput_should_notShowSeparator_When_streamConnectsForFirstTime` | Playwright | 1. Navigate to session terminal. 2. Wait for initial connection. 3. Assert NO `--- reconnected ---` text visible in terminal. |
| AC-18: Tooltip accessible to keyboard users | `tests/e2e/client-reconnect.spec.ts` | `ConnectionIndicator_should_revealTooltipAndReloadLink_When_keyboardFocused` | Playwright | 1. Drop WS. 2. Tab to "Reconnecting…" button (do not hover). 3. Assert tooltip content becomes accessible (aria-describedby value resolves or tooltip visible). 4. Assert "Reload page (resets state)" link is reachable by Tab key. 5. Assert pressing Escape dismisses tooltip without moving focus. |

---

## Test Stack

- **Unit**: Jest + `@testing-library/react` / `@testing-library/react-hooks` (existing Jest config in `web-app/`)
- **Integration**: Jest with MSW (Mock Service Worker) for ConnectRPC/WebSocket stubs; fake timers for backoff delay control
- **E2E / UX**: Playwright (`tests/e2e/`) against the local test server (`http://localhost:8544`); axe-core for WCAG AC-7
- **Timer control**: `jest.useFakeTimers()` + `jest.advanceTimersByTime(ms)` for all debounce/backoff assertions

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest (backoff utility) | `cd web-app && npx jest --coverage --testPathPatterns="backoff.test"` | 100% line (pure functions) |
| TypeScript/Jest (hooks) | `cd web-app && npx jest --coverage --testPathPatterns="useSessionService\|useTerminalStream"` | ≥80% line |
| TypeScript/Jest (components) | `cd web-app && npx jest --coverage --testPathPatterns="ConnectionIndicator\|TerminalOutput"` | ≥80% line |
| TypeScript/Jest (all) | `cd web-app && npx jest --coverage` | ≥80% line overall |
| E2E / Playwright | `cd tests/e2e && npx playwright test client-reconnect.spec.ts` | All 18 UX acceptance tests pass |
| CI gate | `make ci` | Exit 0 (TypeScript, lint, unit tests) |

---

## Test File Summary

| File | New or Extend | Tests |
|---|---|---|
| `web-app/src/lib/utils/backoff.test.ts` | New | 10 unit |
| `web-app/src/lib/transport/watch-ws-transport.test.ts` | New | 3 unit |
| `web-app/src/lib/transport/websocket-transport.test.ts` | New | 1 integration |
| `web-app/src/lib/hooks/useSessionService.test.ts` | Extend existing | 17 unit |
| `web-app/src/components/layout/ConnectionIndicator.test.tsx` | New | 10 unit |
| `web-app/src/lib/contexts/SessionServiceContext.test.ts` | Extend existing | 1 integration |
| `web-app/src/lib/hooks/useTerminalStream.test.ts` | Extend existing | 13 unit |
| `web-app/src/components/sessions/TerminalOutput.test.tsx` | Extend existing | 10 unit |
| `tests/e2e/client-reconnect.spec.ts` | New | 18 UX acceptance |
