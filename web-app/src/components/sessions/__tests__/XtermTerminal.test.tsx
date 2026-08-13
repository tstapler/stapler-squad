/**
 * Regression tests for XtermTerminal's decoupled resize sampler (Epic 1),
 * value-dedup/force-bypass interplay (Epic 2, covered in useTerminalFlowControl
 * and TerminalOutput tests), and WebGL->Canvas mismatch fallback (Epic 3).
 *
 * WebGL/Canvas are unusable in jsdom (see
 * project_plans/terminal-resize-fit-loop/research/pitfalls.md §5), so
 * @xterm/xterm and all its addons are mocked at the module boundary. These
 * tests exercise the fallback's *decision logic and guard conditions in
 * isolation* -- not real WebGL/Canvas rendering behavior.
 *
 * Per pitfalls §5: the fake-timer "drain the whole queue" APIs must never be
 * used here -- the bug this suite guards against is a self-rescheduling
 * timer loop, and those APIs would hang the Jest worker on a regression
 * instead of failing with a useful assertion. Only jest.advanceTimersByTime()
 * with bounded, fixed millisecond budgets is used throughout.
 */

import { render, act } from "@testing-library/react";
import React from "react";

// ---- Module-boundary mocks (must precede the component-under-test import) ----

jest.mock("@xterm/xterm", () => {
  class MockTerminal {
    static instances: MockTerminal[] = [];
    cols = 80;
    rows = 24;
    options: any;
    _core: any = { _renderService: { dimensions: undefined } };
    loadedAddons: any[] = [];
    containerEl: HTMLElement | null = null;
    disposed = false;
    // updateScrollbar() (origin/main's mobile-scrollbar feature) reads buffer.active
    // unconditionally from the startup double-RAF fit and every confirmed sampler tick.
    buffer = { active: { length: 0, viewportY: 0 } };

    constructor(options: any) {
      this.options = options ?? {};
      MockTerminal.instances.push(this);
    }
    loadAddon(addon: any) {
      this.loadedAddons.push(addon);
    }
    open(container: HTMLElement) {
      this.containerEl = container;
    }
    onData(_cb: any) {
      return { dispose: jest.fn() };
    }
    onSelectionChange(_cb: any) {
      return { dispose: jest.fn() };
    }
    onResize(_cb: any) {
      return { dispose: jest.fn() };
    }
    getSelection() {
      return "";
    }
    refresh() {}
    write() {}
    writeln() {}
    clear() {}
    focus() {}
    dispose() {
      this.disposed = true;
    }
  }
  return { Terminal: MockTerminal };
});

jest.mock("@xterm/addon-fit", () => {
  class MockFitAddon {
    static instances: MockFitAddon[] = [];
    fit = jest.fn();
    proposeDimensions = jest.fn(() => undefined as { cols: number; rows: number } | undefined);
    constructor() {
      MockFitAddon.instances.push(this);
    }
  }
  return { FitAddon: MockFitAddon };
});

jest.mock("@xterm/addon-webgl", () => {
  class MockWebglAddon {
    static instances: MockWebglAddon[] = [];
    dispose = jest.fn();
    onContextLoss = jest.fn((cb: () => void) => {
      (this as any)._contextLossCb = cb;
    });
    constructor() {
      MockWebglAddon.instances.push(this);
    }
  }
  return { WebglAddon: MockWebglAddon };
});

jest.mock("@xterm/addon-canvas", () => {
  class MockCanvasAddon {
    static instances: MockCanvasAddon[] = [];
    static shouldThrow = false;
    dispose = jest.fn();
    constructor() {
      if (MockCanvasAddon.shouldThrow) {
        throw new Error("mock CanvasAddon construction failed");
      }
      MockCanvasAddon.instances.push(this);
    }
  }
  return { CanvasAddon: MockCanvasAddon };
});

jest.mock("@xterm/addon-web-links", () => {
  class MockWebLinksAddon {}
  return { WebLinksAddon: MockWebLinksAddon };
});

jest.mock("@xterm/addon-search", () => {
  class MockSearchAddon {
    findNext = jest.fn(() => true);
    findPrevious = jest.fn(() => true);
  }
  return { SearchAddon: MockSearchAddon };
});

import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { CanvasAddon } from "@xterm/addon-canvas";
import {
  XtermTerminal,
  shouldScheduleFit,
  isSustainedMismatch,
  extractCellMismatchInputs,
  SAMPLE_INTERVAL_MS,
  MAX_SAMPLES,
  type XtermTerminalHandle,
} from "../XtermTerminal";

// Test-local shapes for the mocked addon/terminal instances -- these
// deliberately diverge from the real @xterm/* type declarations (which
// TypeScript still resolves statically even though jest.mock() replaces the
// module at runtime), since the mocks expose jest.fn()-backed members.
interface MockTerminalInstance {
  cols: number;
  rows: number;
  options: any;
  _core: { _renderService: { dimensions: any } };
  loadedAddons: any[];
  containerEl: HTMLElement | null;
  disposed: boolean;
}
interface MockFitAddonInstance {
  fit: jest.Mock<void, []>;
  proposeDimensions: jest.Mock<{ cols: number; rows: number } | undefined, []>;
}
interface MockWebglAddonInstance {
  dispose: jest.Mock;
  onContextLoss: jest.Mock;
  _contextLossCb?: () => void;
}
interface MockCanvasAddonInstance {
  dispose: jest.Mock;
}

const MockTerminal = Terminal as unknown as { instances: MockTerminalInstance[] };
const MockFitAddon = FitAddon as unknown as { instances: MockFitAddonInstance[] };
const MockWebglAddon = WebglAddon as unknown as { instances: MockWebglAddonInstance[] };
const MockCanvasAddon = CanvasAddon as unknown as {
  instances: MockCanvasAddonInstance[];
  shouldThrow: boolean;
};

// ---- ResizeObserver capture harness (Task 4.1.1/4.1.2) ----
// The global MockResizeObserver installed in jest.setup.js is intentionally
// inert (never invokes its callback). Here we swap in a capturing variant so
// individual tests can drive fabricated ResizeObserverEntry deliveries.
interface CapturedRO {
  callback: (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
  observe: jest.Mock;
  unobserve: jest.Mock;
  disconnect: jest.Mock;
}

let capturedResizeObservers: CapturedRO[];

function installCapturingResizeObserver() {
  capturedResizeObservers = [];
  class CapturingResizeObserver {
    callback: CapturedRO["callback"];
    observe = jest.fn();
    unobserve = jest.fn();
    disconnect = jest.fn();
    constructor(cb: CapturedRO["callback"]) {
      this.callback = cb;
      capturedResizeObservers.push(this as unknown as CapturedRO);
    }
  }
  (global as any).ResizeObserver = CapturingResizeObserver;
}

beforeEach(() => {
  jest.useFakeTimers();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
  installCapturingResizeObserver();
  // jsdom has no WebGL2 support and therefore no WebGL2RenderingContext global.
  // XtermTerminal's xterm.js issue #2033 Android guard (`typeof
  // WebGL2RenderingContext !== 'undefined'`) gates the dynamic addon-webgl
  // import on this, so without a stub the WebGL->Canvas fallback suite below
  // would never even attempt to load @xterm/addon-webgl.
  if (typeof (global as any).WebGL2RenderingContext === "undefined") {
    (global as any).WebGL2RenderingContext = class WebGL2RenderingContext {};
  }
  MockTerminal.instances.length = 0;
  MockFitAddon.instances.length = 0;
  MockWebglAddon.instances.length = 0;
  MockCanvasAddon.instances.length = 0;
  MockCanvasAddon.shouldThrow = false;
});

afterEach(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
});

/** Mounts a single XtermTerminal and flushes its double-RAF init-fit chain with
 * one bounded, fixed-budget advance (never an unbounded drain-the-queue call).
 * Also flushes the microtask queue so the dynamic `import('@xterm/addon-webgl')`
 * (xterm.js issue #2033 Android guard) resolves and populates webglAddonRef
 * before callers assert against MockWebglAddon.instances. */
async function mountAndSettle() {
  const ref = React.createRef<XtermTerminalHandle>();
  render(<XtermTerminal ref={ref} />);
  await act(async () => {
    jest.advanceTimersByTime(200);
    // Flush the dynamic import()'s promise chain (a microtask, not a timer --
    // advanceTimersByTime alone does not resolve it).
    await Promise.resolve();
    await Promise.resolve();
  });
  const terminal = MockTerminal.instances[MockTerminal.instances.length - 1];
  const fitAddon = MockFitAddon.instances[MockFitAddon.instances.length - 1];
  const ro = capturedResizeObservers[capturedResizeObservers.length - 1];
  // Mount settles with fit() called twice (initial + secondary) -- reset so
  // subsequent assertions measure only the scenario under test.
  fitAddon.fit.mockClear();
  fitAddon.proposeDimensions.mockClear();
  // Default fit() behavior: apply whatever proposeDimensions() currently
  // returns to the mock terminal, mirroring real FitAddon.fit() measuring
  // and applying -- this keeps `applied` (terminal.cols/rows) tracking each
  // confirmed resize across multiple cycles in the same test. Individual
  // tests may override this with their own mockImplementation.
  fitAddon.fit.mockImplementation(() => {
    const proposed = fitAddon.proposeDimensions();
    if (proposed) {
      terminal.cols = proposed.cols;
      terminal.rows = proposed.rows;
    }
  });
  return { ref, terminal, fitAddon, ro };
}

/** Fires an RO delivery and flushes the flat 150ms debounce (R1.2, replacing
 * the old adaptive 10ms-for-first-3 debounce) -> sampler-start tick 1, which
 * runs synchronously inside startSamplerIfNeeded(). Advances just past 150ms
 * -- deliberately NOT a full SAMPLE_INTERVAL_MS beyond that, which would also
 * sweep up tick 2 in the same advance and collapse the two-tick dead-band
 * confirmation this whole suite exists to exercise. */
function fireResizeAndStartSampler(ro: CapturedRO, width: number, height = 480) {
  act(() => {
    ro.callback([{ contentRect: { width, height } }]);
  });
  act(() => {
    jest.advanceTimersByTime(151);
  });
}

/** Drives one full 2-tick convergent resize: RO delivery -> tick1 (registers
 * pending) -> tick2 (confirms, fit() called). `fitAddon.proposeDimensions` is
 * pinned to a constant value for the whole cycle via mockReturnValue, per
 * pitfalls §5's guidance to avoid brittle mockReturnValueOnce chaining. */
function driveConvergentResize(
  ro: CapturedRO,
  fitAddon: Awaited<ReturnType<typeof mountAndSettle>>["fitAddon"],
  targetCols: number,
  targetRows: number,
  width: number
) {
  fitAddon.proposeDimensions.mockReturnValue({ cols: targetCols, rows: targetRows });
  fireResizeAndStartSampler(ro, width);
  act(() => {
    jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
  });
}

describe("shouldScheduleFit (pure)", () => {
  // Task 4.1.3(a): sub-cell jitter -- proposed reverts to applied on the very
  // next tick, never commits.
  it("returns schedule:false and stores nextPending when a sub-cell-jitter candidate reverts to applied on the next tick", () => {
    const applied = { cols: 80, rows: 24 };
    const tick1 = shouldScheduleFit({ cols: 81, rows: 24 }, applied, null);
    expect(tick1).toEqual({ schedule: false, nextPending: { cols: 81, rows: 24 } });

    const tick2 = shouldScheduleFit({ cols: 80, rows: 24 }, applied, tick1.nextPending);
    expect(tick2).toEqual({ schedule: false, nextPending: null });
  });

  // Task 4.1.3(c): genuine convergent resize -- differs from applied and
  // repeats unchanged on the immediate next call.
  it("returns schedule:true only on the second consecutive tick when a genuine resize candidate repeats", () => {
    const applied = { cols: 80, rows: 24 };
    const proposed = { cols: 100, rows: 30 };

    const tick1 = shouldScheduleFit(proposed, applied, null);
    expect(tick1.schedule).toBe(false);
    expect(tick1.nextPending).toEqual(proposed);

    const tick2 = shouldScheduleFit(proposed, applied, tick1.nextPending);
    expect(tick2).toEqual({ schedule: true, nextPending: null });
  });

  // Task 4.1.3(b): boundary-flapping / sustained oscillation -- 20 successive
  // calls with never-repeating candidates, schedule:false on every one.
  it("returns schedule:false on every call for a boundary-flapping candidate that never repeats across 20 samples", () => {
    const applied = { cols: 80, rows: 24 };
    let pending: { cols: number; rows: number } | null = null;

    for (let i = 0; i < 20; i++) {
      // Alternates 78/79 in a way that never matches the immediately
      // preceding pending value: each call is compared against the previous
      // tick's nextPending, and since 78 !== 79 always, no tick ever repeats.
      const proposed = { cols: i % 2 === 0 ? 78 : 79, rows: 24 };
      const result = shouldScheduleFit(proposed, applied, pending);
      expect(result.schedule).toBe(false);
      pending = result.nextPending;
    }
  });
});

describe("isSustainedMismatch (pure)", () => {
  it("returns false via the Number.isFinite guard for isSustainedMismatch(Infinity, 8.0, 1) and isSustainedMismatch(9.2, NaN, 1)", () => {
    expect(isSustainedMismatch(Infinity, 8.0, 1)).toBe(false);
    expect(isSustainedMismatch(9.2, NaN, 1)).toBe(false);
  });

  it("returns true when the absolute mismatch exceeds tolerance and false when within it", () => {
    expect(isSustainedMismatch(9.2, 8.0, 1)).toBe(true);
    expect(isSustainedMismatch(8.5, 8.0, 1)).toBe(false);
  });
});

describe("extractCellMismatchInputs (pure-ish, DOM+object inputs)", () => {
  it("returns null when the renderer has not measured cell dimensions yet", () => {
    const terminal = { cols: 80, _core: { _renderService: { dimensions: undefined } } } as any;
    const el = document.createElement("div");
    expect(extractCellMismatchInputs(terminal, el)).toBeNull();
  });
});

describe("XtermTerminal sampler (component + fake timers)", () => {
  // Task 4.1.4, Happy path row (AC1): unchanged contentRect -> no fit(), no
  // sampler start at all (widthChanged/heightChanged both false since RO's
  // internal lastContainerSize starts at {0,0}... note a truly "unchanged"
  // delivery relative to *mount* is exercised by never firing RO at all; this
  // test instead exercises a converge-to-noop tick pair which is the sampler
  // -level unchanged case per shouldScheduleFit's own contract).
  it("calls fit() 0 times when the sampler's proposed dimensions already equal the applied dimensions", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    fitAddon.proposeDimensions.mockReturnValue({ cols: terminal.cols, rows: terminal.rows });

    fireResizeAndStartSampler(ro, 800);
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
    });

    expect(fitAddon.fit).not.toHaveBeenCalled();
  });

  // Task 4.1.4 (edge path): 20-tick never-repeating oscillation past
  // MAX_SAMPLES, then re-arm and converge on a genuinely new resize.
  it("stops the sampler and warns after MAX_SAMPLES=20 ticks without confirmation, then re-arms and converges on the next distinct resize", async () => {
    const { fitAddon, ro } = await mountAndSettle();
    const warnSpy = console.warn as jest.Mock;

    let callCount = 0;
    fitAddon.proposeDimensions.mockImplementation(() => {
      callCount += 1;
      if (callCount <= MAX_SAMPLES) {
        // Strictly monotonic -> never equals the immediately preceding
        // tick's pending value, so shouldScheduleFit never returns true.
        return { cols: 80 + callCount, rows: 24 };
      }
      return { cols: 90, rows: 30 };
    });

    fireResizeAndStartSampler(ro, 800); // tick 1 (synchronous inside the debounce timeout)
    for (let i = 0; i < MAX_SAMPLES - 1; i++) {
      act(() => {
        jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
      });
    }

    expect(fitAddon.fit).not.toHaveBeenCalled();
    const convergeWarnings = warnSpy.mock.calls.filter((c) => /did not converge/.test(String(c[0])));
    expect(convergeWarnings).toHaveLength(1);
    expect(jest.getTimerCount()).toBe(0);

    // Re-arm: a genuinely new resize converges cleanly on its next two ticks.
    fireResizeAndStartSampler(ro, 900); // tick 21 (call 21 -> {90,30}, pending set)
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS); // tick 22 (call 22 -> {90,30} again -> confirms)
    });

    expect(fitAddon.fit).toHaveBeenCalledTimes(1);
  });
});

describe("XtermTerminal WebGL->Canvas fallback (mocked renderer, not real WebGL)", () => {
  function setMismatch(terminal: Awaited<ReturnType<typeof mountAndSettle>>["terminal"], actualPxPerCol: number, expectedPxPerCol: number) {
    terminal._core._renderService.dimensions = { css: { cell: { width: expectedPxPerCol } } };
    if (terminal.containerEl) {
      terminal.containerEl.getBoundingClientRect = jest.fn(
        () =>
          ({
            width: actualPxPerCol * terminal.cols,
            height: 480,
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            x: 0,
            y: 0,
            toJSON() {},
          }) as DOMRect
      );
    }
  }

  it("trips the canvas fallback exactly once after 3 consecutive mismatch samples exceeding MISMATCH_TOLERANCE_PX (mocked renderer, not real WebGL)", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    setMismatch(terminal, 9.2, 8.0);

    driveConvergentResize(ro, fitAddon, 81, 24, 800);
    driveConvergentResize(ro, fitAddon, 82, 24, 900);
    driveConvergentResize(ro, fitAddon, 83, 24, 1000);

    expect(fitAddon.fit).toHaveBeenCalledTimes(3);
    const webglInstance = MockWebglAddon.instances[MockWebglAddon.instances.length - 1];
    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(MockCanvasAddon.instances).toHaveLength(1);
    expect(terminal.loadedAddons).toContain(MockCanvasAddon.instances[0]);

    // After one RAF, proposeDimensions() is checked via isFiniteResizeDimensions()
    // before the post-fallback fit() runs.
    fitAddon.proposeDimensions.mockReturnValue({ cols: 83, rows: 24 });
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
    });
    expect(fitAddon.fit).toHaveBeenCalledTimes(4);
  });

  // Guards the fix for the semantic bug where webglMismatchCount was
  // cumulative instead of consecutive: a clean (non-mismatching) sample
  // between two mismatching ones must reset the counter to 0, so the
  // fallback does NOT trip on "2 mismatches with a clean sample between
  // them" -- only on genuinely consecutive mismatches. Limited to 3 RO
  // deliveries (matching every other test in this file, per
  // fireResizeAndStartSampler's <=3 adaptive-debounce assumption).
  it("resets the consecutive mismatch count on a clean sample, so the fallback does not trip on 2 mismatches separated by a clean sample", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();

    // Cycle 1: mismatch (count -> 1).
    setMismatch(terminal, 9.2, 8.0);
    driveConvergentResize(ro, fitAddon, 81, 24, 800);

    // Cycle 2: clean sample resets the count back to 0.
    setMismatch(terminal, 8.0, 8.0);
    driveConvergentResize(ro, fitAddon, 82, 24, 900);

    // Cycle 3: mismatch again -- count restarts at 1, not 3, so no trip.
    setMismatch(terminal, 9.2, 8.0);
    driveConvergentResize(ro, fitAddon, 83, 24, 1000);

    expect(fitAddon.fit).toHaveBeenCalledTimes(3);
    expect(MockCanvasAddon.instances).toHaveLength(0);
    const webglInstance = MockWebglAddon.instances[MockWebglAddon.instances.length - 1];
    expect(webglInstance.dispose).not.toHaveBeenCalled();
  });

  it("does not call dispose()/loadAddon() again on a 4th mismatch sample once the fallback is already triggered", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    setMismatch(terminal, 9.2, 8.0);

    driveConvergentResize(ro, fitAddon, 81, 24, 800);
    driveConvergentResize(ro, fitAddon, 82, 24, 900);
    driveConvergentResize(ro, fitAddon, 83, 24, 1000);

    // Flush the post-fallback RAF before continuing, so it doesn't bleed
    // into the 4th cycle's own timer budget.
    fitAddon.proposeDimensions.mockReturnValue({ cols: 83, rows: 24 });
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
    });

    const webglInstance = MockWebglAddon.instances[MockWebglAddon.instances.length - 1];
    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(MockCanvasAddon.instances).toHaveLength(1);

    driveConvergentResize(ro, fitAddon, 84, 24, 1100);

    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(MockCanvasAddon.instances).toHaveLength(1);
  });

  it("falls through to xterm's built-in DOM renderer without crashing when CanvasAddon also fails to load", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    setMismatch(terminal, 9.2, 8.0);
    MockCanvasAddon.shouldThrow = true;
    const errorSpy = console.error as jest.Mock;

    expect(() => {
      driveConvergentResize(ro, fitAddon, 81, 24, 800);
      driveConvergentResize(ro, fitAddon, 82, 24, 900);
      driveConvergentResize(ro, fitAddon, 83, 24, 1000);
    }).not.toThrow();

    expect(fitAddon.fit).toHaveBeenCalledTimes(3);
    const canvasErrors = errorSpy.mock.calls.filter((c) => /Canvas renderer also failed/.test(String(c[0])));
    expect(canvasErrors).toHaveLength(1);
    expect(MockCanvasAddon.instances).toHaveLength(0);

    // The latch stays tripped -- fit() is not called again for this failed
    // attempt (no RAF-guarded post-fallback fit runs, since the addon never
    // loaded).
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
    });
    expect(fitAddon.fit).toHaveBeenCalledTimes(3);
  });

  it("skips the post-fallback fit() and warns when proposeDimensions() is not finite after the RAF-guarded CanvasAddon swap", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    setMismatch(terminal, 9.2, 8.0);

    driveConvergentResize(ro, fitAddon, 81, 24, 800);
    driveConvergentResize(ro, fitAddon, 82, 24, 900);
    driveConvergentResize(ro, fitAddon, 83, 24, 1000);

    expect(fitAddon.fit).toHaveBeenCalledTimes(3);

    fitAddon.proposeDimensions.mockReturnValue(undefined);
    const warnSpy = console.warn as jest.Mock;
    act(() => {
      jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
    });

    expect(fitAddon.fit).toHaveBeenCalledTimes(3);
    const skipWarnings = warnSpy.mock.calls.filter((c) => /Skipped post-fallback fit/.test(String(c[0])));
    expect(skipWarnings).toHaveLength(1);
  });

  it("routes onContextLoss through the same latch as the mismatch-threshold path, without double-disposing when both fire", async () => {
    const { fitAddon, ro, terminal } = await mountAndSettle();
    setMismatch(terminal, 9.2, 8.0);

    driveConvergentResize(ro, fitAddon, 81, 24, 800);
    driveConvergentResize(ro, fitAddon, 82, 24, 900);

    const webglInstance = MockWebglAddon.instances[MockWebglAddon.instances.length - 1];
    // Fire onContextLoss before the 3rd mismatch sample would otherwise trip it.
    act(() => {
      (webglInstance as any)._contextLossCb();
    });

    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(MockCanvasAddon.instances).toHaveLength(1);

    // The 3rd mismatch sample's own trigger attempt short-circuits on the
    // latch -- no second dispose()/loadAddon().
    driveConvergentResize(ro, fitAddon, 83, 24, 1000);

    expect(webglInstance.dispose).toHaveBeenCalledTimes(1);
    expect(MockCanvasAddon.instances).toHaveLength(1);
  });
});

describe("XtermTerminal cross-instance perturbation (multi-mount, bounded settling)", () => {
  // Task 4.1.6 (AC1 cross-instance-perturbation gap): 3 instances share one
  // parent; instance 0's fit() perturbs siblings' proposed dimensions,
  // simulating a shared-layout side effect. Total fit() calls across all
  // instances must settle to a bounded ceiling, not grow unboundedly.
  it("sums fit() calls to a bounded total across 3 same-parent instances when instance 0's fit() perturbs the shared layout", async () => {
    const refs = [React.createRef<XtermTerminalHandle>(), React.createRef<XtermTerminalHandle>(), React.createRef<XtermTerminalHandle>()];
    render(
      <div data-testid="shared-parent">
        <XtermTerminal ref={refs[0]} />
        <XtermTerminal ref={refs[1]} />
        <XtermTerminal ref={refs[2]} />
      </div>
    );
    await act(async () => {
      jest.advanceTimersByTime(200);
      // Flush each instance's dynamic import('@xterm/addon-webgl') microtask chain.
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(MockFitAddon.instances).toHaveLength(3);
    expect(capturedResizeObservers).toHaveLength(3);

    const [fit0, fit1, fit2] = MockFitAddon.instances;
    const [ro0, ro1, ro2] = capturedResizeObservers;
    [fit0, fit1, fit2].forEach((f) => {
      f.fit.mockClear();
      f.proposeDimensions.mockClear();
    });

    fit1.proposeDimensions.mockReturnValue({ cols: 101, rows: 30 });
    fit2.proposeDimensions.mockReturnValue({ cols: 102, rows: 30 });

    // Instance 0's fit() perturbs siblings' proposed dimensions once, as a
    // stand-in for "instance 0 fitting shrank the shared parent, changing
    // what its siblings should now propose."
    fit0.proposeDimensions.mockReturnValue({ cols: 90, rows: 30 });
    fit0.fit.mockImplementation(() => {
      fit1.proposeDimensions.mockReturnValue({ cols: 111, rows: 30 });
      fit2.proposeDimensions.mockReturnValue({ cols: 112, rows: 30 });
    });

    // Fire a shared-parent resize reaching all 3 siblings simultaneously.
    act(() => {
      ro0.callback([{ contentRect: { width: 800, height: 480 } }]);
      ro1.callback([{ contentRect: { width: 800, height: 480 } }]);
      ro2.callback([{ contentRect: { width: 800, height: 480 } }]);
    });
    act(() => {
      jest.advanceTimersByTime(151); // flat 150ms debounce -> tick 1 for all 3 (registers pending)
    });
    // Bounded settling window: give the perturbed siblings a few extra ticks
    // to re-converge after instance 0's mutation, but never run unbounded.
    for (let i = 0; i < 4; i++) {
      act(() => {
        jest.advanceTimersByTime(SAMPLE_INTERVAL_MS);
      });
    }

    const totalFitCalls = fit0.fit.mock.calls.length + fit1.fit.mock.calls.length + fit2.fit.mock.calls.length;
    expect(fit0.fit).toHaveBeenCalledTimes(1);
    expect(totalFitCalls).toBeLessThanOrEqual(6);
    expect(jest.getTimerCount()).toBe(0);
  });
});

// ---- Basic render smoke tests (from origin/main's XtermTerminal.test.tsx) ----
// render/act and XtermTerminal are already imported above; reuses this file's
// module-boundary mocks rather than re-declaring them.
describe('XtermTerminal', () => {
  test('renders without error', () => {
    const { container } = render(<XtermTerminal />);
    expect(container.firstChild).toBeInTheDocument();
  });

  // mouseTracking prop removed (X.1): mouse tracking mode is set at runtime by PTY escape
  // sequences and read via terminal.modes.mouseTrackingMode — not configurable via prop.
  test('renders with only valid props', () => {
    const { container } = render(<XtermTerminal fontSize={14} scrollback={5000} />);
    expect(container.firstChild).toBeInTheDocument();
  });

  test('renders with default props', () => {
    const { container } = render(<XtermTerminal />);
    expect(container.firstChild).toBeInTheDocument();
  });
});
