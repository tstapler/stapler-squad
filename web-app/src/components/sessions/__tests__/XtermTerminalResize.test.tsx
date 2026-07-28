/**
 * Component-level tests for XtermTerminal.tsx's resize-convergence behavior.
 *
 * Covers (see project_plans/terminal-resize-fit-loop/implementation/plan.md, Phase 4,
 * Epic 4.2 + Epic 4.3):
 *   - AC1/AC7: the imperative `fit()` handle (used by TerminalOutput.tsx's tab-visibility /
 *     visualViewport.resize handlers) is gated by the same `shouldFit` predicate as the
 *     ResizeObserver path.
 *   - AC4: a resize oscillation burst (>=3 recurrences of the same cols/rows within a rolling
 *     2000ms window) trips a one-shot fallback off the WebGL renderer, with a console-only
 *     backstop when WebGL never loaded and no re-logging on a later post-fallback burst.
 *   - AC5: a sub-cell/sub-pixel container wobble that never changes FitAddon's proposed
 *     integer grid is a no-op (no extra fit() calls, no extra onResize firings).
 *   - AC6: a genuine cols/rows-crossing resize still converges in exactly one fit()/onResize
 *     cycle (no regression from the AC2/AC5 gating).
 *   - The `webglAddonRef` `cancelled`-guard survives a React StrictMode double-mount.
 *
 * Harness shape follows XtermTerminalBug.test.tsx's mock-xterm.js convention, extended per
 * plan.md Task 4.2.1a: MockTerminal gains a real `resize(cols, rows)` that mutates state and
 * only fires the registered onResize callback on a genuine change (mirroring xterm.js's own
 * dedup), harness.proposedDimensions is settable per-test, and MockFitAddon.fit() drives
 * MockTerminal.resize() using it. A separate observable WebglAddon mock (tagged instances,
 * jest.fn() dispose) supports the StrictMode and AC4 assertions.
 *
 * `terminal.element` in this harness is a single shared module-level DOM node reused by every
 * MockTerminal instance (mirrors XtermTerminalSelection.test.tsx's harness) — required because
 * the component reads `terminal.element!` to attach its contextmenu listener; unrelated to the
 * resize logic under test here.
 *
 * Timer/frame control convention: `jest.useFakeTimers()` (modern — fakes Date and
 * requestAnimationFrame too) + `jest.advanceTimersByTime()`/`jest.runAllTimers()` throughout,
 * matching XtermTerminalBug.test.tsx's "Bug 3" (resize-specific) describe block. The manual
 * `captureRaf()` spy style used by that file's Bug-1 tests is intentionally NOT used here (see
 * plan.md's pitfalls research warning against mixing the two within one describe block).
 */

import React from "react";
import { render, act } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mock xterm.js — keep all state inside the factory so Jest can hoist the call.
// ---------------------------------------------------------------------------

interface MockTerminalLike {
  cols: number;
  rows: number;
  resize(cols: number, rows: number): void;
}

interface XtermResizeTestHarness {
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  terminal: MockTerminalLike | null;
  proposedDimensions: { cols: number; rows: number } | undefined;
  element: HTMLDivElement;
  modes: { mouseTrackingMode: string };
  setProposedDimensions(cols: number, rows: number): void;
  reset(): void;
}

jest.mock("@xterm/xterm", () => {
  const el = document.createElement("div");
  el.getBoundingClientRect = () =>
    ({
      left: 0,
      top: 0,
      width: 0,
      height: 0,
      right: 0,
      bottom: 0,
      x: 0,
      y: 0,
      toJSON: () => {},
    }) as DOMRect;

  const harness: XtermResizeTestHarness = {
    fitCalledCount: 0,
    onResizeCb: null,
    terminal: null,
    proposedDimensions: { cols: 80, rows: 24 },
    element: el,
    modes: { mouseTrackingMode: "none" },
    setProposedDimensions(cols: number, rows: number) {
      this.proposedDimensions = { cols, rows };
    },
    reset() {
      this.fitCalledCount = 0;
      this.onResizeCb = null;
      this.terminal = null;
      this.proposedDimensions = { cols: 80, rows: 24 };
      this.modes = { mouseTrackingMode: "none" };
    },
  };

  class MockTerminal implements MockTerminalLike {
    cols = 80;
    rows = 24;
    options: Record<string, unknown> = {};
    element = el;
    modes = harness.modes;

    constructor(opts?: Record<string, unknown>) {
      if (opts) Object.assign(this.options, opts);
      harness.terminal = this;
    }

    buffer = {
      active: { length: 0, cursorY: 0, viewportY: 0 },
      normal: { length: 0 },
    };

    // Mirrors real xterm.js Terminal.resize(): a no-op (no onResize fired) when the value is
    // unchanged; mutates cols/rows and fires the registered onResize callback only on a
    // genuine change. MockFitAddon.fit() drives this using harness.proposedDimensions.
    resize(cols: number, rows: number) {
      if (cols !== this.cols || rows !== this.rows) {
        this.cols = cols;
        this.rows = rows;
        harness.onResizeCb?.({ cols, rows });
      }
    }

    onResize(cb: (p: { cols: number; rows: number }) => void) {
      harness.onResizeCb = cb;
      return { dispose: jest.fn() };
    }
    onData() {
      return { dispose: jest.fn() };
    }
    onSelectionChange() {
      return { dispose: jest.fn() };
    }
    attachCustomKeyEventHandler() {}
    loadAddon() {}
    open() {}
    dispose() {}
    getSelection() {
      return "";
    }
    getSelectionPosition() {
      return undefined;
    }
    clearSelection() {}
    selectAll() {}
    refresh() {}
    scrollToBottom() {}
    focus() {}
  }

  (MockTerminal as unknown as { __harness: XtermResizeTestHarness }).__harness = harness;
  return { Terminal: MockTerminal };
});

jest.mock("@xterm/addon-fit", () => {
  const Terminal = require("@xterm/xterm").Terminal;
  const harness: XtermResizeTestHarness = (
    Terminal as unknown as { __harness: XtermResizeTestHarness }
  ).__harness;

  return {
    FitAddon: class MockFitAddon {
      fit() {
        // fit() being *called* is tracked unconditionally — it's gated upstream by
        // shouldFit in XtermTerminal.tsx, so fitCalledCount measures "how many times
        // XtermTerminal decided to call fit()", which is what AC5/AC6 assert on.
        harness.fitCalledCount++;
        const p = harness.proposedDimensions;
        if (p && harness.terminal) {
          harness.terminal.resize(p.cols, p.rows);
        }
      }
      proposeDimensions() {
        return harness.proposedDimensions;
      }
      dispose() {}
    },
  };
});

jest.mock("@xterm/addon-search", () => ({
  SearchAddon: class {
    findNext() {
      return false;
    }
    findPrevious() {
      return false;
    }
    dispose() {}
  },
}));
jest.mock("@xterm/addon-web-links", () => ({
  WebLinksAddon: class {
    dispose() {}
  },
}));
jest.mock("@xterm/addon-serialize", () => ({
  SerializeAddon: class {
    serialize() {
      return "";
    }
    dispose() {}
  },
}));

// Observable WebGL addon mock — tagged instances (id) + jest.fn() dispose, so the StrictMode
// (Task 4.2.1b) and AC4 (Epic 4.3) tests can assert on which instance got disposed and how
// many instances were ever constructed.
jest.mock("@xterm/addon-webgl", () => {
  const instances: Array<{ id: number; dispose: jest.Mock }> = [];
  let nextId = 0;

  class MockWebglAddon {
    id: number;
    dispose: jest.Mock;
    onContextLossCb: (() => void) | null = null;

    constructor() {
      this.id = nextId++;
      this.dispose = jest.fn();
      instances.push(this);
    }

    onContextLoss(cb: () => void) {
      this.onContextLossCb = cb;
    }
  }

  (MockWebglAddon as unknown as { __instances: unknown[] }).__instances = instances;
  (MockWebglAddon as unknown as { __reset: () => void }).__reset = () => {
    instances.length = 0;
    nextId = 0;
  };

  return { WebglAddon: MockWebglAddon };
});

jest.mock("@/lib/hooks/useMobileTerminalGestures", () => ({
  useMobileTerminalGestures: () => {},
}));
jest.mock("@/lib/hooks/useTouchScroll", () => ({
  useTouchScroll: () => {},
}));
jest.mock("@/lib/config/terminalConfig", () => ({
  loadTerminalConfig: () => null,
  darkTerminalTheme: {},
  lightTerminalTheme: {},
}));
jest.mock("@/lib/terminal/cellDimensions", () => ({
  getCellDimensions: () => ({ cellH: 20, cellW: 8 }),
}));

// ---------------------------------------------------------------------------
// Imports (after mocks are hoisted)
// ---------------------------------------------------------------------------
// eslint-disable-next-line import/first
import { XtermTerminal, type XtermTerminalHandle } from "../XtermTerminal";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getHarness(): XtermResizeTestHarness {
  const { Terminal } = jest.requireMock<any>("@xterm/xterm");
  return Terminal.__harness as XtermResizeTestHarness;
}

interface WebglMockInstance {
  id: number;
  dispose: jest.Mock;
}
interface WebglMockClass {
  __instances: WebglMockInstance[];
  __reset(): void;
}

function getWebglMock(): WebglMockClass {
  const { WebglAddon } = jest.requireMock<any>("@xterm/addon-webgl");
  return WebglAddon as WebglMockClass;
}

let observerCallback: ResizeObserverCallback | null = null;

function fireResizeObserver(width: number, height: number) {
  const cb = observerCallback;
  if (!cb) return;
  const entry = {
    contentRect: { width, height, top: 0, left: 0, bottom: height, right: width },
    borderBoxSize: [],
    contentBoxSize: [],
    devicePixelContentBoxSize: [],
    target: document.createElement("div"),
  } as unknown as ResizeObserverEntry;
  act(() => {
    cb([entry], {} as ResizeObserver);
  });
}

/** Flushes the 150ms ResizeObserver debounce + the nested double-rAF that follows it. */
function flushDebounce() {
  act(() => {
    jest.advanceTimersByTime(150);
  });
  act(() => {
    jest.runAllTimers();
  });
}

/** Flushes the initial-mount double-rAF (no debounce timer involved). */
function flushInitialMount() {
  act(() => {
    jest.runAllTimers();
  });
}

/** Flushes the microtask chain the WebGL `await import(...)` IIFE resolves through. */
async function flushMicrotasks(times = 5): Promise<void> {
  for (let i = 0; i < times; i++) {
    await Promise.resolve();
  }
}

function renderTerminal(onResize: (cols: number, rows: number) => void = jest.fn()) {
  const ref = React.createRef<XtermTerminalHandle>();
  render(<XtermTerminal ref={ref} onResize={onResize} />);
  return ref;
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

let logSpy: jest.SpyInstance;
let warnSpy: jest.SpyInstance;
let errorSpy: jest.SpyInstance;

beforeEach(() => {
  jest.useFakeTimers();

  getHarness().reset();
  getWebglMock().__reset();
  observerCallback = null;

  logSpy = jest.spyOn(console, "log").mockImplementation(() => {});
  warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
  errorSpy = jest.spyOn(console, "error").mockImplementation(() => {});

  Object.defineProperty(navigator, "clipboard", {
    value: {
      writeText: jest.fn().mockResolvedValue(undefined),
      readText: jest.fn().mockResolvedValue(""),
    },
    configurable: true,
    writable: true,
  });

  // Mock window.matchMedia (not available in jsdom) — required because the component's
  // system-color-scheme effect calls this unconditionally when no explicit `theme` prop is
  // passed.
  Object.defineProperty(window, "matchMedia", {
    value: jest.fn().mockReturnValue({
      matches: false,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
    }),
    configurable: true,
    writable: true,
  });

  // Mock ResizeObserver (not available in jsdom) with a controllable callback.
  Object.defineProperty(global, "ResizeObserver", {
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

afterEach(() => {
  delete (global as any).WebGL2RenderingContext;
  jest.clearAllTimers();
  jest.useRealTimers();
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Task 4.2.1b — StrictMode double-mount webglAddonRef regression test
// ---------------------------------------------------------------------------
describe("webglAddonRef survives StrictMode double-mount", () => {
  it("webglAddonRef_should_wireToSecondMountAddon_When_StrictModeDoubleInvokesEffect", async () => {
    // jsdom doesn't define WebGL2RenderingContext by default — define it so the mount
    // effect's async WebGL-load branch actually runs for both StrictMode invocations.
    (global as any).WebGL2RenderingContext = class {};

    const ref = React.createRef<XtermTerminalHandle>();
    render(
      <React.StrictMode>
        <XtermTerminal ref={ref} onResize={jest.fn()} />
      </React.StrictMode>,
    );

    // Flush the async `await import('@xterm/addon-webgl')` microtask chain for both the
    // throwaway (cancelled) first mount's IIFE and the live second mount's IIFE.
    await act(async () => {
      await flushMicrotasks();
    });
    // Flush the initial double-rAF fit() for the live mount.
    flushInitialMount();

    const webglMock = getWebglMock();
    // The `cancelled` guard (checked immediately after `await import()`, before
    // `new WebglAddon()` is ever constructed) must prevent the first (StrictMode-cleaned-up)
    // mount's IIFE from constructing an addon at all — only the live (second) mount's IIFE
    // should complete construction. If the guard regressed (removed or checked too late),
    // TWO instances would exist here instead of one.
    expect(webglMock.__instances).toHaveLength(1);
    const liveInstance = webglMock.__instances[0];

    // Prove webglAddonRef (not directly exposed) is wired to *this* instance — not a
    // phantom/never-existing throwaway one — by tripping the oscillation detector against the
    // live terminal and observing dispose() land on it.
    const harness = getHarness();
    const sequence = [
      { cols: 84, rows: 60 },
      { cols: 85, rows: 60 },
      { cols: 84, rows: 60 },
      { cols: 85, rows: 60 },
      { cols: 84, rows: 60 },
    ];
    let width = 800;
    for (const dims of sequence) {
      harness.setProposedDimensions(dims.cols, dims.rows);
      fireResizeObserver(width, 600);
      width += 40;
      flushDebounce();
    }

    expect(liveInstance.dispose).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Task 4.2.2a — AC5: sub-cell resize wobble is a no-op
// ---------------------------------------------------------------------------
describe("AC5: sub-cell resize wobble is a no-op", () => {
  it("fitCalledCount_should_notIncrease_and_onResize_should_notFireAgain_When_proposedDimsUnchangedAcrossWobble", () => {
    const harness = getHarness();
    harness.setProposedDimensions(120, 40);
    const onResize = jest.fn();

    renderTerminal(onResize);
    flushInitialMount();

    // Initial fit settles at 120x40, matching harness.proposedDimensions.
    expect(harness.terminal?.cols).toBe(120);
    expect(harness.terminal?.rows).toBe(40);
    expect(onResize).toHaveBeenCalledWith(120, 40);
    onResize.mockClear();
    const fitCountAfterMount = harness.fitCalledCount;

    // Sub-cell container wobble: real pixel size changes on every ResizeObserver entry, but
    // FitAddon.proposeDimensions() keeps returning the same {120,40} integer grid the whole
    // time — the shouldFit gate must skip fit() every cycle.
    fireResizeObserver(801, 600);
    flushDebounce();
    fireResizeObserver(802, 601);
    flushDebounce();
    fireResizeObserver(801, 600);
    flushDebounce();

    expect(harness.fitCalledCount).toBe(fitCountAfterMount);
    expect(onResize).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Task 4.2.3a — AC6: genuine cols/rows change still converges exactly once
// ---------------------------------------------------------------------------
describe("AC6: genuine cols/rows change still converges exactly once", () => {
  it("fitCalledCount_should_increaseByOne_and_onResize_should_fireOnce_When_realResizeCrossesCellBoundary", () => {
    const harness = getHarness(); // default proposedDimensions {80,24}
    const onResize = jest.fn();

    renderTerminal(onResize);
    flushInitialMount();

    expect(harness.terminal?.cols).toBe(80);
    expect(harness.terminal?.rows).toBe(24);
    onResize.mockClear();
    const fitCountBaseline = harness.fitCalledCount;

    harness.setProposedDimensions(150, 45);
    fireResizeObserver(1200, 900);
    flushDebounce();

    expect(harness.fitCalledCount).toBe(fitCountBaseline + 1);
    expect(harness.terminal?.cols).toBe(150);
    expect(harness.terminal?.rows).toBe(45);
    expect(onResize).toHaveBeenCalledTimes(1);
    expect(onResize).toHaveBeenCalledWith(150, 45);
  });
});

// ---------------------------------------------------------------------------
// Task 4.2.4a — AC1/AC7: imperative fit() handle is gated the same as the ResizeObserver path
// ---------------------------------------------------------------------------
describe("AC1/AC7: imperative fit() handle is gated the same as the ResizeObserver path", () => {
  it("fitCalledCount_should_notIncrease_When_refFitCalledWithNoActualLayoutChange", () => {
    const harness = getHarness();
    harness.setProposedDimensions(84, 60);

    const ref = renderTerminal();
    flushInitialMount();

    expect(harness.terminal?.cols).toBe(84);
    expect(harness.terminal?.rows).toBe(60);
    const fitCountBaseline = harness.fitCalledCount;

    // No actual layout change: proposedDimensions still {84,60} — mirrors
    // TerminalOutput.tsx's tab-visibility/visualViewport.resize handlers calling ref.fit()
    // with nothing having actually changed.
    act(() => {
      ref.current?.fit();
    });

    expect(harness.fitCalledCount).toBe(fitCountBaseline);
  });

  it("fitCalledCount_should_increaseByOne_When_refFitCalledWithGenuineLayoutChange", () => {
    const harness = getHarness();
    harness.setProposedDimensions(84, 60);

    const ref = renderTerminal();
    flushInitialMount();

    const fitCountBaseline = harness.fitCalledCount;

    // Container genuinely changed size while backgrounded (e.g. visualViewport reports a new
    // size on resume).
    harness.setProposedDimensions(90, 62);

    act(() => {
      ref.current?.fit();
    });

    expect(harness.fitCalledCount).toBe(fitCountBaseline + 1);
    expect(harness.terminal?.cols).toBe(90);
    expect(harness.terminal?.rows).toBe(62);
  });
});

// ---------------------------------------------------------------------------
// Task 4.3.1a — AC4: oscillation burst falls back off WebGL
// ---------------------------------------------------------------------------
describe("AC4: oscillation burst falls back off WebGL", () => {
  // shouldAbandonWebgl trips when the most recent value recurs >=3 times within the window.
  // Since a real terminal.onResize only fires on a genuine value change (mirrored by
  // MockTerminal.resize()'s no-op-if-unchanged guard), getting 3 non-consecutive same-value
  // entries requires 5 alternating firings (A,B,A,B,A) — this is the minimal sequence
  // consistent with resizeConvergence.ts's actual threshold=3 semantics (see plan.md Story
  // 2.3.1's own worked example, which likewise needs a 4th/discontiguous occurrence, not 3
  // consecutive-looking toggles).
  function runBurst(
    harness: XtermResizeTestHarness,
    sequence: Array<{ cols: number; rows: number }>,
    startWidth: number,
  ) {
    let width = startWidth;
    for (const dims of sequence) {
      harness.setProposedDimensions(dims.cols, dims.rows);
      fireResizeObserver(width, 600);
      width += 40;
      flushDebounce();
    }
  }

  const burstA = [84, 85, 84, 85, 84].map((cols) => ({ cols, rows: 60 }));
  const burstB = [90, 91, 90, 91, 90].map((cols) => ({ cols, rows: 60 }));

  it("dispose_should_beCalledOnce_and_warnFallsBackToDefaultRenderer_When_burstOccursWithWebglLoaded", async () => {
    (global as any).WebGL2RenderingContext = class {};

    const harness = getHarness();
    renderTerminal();

    await act(async () => {
      await flushMicrotasks();
    });
    flushInitialMount();

    const webglMock = getWebglMock();
    expect(webglMock.__instances).toHaveLength(1);
    const webglInstance = webglMock.__instances[0];

    runBurst(harness, burstA, 800);

    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    const warnMessages = warnSpy.mock.calls.map((c) => String(c[0]));
    expect(warnMessages.some((m) => m.includes("falling back to default renderer"))).toBe(true);
    expect(warnMessages.some((m) => m.includes("canvas renderer"))).toBe(false);
  });

  it("consoleError_should_logBackstop_and_notThrow_When_burstOccursWithNoWebglAddonLoaded", () => {
    // WebGL2RenderingContext intentionally left undefined (jsdom's real default — see
    // resizeConvergence.ts/XtermTerminal.tsx's `else` branch) — webglAddonRef never gets set.
    const harness = getHarness();
    renderTerminal();
    flushInitialMount();

    expect(getWebglMock().__instances).toHaveLength(0);

    expect(() => runBurst(harness, burstA, 800)).not.toThrow();

    const errorMessages = errorSpy.mock.calls.map((c) => String(c[0]));
    expect(errorMessages.some((m) => m.includes("no WebGL addon to dispose"))).toBe(true);
  });

  it("dispose_should_NOT_beCalledAgain_and_error_should_NOT_reLog_When_secondBurstOccursAfterFallback", async () => {
    (global as any).WebGL2RenderingContext = class {};

    const harness = getHarness();
    renderTerminal();

    await act(async () => {
      await flushMicrotasks();
    });
    flushInitialMount();

    const webglMock = getWebglMock();
    const webglInstance = webglMock.__instances[0];

    // First burst trips the fallback (dispose #1).
    runBurst(harness, burstA, 800);
    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);

    // Move well past the 2000ms window so the second burst is genuinely new, not a
    // continuation of the first (oscillationHistory was already cleared on trip regardless).
    act(() => {
      jest.advanceTimersByTime(2100);
    });

    // Second, independent burst — starts from a value (90) that differs from wherever the
    // first burst left the terminal (84), so the first firing of this burst still passes the
    // shouldFit gate.
    runBurst(harness, burstB, 2000);

    // dispose() is not called a second time — nothing left to dispose.
    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(errorSpy).not.toHaveBeenCalled();

    const logMessages = logSpy.mock.calls.map((c) => String(c[0]));
    expect(logMessages.some((m) => m.includes("persists after WebGL fallback"))).toBe(true);
  });
});
