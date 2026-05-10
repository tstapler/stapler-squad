# Validation Plan: terminal-robustness

**Date**: 2026-05-09

---

## Requirement → Test Mapping

| Test ID | Requirement | Test File | Test Function / Name | Type | Scenario |
|---------|-------------|-----------|----------------------|------|----------|
| T-UNIT-GO-001 | R1.5 | `server/services/connectrpc_websocket_test.go` | `TestResizeCoalescing_should_callSetWindowSizeOnce_When_duplicateResizeWithin50ms` | Unit | Sends two identical resize messages within 10 ms; asserts `SetWindowSize` is called exactly once |
| T-UNIT-GO-002 | R1.5 | `server/services/connectrpc_websocket_test.go` | `TestResizeCoalescing_should_callSetWindowSizeTwice_When_differentSizesWithin50ms` | Unit | Sends two different sizes within 10 ms; asserts `SetWindowSize` is called twice |
| T-UNIT-GO-003 | R1.1 | `server/services/connectrpc_websocket_test.go` | `TestResizeQuiescence_should_waitForTmuxQuiescence_When_resizeApplied` | Unit | Mocks `waitForQuiescence`; asserts it is called with 300 ms timeout and 100 ms quiet window after `SetWindowSize` succeeds |
| T-UNIT-GO-004 | R1.1 | `server/services/connectrpc_websocket_test.go` | `TestResizeQuiescence_should_returnError_When_quiescenceTimesOut` | Unit | `waitForQuiescence` returns timeout error; asserts resize handler logs and continues without sending snapshot |
| T-UNIT-GO-005 | R1.4 | `server/services/connectrpc_websocket_test.go` | `TestResizeQuiescenceProto_should_marshalAndUnmarshal_When_resizingTrue` | Unit | Marshal `TerminalData{resize_quiescence: {resizing:true, cols:220, rows:50}}`, unmarshal; asserts all fields equal (proto round-trip) |
| T-UNIT-GO-006 | R1.4 | `server/services/connectrpc_websocket_test.go` | `TestResizeMessageSequence_should_sendTrueSnapshotFalse_When_resizeHandled` | Unit | Captures messages sent to stream during resize; asserts sequence is `ResizeQuiescence(resizing=true)` → `TerminalData_Output` (snapshot) → `ResizeQuiescence(resizing=false)` |
| T-UNIT-GO-007 | R2.4 | `server/services/connectrpc_websocket_test.go` | `TestScrollbackDispatch_should_returnScrollbackResponse_When_scrollbackRequestReceived` | Unit | Sends `ScrollbackRequest{FromSequence:0, Limit:100}` to input goroutine; asserts `ScrollbackResponse` with ≥1 chunk is written to stream |
| T-UNIT-GO-008 | R2.4 | `server/services/connectrpc_websocket_test.go` | `TestScrollbackDispatch_should_logAndContinue_When_scrollbackManagerReturnsError` | Unit | Mock `scrollbackManager.GetScrollback` returns error; asserts no panic, error is logged, stream continues |
| T-UNIT-GO-009 | R2.4 | `server/services/connectrpc_websocket_test.go` | `TestScrollbackDispatch_should_setHasMoreTrue_When_limitExactlyMet` | Unit | `GetScrollback` returns exactly `Limit` entries; asserts `has_more = true` in response |
| T-UNIT-GO-010 | R2.2 | `server/services/connectrpc_websocket_test.go` | `TestInitialScrollbackSend_should_sendScrollbackAfterSnapshot_When_sessionHasHistory` | Unit | Mocks session with 600 lines of history; asserts `TerminalData_ScrollbackResponse` is emitted after initial `TerminalData_Output` within the connection setup |
| T-UNIT-GO-011 | R2.6 | `server/services/connectrpc_websocket_test.go` | `TestScrollbackLinesDefault_should_use500_When_handshakeOmitsScrollbackLines` | Unit | Parses handshake with `scrollback_lines = 0` (unset); asserts server-side default of 500 is applied |
| T-UNIT-TS-001 | R2.1 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_constructTerminalWithScrollback5000_When_scrollbackPropIsUndefined` | Unit | Renders `XtermTerminal` without `scrollback` prop; spies on `Terminal` constructor; asserts `scrollback: 5000` |
| T-UNIT-TS-002 | R2.1 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_constructTerminalWithScrollbackProp_When_scrollbackPropProvided` | Unit | Renders with `scrollback={3000}`; asserts `Terminal` constructor called with `scrollback: 3000` |
| T-UNIT-TS-003 | R1.3 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_callFitAddonFitOnce_When_componentMounts` | Unit | Mounts component; spies on `FitAddon.prototype.fit`; asserts called exactly once during initialization (no secondary 100 ms fit) |
| T-UNIT-TS-004 | R1.2 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_callFitOnce_When_fiveRapidResizeObserverCallbacksWithin100ms` | Unit | Fires 5 ResizeObserver callbacks within 100 ms; advances fake timers to 150 ms; asserts `fitAddon.fit` called exactly once |
| T-UNIT-TS-005 | R1.2 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_callFitTwice_When_twoResizesSpacedMoreThan150msApart` | Unit | Fires two ResizeObserver callbacks 200 ms apart; asserts `fitAddon.fit` called exactly twice |
| T-UNIT-TS-006 | R1.6 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_ignoreCachedCellDimensions_When_fontSizeDiffers` | Unit | Stores localStorage cache with `fontSize: 12`; renders with `fontSize={14}`; spies on `fitAddon.proposeDimensions`; asserts cached dimensions are not applied |
| T-UNIT-TS-007 | R1.6 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_useCachedCellDimensions_When_fontConfigMatches` | Unit | Stores localStorage cache with matching `fontSize` and `fontFamily`; asserts cached dimensions ARE applied (no wasted measurement) |
| T-UNIT-TS-008 | R2.7 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleScrollbackReceived_should_callWriteInitialContent_When_metadataPresent` | Unit | Calls `handleScrollbackReceived` with non-empty scrollback string and a `metadata` object; asserts `manager.writeInitialContent` is called (no early return) |
| T-UNIT-TS-009 | R2.7 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleScrollbackReceived_should_callWriteInitialContent_When_noMetadata` | Unit | Calls with scrollback string, no metadata; asserts `manager.writeInitialContent` is called |
| T-UNIT-TS-010 | R2.3 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `scrollListener_should_callRequestScrollback_When_viewportYBelow200AndNotFetching` | Unit | Renders `TerminalOutput` with mocked terminal `viewportY = 100`; fires DOM scroll event on terminal element; asserts `requestScrollback` called with correct `fromSequence` |
| T-UNIT-TS-011 | R2.3 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `scrollListener_should_notCallRequestScrollback_When_alreadyFetching` | Unit | Sets `isFetchingScrollback = true`; fires scroll event with `viewportY = 100`; asserts `requestScrollback` NOT called (deduplicate guard) |
| T-UNIT-TS-012 | R2.3 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `scrollListener_should_notCallRequestScrollback_When_noMoreScrollback` | Unit | Sets `hasMoreScrollback = false`; fires scroll event; asserts `requestScrollback` NOT called |
| T-UNIT-TS-013 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `TerminalStreamManager_should_writeHistoryBeforeLiveChunks_When_liveWritesDuringInitialContent` | Unit | Calls `writeInitialContent('history')`; before it resolves calls `write('live')` three times; asserts xterm.js `write()` called with `'history'` before any `'live'` chunk |
| T-UNIT-TS-014 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `TerminalStreamManager_should_queueLiveWrites_When_isWritingInitialContentTrue` | Unit | Sets `isWritingInitialContent = true` directly; calls `write('queued')`; asserts xterm.js `write` is NOT called immediately; asserts written after flag clears |
| T-UNIT-TS-015 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `TerminalStreamManager_should_flushPendingInOrder_When_writeInitialContentCompletes` | Unit | Queues 3 live writes during `writeInitialContent`; asserts all 3 flushed in original order after initial content completes |
| T-UNIT-TS-016 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `prependScrollbackBatch_should_writeOldHistoryBeforeCurrentContent_When_bufferHasExistingContent` | Unit | Calls `prependScrollbackBatch('old-history')` with terminal already having content via `serializeAddon.serialize()`; asserts terminal `write()` called with `'old-history'` before serialized current content |
| T-UNIT-TS-017 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `prependScrollbackBatch_should_clearTerminalAndRewrite_When_called` | Unit | Spies on `terminal.clear()`; asserts it is called before any writes in `prependScrollbackBatch` |
| T-UNIT-TS-018 | R2.5 | `web-app/src/lib/terminal/TerminalStreamManager.test.ts` | `cleanup_should_nullOriginalWriteAndRefresh_When_called` | Unit | Creates manager, installs debug monitor, calls `cleanup()`; asserts `manager['originalWrite']` and `manager['originalRefresh']` are null |
| T-UNIT-TS-019 | R2.3 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleScrollbackReceived_should_callWriteInitialContent_When_firstCall` | Unit | Calls `handleScrollbackReceived` for the first time (initial load); asserts `writeInitialContent` is called, `prependScrollbackBatch` is NOT |
| T-UNIT-TS-020 | R2.3 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleScrollbackReceived_should_callPrependScrollbackBatch_When_subsequentCall` | Unit | Calls `handleScrollbackReceived` twice; asserts second call invokes `prependScrollbackBatch`, not `writeInitialContent` |
| T-UNIT-TS-021 | R4.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `useTerminalGestures_should_transitionIdlePendingScrollingIdle_When_touchStartMoveEnd` | Unit | Simulates `touchstart` + short-move `touchmove` (dy > 8px) + `touchend`; asserts state transitions IDLE→PENDING→SCROLLING→IDLE |
| T-UNIT-TS-022 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `useTerminalGestures_should_transitionToSelecting_When_longPressAfter400ms` | Unit | Simulates `touchstart`, advances timers 400 ms, fires timer callback; asserts state transitions IDLE→PENDING→SELECTING |
| T-UNIT-TS-023 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `useTerminalGestures_should_transitionToIdle_When_multiTouch` | Unit | Simulates `touchstart` with 2 touches; asserts state transitions directly to IDLE (cancels any in-progress gesture) |
| T-UNIT-TS-024 | R4.4 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `getCellDimensions_should_returnElementBasedDimensions_When_elementAvailable` | Unit | Mocks terminal with `element.clientHeight = 480`, `rows = 20`, `element.clientWidth = 800`, `cols = 80`; asserts `getCellDimensions` returns `{ cellH: 24, cellW: 10 }` |
| T-UNIT-TS-025 | R3.4 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `getCellDimensions_should_useFontMetricsFallback_When_elementNotAvailable` | Unit | Mocks terminal with no element (before `open()`); asserts `getCellDimensions` returns fallback derived from `fontSize` and `lineHeight` |
| T-UNIT-TS-026 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `isMouseTracking_should_returnTrue_When_modeIsVt200` | Unit | Sets `terminal.modes.mouseTrackingMode = 'vt200'`; asserts `isMouseTracking()` returns `true` |
| T-UNIT-TS-027 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `isMouseTracking_should_returnFalse_When_modeIsNone` | Unit | Sets `terminal.modes.mouseTrackingMode = 'none'`; asserts `isMouseTracking()` returns `false` |
| T-UNIT-TS-028 | R4.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `scrolling_should_callScrollLinesMinus1_When_touchmoveDy24WithCellH24` | Unit | Simulates 3 `touchmove` events each with dy=24 px, `cellH = 24`; asserts `terminal.scrollLines(-1)` called exactly 3 times (per-event delta, not cumulative) |
| T-UNIT-TS-029 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `selecting_should_callTerminalSelect_When_mouseTrackingVt200` | Unit | Long-press with `mouseTrackingMode = 'vt200'`; asserts `terminal.select` is called, `dispatchEvent(mousedown)` is NOT called |
| T-UNIT-TS-030 | R3.3 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `selecting_should_dispatchMousedown_When_mouseTrackingNone` | Unit | Long-press with `mouseTrackingMode = 'none'`; asserts synthetic `mousedown` is dispatched to `.xterm-screen`, `terminal.select` is NOT called |
| T-UNIT-TS-031 | R4.1 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `tapping_should_sendMouseEscapeSequence_When_mouseTrackingVt200` | Unit | Simulates tap at pixel `(100, 50)` with `cellH = 25, cellW = 10`; asserts `onSendData` called with correct `\x1b[M...` press+release sequence |
| T-UNIT-TS-032 | R4.2 | `web-app/src/lib/hooks/useTerminalGestures.test.ts` | `tapping_should_callTerminalFocus_When_mouseTrackingNone` | Unit | Simulates tap with `mouseTrackingMode = 'none'`; asserts `terminal.focus()` called, `onSendData` NOT called |
| T-UNIT-TS-033 | R3.1 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_showCopyButton_When_selectionChangeReturnsNonEmptyString` | Unit | Fires `terminal.onSelectionChange` callback with `getSelection()` returning `'hello'`; asserts Copy button element is in the document |
| T-UNIT-TS-034 | R3.1 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `XtermTerminal_should_hideCopyButton_When_selectionChangeReturnsEmptyString` | Unit | Fires `onSelectionChange` with `getSelection()` returning `''`; asserts Copy button is absent from the document |
| T-UNIT-TS-035 | R3.2 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `copyButton_should_callClipboardWriteText_When_pointerDown` | Unit | Sets `copyButtonPos` non-null; fires `pointerDown` on the Copy button; asserts `navigator.clipboard.writeText` called with mocked `getSelection()` return value |
| T-UNIT-TS-036 | R3.2 | `web-app/src/components/sessions/XtermTerminal.test.tsx` | `copyButton_should_showCopiedToast_When_pointerDown` | Unit | Fires `pointerDown` on Copy button; asserts "Copied" toast element appears; advances timers 1500 ms; asserts toast is gone |
| T-UNIT-TS-037 | R1.4 | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `useTerminalStream_should_setStateToResizing_When_resizeQuiescenceTrueReceived` | Unit | Injects `TerminalData{resize_quiescence: {resizing: true}}` into mock WebSocket; asserts `terminalState === 'RESIZING'` |
| T-UNIT-TS-038 | R1.4 | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `useTerminalStream_should_setStateToStable_When_resizeQuiescenceFalseReceived` | Unit | After RESIZING state, injects `{resizing: false}`; asserts `terminalState === 'STABLE'` |
| T-UNIT-TS-039 | R1.4 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_showResizingOverlay_When_terminalStateIsResizing` | Unit | Renders `TerminalOutput` with `terminalState = 'RESIZING'`; asserts overlay element in document with `aria-label="Terminal resizing"` |
| T-UNIT-TS-040 | R1.4 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `TerminalOutput_should_hideResizingOverlay_When_terminalStateIsStable` | Unit | Renders with `terminalState = 'STABLE'`; asserts overlay is absent |
| T-UNIT-TS-041 | R1.4 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleOutput_should_queueBytes_When_terminalStateIsResizing` | Unit | Simulates `terminalState = 'RESIZING'`; calls `handleOutput('live bytes')`; asserts `manager.write` NOT called |
| T-UNIT-TS-042 | R1.4 | `web-app/src/components/sessions/TerminalOutput.test.tsx` | `handleOutput_should_flushQueuedBytes_When_terminalStateTransitionsToStable` | Unit | Queues bytes in RESIZING; transitions to STABLE; asserts `manager.write('live bytes')` called in order |
| T-UNIT-TS-043 | R2.6 | `web-app/src/lib/hooks/useTerminalStream.test.ts` | `useTerminalStream_should_sendScrollbackLines500_When_connecting` | Unit | Captures the handshake proto message; asserts `scrollback_lines === 500` |
| T-E2E-001 | R2.1, R2.2, R2.3 | `tests/e2e/scrollback-robustness.spec.ts` | `scrollback_should_showHistoricalContent_When_userScrollsUp` | E2E | Connect to a session with >500 lines of history; scroll up past viewport; verify xterm.js shows historical output (not current screen repeated) by checking text content of scrolled-to lines |
| T-E2E-002 | R2.3 | `tests/e2e/scrollback-robustness.spec.ts` | `scrollback_should_loadMoreHistory_When_userScrollsNearTop` | E2E | Scroll to within 200 lines of top of xterm.js buffer; wait for `ScrollbackRequest` message (intercepted via WebSocket); assert server responds with a `ScrollbackResponse` containing older lines |
| T-E2E-003 | R2.5 | `tests/e2e/scrollback-robustness.spec.ts` | `scrollback_should_notCorruptDisplay_When_paginatedHistoryLoaded` | E2E | Load initial scrollback, scroll to trigger a second page load, then scroll back down; assert visible terminal content matches live output, not garbled or doubled |
| T-E2E-004 | R1.1, R1.2, R1.4 | `tests/e2e/resize-robustness.spec.ts` | `resize_should_showNoCorruption_When_browserPaneResized` | E2E | Open session with terminal text at a known column width; resize browser viewport by 200 px; wait for overlay to appear and disappear; assert terminal text reflows cleanly (no old-width lines remain) |
| T-E2E-005 | R1.4 | `tests/e2e/resize-robustness.spec.ts` | `resize_should_showOverlayDuringResize_When_resizeEventFires` | E2E | Trigger resize; assert the `.resizingOverlay` element (or `aria-label="Terminal resizing"`) is visible during quiescence wait; assert it disappears after snapshot arrives |
| T-E2E-006 | R1.3 | `tests/e2e/resize-robustness.spec.ts` | `resize_should_sendExactlyOneResizeRPC_When_singleResizeEvent` | E2E | Intercept WebSocket messages; trigger one resize; assert exactly one resize message sent to server (no double-fit artifact) |
| T-E2E-007 | R1.5 | `tests/e2e/resize-robustness.spec.ts` | `resize_should_coalesceToOneRPC_When_rapidIdenticalResizeEvents` | E2E | Programmatically trigger 5 identical resize events within 50 ms; assert server receives only 1 resize message |
| T-E2E-008 | R3.1, R3.2 | `tests/e2e/mobile-copy.spec.ts` | `copyFlow_should_showCopyButtonAndCopyToClipboard_When_textSelected` | E2E | Set mobile viewport (`375x812`); simulate long-press gesture to select text; assert floating Copy button appears; tap it; assert clipboard contains selected text |
| T-E2E-009 | R3.2 | `tests/e2e/mobile-copy.spec.ts` | `copyFlow_should_showCopiedToast_When_copyButtonTapped` | E2E | Tap Copy button; assert "Copied" toast element is visible; wait 1600 ms; assert toast is gone |
| T-E2E-010 | R3.3 | `tests/e2e/mobile-copy.spec.ts` | `copyFlow_should_selectTextViaTerminalSelectAPI_When_mouseTrackingVt200` | E2E | Launch Claude Code session (sets vt200 tracking); simulate 400 ms long-press; assert selection is non-empty (via `terminal.getSelection()`); no synthetic mousedown dispatched |
| T-E2E-011 | R4.1 | `tests/e2e/mobile-tap.spec.ts` | `tap_should_sendMouseEscapeSequenceToServer_When_mouseTrackingVt200` | E2E | Set mobile viewport; open Claude Code session; simulate single tap at a known pixel; capture PTY input bytes via server-side log; assert `\x1b[M` escape sequence received |
| T-E2E-012 | R4.2 | `tests/e2e/mobile-tap.spec.ts` | `tap_should_focusTerminal_When_mouseTrackingNone` | E2E | Set mobile viewport; open plain shell session (no mouse tracking); simulate single tap; assert terminal element receives focus (on-screen keyboard trigger) |
| T-E2E-013 | R5.4 | `tests/e2e/regression.spec.ts` | `desktop_should_passAllExistingTests_When_noRegressions` | E2E | Run full `make ci` suite; assert exit code 0 (no regressions in desktop keyboard shortcuts, search, WebGL rendering) |

---

## Test Stack

- **Unit (Go)**: `testing` stdlib + `testify/assert` + mock interfaces for `SessionInstance`, `ScrollbackManager`, and `StreamWriter`
- **Unit (TypeScript)**: Jest + React Testing Library (`@testing-library/react`) + `@testing-library/user-event`; xterm.js `Terminal` mocked via `jest.mock('@xterm/xterm')` with controllable `modes`, `element`, `rows`, `cols` stubs
- **E2E**: Playwright + Allure reporter; runs against `http://localhost:8544` (test server port); WebSocket message interception via `page.on('websocket', ...)`; mobile viewport set with `page.setViewportSize`

---

## How to Run

### Go unit tests

```bash
# Build protos first (required for generated bindings)
make build

# Run all resize/scrollback backend tests
go test ./server/services/... -run "TestResizeCoalescing|TestResizeQuiescence|TestResizeMessageSequence|TestResizeQuiescenceProto|TestScrollbackDispatch|TestInitialScrollbackSend|TestScrollbackLinesDefault" -v

# Run with race detector (recommended for goroutine interaction tests)
go test -race ./server/services/... -run "TestResizeCoalescing|TestScrollbackDispatch"
```

### TypeScript/Jest unit tests

```bash
cd web-app

# All terminal-robustness unit tests
npx jest --no-coverage --testPathPatterns="XtermTerminal.test|TerminalOutput.test|TerminalStreamManager.test|useTerminalGestures.test|useTerminalStream.test"

# Individual suites
npx jest --no-coverage --testPathPatterns="TerminalStreamManager.test"
npx jest --no-coverage --testPathPatterns="useTerminalGestures.test"
npx jest --no-coverage --testPathPatterns="XtermTerminal.test"
```

### E2E tests

```bash
# Start test server first
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &

cd tests/e2e

# Full robustness suite
npx playwright test scrollback-robustness.spec.ts resize-robustness.spec.ts mobile-copy.spec.ts mobile-tap.spec.ts regression.spec.ts

# Individual specs
npx playwright test resize-robustness.spec.ts
npx playwright test mobile-copy.spec.ts --project=mobile-chrome

# Allure report
make e2e-report
```

---

## Coverage Targets

- **Go unit test coverage**: ≥80% line coverage on `server/services/connectrpc_websocket.go` resize and scrollback dispatch paths
- **TypeScript unit test coverage**: ≥80% line coverage on `TerminalStreamManager.ts`, `useTerminalGestures.ts`, and `useTerminalStream.ts`
- **All public service methods**: happy path + at least one error path covered
- **All external integrations** (`ScrollbackManager`, `SessionInstance.SetWindowSize`, xterm.js `Terminal`): mocked in unit tests + at least one E2E test exercising the real stack

---

## Requirement Coverage Matrix

| Requirement | Unit Go | Unit TS | E2E | Covered? |
|-------------|---------|---------|-----|----------|
| R1.1 — quiescence wait before snapshot | T-UNIT-GO-003, T-UNIT-GO-004 | — | T-E2E-004 | YES |
| R1.2 — 150 ms flat debounce | — | T-UNIT-TS-004, T-UNIT-TS-005 | T-E2E-004 | YES |
| R1.3 — single fit() per resize cycle | — | T-UNIT-TS-003 | T-E2E-006 | YES |
| R1.4 — resize overlay (RESIZING state machine) | T-UNIT-GO-005, T-UNIT-GO-006 | T-UNIT-TS-037–042 | T-E2E-005 | YES |
| R1.5 — 50 ms coalescing guard | T-UNIT-GO-001, T-UNIT-GO-002 | — | T-E2E-007 | YES |
| R1.6 — localStorage font cache validation | — | T-UNIT-TS-006, T-UNIT-TS-007 | — | YES |
| R2.1 — xterm.js scrollback ≥ 5000 | — | T-UNIT-TS-001, T-UNIT-TS-002 | T-E2E-001 | YES |
| R2.2 — 500-line initial scrollback payload | T-UNIT-GO-010 | — | T-E2E-001 | YES |
| R2.3 — scroll-near-top triggers server fetch | — | T-UNIT-TS-010–012, T-UNIT-TS-019, T-UNIT-TS-020 | T-E2E-002 | YES |
| R2.4 — ScrollbackRequest dispatch wired | T-UNIT-GO-007, T-UNIT-GO-008, T-UNIT-GO-009 | — | T-E2E-002 | YES |
| R2.5 — write-lock ordering + prepend batch | — | T-UNIT-TS-013–018 | T-E2E-003 | YES |
| R2.6 — handshake scrollbackLines = 500 | T-UNIT-GO-011 | T-UNIT-TS-043 | — | YES |
| R2.7 — remove metadata guard | — | T-UNIT-TS-008, T-UNIT-TS-009 | — | YES |
| R3.1 — floating Copy button on selection | — | T-UNIT-TS-033, T-UNIT-TS-034 | T-E2E-008 | YES |
| R3.2 — Copy button invokes clipboard + toast | — | T-UNIT-TS-035, T-UNIT-TS-036 | T-E2E-009 | YES |
| R3.3 — long-press selection in all tracking modes | — | T-UNIT-TS-022, T-UNIT-TS-026–030 | T-E2E-010 | YES |
| R3.4 — replace private cell height API | — | T-UNIT-TS-024, T-UNIT-TS-025 | — | YES |
| R3.5 — touchmove extends selection | — | T-UNIT-TS-029 | T-E2E-010 | YES |
| R4.1 — tap sends mouse escape sequence | — | T-UNIT-TS-031 | T-E2E-011 | YES |
| R4.2 — tap focuses terminal (no tracking) | — | T-UNIT-TS-032 | T-E2E-012 | YES |
| R4.3 — merged gesture recognizer | — | T-UNIT-TS-021, T-UNIT-TS-023, T-UNIT-TS-028 | — | YES |
| R4.4 — public cell dimensions API | — | T-UNIT-TS-024, T-UNIT-TS-025 | — | YES |
| R5.4 — no desktop regressions | — | — | T-E2E-013 | YES |

**All 21 functional requirements have at least 1 test. R5.1–R5.3 (performance) are validated indirectly by E2E timeout assertions (T-E2E-001 ≤300 ms, T-E2E-004 ≤800 ms) rather than dedicated performance tests.**
