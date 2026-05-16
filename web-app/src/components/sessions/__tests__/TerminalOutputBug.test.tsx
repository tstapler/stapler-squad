/**
 * Tests that demonstrate the thin-PTY sizing bug in TerminalOutput.tsx.
 *
 * Root cause chain:
 *   Bug 1 (XtermTerminal.tsx:287-290) — fires onResize(80, 24) before fitAddon.fit()
 *     → saveDimensions(sessionId, 80, 24) runs, corrupting the localStorage cache.
 *
 *   Bug 2 (TerminalOutput.tsx:394) — On the next load the cache has the stale 80×24.
 *     The session-switch effect (line 555) reads lastResizeRef (= stale 80×24) and
 *     immediately calls connect(80, 24).  Even if the terminal later reports 200×50,
 *     hasInitiatedConnectionRef is already true so connect is never retried.
 *     Additionally, handleTerminalResize uses `const initDims = lastResize ?? {cols, rows}`
 *     where `lastResize` is the OLD value of the ref — so when onResize(200,50) fires,
 *     initDims is still {80, 24}.
 *
 * The tests assert CORRECT behaviour; they therefore FAIL today and will PASS once
 * the bugs are fixed.
 */

import React from 'react';
import { render, act } from '@testing-library/react';

// ---------------------------------------------------------------------------
// Mocks — must be registered before importing TerminalOutput
// ---------------------------------------------------------------------------

/** Shared handle injected via ref by the XtermTerminal mock */
const mockXtermHandle = {
  terminal: null as null,
  fit: jest.fn(),
  write: jest.fn(),
  writeln: jest.fn(),
  clear: jest.fn(),
  focus: jest.fn(),
  search: jest.fn().mockReturnValue(false),
  searchNext: jest.fn().mockReturnValue(false),
  searchPrevious: jest.fn().mockReturnValue(false),
};

/** Latest `onResize` prop received by the mock XtermTerminal */
let capturedOnResize: ((cols: number, rows: number) => void) | null = null;

/**
 * Mock XtermTerminal — a simple div that:
 *  - forwards the ref with a stable handle stub
 *  - captures the onResize prop so tests can trigger it at will
 */
jest.mock('../XtermTerminal', () => {
  const React = require('react');
  const XtermTerminal = React.forwardRef((props: any, ref: any) => {
    capturedOnResize = props.onResize ?? null;
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return React.createElement('div', { 'data-testid': 'mock-xterm' });
  });
  XtermTerminal.displayName = 'XtermTerminal';
  return { XtermTerminal };
});

jest.mock('@/lib/hooks/useTerminalStream', () => ({
  useTerminalStream: jest.fn(),
}));

jest.mock('@/lib/terminal/TerminalDimensionCache', () => ({
  getCachedDimensions: jest.fn(),
  saveDimensions: jest.fn(),
  validateCellDimensions: jest.fn((cached: unknown) => cached),
}));

jest.mock('@/lib/terminal/TerminalStreamManager', () => ({
  TerminalStreamManager: jest.fn().mockImplementation(() => ({
    setOnFirstOutput: jest.fn(),
    installDebugMonitor: jest.fn(),
    writeInitialContent: jest.fn().mockResolvedValue(undefined),
    write: jest.fn(),
    cleanup: jest.fn(),
    updateSendFlowControl: jest.fn(),
    prependScrollbackBatch: jest.fn().mockResolvedValue(undefined),
  })),
}));

const mockTrack = jest.fn();
jest.mock('@/lib/contexts/AnalyticsContext', () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));

// ---------------------------------------------------------------------------
// Imports (after jest.mock calls)
// ---------------------------------------------------------------------------
// eslint-disable-next-line import/first
import { TerminalOutput } from '../TerminalOutput';
// eslint-disable-next-line import/first
import { useTerminalStream } from '@/lib/hooks/useTerminalStream';
// eslint-disable-next-line import/first
import { getCachedDimensions, saveDimensions } from '@/lib/terminal/TerminalDimensionCache';
// eslint-disable-next-line import/first
import { TerminalStreamManager } from '@/lib/terminal/TerminalStreamManager';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type StreamMock = {
  isConnected: boolean;
  error: Error | null;
  connect: jest.Mock;
  disconnect: jest.Mock;
  sendInput: jest.Mock;
  sendInputWithEcho: jest.Mock;
  resize: jest.Mock;
  scrollbackLoaded: boolean;
  requestScrollback: jest.Mock;
  sendFlowControl: jest.Mock;
  getIsApplyingState: jest.Mock;
  sspNegotiated: boolean;
  startRecording: jest.Mock;
  stopRecording: jest.Mock;
  output: string;
  terminalState: string;
};

function makeStreamMock(overrides: Partial<StreamMock> = {}): StreamMock {
  const mockConnect = jest.fn();
  return {
    isConnected: false,
    error: null as Error | null,
    connect: mockConnect,
    disconnect: jest.fn(),
    sendInput: jest.fn(),
    sendInputWithEcho: jest.fn().mockReturnValue(BigInt(0)),
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    getIsApplyingState: jest.fn().mockReturnValue(false),
    sspNegotiated: false,
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    output: '',
    terminalState: 'STABLE',
    ...overrides,
  };
}

function renderTerminalOutput(sessionId = 'session-1') {
  return render(
    <TerminalOutput sessionId={sessionId} baseUrl="http://localhost:8543" />
  );
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();
  capturedOnResize = null;

  // Default: useTerminalStream returns a stable mock with a connect spy
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());

  // Default: no cached dimensions (first-ever load)
  (getCachedDimensions as jest.Mock).mockReturnValue(null);

  jest.spyOn(console, 'log').mockImplementation(() => {});
  jest.spyOn(console, 'warn').mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Bug 2a — stale cache causes wrong connect() call via session-switch effect
// ---------------------------------------------------------------------------
describe('Bug 2a: stale cache causes connect() with wrong dims on mount', () => {
  it('FAILS today — connect should NOT be called with stale 80×24 dims', () => {
    // Simulate: cache was corrupted by Bug 1 on a previous session
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 80, rows: 24 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // Bug 2a: the session-switch useEffect reads lastResizeRef (= {80,24} from cache)
    // and calls connect(80, 24) immediately — before the actual terminal renders
    // and reports its real dimensions.
    //
    // Expected correct behaviour: connect should NOT be called with stale xterm-default
    // dims that were never verified against the actual container size.
    expect(stream.connect).not.toHaveBeenCalledWith(80, 24); // ← FAILS today
  });

  it('FAILS today — connect should eventually be called with actual terminal dims (200×50)', async () => {
    // Stale cache from Bug 1 corruption
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 80, rows: 24 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // Simulate fitAddon.fit() firing and XtermTerminal reporting the real size
    await act(async () => {
      capturedOnResize?.(200, 50);
    });

    // Expected: connect was called with (200, 50) — the real terminal size.
    // Bug 2a: connect was already called with (80, 24) from the stale cache
    // before onResize(200, 50) even fired, so hasInitiatedConnectionRef is true
    // and connect(200, 50) is never called.
    expect(stream.connect).toHaveBeenCalledWith(200, 50); // ← FAILS today
    expect(stream.connect).not.toHaveBeenCalledWith(80, 24);
  });
});

// ---------------------------------------------------------------------------
// Bug 2b fix — fast-connect uses current event dims, not stale lastResizeRef
//
// Old code: `const initDims = lastResize ?? { cols, rows }` — where lastResize
// was pre-seeded from the cache in the init effect.  If the cache held (220,55)
// and the actual container was (200,50), connect fired at (220,55).
//
// Fix (de78256f): init effect no longer seeds lastResizeRef; initDims changed
// to `{ cols, rows }` — always the current resize event's dimensions.
// ---------------------------------------------------------------------------
describe('Bug 2b fix: fast-connect uses current event dims, not stale lastResizeRef', () => {
  it('connect uses current resize event dims when they differ from the cached value', async () => {
    // Cache was saved when the window was 220×55.  The window has since been resized.
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 220, rows: 55 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // First resize fires with the real current container size (200×50).
    // Old bug (initDims = lastResizeRef): would connect(220,55) from the stale cache.
    // Fix (initDims = {cols,rows}):      must connect(200,50).
    await act(async () => { capturedOnResize?.(200, 50); });

    expect(stream.connect).toHaveBeenCalledWith(200, 50);
    expect(stream.connect).not.toHaveBeenCalledWith(220, 55);
  });
});

// ---------------------------------------------------------------------------
// Correct behaviour reference — what should happen with a VALID cache
// ---------------------------------------------------------------------------
describe('Baseline: valid cache should fast-connect with correct dims', () => {
  it('connect is called with valid cached dims (220×55) on first resize', async () => {
    // This test should PASS today and after the fix — it verifies that the
    // fast-connect optimisation still works when the cache is valid.
    // Fast-connect happens on first resize (not synchronously on mount) so that
    // the actual terminal dimensions are used rather than stale cached defaults.
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 220, rows: 55 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // No connect before any resize fires (cache no longer triggers immediate connect)
    expect(stream.connect).not.toHaveBeenCalled();

    // First resize fires with the actual container dims (matching cache)
    await act(async () => { capturedOnResize?.(220, 55); });

    // Fast-connect uses the actual resize dims (which happen to match the cache)
    expect(stream.connect).toHaveBeenCalledWith(220, 55);
  });

  it('with no cache, connect waits for the stability timer (50ms)', async () => {
    // No cache → no immediate connect
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    jest.useFakeTimers();
    renderTerminalOutput();

    // No connect before any resize fires
    expect(stream.connect).not.toHaveBeenCalled();

    // Resize fires and stability timer starts
    await act(async () => { capturedOnResize?.(200, 50); });
    expect(stream.connect).not.toHaveBeenCalled(); // still waiting

    // Timer expires → connect with actual dims
    await act(async () => { jest.advanceTimersByTime(100); });
    expect(stream.connect).toHaveBeenCalledWith(200, 50);

    jest.useRealTimers();
  });
});

// ---------------------------------------------------------------------------
// MIN_COLS / MIN_ROWS guards — prevent cache poisoning from transient tiny dims
//
// xterm.js may fire onResize(10, 6) or similar before CSS layout is complete.
// If persisted to localStorage, the next session load would connect at those
// dimensions, re-render the TUI at tiny size, and produce garbled output.
// Guards: saveDimensions is skipped and hasCachedDimensionsRef is reset when
// cols < 30 (MIN_COLS) or rows < 10 (MIN_ROWS).
// ---------------------------------------------------------------------------
describe('MIN_COLS/MIN_ROWS: tiny dims do not corrupt the cache or trigger fast-connect', () => {
  it('resize below MIN_COLS/MIN_ROWS does not call saveDimensions', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    renderTerminalOutput();

    // Pre-layout transient resize (common on hidden tabs and early xterm mounts)
    await act(async () => { capturedOnResize?.(10, 6); });

    expect(saveDimensions).not.toHaveBeenCalled();
  });

  it('valid cache + tiny first resize: resets fast-connect path and does not connect', async () => {
    // Cache is valid but the first resize event is at tiny pre-layout dims.
    // The guard must reset hasCachedDimensionsRef so we fall through to the
    // stability wait rather than fast-connecting at 10×6.
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 220, rows: 55 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    renderTerminalOutput();

    await act(async () => { capturedOnResize?.(10, 6); });

    expect(stream.connect).not.toHaveBeenCalled();
  });

  it('resize at exactly MIN boundary (30×10) is accepted: saves and fast-connects', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 30, rows: 10 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    renderTerminalOutput();

    await act(async () => { capturedOnResize?.(30, 10); });

    expect(saveDimensions).toHaveBeenCalledWith(expect.any(String), 30, 10, undefined, undefined, expect.anything(), expect.anything());
    expect(stream.connect).toHaveBeenCalledWith(30, 10);
  });

  it('resize one below MIN boundary (29×9) is rejected: no save, no connect', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    renderTerminalOutput();

    await act(async () => { capturedOnResize?.(29, 9); });

    expect(saveDimensions).not.toHaveBeenCalled();
    expect(stream.connect).not.toHaveBeenCalled();
  });

  it('sub-minimum cache entry at load time is ignored (hasCached stays false)', async () => {
    // Even if a stale tiny entry somehow reached localStorage, the init effect
    // must discard it so hasCachedDimensionsRef is never set to true for bad dims.
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 20, rows: 8 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    renderTerminalOutput();

    // First resize at real dims: no cached fast-connect because cache was rejected
    jest.useFakeTimers();
    await act(async () => { capturedOnResize?.(200, 50); });
    // Still in stability wait (no valid cache to skip it)
    expect(stream.connect).not.toHaveBeenCalled();

    await act(async () => { jest.advanceTimersByTime(100); });
    expect(stream.connect).toHaveBeenCalledWith(200, 50);
    jest.useRealTimers();
  });
});

// ---------------------------------------------------------------------------
// Pre-sizing: immediate connect using cached cell pixel dimensions
//
// When the dimension cache includes cellWidth/cellHeight (pixels per col/row),
// TerminalOutput can pre-calculate cols×rows from the container's pixel size
// in the init effect — before xterm.js fires its first onResize event.
// The session-switch effect then finds lastResizeRef already populated and
// calls connect() immediately, eliminating the 50ms stability wait entirely.
// ---------------------------------------------------------------------------
describe('Pre-sizing: immediate connect using cached cell pixel metrics', () => {
  // Container size that yields exactly 200×50 with cell dims 8.4×17.0:
  //   floor(1680 / 8.4)  = floor(200)   = 200 cols
  //   floor(850  / 17.0) = floor(50)    = 50  rows
  const CELL_WIDTH = 8.4;
  const CELL_HEIGHT = 17.0;
  const CONTAINER_W = 1680;
  const CONTAINER_H = 850;

  function mockRect(width: number, height: number) {
    jest.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue({
      width, height, top: 0, left: 0, bottom: height, right: width, x: 0, y: 0,
      toJSON: () => ({}),
    } as DOMRect);
  }

  it('connects immediately before any onResize when cell metrics are cached', () => {
    mockRect(CONTAINER_W, CONTAINER_H);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellWidth: CELL_WIDTH, cellHeight: CELL_HEIGHT,
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // Pre-sized: floor(1680/8.4)=200, floor(850/17.0)=50 → connect fires on mount
    expect(stream.connect).toHaveBeenCalledWith(200, 50);
    expect(stream.connect).toHaveBeenCalledTimes(1);
  });

  it('uses floor division so cols/rows never exceed what fits in the container', () => {
    // 1682 / 8.4 = 200.24 → floor = 200 (not 201)
    mockRect(1682, 851);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellWidth: CELL_WIDTH, cellHeight: CELL_HEIGHT,
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    expect(stream.connect).toHaveBeenCalledWith(200, 50);
  });

  it('does not connect when container has zero size (hidden / not yet laid out)', () => {
    mockRect(0, 0);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellWidth: CELL_WIDTH, cellHeight: CELL_HEIGHT,
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // Container is zero-size → pre-sizing skipped → no immediate connect
    expect(stream.connect).not.toHaveBeenCalled();
  });

  it('falls back to fast-connect path when cache has no cell metrics', async () => {
    mockRect(CONTAINER_W, CONTAINER_H);
    // Cache without cell dimensions → existing fast-connect behaviour
    (getCachedDimensions as jest.Mock).mockReturnValue({ cols: 200, rows: 50 });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // No immediate connect (no cell metrics → no pre-sizing)
    expect(stream.connect).not.toHaveBeenCalled();

    // Fast-connect fires on first resize event (hasCachedDimensionsRef=true path)
    await act(async () => { capturedOnResize?.(200, 50); });
    expect(stream.connect).toHaveBeenCalledWith(200, 50);
  });

  it('skips pre-sizing when calculated dims are below MIN_COLS/MIN_ROWS threshold', () => {
    // Tiny container: floor(100/8.4) = 11 < MIN_COLS=30
    mockRect(100, 50);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellWidth: CELL_WIDTH, cellHeight: CELL_HEIGHT,
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    // Pre-calculated dims are sub-minimum → no pre-sizing → no immediate connect
    expect(stream.connect).not.toHaveBeenCalled();
  });

  it('skips pre-sizing when cache has cellWidth but no cellHeight', () => {
    mockRect(CONTAINER_W, CONTAINER_H);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellWidth: CELL_WIDTH,
      // cellHeight absent — partial cache entry; both required
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    expect(stream.connect).not.toHaveBeenCalled();
  });

  it('skips pre-sizing when cache has cellHeight but no cellWidth', () => {
    mockRect(CONTAINER_W, CONTAINER_H);
    (getCachedDimensions as jest.Mock).mockReturnValue({
      cols: 200, rows: 50, cellHeight: CELL_HEIGHT,
      // cellWidth absent — partial cache entry; both required
    });
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    renderTerminalOutput();

    expect(stream.connect).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Output queuing: pending output flushed on RESIZING → STABLE transition
//
// Task 4.2.2 — handleOutput pushes chunks to pendingOutputDuringResizeRef when
// terminalStateRef.current === 'RESIZING'. The useEffect that syncs terminalState
// into terminalStateRef also flushes the queue when the previous state was RESIZING
// and the new state is STABLE.
// ---------------------------------------------------------------------------
describe('Output queuing: pending output flushed on RESIZING → STABLE', () => {
  beforeEach(() => {
    // TerminalStreamManager is lazily created only when xtermRef.current?.terminal is non-null.
    // Provide a minimal stub so getOrCreateStreamManager succeeds.
    (mockXtermHandle as any).terminal = {};
  });

  afterEach(() => {
    // Restore so other describe blocks are unaffected.
    (mockXtermHandle as any).terminal = null;
  });

  it('output during RESIZING is queued, not written; flushed on transition to STABLE', async () => {
    // Capture the onOutput callback that TerminalOutput passes into useTerminalStream,
    // and control terminalState from the mock's return value.
    let capturedOnOutput: ((output: string) => void) | undefined;

    // Use shared mock functions so that the disconnect reference is stable across rerenders.
    // If disconnect changes identity, the useEffect([disconnect]) cleanup fires and nullifies
    // streamManagerRef.current — preventing the RESIZING→STABLE flush from reaching write().
    const sharedMockFns = makeStreamMock({ isConnected: true });
    let currentTerminalState = 'STABLE';

    (useTerminalStream as jest.Mock).mockImplementation((opts: { onOutput?: (output: string) => void }) => {
      capturedOnOutput = opts.onOutput;
      return { ...sharedMockFns, terminalState: currentTerminalState };
    });

    const { rerender } = render(
      <TerminalOutput sessionId="session-queue" baseUrl="http://localhost:8543" />
    );

    // Trigger a resize so XtermTerminal's ref is populated
    await act(async () => {
      capturedOnResize?.(200, 50);
    });

    // Sanity: onOutput should have been captured by now
    expect(capturedOnOutput).toBeDefined();

    // Call onOutput while STABLE → write should be called immediately
    await act(async () => {
      capturedOnOutput!('chunk-stable');
    });

    // Grab the TerminalStreamManager instance that was created.
    // mock.results gives the return value of the constructor (the plain object), which is
    // what the component holds in streamManagerRef. mock.instances gives `this`, which is
    // different when mockImplementation returns a plain object literal.
    const MockTSM = TerminalStreamManager as unknown as jest.MockedClass<new (...args: unknown[]) => TerminalStreamManager>;
    const managerResult = MockTSM.mock.results[MockTSM.mock.results.length - 1];
    const managerInstance = managerResult?.value as TerminalStreamManager;
    expect(managerInstance.write).toHaveBeenCalledWith('chunk-stable');

    const writeCallsBefore = (managerInstance.write as jest.Mock).mock.calls.length;

    // Re-render with terminalState = 'RESIZING' — subsequent onOutput calls must be queued.
    // Sharing sharedMockFns keeps disconnect stable so the cleanup effect does NOT fire.
    currentTerminalState = 'RESIZING';

    await act(async () => {
      rerender(<TerminalOutput sessionId="session-queue" baseUrl="http://localhost:8543" />);
    });

    // Output during RESIZING must NOT reach write()
    await act(async () => {
      capturedOnOutput!('chunk-during-resize');
    });

    expect((managerInstance.write as jest.Mock).mock.calls.length).toBe(writeCallsBefore);

    // Re-render with terminalState = 'STABLE' — the useEffect fires with
    // prevState='RESIZING', terminalState='STABLE' and flushes the pending queue.
    currentTerminalState = 'STABLE';

    await act(async () => {
      rerender(<TerminalOutput sessionId="session-queue" baseUrl="http://localhost:8543" />);
    });

    // The queued chunk should have been flushed by the RESIZING→STABLE useEffect
    expect(managerInstance.write).toHaveBeenCalledWith('chunk-during-resize');
  });
});

// ---------------------------------------------------------------------------
// Scrollback paging: isFetchingScrollbackRef reset on prependScrollbackBatch error
//
// The finally block in handleScrollbackReceived always sets isFetchingScrollbackRef
// to false even when prependScrollbackBatch throws. Without this reset, the second
// call to onScrollbackReceived would silently skip writing and requestScrollback
// could never be triggered again.
// ---------------------------------------------------------------------------
describe('Scrollback paging: isFetchingScrollbackRef reset on prependScrollbackBatch error', () => {
  beforeEach(() => {
    // TerminalStreamManager is lazily created only when xtermRef.current?.terminal is non-null.
    (mockXtermHandle as any).terminal = { scrollToBottom: jest.fn() };
  });

  afterEach(() => {
    (mockXtermHandle as any).terminal = null;
  });

  it('isFetchingScrollbackRef is reset to false even when prependScrollbackBatch throws', async () => {

    let capturedOnScrollbackReceived: ((scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => void) | undefined;

    const stream = makeStreamMock({ isConnected: true, terminalState: 'STABLE' });
    (useTerminalStream as jest.Mock).mockImplementation((opts: {
      onScrollbackReceived?: (scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => void;
    }) => {
      capturedOnScrollbackReceived = opts.onScrollbackReceived;
      return stream;
    });

    render(<TerminalOutput sessionId="session-scrollback-err" baseUrl="http://localhost:8543" />);

    expect(capturedOnScrollbackReceived).toBeDefined();

    jest.spyOn(console, 'error').mockImplementation(() => {});

    // First call: initial scrollback — creates the TerminalStreamManager lazily and
    // writes via writeInitialContent, sets isInitialScrollbackDoneRef=true.
    await act(async () => {
      await capturedOnScrollbackReceived!('initial-content', {
        hasMore: true,
        oldestSequence: 50,
        newestSequence: 100,
        totalLines: 50,
      });
    });

    // Now the manager has been created — grab it via mock.results.
    const MockTSM = TerminalStreamManager as unknown as jest.MockedClass<new (...args: unknown[]) => TerminalStreamManager>;
    const managerResult = MockTSM.mock.results[MockTSM.mock.results.length - 1];
    const managerInstance = managerResult?.value as TerminalStreamManager;

    expect(managerInstance).toBeDefined();

    // Make prependScrollbackBatch reject to simulate an error
    (managerInstance.prependScrollbackBatch as jest.Mock).mockRejectedValue(new Error('prepend failed'));

    // Second call: paged history — triggers prependScrollbackBatch which will throw
    await act(async () => {
      await capturedOnScrollbackReceived!('paged-content', {
        hasMore: false,
        oldestSequence: 1,
        newestSequence: 49,
        totalLines: 49,
      });
    });

    // prependScrollbackBatch should have been called and thrown
    expect(managerInstance.prependScrollbackBatch).toHaveBeenCalledWith('paged-content');
    expect(console.error).toHaveBeenCalledWith(
      '[TerminalOutput] prependScrollbackBatch failed:',
      expect.any(Error),
    );

    // After the error, isFetchingScrollbackRef must be reset (false).
    // We verify this indirectly: a subsequent paged scrollback call still reaches
    // prependScrollbackBatch. If isFetchingScrollbackRef were stuck at true, the
    // call would be short-circuited and prependScrollbackBatch would NOT be called again.
    (managerInstance.prependScrollbackBatch as jest.Mock).mockResolvedValue(undefined);

    await act(async () => {
      await capturedOnScrollbackReceived!('paged-content-2', {
        hasMore: false,
        oldestSequence: 1,
        newestSequence: 49,
        totalLines: 49,
      });
    });

    // The finally block reset ensures prependScrollbackBatch is reachable after the error.
    expect(managerInstance.prependScrollbackBatch).toHaveBeenCalledTimes(2);
  });
});

// ---------------------------------------------------------------------------
// Cell dim extraction: saving cellWidth/cellHeight from xterm's private API
//
// When xterm provides _core._renderService.dimensions.css.cell, TerminalOutput
// should save those pixel metrics alongside cols/rows so future mounts can
// pre-size without waiting for xterm to fire onResize.
// ---------------------------------------------------------------------------
describe('Cell dim extraction: saves pixel metrics from xterm private API', () => {
  it('saves cell pixel dimensions to cache when terminal provides them', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    // Give the mock handle a terminal with cell dimensions available
    (mockXtermHandle as any).terminal = {
      scrollToBottom: jest.fn(),
      _core: {
        _renderService: {
          dimensions: {
            css: { cell: { width: 8.4, height: 17.0 } },
          },
        },
      },
    };

    renderTerminalOutput();
    await act(async () => { capturedOnResize?.(200, 50); });

    expect(saveDimensions).toHaveBeenCalledWith(
      expect.any(String), 200, 50, 8.4, 17.0, expect.anything(), expect.anything(),
    );

    // Restore
    (mockXtermHandle as any).terminal = null;
  });

  it('saves only cols/rows when terminal cell API is unavailable', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);
    // terminal remains null → no private API available

    renderTerminalOutput();
    await act(async () => { capturedOnResize?.(200, 50); });

    // Called without cellWidth/cellHeight (undefined), but with fontSize/fontFamily
    expect(saveDimensions).toHaveBeenCalledWith(expect.any(String), 200, 50, undefined, undefined, expect.anything(), expect.anything());
    expect(saveDimensions).not.toHaveBeenCalledWith(
      expect.any(String), 200, 50, expect.any(Number), expect.any(Number),
    );
  });

  it('saves only cols/rows when cell dims are non-finite', async () => {
    (getCachedDimensions as jest.Mock).mockReturnValue(null);
    const stream = makeStreamMock();
    (useTerminalStream as jest.Mock).mockReturnValue(stream);

    (mockXtermHandle as any).terminal = {
      scrollToBottom: jest.fn(),
      _core: {
        _renderService: {
          dimensions: {
            css: { cell: { width: NaN, height: 17.0 } },
          },
        },
      },
    };

    renderTerminalOutput();
    await act(async () => { capturedOnResize?.(200, 50); });

    // Called without cellWidth/cellHeight (undefined), but with fontSize/fontFamily
    expect(saveDimensions).toHaveBeenCalledWith(expect.any(String), 200, 50, undefined, undefined, expect.anything(), expect.anything());
    expect(saveDimensions).not.toHaveBeenCalledWith(
      expect.any(String), 200, 50, expect.any(Number), expect.any(Number),
    );

    (mockXtermHandle as any).terminal = null;
  });
});
