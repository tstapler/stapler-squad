# Feature Plan: Decompose TerminalOutput.tsx and useTerminalStream.ts

**Date**: 2026-03-18
**Status**: Draft
**Scope**: Split two monolithic files (~2,185 lines combined) into focused, testable modules

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Architecture](#research-findings-current-architecture)
- [ADR-015: Hook Composition Strategy for useTerminalStream](#adr-015-hook-composition-strategy-for-useterminalstream)
- [ADR-016: Class vs Hook for TerminalStreamManager](#adr-016-class-vs-hook-for-terminalstreammanager)
- [ADR-017: Backward Compatibility Contract for TerminalOutput Props](#adr-017-backward-compatibility-contract-for-terminaloutput-props)
- [Architecture Overview](#architecture-overview)
- [Story 1: Extract useTerminalConnection from useTerminalStream](#story-1-extract-useterminalconnection-from-useterminalstream)
- [Story 2: Extract useTerminalFlowControl from useTerminalStream](#story-2-extract-useterminalflowcontrol-from-useterminalstream)
- [Story 3: Extract useTerminalMetrics from useTerminalStream](#story-3-extract-useterminalmetrics-from-useterminalstream)
- [Story 4: Compose useTerminalStream from Extracted Hooks](#story-4-compose-useterminalstream-from-extracted-hooks)
- [Story 5: Extract TerminalStreamManager and TerminalDimensionCache from TerminalOutput](#story-5-extract-terminalstreammanager-and-terminaldimensioncache-from-terminaloutput)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

Two files form the core of the terminal streaming feature and are responsible for the
primary user-facing experience (real-time terminal output). Together they contain ~2,185
lines bundling 6-7 distinct concerns each, making the feature:

1. **Untestable in isolation** -- `useTerminalStream.ts` (968 lines) mixes WebSocket
   lifecycle, flow control, SSP protocol negotiation, LZMA decompression, RAF batching,
   and metrics into a single hook with 20+ refs. No unit tests exist for it today.
2. **Hard to debug** -- `TerminalOutput.tsx` (1,217 lines) mixes streaming setup,
   dimension caching, write buffering, flow control watermarks, escape sequence parsing,
   redraw throttling, connection management, and React rendering. A bug in flow control
   requires reading through unrelated dimension-caching code.
3. **Risky to modify** -- Any change to one concern risks regressions in others because
   they share mutable refs and closure state within a single scope.

**Goal**: Decompose both files into focused modules with clear boundaries, each independently
testable, while preserving the existing public API (the `TerminalOutputProps` interface and
the `TerminalStreamResult` return type).

---

## Research Findings: Current Architecture

### useTerminalStream.ts (968 lines)

**File**: `web-app/src/lib/hooks/useTerminalStream.ts`

Concerns identified by analyzing the source:

| Concern | Lines (approx) | Refs Used | Description |
|---------|----------------|-----------|-------------|
| **Connection lifecycle** | 290-665 | `clientRef`, `abortControllerRef`, `messageQueueRef`, `isConnectedRef`, `isDisconnectingRef` | WebSocket open/close via ConnectRPC `streamTerminal`, initial handshake with `CurrentPaneRequest`, SSP negotiation handling, reconnect guard |
| **Message dispatch** | 714-912 | `messageQueueRef` | `sendInput`, `sendInputWithEcho`, `resize`, `requestScrollback`, `sendFlowControl` -- all push `TerminalData` messages onto `MessageQueue` |
| **Flow control / backpressure** | 226-288 | `isResyncingRef`, `waitingForPaneResponseRef`, `lastResyncTimeRef`, `dimensionSyncRef` | `requestFullResync` with throttling, resync state machine, dimension mismatch detection |
| **SSP echo tracking** | 741-789 | `echoOverlayRef`, `echoCounterRef`, `echoTimestampsRef` | Predictive echo with RTT calculation and `EchoOverlay` initialization |
| **Output batching (RAF)** | 157-203 | `outputBufferRef`, `pendingUpdateRef`, `bufferSizeRef`, `textDecoderRef` | Adaptive requestAnimationFrame batching with 4KB threshold for burst scenarios |
| **Recording / debug** | 131-222 | `recordedMessagesRef`, `isRecordingRef` | WebSocket message recording for debugging terminal flickering |
| **State applicator integration** | 369-505 | `stateApplicatorRef` | Lazy initialization of `StateApplicator`, dimension mismatch handler wiring, state/diff message processing |
| **Cleanup** | 919-948 | all | Auto-connect/disconnect, RAF cleanup, echo overlay teardown |

**Key type**: `MessageQueue` class (lines 57-98) is a standalone async-iterable queue
that bridges React callbacks to the ConnectRPC bidirectional stream.

**Streaming modes**: `"raw"`, `"raw-compressed"`, `"state"`, `"hybrid"`, `"ssp"` --
each has different message processing in the `for await` loop.

### TerminalOutput.tsx (1,217 lines)

**File**: `web-app/src/components/sessions/TerminalOutput.tsx`

Concerns identified:

| Concern | Lines (approx) | Description |
|---------|----------------|-------------|
| **Dimension caching** | 94-120, 899-907 | `localStorage` read/write of `terminal-dimensions-{sessionId}`, used to skip size stability wait on reconnection |
| **Write buffering + flow control** | 129-315, 408-551 | `writeBufferRef`, `watermarkRef`, `isPausedRef`, HIGH/LOW watermark constants, `processWriteQueue`, `enqueueWrite`, `flushWriteBuffer` -- watermark-based backpressure against xterm.js |
| **Redraw throttling** | 155-203 | `RedrawThrottler` class defined inline, coalesces rapid full-screen redraws to 10 FPS max |
| **Escape sequence safety** | 147, 556-585 | `EscapeSequenceParser` instance ensuring ANSI sequences are not split across `terminal.write()` calls |
| **Size stability detection** | 745-818 | Debounced resize handler with cached-dimension fast path, double-RAF layout stability detection |
| **Connection management** | 836-897 | Connection state monitoring, reconnect button timer, auto-reconnect with exponential backoff |
| **Loading metrics** | 36-92 | `metricsRef` tracking mount, resize, connection, and first-output times; `performance.mark` integration |
| **Debug instrumentation** | 588-651 | Monkey-patching `terminal.write` and `terminal.refresh` with debug logging |
| **Rendering** | 1036-1217 | Toolbar (status, debug toggle, recording, mode selector, resize, clear, scroll, copy), terminal container with loading overlay, mobile keyboard |

### Supporting Modules (already well-factored)

- `XtermTerminal.tsx` (446 lines) -- xterm.js wrapper, already separate
- `EscapeSequenceParser.ts` (208 lines) -- standalone class with tests
- `StateApplicator.ts` -- standalone class with tests
- `EchoOverlay.ts` -- standalone class
- `CircularBuffer.ts` -- standalone class with tests
- `lzma.ts` -- standalone decompression utility

### Test Infrastructure

- **Runner**: Jest with `ts-jest` preset, `jsdom` environment
- **Pattern**: Co-located test files (`*.test.ts`) and `__tests__/` directories
- **Existing tests**: `StateApplicator.test.ts`, `CircularBuffer.test.ts`,
  `EscapeSequenceParser.test.ts`, `flow-control-stress.test.ts`,
  `websocket-transport.test.ts`
- **No Vitest**: The project uses Jest (see `web-app/package.json` and `jest.config.js`).
  Tests for this decomposition will use Jest, consistent with existing patterns.

---

## ADR-015: Hook Composition Strategy for useTerminalStream

### Status: Proposed

### Context

`useTerminalStream` is a 968-line hook containing 6-7 concerns sharing 20+ refs.
We need to decompose it while maintaining the existing `TerminalStreamResult` return type.

### Options Considered

1. **Custom hooks with shared ref object** -- Create a `TerminalStreamContext` ref
   object passed between hooks. Each sub-hook receives and mutates it.
2. **Custom hooks with callback coordination** -- Each sub-hook returns callbacks.
   The parent hook wires them together via dependency injection.
3. **Extract to classes, wrap in single hook** -- Move logic to plain TypeScript
   classes (`TerminalConnection`, `FlowController`, `MetricsCollector`), keep one
   thin hook that instantiates and coordinates them.

### Decision: Option 2 -- Custom hooks with callback coordination

**Rationale**:
- Follows React conventions (hooks compose via return values, not shared mutation).
- Each hook owns its own refs and state, eliminating cross-concern ref entanglement.
- The parent `useTerminalStream` becomes a thin "wiring" layer (estimated ~150 lines)
  that calls the three sub-hooks and returns the unified `TerminalStreamResult`.
- Testable: each sub-hook can be tested with `@testing-library/react-hooks`
  (renderHook) using mock callbacks for its dependencies.

**Trade-offs**:
- Slightly more verbose than shared-ref approach.
- Callback wiring in the parent hook must be carefully ordered to avoid stale closures.
  Mitigated by using `useCallback` with explicit dependency arrays and ref-based
  escape hatches where needed (following existing patterns in the codebase).

### Consequences

- `useTerminalConnection` owns: `clientRef`, `messageQueueRef`, `abortControllerRef`,
  `isConnectedRef`, `isDisconnectingRef`. Returns: `connect()`, `disconnect()`,
  `pushMessage()`, `isConnected`, `error`.
- `useTerminalFlowControl` owns: `isResyncingRef`, `waitingForPaneResponseRef`,
  `lastResyncTimeRef`, `dimensionSyncRef`, `lastResizeTimeRef`. Accepts: `pushMessage`
  callback. Returns: `requestFullResync()`, `resize()`, `sendFlowControl()`,
  `sendInput()`, `sendInputWithEcho()`, `requestScrollback()`.
- `useTerminalMetrics` owns: `outputBufferRef`, `pendingUpdateRef`, `bufferSizeRef`,
  `recordedMessagesRef`, `isRecordingRef`. Returns: `scheduleOutputUpdate()`,
  `flushOutputBuffer()`, `startRecording()`, `stopRecording()`.
- `useTerminalStream` composes all three + handles the message processing loop
  (state/diff/output/scrollback/SSP dispatch).

---

## ADR-016: Class vs Hook for TerminalStreamManager

### Status: Proposed

### Context

`TerminalOutput.tsx` contains ~500 lines of write buffering, flow control (watermarks),
escape sequence parsing, and redraw throttling. These are stateful but not React-state --
they are imperative buffer management using refs. We need to decide whether to extract
this as a React hook or a plain TypeScript class.

### Options Considered

1. **React hook (`useTerminalWriteManager`)** -- Keeps React lifecycle integration;
   returns `handleOutput`, `enqueueWrite`, `flushWriteBuffer` callbacks.
2. **Plain class (`TerminalStreamManager`)** -- Pure imperative logic; `TerminalOutput`
   creates an instance in a ref and calls methods directly.

### Decision: Option 2 -- Plain class (`TerminalStreamManager`)

**Rationale**:
- The write buffering logic is fundamentally imperative (watermarks, RAF scheduling,
  chunked writes with `terminal.write` callbacks). It does not use React state -- only
  refs. Wrapping this in a hook adds overhead without benefit.
- A class is trivially testable without `renderHook` -- instantiate with a mock
  terminal and call methods directly (same pattern as `StateApplicator.test.ts`).
- The existing `RedrawThrottler` (lines 155-203) is already defined as a class inside
  `TerminalOutput.tsx`, confirming this is the natural pattern.
- Follows the precedent set by `StateApplicator`, `EscapeSequenceParser`,
  `EchoOverlay`, and `CircularBuffer` -- all plain classes.

**Trade-offs**:
- Requires manual lifecycle management (create in `useEffect`, cleanup on unmount).
  Mitigated by a simple ref-based pattern already used for `stateApplicatorRef`.

### Consequences

- `TerminalStreamManager` class encapsulates: write buffering, watermark flow control,
  chunked writing, redraw throttling, escape sequence parsing integration.
- Constructor takes: xterm `Terminal` instance, `sendFlowControl` callback.
- `TerminalOutput.tsx` reduces to: hook calls, connection management effects, and
  pure React rendering.

---

## ADR-017: Backward Compatibility Contract for TerminalOutput Props

### Status: Proposed

### Context

`TerminalOutput` is consumed by parent components (session detail view, external
session view). Its props interface is:

```typescript
interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
  isExternal?: boolean;
  tmuxSessionName?: string;
}
```

And `useTerminalStream` returns `TerminalStreamResult` with 14 fields consumed by
`TerminalOutput`.

### Decision

Both interfaces are **frozen** for this refactoring:

1. `TerminalOutputProps` must not change. No new required props. No removed props.
2. `TerminalStreamResult` return type must not change. Sub-hooks may return subsets,
   but `useTerminalStream` must aggregate them into the same shape.
3. `UseTerminalStreamOptions` must not change. New sub-hooks may accept subsets,
   but the parent hook's options signature is preserved.

### Verification

A type-level compatibility test will be added that imports both interfaces and
asserts structural equivalence using TypeScript's `Exact` and `Extends` type helpers.

---

## Architecture Overview

### Before (Current)

```
TerminalOutput.tsx (1,217 lines)
  |-- useTerminalStream.ts (968 lines)
  |     |-- MessageQueue class
  |     |-- Connection lifecycle
  |     |-- Flow control / resync
  |     |-- SSP echo tracking
  |     |-- Output batching (RAF)
  |     |-- Recording / debug
  |     |-- State applicator integration
  |     \-- Cleanup
  |-- Write buffering + watermarks
  |-- RedrawThrottler class
  |-- Escape sequence parsing
  |-- Dimension caching (localStorage)
  |-- Size stability detection
  |-- Connection management (reconnect)
  |-- Loading metrics
  |-- Debug instrumentation
  \-- React rendering (toolbar + terminal + mobile keys)
```

### After (Target)

```
TerminalOutput.tsx (~350 lines) -- rendering, effects, hook wiring
  |-- useTerminalStream.ts (~150 lines) -- thin composition layer
  |     |-- useTerminalConnection.ts (~250 lines) -- WebSocket lifecycle
  |     |     \-- MessageQueue.ts (~50 lines) -- extracted standalone class
  |     |-- useTerminalFlowControl.ts (~200 lines) -- resync, resize, message dispatch
  |     \-- useTerminalMetrics.ts (~100 lines) -- RAF batching, recording
  |-- TerminalStreamManager.ts (~300 lines) -- write buffering, watermarks, throttling
  |     |-- RedrawThrottler (moved inside or co-located)
  |     \-- uses EscapeSequenceParser (existing)
  \-- TerminalDimensionCache.ts (~50 lines) -- localStorage dimension persistence
```

### File Inventory (New Files)

| File | Type | Lines (est) | Concern |
|------|------|-------------|---------|
| `web-app/src/lib/hooks/useTerminalConnection.ts` | Hook | ~250 | WebSocket open/close, ConnectRPC client, message iteration |
| `web-app/src/lib/hooks/useTerminalFlowControl.ts` | Hook | ~200 | Resync, resize throttle, dimension sync, message dispatch |
| `web-app/src/lib/hooks/useTerminalMetrics.ts` | Hook | ~100 | RAF batching, recording start/stop |
| `web-app/src/lib/terminal/MessageQueue.ts` | Class | ~50 | Async-iterable message queue (extracted from useTerminalStream) |
| `web-app/src/lib/terminal/TerminalStreamManager.ts` | Class | ~300 | Write buffering, watermarks, chunking, redraw throttling |
| `web-app/src/lib/terminal/TerminalDimensionCache.ts` | Utility | ~50 | localStorage get/save for terminal dimensions |

### Files Modified

| File | Change |
|------|--------|
| `web-app/src/lib/hooks/useTerminalStream.ts` | Rewrite to ~150 lines composing 3 sub-hooks |
| `web-app/src/components/sessions/TerminalOutput.tsx` | Rewrite to ~350 lines using TerminalStreamManager, TerminalDimensionCache |

---

## Story 1: Extract useTerminalConnection from useTerminalStream

**Goal**: Isolate WebSocket lifecycle management into a focused hook.

### Acceptance Criteria

- `useTerminalConnection` manages the ConnectRPC `streamTerminal` bidirectional stream.
- It owns `connect()`, `disconnect()`, `isConnected`, `error` state.
- It exposes `pushMessage(msg: TerminalData)` for other hooks to send messages.
- It exposes `onMessage` callback registration for processing incoming messages.
- `MessageQueue` class is extracted to `web-app/src/lib/terminal/MessageQueue.ts`.
- The hook handles graceful shutdown (close queue, wait for stream, abort).
- Auto-connect logic (`autoConnect` option) is preserved.
- Connection guard (prevent double-connect, prevent disconnect during resync) is preserved.

### Atomic Tasks

**Task 1.1: Extract MessageQueue to standalone module**
- Create `web-app/src/lib/terminal/MessageQueue.ts`
- Move `MessageQueue` class (lines 57-98 of `useTerminalStream.ts`) verbatim
- Export as named export
- Add JSDoc with usage example
- Update import in `useTerminalStream.ts` to verify no breakage

**Task 1.2: Create useTerminalConnection hook skeleton**
- Create `web-app/src/lib/hooks/useTerminalConnection.ts`
- Define `UseTerminalConnectionOptions` interface (subset of current options):
  `baseUrl`, `sessionId`, `getTerminal`, `autoConnect`, `onError`
- Define `UseTerminalConnectionResult` interface:
  `isConnected`, `error`, `connect(cols?, rows?)`, `disconnect()`,
  `pushMessage(msg)`, `setMessageHandler(handler)`
- Implement with refs: `clientRef`, `messageQueueRef`, `abortControllerRef`,
  `isConnectedRef`, `isDisconnectingRef`

**Task 1.3: Move connect/disconnect logic**
- Move `connect` function (lines 290-665) into `useTerminalConnection`
- The `connect` function currently contains the message processing loop (`for await`).
  Extract the loop body into a pluggable `onMessage` callback that the parent hook
  provides. The connection hook calls `onMessage(msg)` for each received message.
- Move `disconnect` function (lines 667-712)
- Move auto-connect `useEffect` (lines 919-949)
- Preserve resync guard: accept `isResyncingRef` as an option or expose a
  `setResyncGuard(value)` method.

**Task 1.4: Write unit tests for useTerminalConnection**
- Create `web-app/src/lib/hooks/__tests__/useTerminalConnection.test.ts`
- Test: connect sets `isConnected` to true after first message
- Test: disconnect closes queue and aborts controller
- Test: double-connect is a no-op
- Test: disconnect during resync is deferred
- Test: `pushMessage` forwards to queue
- Test: auto-connect on mount when `autoConnect` is true
- Test: cleanup on unmount calls disconnect
- Mock: `createPromiseClient` and `createWebsocketBasedTransport`

**Task 1.5: Write unit tests for MessageQueue**
- Create `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts`
- Test: push and iterate yields messages in order
- Test: close unblocks waiting iterator
- Test: sentinel messages are filtered out
- Test: push after close is ignored

---

## Story 2: Extract useTerminalFlowControl from useTerminalStream

**Goal**: Isolate resync logic, resize throttling, and message dispatch into a focused hook.

### Acceptance Criteria

- `useTerminalFlowControl` owns resync state machine (`isResyncingRef`,
  `waitingForPaneResponseRef`, throttling).
- It owns resize throttling (`lastResizeTimeRef`, 200ms throttle, post-resize pane request).
- It provides `sendInput()`, `sendInputWithEcho()`, `resize()`, `requestScrollback()`,
  `sendFlowControl()`, `requestFullResync()`.
- All message-sending functions accept a `pushMessage` callback (injected by parent).
- SSP echo tracking (`echoCounterRef`, `echoTimestampsRef`, `echoOverlayRef`) lives here
  because `sendInputWithEcho` needs it.
- The hook does NOT own the WebSocket connection or message iteration.

### Atomic Tasks

**Task 2.1: Create useTerminalFlowControl hook**
- Create `web-app/src/lib/hooks/useTerminalFlowControl.ts`
- Define options: `sessionId`, `streamingMode`, `enablePredictiveEcho`, `getTerminal`,
  `pushMessage`, `onError`, `onEchoAck`
- Define result: `sendInput()`, `sendInputWithEcho()`, `resize()`,
  `requestScrollback()`, `sendFlowControl()`, `requestFullResync()`,
  `getIsResyncingRef()`, `markResyncComplete()`, `markPaneResponseReceived()`,
  `sspNegotiated`, `setSspNegotiated()`, `initEchoOverlay()`, `getIsApplyingState()`

**Task 2.2: Move message dispatch functions**
- Move `sendInput` (lines 714-739)
- Move `sendInputWithEcho` (lines 741-789)
- Move `resize` (lines 791-852)
- Move `requestScrollback` (lines 854-882)
- Move `sendFlowControl` (lines 884-912)
- Move `requestFullResync` (lines 226-288)
- All functions call `pushMessage(new TerminalData({...}))` instead of
  `messageQueueRef.current.push()`

**Task 2.3: Move SSP echo tracking**
- Move `echoOverlayRef`, `echoCounterRef`, `echoTimestampsRef` refs
- Move echo overlay initialization logic (from SSP negotiation handler)
- Move echo timestamp recording and RTT calculation

**Task 2.4: Move StateApplicator integration**
- Move `stateApplicatorRef` and lazy initialization logic
- Move dimension mismatch handler wiring
- Move `getIsApplyingState` helper
- The connection hook's message handler will call into flow control methods:
  `flowControl.handleStateMessage(msg)`, `flowControl.handleDiffMessage(msg)`,
  `flowControl.handleSSPNegotiation(msg)`

**Task 2.5: Write unit tests for useTerminalFlowControl**
- Create `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`
- Test: `sendInput` calls `pushMessage` with correct `TerminalData`
- Test: `resize` is throttled to 200ms
- Test: `resize` sends follow-up `CurrentPaneRequest` after 100ms delay
- Test: `requestFullResync` is throttled to 2s (unless urgent)
- Test: urgent resync bypasses throttle
- Test: `sendInputWithEcho` increments echo counter and stores timestamp
- Test: `sendFlowControl` sends correct `FlowControl` message
- Mock: `pushMessage` callback, `getTerminal` callback

---

## Story 3: Extract useTerminalMetrics from useTerminalStream

**Goal**: Isolate output batching (RAF) and debug recording into a focused hook.

### Acceptance Criteria

- `useTerminalMetrics` owns the RAF-based output buffer (`outputBufferRef`,
  `pendingUpdateRef`, `bufferSizeRef`).
- It provides `scheduleOutputUpdate(text)` and `flushOutputBuffer()`.
- It owns recording state (`recordedMessagesRef`, `isRecordingRef`).
- It provides `startRecording()`, `stopRecording()`.
- It provides `recordMessage(msg)` for the message handler to call.
- Cleanup cancels pending RAF on unmount.

### Atomic Tasks

**Task 3.1: Create useTerminalMetrics hook**
- Create `web-app/src/lib/hooks/useTerminalMetrics.ts`
- Define options: `onOutput` (callback from parent for direct output mode)
- Define result: `scheduleOutputUpdate()`, `flushOutputBuffer()`,
  `startRecording()`, `stopRecording()`, `recordMessage()`, `output` (deprecated state)

**Task 3.2: Move RAF batching logic**
- Move `outputBufferRef`, `pendingUpdateRef`, `bufferSizeRef`, `textDecoderRef` refs
- Move `flushOutputBuffer` (lines 170-178)
- Move `scheduleOutputUpdate` (lines 184-203)
- Move 4KB immediate-flush threshold logic

**Task 3.3: Move recording logic**
- Move `recordedMessagesRef`, `isRecordingRef` refs
- Move `startRecording` (lines 206-209)
- Move `stopRecording` (lines 212-222) -- Blob creation and download

**Task 3.4: Write unit tests for useTerminalMetrics**
- Create `web-app/src/lib/hooks/__tests__/useTerminalMetrics.test.ts`
- Test: `scheduleOutputUpdate` with small text schedules RAF (not immediate flush)
- Test: `scheduleOutputUpdate` with >4KB text flushes immediately
- Test: `flushOutputBuffer` joins buffered text and resets
- Test: `startRecording` sets recording flag
- Test: `stopRecording` creates Blob download
- Test: cleanup cancels pending RAF
- Mock: `requestAnimationFrame`, `cancelAnimationFrame`, `URL.createObjectURL`

---

## Story 4: Compose useTerminalStream from Extracted Hooks

**Goal**: Rewrite `useTerminalStream` as a thin composition layer that wires the three
sub-hooks together and handles the message processing loop.

### Acceptance Criteria

- `useTerminalStream` signature unchanged: same `UseTerminalStreamOptions`, same
  `TerminalStreamResult`.
- Internal implementation calls `useTerminalConnection`, `useTerminalFlowControl`,
  `useTerminalMetrics`.
- The message processing loop (currently lines 358-649) lives in a `handleMessage`
  callback passed to `useTerminalConnection`.
- File is ~150 lines (down from 968).
- All existing call sites of `useTerminalStream` work without modification.

### Atomic Tasks

**Task 4.1: Rewrite useTerminalStream as composition**
- Import and call `useTerminalConnection`, `useTerminalFlowControl`, `useTerminalMetrics`
- Wire `pushMessage` from connection into flow control
- Wire `onMessage` handler that dispatches to:
  - `msg.data.case === "state"` -> flow control state handling
  - `msg.data.case === "diff"` -> flow control diff handling
  - `msg.data.case === "sspNegotiation"` -> flow control SSP handling
  - `msg.data.case === "output"` -> metrics recording + decompression + output callback
  - `msg.data.case === "currentPaneResponse"` -> scrollback callback
  - `msg.data.case === "scrollbackResponse"` -> scrollback callback
  - `msg.data.case === "error"` -> error state
- Return unified `TerminalStreamResult` by aggregating sub-hook returns

**Task 4.2: Add type compatibility tests**
- Create `web-app/src/lib/hooks/__tests__/useTerminalStream.compat.test.ts`
- Import `UseTerminalStreamOptions` and `TerminalStreamResult` types
- Assert type structural compatibility using TypeScript's `satisfies` or assertion helpers
- Verify all 14 return fields are present with correct types

**Task 4.3: Add integration test for message dispatching**
- Create `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`
- Test: raw output message is decompressed and forwarded to `onOutput`
- Test: state message initializes StateApplicator and applies state
- Test: SSP negotiation message sets `sspNegotiated` flag
- Test: error message sets error state
- Test: connection + disconnect lifecycle (happy path)
- Mock: ConnectRPC client, terminal instance

**Task 4.4: Delete dead code from useTerminalStream**
- Remove all moved refs, functions, and classes
- Verify `useTerminalStream.ts` is ~150 lines
- Run `tsc --noEmit` to verify type correctness
- Run full test suite to verify no regressions

---

## Story 5: Extract TerminalStreamManager and TerminalDimensionCache from TerminalOutput

**Goal**: Reduce `TerminalOutput.tsx` from 1,217 lines to ~350 lines of rendering and
effect wiring by extracting imperative logic into plain classes.

### Acceptance Criteria

- `TerminalOutputProps` interface is unchanged.
- `TerminalStreamManager` class handles: write buffering, watermark flow control,
  chunked writes, redraw throttling, escape sequence parsing.
- `TerminalDimensionCache` utility handles: localStorage get/save per session.
- `TerminalOutput.tsx` contains only: hook calls, useEffect wiring, React rendering.
- All toolbar buttons continue to work (debug, record, mode select, resize, clear,
  scroll, copy).
- Loading overlay behavior preserved.
- Mobile keyboard preserved.

### Atomic Tasks

**Task 5.1: Extract TerminalDimensionCache**
- Create `web-app/src/lib/terminal/TerminalDimensionCache.ts`
- Move `getCachedDimensions` (lines 95-109) and `saveDimensions` (lines 111-120)
- API: `getCachedDimensions(sessionId): {cols, rows} | null`
- API: `saveDimensions(sessionId, cols, rows): void`
- Pure functions with `localStorage` access (no React state)

**Task 5.2: Extract TerminalStreamManager class**
- Create `web-app/src/lib/terminal/TerminalStreamManager.ts`
- Constructor: `(terminal: Terminal, sendFlowControl: (paused, watermark?) => void)`
- Move constants: `HIGH_WATERMARK`, `LOW_WATERMARK`, `CHUNK_SIZE`, `CHUNK_DELAY_MS`
- Move `RedrawThrottler` class (lines 155-203) as private inner class or co-located export
- Move write buffering: `writeBufferRef`, `writeScheduledRef`, `writeQueueRef`,
  `isProcessingQueueRef`, `pendingWritesRef`, `totalBytesWrittenRef`,
  `totalBytesCompletedRef`, `watermarkRef`, `isPausedRef`
- Move `processWriteQueue` (lines 218-315)
- Move `enqueueWrite` (lines 318-324)
- Move `flushWriteBuffer` (lines 410-467)
- Move `handleProcessedOutput` (lines 470-551)
- Move `handleOutput` (lines 556-586) as `write(output: string)`
- Move `handleScrollbackReceived` (lines 363-406) as `writeInitialContent(content: string)`
- Include `EscapeSequenceParser` instance as private field
- Expose `cleanup()` for unmount

**Task 5.3: Write unit tests for TerminalStreamManager**
- Create `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`
- Follow `StateApplicator.test.ts` pattern with `MockTerminal` class
- Test: small write (<16KB) goes directly to `terminal.write`
- Test: large write (>16KB) is chunked with yields
- Test: watermark exceeds HIGH_WATERMARK triggers pause callback
- Test: watermark drops below LOW_WATERMARK triggers resume callback
- Test: redraw throttler coalesces rapid full-screen redraws
- Test: escape sequence safety (partial ANSI at chunk boundary)
- Test: `writeInitialContent` clears terminal, writes, scrolls to bottom
- Test: `cleanup` flushes pending writes

**Task 5.4: Write unit tests for TerminalDimensionCache**
- Create `web-app/src/lib/terminal/__tests__/TerminalDimensionCache.test.ts`
- Test: `saveDimensions` writes to localStorage
- Test: `getCachedDimensions` reads from localStorage
- Test: `getCachedDimensions` returns null when no cached value
- Test: handles localStorage errors gracefully (quota exceeded, etc.)
- Mock: `localStorage` with Jest spies

**Task 5.5: Rewrite TerminalOutput.tsx to use extracted modules**
- Import `TerminalStreamManager`, `TerminalDimensionCache`
- Replace inline write buffering with `TerminalStreamManager` instance in ref
- Replace inline dimension caching with `TerminalDimensionCache` calls
- Remove `RedrawThrottler` class definition
- Remove all flow-control refs (`watermarkRef`, `isPausedRef`, etc.)
- Preserve: size stability detection, connection state effects, toolbar rendering
- Verify `TerminalOutput.tsx` is ~350 lines
- Run `tsc --noEmit` to verify type correctness

**Task 5.6: Verify backward compatibility**
- Verify `TerminalOutputProps` has not changed
- Verify all toolbar buttons function correctly
- Verify loading overlay appears and dismisses correctly
- Verify terminal connects, displays output, handles resize
- Manual smoke test: open session detail, verify terminal streaming works

---

## Known Issues and Bug Risks

### Bug Risk 1: SSP State Machine Regression During Hook Extraction [SEVERITY: High]

**Description**: The SSP (State Synchronization Protocol) mode has a complex state machine
involving `StateApplicator` lazy initialization, dimension mismatch detection, echo overlay
wiring, and diff sequence validation. The state machine spans three message types (`state`,
`diff`, `sspNegotiation`) and relies on shared refs (`stateApplicatorRef`,
`isResyncingRef`, `waitingForPaneResponseRef`). Extracting these into separate hooks
risks breaking the initialization ordering or losing ref synchronization.

**Affected areas**:
- `useTerminalFlowControl.ts` (new) -- StateApplicator lazy init
- `useTerminalStream.ts` (rewritten) -- message dispatch to flow control
- The dimension mismatch handler calls `requestFullResync`, which sets `isResyncingRef`
  and `waitingForPaneResponseRef`. If these are in a different hook than the message
  handler that reads them, the timing may change.

**Mitigation**:
- Keep StateApplicator initialization and its mismatch handler wiring in the SAME hook
  (`useTerminalFlowControl`) to preserve synchronous ref access.
- Add explicit integration tests that simulate the full SSP handshake sequence:
  `connect -> sspNegotiation -> state -> diff -> dimension_mismatch -> resync -> state`.
- Add a test for the specific race: `requestFullResync` called while
  `waitingForPaneResponseRef` is already true (should be a no-op).

**Prevention checklist**:
- [ ] Verify `stateApplicatorRef` and `isResyncingRef` are in same hook
- [ ] Verify `markResyncComplete()` is called from correct message handler
- [ ] Integration test covers dimension mismatch -> resync -> recovery cycle

---

### Bug Risk 2: RAF Timing Fragility in Tests [SEVERITY: Medium]

**Description**: `useTerminalMetrics` uses `requestAnimationFrame` for adaptive batching.
Jest's `jsdom` environment does not have a real RAF implementation. The existing codebase
does not appear to mock RAF in any test. Tests that rely on RAF timing may be flaky or
may silently skip the batching logic.

**Affected areas**:
- `web-app/src/lib/hooks/__tests__/useTerminalMetrics.test.ts` (new)
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts` (new)
  -- `processWriteQueue` uses RAF for yield between chunks

**Mitigation**:
- Use `jest.useFakeTimers()` combined with a manual RAF mock:
  ```typescript
  let rafCallback: FrameRequestCallback | null = null;
  global.requestAnimationFrame = (cb) => { rafCallback = cb; return 1; };
  global.cancelAnimationFrame = () => { rafCallback = null; };
  const flushRAF = () => { if (rafCallback) { rafCallback(performance.now()); rafCallback = null; } };
  ```
- Add `flushRAF()` helper to test utilities for deterministic testing.
- Document in test file that RAF behavior differs from browser.

**Prevention checklist**:
- [ ] RAF mock is set up in test `beforeEach`
- [ ] Tests explicitly call `flushRAF()` to trigger batched writes
- [ ] Tests verify both immediate-flush (>4KB) and deferred-flush (<4KB) paths

---

### Bug Risk 3: Stale Closure in Callback Wiring [SEVERITY: Medium]

**Description**: When `useTerminalStream` wires `pushMessage` from `useTerminalConnection`
into `useTerminalFlowControl`, the callback may capture a stale reference if the
connection hook recreates `pushMessage` on re-render but the flow control hook has already
closed over the old version. This can cause messages to be pushed to a closed queue.

**Affected areas**:
- `useTerminalStream.ts` (rewritten) -- callback wiring layer
- `useTerminalFlowControl.ts` (new) -- all send* functions use `pushMessage`

**Mitigation**:
- Use a ref to hold the latest `pushMessage`: `pushMessageRef.current = pushMessage`.
  Flow control hook reads from ref, not from closure. This is the same pattern used
  in `XtermTerminal.tsx` (lines 95-101) for `onDataRef` and `onResizeRef`.
- Add a test that simulates reconnection: disconnect, reconnect, verify messages
  go to the new queue (not the old closed one).

**Prevention checklist**:
- [ ] `pushMessage` is stored in a ref, not captured directly in `useCallback`
- [ ] Test: reconnect after disconnect sends messages on new connection
- [ ] Verify no `useCallback` depends directly on `pushMessage` value

---

### Bug Risk 4: Write Queue Interleaving After Extraction [SEVERITY: Medium]

**Description**: `TerminalStreamManager` will own the write queue (`processWriteQueue`,
`enqueueWrite`), but `TerminalOutput` also directly calls `terminal.write` for debug
instrumentation (lines 600-617 monkey-patching). If the monkey-patched write and the
manager's write queue both write to the terminal, output may interleave or the watermark
tracker may lose sync.

**Affected areas**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts` (new)
- `web-app/src/components/sessions/TerminalOutput.tsx` (modified) -- debug instrumentation

**Mitigation**:
- Move the debug instrumentation (write/refresh monkey-patching) INTO the
  `TerminalStreamManager` class as an optional debug mode. This ensures ALL writes
  go through the manager's flow control tracking.
- Alternatively, have `TerminalStreamManager` expose the wrapped terminal write so
  debug logging can be added externally but still respects watermarks.

**Prevention checklist**:
- [ ] All `terminal.write` calls go through `TerminalStreamManager`
- [ ] Debug monkey-patching does not bypass watermark tracking
- [ ] Watermark counts match: bytes written == bytes tracked

---

### Bug Risk 5: Connection State During Size Stability Wait [SEVERITY: Low]

**Description**: `TerminalOutput` currently uses `isWaitingForStableSize` state to gate
connection initiation. After decomposition, `useTerminalConnection` owns the connection
and `TerminalOutput` owns the size stability logic. If the stability detection calls
`connect()` but the connection hook has already auto-connected (race during component
mount), there could be a double-connect attempt.

**Affected areas**:
- `web-app/src/components/sessions/TerminalOutput.tsx` (modified) -- size stability
- `web-app/src/lib/hooks/useTerminalConnection.ts` (new) -- connect guard

**Mitigation**:
- The existing guard in `connect()` (line 291: `if (isConnectedRef.current || !sessionId)
  return`) prevents double-connect. Verify this guard survives the extraction.
- Set `autoConnect: false` from `TerminalOutput` (already the case today, line 676).
  `TerminalOutput` manually calls `connect()` after size stabilization.

**Prevention checklist**:
- [ ] `autoConnect` is explicitly `false` in `TerminalOutput`'s hook call
- [ ] Connection hook's `connect()` guard prevents double-connect
- [ ] Test: calling `connect()` twice is a no-op

---

### Bug Risk 6: LZMA Decompression Error Handling During Mode Switch [SEVERITY: Low]

**Description**: When the user switches streaming mode via the toolbar dropdown (e.g.,
from "raw" to "raw-compressed"), the `streamingMode` state changes but the existing
WebSocket connection may still deliver messages in the old format. The decompression
check (`isLZMACompressed`) may attempt to decompress non-LZMA data or skip decompression
on LZMA data depending on timing.

**Affected areas**:
- `useTerminalStream.ts` message handler -- `streamingMode` check on line 533
- Currently not a regression from this refactoring, but the decomposition surfaces
  it because `streamingMode` must be passed correctly through the hook chain.

**Mitigation**:
- The existing `isLZMACompressed()` magic-byte check (lines 76-90 of `lzma.ts`) is
  format-safe: it checks for XZ header bytes regardless of mode. This provides
  defense-in-depth against mode/data mismatch.
- Ensure `streamingMode` is passed through the hook chain without delay (not derived
  from state that could be stale).

**Prevention checklist**:
- [ ] `streamingMode` is passed as option to sub-hooks, not read from parent state
- [ ] `isLZMACompressed` check remains as safety net in message handler

---

## Testing Strategy

### Unit Tests (Per Module)

| Module | Test File | Key Scenarios |
|--------|-----------|---------------|
| `MessageQueue` | `__tests__/MessageQueue.test.ts` | Push/iterate, close, sentinel filtering |
| `useTerminalConnection` | `__tests__/useTerminalConnection.test.ts` | Connect, disconnect, double-connect guard, resync guard, auto-connect |
| `useTerminalFlowControl` | `__tests__/useTerminalFlowControl.test.ts` | Send methods, resize throttle, resync throttle, SSP echo |
| `useTerminalMetrics` | `__tests__/useTerminalMetrics.test.ts` | RAF batching, 4KB threshold, recording |
| `useTerminalStream` | `__tests__/useTerminalStream.test.ts` | Message dispatch, type compatibility, SSP handshake sequence |
| `TerminalStreamManager` | `__tests__/TerminalStreamManager.test.ts` | Chunked writes, watermarks, redraw throttle, escape safety |
| `TerminalDimensionCache` | `__tests__/TerminalDimensionCache.test.ts` | Save/load, error handling |

### Integration Tests

- **SSP handshake sequence**: connect -> negotiate -> state -> diff -> mismatch -> resync
- **Reconnection**: disconnect -> auto-reconnect -> verify message delivery on new connection
- **Mode switching**: raw -> raw-compressed -> verify decompression activates

### Test Utilities Needed

```typescript
// web-app/src/lib/test-utils/raf-mock.ts
export function createRAFMock() {
  let callback: FrameRequestCallback | null = null;
  global.requestAnimationFrame = (cb) => { callback = cb; return 1; };
  global.cancelAnimationFrame = () => { callback = null; };
  return {
    flush: () => { if (callback) { callback(performance.now()); callback = null; } },
    hasPending: () => callback !== null,
  };
}

// web-app/src/lib/test-utils/mock-terminal.ts
export class MockTerminal {
  rows = 24;
  cols = 80;
  private written: string[] = [];
  write(data: string, cb?: () => void) { this.written.push(data); cb?.(); }
  clear() { this.written.push('CLEAR'); }
  refresh(start: number, end: number) { /* no-op */ }
  scrollToBottom() { /* no-op */ }
  resize(cols: number, rows: number) { this.cols = cols; this.rows = rows; }
  getWritten() { return [...this.written]; }
  get buffer() { return { active: { cursorY: 0, viewportY: 0, length: 0 }, normal: { length: 0 } }; }
}
```

### Regression Verification

After each story, run:
1. `cd web-app && npx jest` -- all existing tests pass
2. `cd web-app && npx tsc --noEmit` -- type-check passes
3. `make restart-web` -- manual smoke test: open session, verify terminal streams

---

## Dependency Graph

```
Stories 1, 2, 3 are independent and can be developed in parallel.
Story 4 depends on Stories 1, 2, 3.
Story 5 is independent of Stories 1-4.

  ┌─────────────────┐   ┌─────────────────────┐   ┌──────────────────┐
  │ Story 1          │   │ Story 2              │   │ Story 3          │
  │ useTerminal-     │   │ useTerminal-         │   │ useTerminal-     │
  │ Connection       │   │ FlowControl          │   │ Metrics          │
  │ + MessageQueue   │   │ + SSP echo           │   │ + RAF batching   │
  └────────┬─────────┘   └──────────┬───────────┘   └────────┬────────┘
           │                        │                         │
           │    ALL THREE REQUIRED  │                         │
           └────────────┬───────────┘─────────────────────────┘
                        │
                        ▼
              ┌─────────────────────┐
              │ Story 4             │
              │ Compose             │
              │ useTerminalStream   │
              │ (thin wrapper)      │
              └─────────────────────┘

  ┌──────────────────────────────────┐
  │ Story 5 (INDEPENDENT)           │
  │ TerminalStreamManager           │
  │ + TerminalDimensionCache        │
  │ + TerminalOutput.tsx rewrite    │
  └──────────────────────────────────┘
```

**Parallelism opportunities**:
- Stories 1, 2, 3 can be assigned to different developers or worked in parallel branches.
- Story 5 can start immediately and proceed in parallel with Stories 1-3.
- Story 4 is the integration point and should be done last (or after 1-3 merge).

**Suggested execution order for a single developer**:
1. Story 5 (TerminalOutput extraction) -- immediate value, independent
2. Story 1 (connection hook) -- foundational for Story 4
3. Story 3 (metrics hook) -- smallest, cleanest extraction
4. Story 2 (flow control hook) -- most complex due to SSP state machine
5. Story 4 (composition) -- final integration