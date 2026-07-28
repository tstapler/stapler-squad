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

/**
 * Per-instance record, one per constructed MockTerminal, in construction order. Added (AC1
 * multi-instance tiled-pane convergence coverage — see describe block near the bottom of this
 * file) so 2-3 concurrently-mounted `<XtermTerminal>` instances each get their own independent
 * `fitCalledCount`/`onResizeCb`/`proposedDimensions`, instead of all sharing the single
 * `XtermResizeTestHarness` singleton fields below (which only ever reflected ONE instance at a
 * time and get silently overwritten by whichever MockTerminal was constructed last).
 */
interface XtermInstanceRecord {
  terminal: MockTerminalLike | null;
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  proposedDimensions: { cols: number; rows: number } | undefined;
}

interface XtermResizeTestHarness {
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  terminal: MockTerminalLike | null;
  proposedDimensions: { cols: number; rows: number } | undefined;
  element: HTMLDivElement;
  modes: { mouseTrackingMode: string };
  /**
   * Per-instance records for multi-instance tests. The singleton fields above are left fully
   * intact (still mirroring the LAST constructed MockTerminal) so every pre-existing
   * single-instance test keeps working unchanged.
   */
  instances: XtermInstanceRecord[];
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
    instances: [],
    setProposedDimensions(cols: number, rows: number) {
      this.proposedDimensions = { cols, rows };
      // Every pre-existing (single-instance) test calls this legacy singleton setter AFTER the
      // component has already mounted (i.e. after MockTerminal's per-instance record has already
      // snapshotted the OLD default). Keep that working by also updating the currently-active
      // instance's own record — multi-instance tests bypass this setter entirely and mutate
      // harness.instances[i].proposedDimensions directly instead.
      const current = this.instances[this.instances.length - 1];
      if (current) current.proposedDimensions = { cols, rows };
    },
    reset() {
      this.fitCalledCount = 0;
      this.onResizeCb = null;
      this.terminal = null;
      this.proposedDimensions = { cols: 80, rows: 24 };
      this.modes = { mouseTrackingMode: "none" };
      this.instances = [];
    },
  };

  class MockTerminal implements MockTerminalLike {
    cols = 80;
    rows = 24;
    options: Record<string, unknown> = {};
    element = el;
    modes = harness.modes;
    __record: XtermInstanceRecord;

    constructor(opts?: Record<string, unknown>) {
      if (opts) Object.assign(this.options, opts);
      harness.terminal = this;
      // Snapshot the harness default at construction time so tests that call
      // harness.setProposedDimensions(...) BEFORE rendering (the existing single-instance
      // convention) still seed this instance's own proposed dims correctly.
      this.__record = {
        terminal: this,
        fitCalledCount: 0,
        onResizeCb: null,
        proposedDimensions: harness.proposedDimensions
          ? { ...harness.proposedDimensions }
          : undefined,
      };
      harness.instances.push(this.__record);
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
        // Fire via this instance's own record — for single-instance tests this is the exact
        // same callback reference that used to be routed through the harness singleton, so
        // behavior there is unchanged. Multi-instance tests each get their own callback.
        this.__record.onResizeCb?.({ cols, rows });
      }
    }

    onResize(cb: (p: { cols: number; rows: number }) => void) {
      harness.onResizeCb = cb;
      this.__record.onResizeCb = cb;
      return { dispose: jest.fn() };
    }
    onData() {
      return { dispose: jest.fn() };
    }
    onSelectionChange() {
      return { dispose: jest.fn() };
    }
    attachCustomKeyEventHandler() {}
    loadAddon(addon: unknown) {
      // Mirrors real xterm.js's Terminal.loadAddon(addon), which calls addon.activate(terminal)
      // — lets MockFitAddon (a separate mock module) learn which MockTerminal instance it's
      // paired with, instead of relying on the module-level harness singleton (needed for
      // multi-instance tests where more than one Terminal/FitAddon pair coexists).
      const a = addon as { __attachTerminal?: (t: MockTerminal) => void };
      a?.__attachTerminal?.(this);
    }
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
      // Wired via MockTerminal.loadAddon() → __attachTerminal(), mirroring real xterm.js's
      // addon.activate(terminal). Lets each FitAddon instance read/resize ITS OWN terminal's
      // per-instance record instead of the module-level harness singleton — required for
      // multi-instance tests (AC1 tiled-pane convergence) where several Terminal/FitAddon pairs
      // coexist. Falls back to the harness singleton when unset (defensive only; every real
      // XtermTerminal mount calls terminal.loadAddon(fitAddon) immediately after construction).
      private __terminal: (MockTerminalLike & { __record?: XtermInstanceRecord }) | null = null;

      __attachTerminal(terminal: MockTerminalLike & { __record?: XtermInstanceRecord }) {
        this.__terminal = terminal;
      }

      fit() {
        // fit() being *called* is tracked unconditionally — it's gated upstream by
        // shouldFit in XtermTerminal.tsx, so fitCalledCount measures "how many times
        // XtermTerminal decided to call fit()", which is what AC5/AC6 assert on.
        harness.fitCalledCount++;
        const terminal = this.__terminal ?? harness.terminal;
        const record = (terminal as { __record?: XtermInstanceRecord } | null)?.__record;
        if (record) record.fitCalledCount++;
        const p = record ? record.proposedDimensions : harness.proposedDimensions;
        if (p && terminal) {
          terminal.resize(p.cols, p.rows);
        }
      }
      proposeDimensions() {
        const terminal = this.__terminal ?? harness.terminal;
        const record = (terminal as { __record?: XtermInstanceRecord } | null)?.__record;
        return record ? record.proposedDimensions : harness.proposedDimensions;
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

/**
 * Per-instance ResizeObserver callbacks, one per constructed observer, in construction order.
 * Populated alongside `observerCallback` (unchanged — still mirrors the LAST constructed
 * observer, for single-instance test backward compatibility). Added so multi-instance tests can
 * fire 2-3 sibling `<XtermTerminal>` instances' own ResizeObservers independently or all within
 * one `act()` batch (AC1 tiled-pane convergence coverage).
 */
let resizeObserverCallbacks: ResizeObserverCallback[] = [];

function makeResizeEntry(width: number, height: number): ResizeObserverEntry {
  return {
    contentRect: { width, height, top: 0, left: 0, bottom: height, right: width },
    borderBoxSize: [],
    contentBoxSize: [],
    devicePixelContentBoxSize: [],
    target: document.createElement("div"),
  } as unknown as ResizeObserverEntry;
}

function fireResizeObserver(width: number, height: number) {
  const cb = observerCallback;
  if (!cb) return;
  act(() => {
    cb([makeResizeEntry(width, height)], {} as ResizeObserver);
  });
}

/** Fires a single sibling instance's own ResizeObserver, by construction-order index. */
function fireResizeObserverAt(index: number, width: number, height: number) {
  const cb = resizeObserverCallbacks[index];
  if (!cb) return;
  act(() => {
    cb([makeResizeEntry(width, height)], {} as ResizeObserver);
  });
}

/**
 * Fires every currently-registered sibling ResizeObserver within a single `act()` batch —
 * simulates a real window/shared-container resize cascading into all tiled sibling panes'
 * individual ResizeObservers at (effectively) the same tick, i.e. the "panes resize in
 * lockstep" topology from the original bug report. `dims[i]` pairs positionally with
 * `resizeObserverCallbacks[i]`; a missing entry leaves that instance untouched.
 */
function fireResizeObserverOnAll(dims: Array<{ width: number; height: number } | undefined>) {
  act(() => {
    resizeObserverCallbacks.forEach((cb, i) => {
      const d = dims[i];
      if (!d) return;
      cb([makeResizeEntry(d.width, d.height)], {} as ResizeObserver);
    });
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

/**
 * Mounts N independent `<XtermTerminal>` instances as sibling leaf panes inside one shared flex
 * container — mirrors the real tiled-pane shape (`PaneSplitRenderer.tsx`'s `PaneSplitComponent`
 * renders sibling `leafContainer` panes side by side; `web-app/src/styles/pane/paneSplit.css.ts`'s
 * `leafContainer`/`paneBody` recipes are `display:flex` column, wrapped by the split grid) without
 * pulling in the real Pane/Redux/WebSocket stack — see task context for why that's disproportionate
 * to what this test needs to prove about sibling `XtermTerminal` independence.
 */
function renderMultiTerminals(onResizeFns: Array<(cols: number, rows: number) => void>) {
  const refs = onResizeFns.map(() => React.createRef<XtermTerminalHandle>());
  render(
    <div style={{ display: "flex", flexDirection: "row", width: "100%", height: "100%" }}>
      {onResizeFns.map((onResize, i) => (
        <div key={i} style={{ flex: 1, minWidth: 0, minHeight: 0, overflow: "hidden" }}>
          <XtermTerminal ref={refs[i]} onResize={onResize} />
        </div>
      ))}
    </div>,
  );
  return refs;
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
  resizeObserverCallbacks = [];

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
        resizeObserverCallbacks.push(cb);
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

// ---------------------------------------------------------------------------
// webglFallbackTrippedRef reset on effect cleanup — regression test for the fix that adds
// `webglFallbackTrippedRef.current = false;` alongside `webglAddonRef.current = null;` in the
// mount effect's cleanup. Without the fix, a [scrollback]-triggered remount whose new instance
// never successfully loads WebGL would carry the stale `true` forward, so a fresh oscillation
// burst on the new instance would wrongly hit the "persists after WebGL fallback" console.log
// branch instead of the "no WebGL addon to dispose" console.error backstop.
// ---------------------------------------------------------------------------
describe("webglFallbackTrippedRef resets across a scrollback-triggered remount", () => {
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

  it("secondInstance_should_logNoWebglAddonBackstop_not_persistsMessage_When_burstOccursAfterRemountWhoseWebglNeverLoaded", async () => {
    (global as any).WebGL2RenderingContext = class {};

    const harness = getHarness();
    const onResize = jest.fn();
    const { rerender } = render(
      <XtermTerminal onResize={onResize} scrollback={100} />,
    );

    await act(async () => {
      await flushMicrotasks();
    });
    flushInitialMount();

    const webglMock = getWebglMock();
    expect(webglMock.__instances).toHaveLength(1);
    const firstInstance = webglMock.__instances[0];

    // Trip the fallback on the first instance.
    runBurst(harness, burstA, 800);
    expect(firstInstance.dispose).toHaveBeenCalledTimes(1);

    // Simulate the new instance's WebGL failing to load (transient failure) by making
    // WebGL2RenderingContext unavailable before the [scrollback]-triggered remount — the
    // mount effect's `else` branch then never constructs a WebglAddon at all.
    delete (global as any).WebGL2RenderingContext;

    rerender(<XtermTerminal onResize={onResize} scrollback={200} />);
    flushInitialMount();

    expect(getWebglMock().__instances).toHaveLength(1); // no second WebGL instance was created

    errorSpy.mockClear();
    logSpy.mockClear();

    // Fresh burst on the new (never-loaded-WebGL) instance.
    runBurst(harness, burstB, 2000);

    const errorMessages = errorSpy.mock.calls.map((c) => String(c[0]));
    expect(errorMessages.some((m) => m.includes("no WebGL addon to dispose"))).toBe(true);

    const logMessages = logSpy.mock.calls.map((c) => String(c[0]));
    expect(logMessages.some((m) => m.includes("persists after WebGL fallback"))).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// AC1 (corrected Problem Statement/Success Criteria — see
// project_plans/terminal-resize-fit-loop/requirements.md): "panes resize in lockstep... ping-pong
// off each other" describes 3 terminals tiled as sibling panes on ONE page (PaneSplitRenderer.tsx's
// PaneSplitComponent renders sibling leafContainer panes via CSS flex/grid, each holding an
// independent session's XtermTerminal). Every test above this point instantiates exactly ONE
// XtermTerminal at a time, so none of them exercise the real bug topology: multiple concurrently-
// mounted XtermTerminal instances sharing a page, where a resize can cascade into all siblings'
// ResizeObservers at once. These tests mount 2-3 real XtermTerminal instances side by side inside
// one shared flex container (renderMultiTerminals, above) and assert each instance's fit/resize
// state converges independently — bounded, and with no coordination between instances.
// ---------------------------------------------------------------------------
describe("AC1: multiple sibling XtermTerminal instances converge independently in a tiled layout", () => {
  it("fitCalledCount_should_increaseByOneEach_and_convergeToDistinctDimensions_When_sharedParentResizeCascadesGenuineChangesToAllSiblings", () => {
    const onResize1 = jest.fn();
    const onResize2 = jest.fn();
    const onResize3 = jest.fn();
    renderMultiTerminals([onResize1, onResize2, onResize3]);
    flushInitialMount();

    const harness = getHarness();
    expect(harness.instances).toHaveLength(3);
    const [rec1, rec2, rec3] = harness.instances;

    onResize1.mockClear();
    onResize2.mockClear();
    onResize3.mockClear();
    const baseline = harness.instances.map((r) => r.fitCalledCount);

    // Each sibling pane gets a genuinely different proposed grid — a real cell-boundary
    // crossing per instance, mirroring 3 independent terminal sessions tiled at different
    // widths, not a sub-pixel wobble.
    rec1.proposedDimensions = { cols: 100, rows: 30 };
    rec2.proposedDimensions = { cols: 120, rows: 35 };
    rec3.proposedDimensions = { cols: 90, rows: 28 };

    // Simulate a window resize cascading into all 3 sibling panes' ResizeObservers at once
    // ("the panes resize in lockstep" topology from the original bug report).
    fireResizeObserverOnAll([
      { width: 900, height: 700 },
      { width: 1000, height: 750 },
      { width: 850, height: 650 },
    ]);
    flushDebounce();

    expect(rec1.fitCalledCount).toBe(baseline[0] + 1);
    expect(rec2.fitCalledCount).toBe(baseline[1] + 1);
    expect(rec3.fitCalledCount).toBe(baseline[2] + 1);

    expect(rec1.terminal?.cols).toBe(100);
    expect(rec1.terminal?.rows).toBe(30);
    expect(rec2.terminal?.cols).toBe(120);
    expect(rec2.terminal?.rows).toBe(35);
    expect(rec3.terminal?.cols).toBe(90);
    expect(rec3.terminal?.rows).toBe(28);

    expect(onResize1).toHaveBeenCalledTimes(1);
    expect(onResize1).toHaveBeenCalledWith(100, 30);
    expect(onResize2).toHaveBeenCalledTimes(1);
    expect(onResize2).toHaveBeenCalledWith(120, 35);
    expect(onResize3).toHaveBeenCalledTimes(1);
    expect(onResize3).toHaveBeenCalledWith(90, 28);
  });

  it("fitCalledCount_should_notIncrease_and_onResize_should_notFire_When_subPixelWobbleCascadesToAllSiblingsSimultaneously", () => {
    const onResize1 = jest.fn();
    const onResize2 = jest.fn();
    const onResize3 = jest.fn();
    renderMultiTerminals([onResize1, onResize2, onResize3]);
    flushInitialMount();

    const harness = getHarness();
    const [rec1, rec2, rec3] = harness.instances;

    onResize1.mockClear();
    onResize2.mockClear();
    onResize3.mockClear();
    const baseline = harness.instances.map((r) => r.fitCalledCount);

    // proposedDimensions is left untouched (still each instance's default {80,24}, matching the
    // terminal's already-applied cols/rows) — every sibling's ResizeObserver reports a raw pixel
    // wobble (the "ping-pong off each other" symptom from the original report) that never
    // actually changes FitAddon's proposed integer grid for ANY instance.
    fireResizeObserverOnAll([
      { width: 801, height: 600 },
      { width: 701, height: 500 },
      { width: 601, height: 400 },
    ]);
    flushDebounce();
    fireResizeObserverOnAll([
      { width: 802, height: 601 },
      { width: 702, height: 501 },
      { width: 602, height: 401 },
    ]);
    flushDebounce();
    fireResizeObserverOnAll([
      { width: 801, height: 600 },
      { width: 701, height: 500 },
      { width: 601, height: 400 },
    ]);
    flushDebounce();

    expect(rec1.fitCalledCount).toBe(baseline[0]);
    expect(rec2.fitCalledCount).toBe(baseline[1]);
    expect(rec3.fitCalledCount).toBe(baseline[2]);
    expect(onResize1).not.toHaveBeenCalled();
    expect(onResize2).not.toHaveBeenCalled();
    expect(onResize3).not.toHaveBeenCalled();
  });

  it("unaffectedSiblings_fitCalledCount_should_NOT_move_When_onlyOneSiblingsContainerActuallyResizes", () => {
    const onResize1 = jest.fn();
    const onResize2 = jest.fn();
    const onResize3 = jest.fn();
    renderMultiTerminals([onResize1, onResize2, onResize3]);
    flushInitialMount();

    const harness = getHarness();
    const [rec1, rec2, rec3] = harness.instances;

    onResize1.mockClear();
    onResize2.mockClear();
    onResize3.mockClear();
    const baseline = harness.instances.map((r) => r.fitCalledCount);

    // Only the middle sibling (index 1) genuinely resizes; instances 0 and 2 receive no
    // ResizeObserver firing at all. There is no shared/global state in the real implementation
    // that would let instance 1's resize perturb 0 or 2 — this proves that directly.
    rec2.proposedDimensions = { cols: 130, rows: 40 };
    fireResizeObserverAt(1, 1100, 850);
    flushDebounce();

    expect(rec1.fitCalledCount).toBe(baseline[0]);
    expect(rec3.fitCalledCount).toBe(baseline[2]);
    expect(onResize1).not.toHaveBeenCalled();
    expect(onResize3).not.toHaveBeenCalled();

    expect(rec2.fitCalledCount).toBe(baseline[1] + 1);
    expect(rec2.terminal?.cols).toBe(130);
    expect(rec2.terminal?.rows).toBe(40);
    expect(onResize2).toHaveBeenCalledTimes(1);
    expect(onResize2).toHaveBeenCalledWith(130, 40);
  });

  it("fitCalledCount_should_NOT_growUnbounded_When_theSameGenuineResizeRepeatsAcrossAllSiblings", () => {
    const onResize1 = jest.fn();
    const onResize2 = jest.fn();
    renderMultiTerminals([onResize1, onResize2]);
    flushInitialMount();

    const harness = getHarness();
    const [rec1, rec2] = harness.instances;

    onResize1.mockClear();
    onResize2.mockClear();
    const baseline = harness.instances.map((r) => r.fitCalledCount);

    rec1.proposedDimensions = { cols: 150, rows: 45 };
    rec2.proposedDimensions = { cols: 160, rows: 48 };

    const dims = [
      { width: 1200, height: 900 },
      { width: 1300, height: 950 },
    ];

    // Fire the identical genuine-resize entries 3 times across both siblings. Each instance must
    // converge to a fixed point after the first cycle — a regression that let one instance's
    // settle-then-refire loop leak into (or amplify via) a sibling would show up here as
    // unbounded growth instead of a flat count on cycles 2 and 3.
    fireResizeObserverOnAll(dims);
    flushDebounce();
    fireResizeObserverOnAll(dims);
    flushDebounce();
    fireResizeObserverOnAll(dims);
    flushDebounce();

    expect(rec1.fitCalledCount).toBe(baseline[0] + 1);
    expect(rec2.fitCalledCount).toBe(baseline[1] + 1);
    expect(onResize1).toHaveBeenCalledTimes(1);
    expect(onResize2).toHaveBeenCalledTimes(1);
  });

  it("fitCalledCount_should_increaseByOneEach_When_twoSiblingsResizeToTheIdenticalNewPixelDimensionsSimultaneously", () => {
    // Regression guard for a hypothetical cross-instance state leak: if XtermTerminal's
    // ResizeObserver-gate tracking (e.g. `lastContainerSize`) were ever accidentally shared
    // across instances instead of scoped per-mount, then two sibling panes reporting the exact
    // same incoming pixel size in the same tick (a very plausible tiled-layout scenario — e.g.
    // two equal-width panes both widening to the same new width after a 3-way split rebalances)
    // would make the SECOND instance's ResizeObserver handler see "no change" (because it'd be
    // comparing against whatever the FIRST instance just wrote to the shared variable, not its
    // own prior size), silently dropping its genuine resize. Each instance must still fit/converge
    // independently even when the raw incoming numbers happen to collide.
    const onResize1 = jest.fn();
    const onResize2 = jest.fn();
    renderMultiTerminals([onResize1, onResize2]);
    flushInitialMount();

    const harness = getHarness();
    const [rec1, rec2] = harness.instances;

    onResize1.mockClear();
    onResize2.mockClear();
    const baseline = harness.instances.map((r) => r.fitCalledCount);

    rec1.proposedDimensions = { cols: 140, rows: 42 };
    rec2.proposedDimensions = { cols: 145, rows: 44 };

    // Identical width/height fired for BOTH siblings, in the same act() batch (before any
    // debounce/rAF flush) — the scenario that exposes a shared (rather than per-instance)
    // "last known container size" gate.
    fireResizeObserverOnAll([
      { width: 950, height: 720 },
      { width: 950, height: 720 },
    ]);
    flushDebounce();

    expect(rec1.fitCalledCount).toBe(baseline[0] + 1);
    expect(rec2.fitCalledCount).toBe(baseline[1] + 1);
    expect(rec1.terminal?.cols).toBe(140);
    expect(rec2.terminal?.cols).toBe(145);
    expect(onResize1).toHaveBeenCalledTimes(1);
    expect(onResize2).toHaveBeenCalledTimes(1);
  });
});
