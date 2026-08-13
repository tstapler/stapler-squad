/**
 * Tests that demonstrate the PTY-sizing bugs in XtermTerminal.tsx.
 *
 * Bug 1 (lines 287-290): XtermTerminal calls onResize(terminal.cols, terminal.rows)
 *   synchronously at mount — before fitAddon.fit() runs in the double rAF.
 *   terminal.cols/rows are xterm's defaults (80×24) at that point.
 *   TerminalOutput then calls saveDimensions(sessionId, 80, 24), corrupting the cache.
 *
 * Bug 3 (lines 310-338): The ResizeObserver callback has no zero-size guard.
 *   When a terminal tab is closed or moved to the background the container
 *   shrinks to 0px. The observer fires, a debounced fit() runs on the
 *   zero-size container, and onResize(80, 24) is persisted again.
 *
 * Each test uses "@testing-library/react" and the real XtermTerminal component,
 * with only xterm.js and its addons mocked.  The tests assert the CORRECT behaviour;
 * they therefore FAIL today and will PASS once the bugs are fixed.
 */

import React from 'react';
import { render, act } from '@testing-library/react';

// ---------------------------------------------------------------------------
// Mock xterm.js — keep all state inside the factory so Jest can hoist the call
// ---------------------------------------------------------------------------

// Shared state accessible from tests via jest.requireMock()
interface XtermTestHarness {
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  triggerFit(cols?: number, rows?: number): void;
  reset(): void;
}

jest.mock('@xterm/xterm', () => {
  const harness: XtermTestHarness = {
    fitCalledCount: 0,
    onResizeCb: null,
    triggerFit(cols = 200, rows = 50) {
      if (this.onResizeCb) this.onResizeCb({ cols, rows });
    },
    reset() {
      this.fitCalledCount = 0;
      this.onResizeCb = null;
    },
  };

  class MockTerminal {
    cols = 80;
    rows = 24;
    options: Record<string, unknown> = {};

    constructor(opts?: Record<string, unknown>) {
      if (opts) Object.assign(this.options, opts);
    }
    buffer = {
      active: { length: 0, cursorY: 0, viewportY: 0 },
      normal: { length: 0 },
    };

    onResize(cb: (p: { cols: number; rows: number }) => void) {
      harness.onResizeCb = cb;
      return { dispose: jest.fn() };
    }
    onData() { return { dispose: jest.fn() }; }
    onSelectionChange() { return { dispose: jest.fn() }; }
    loadAddon() {}
    open() {}
    dispose() {}
    getSelection() { return ''; }
    refresh() {}
    scrollToBottom() {}
    focus() {}
  }

  (MockTerminal as any).__harness = harness;
  return { Terminal: MockTerminal };
});

jest.mock('@xterm/addon-fit', () => {
  const Terminal = require('@xterm/xterm').Terminal;
  const harness: XtermTestHarness = (Terminal as any).__harness;

  return {
    FitAddon: class MockFitAddon {
      fit() {
        harness.fitCalledCount++;
        harness.triggerFit();
      }
      proposeDimensions() { return { cols: 200, rows: 50 }; }
      dispose() {}
    },
  };
});

jest.mock('@xterm/addon-search', () => ({
  SearchAddon: class { findNext() { return false; } findPrevious() { return false; } dispose() {} },
}));
jest.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: class { dispose() {} },
}));
jest.mock('@xterm/addon-webgl', () => ({
  WebglAddon: class {
    onContextLoss() {}
    dispose() {}
  },
}));

jest.mock('@/lib/hooks/useMobileTerminalGestures', () => ({
  useMobileTerminalGestures: () => {},
}));
jest.mock('@/lib/hooks/useTouchScroll', () => ({
  useTouchScroll: () => {},
}));
jest.mock('@/lib/config/terminalConfig', () => ({
  loadTerminalConfig: () => null,
  darkTerminalTheme: {},
  lightTerminalTheme: {},
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getHarness(): XtermTestHarness {
  const { Terminal } = jest.requireMock<any>('@xterm/xterm');
  return Terminal.__harness as XtermTestHarness;
}

function captureRaf(): { flush: () => void } {
  const callbacks: FrameRequestCallback[] = [];
  const spy = jest.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
    callbacks.push(cb);
    return callbacks.length;
  });

  return {
    flush() {
      // One-level flush (first rAF)
      const batch = callbacks.splice(0);
      batch.forEach((cb) => cb(0));
      // Second-level flush (nested rAF)
      const batch2 = callbacks.splice(0);
      batch2.forEach((cb) => cb(0));
      spy.mockRestore();
    },
  };
}

// ---------------------------------------------------------------------------
// Imports (after mocks are hoisted)
// ---------------------------------------------------------------------------
// eslint-disable-next-line import/first
import { XtermTerminal } from '../XtermTerminal';

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

beforeEach(() => {
  getHarness().reset();
  jest.spyOn(console, 'log').mockImplementation(() => {});
  jest.spyOn(console, 'warn').mockImplementation(() => {});
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: jest.fn().mockResolvedValue(undefined) },
    configurable: true,
    writable: true,
  });
});

afterEach(() => {
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Bug 1
// ---------------------------------------------------------------------------
describe('Bug 1: onResize fired with xterm defaults before fitAddon.fit()', () => {
  it('FAILS today — onResize(80, 24) should NOT fire before fitAddon.fit() runs', () => {
    const { flush } = captureRaf();
    const onResize = jest.fn();

    render(<XtermTerminal onResize={onResize} />);

    // Immediately after mount (effects ran, but rAF has not):
    // Bug 1 makes onResize(80, 24) fire synchronously.
    // Correct behaviour: onResize should NOT have been called yet —
    // it should wait until fitAddon.fit() measures the real container size.
    expect(onResize).not.toHaveBeenCalledWith(80, 24); // ← FAILS today

    // After flushing the double rAF, fit() runs and fires the real dims.
    flush();
    expect(onResize).toHaveBeenCalledWith(200, 50);
  });

  it('FAILS today — fitAddon.fit() should be called before first onResize fires', () => {
    const { flush } = captureRaf();
    const harness = getHarness();
    const callOrder: string[] = [];

    const onResize = jest.fn(() => {
      callOrder.push(`onResize(fit=${harness.fitCalledCount})`);
    });

    render(<XtermTerminal onResize={onResize} />);

    // After render but before rAF flushes:
    // Bug 1 has called onResize(80, 24) — BEFORE fit() ran (fitCalledCount=0).
    // Correct behaviour: first onResize call should happen AFTER fit() ran.
    const firstCallBeforeFit = callOrder.find((e) => e.includes('fit=0'));
    expect(firstCallBeforeFit).toBeUndefined(); // ← FAILS today

    flush();
    // After flush, the fit-triggered resize should have happened
    const callAfterFit = callOrder.find((e) => e.includes('fit=1'));
    expect(callAfterFit).toBeDefined();
  });

  it('FAILS today — saveDimensions should NOT be called with 80×24 default dims', () => {
    const { flush } = captureRaf();
    // saveDimensions is called by TerminalOutput, not XtermTerminal directly.
    // This test captures the full round-trip via onResize to show that an upstream
    // parent using handleTerminalResize WOULD persist 80×24 because of Bug 1.
    const onResize = jest.fn();

    render(<XtermTerminal onResize={onResize} />);

    // Before fit() runs, onResize was called with xterm defaults.
    // A parent that calls saveDimensions on every resize will corrupt the cache here.
    const prematureCalls = onResize.mock.calls.filter(
      ([cols, rows]) => cols === 80 && rows === 24
    );
    expect(prematureCalls).toHaveLength(0); // ← FAILS today (there is 1 premature call)

    flush();
  });
});

// ---------------------------------------------------------------------------
// Bug 3
// ---------------------------------------------------------------------------
describe('Bug 3: ResizeObserver calls fitAddon.fit() on zero-size container', () => {
  let observerCallback: ResizeObserverCallback | null = null;

  beforeEach(() => {
    observerCallback = null;

    // Override the global ResizeObserver stub with a controllable one
    Object.defineProperty(global, 'ResizeObserver', {
      writable: true,
      configurable: true,
      value: class MockResizeObserver {
        constructor(cb: ResizeObserverCallback) {
          observerCallback = cb;
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    });
  });

  function fireResizeObserver(width: number, height: number) {
    if (!observerCallback) return;
    const entry = {
      contentRect: { width, height, top: 0, left: 0, bottom: height, right: width },
      borderBoxSize: [],
      contentBoxSize: [],
      devicePixelContentBoxSize: [],
      target: document.createElement('div'),
    } as unknown as ResizeObserverEntry;
    act(() => {
      observerCallback!([entry], {} as ResizeObserver);
    });
  }

  it('fit() should NOT run when container collapses to zero size', () => {
    jest.useFakeTimers();
    const harness = getHarness();
    const { flush } = captureRaf();

    render(<XtermTerminal onResize={jest.fn()} />);
    flush(); // initial fit via double-rAF

    // XtermTerminal schedules a secondary setTimeout(fit, 100) inside the double-rAF
    // to handle edge cases where the initial fit runs before cell metrics are ready.
    // Drain that timer (and any rAF it schedules) before capturing the baseline count,
    // so we only measure fit() calls caused by the zero-size collapse.
    act(() => { jest.runAllTimers(); });

    const fitCountAfterInit = harness.fitCalledCount;

    // Simulate container collapsing to zero (tab goes to background / is hidden)
    fireResizeObserver(0, 0);

    // Advance past the debounce delay — the guard at XtermTerminal.tsx line 304
    // (`width > 0 && height > 0`) must prevent the debounced fit() from being scheduled.
    act(() => { jest.advanceTimersByTime(20); });

    expect(harness.fitCalledCount).toBe(fitCountAfterInit);

    jest.useRealTimers();
  });

  it('FAILS today — onResize should NOT fire with 80×24 defaults after zero-size collapse', () => {
    jest.useFakeTimers();
    const onResize = jest.fn();
    const { flush } = captureRaf();

    render(<XtermTerminal onResize={onResize} />);
    flush();
    onResize.mockClear(); // ignore initial resize events

    // Tab goes to background → container size becomes 0×0
    fireResizeObserver(0, 0);
    act(() => { jest.advanceTimersByTime(20); });
    flush();

    // Bug 3: fit() on a zero-size container triggers onResize(80, 24) again,
    // which TerminalOutput will then persist to localStorage via saveDimensions.
    // Correct behaviour: no onResize should fire for a zero-size container.
    expect(onResize).not.toHaveBeenCalled(); // ← FAILS today

    jest.useRealTimers();
  });
});
