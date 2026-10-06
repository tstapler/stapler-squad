/**
 * refit() / repaint-seam tests for XtermTerminal (Story 2.1.2): every refit()
 * repaints, including at unchanged dims, the zero-size retry is bounded and
 * non-silent, and a restore after zero-size repaints even at an identical size.
 * Fake timers only; no unbounded drain-the-queue APIs (see XtermTerminal.test.tsx).
 */
import { render, act } from "@testing-library/react";
import React from "react";

jest.mock("@xterm/xterm", () => require("./xtermRefitMocks").xtermModule());
jest.mock("@xterm/addon-fit", () => require("./xtermRefitMocks").fitModule());
jest.mock("@xterm/addon-webgl", () => require("./xtermRefitMocks").webglModule());
jest.mock("@xterm/addon-canvas", () => require("./xtermRefitMocks").canvasModule());
jest.mock("@xterm/addon-search", () => require("./xtermRefitMocks").searchModule());
jest.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));
jest.mock("@/lib/terminal/mobileDebug", () => ({ mobileDebug: { log: jest.fn() } }));
jest.mock("@/lib/hooks/useTerminalGestures", () => ({ useTerminalGestures: jest.fn() }));

import { XtermTerminal, SAMPLE_INTERVAL_MS, MAX_SAMPLES, type XtermTerminalHandle, type XtermTerminalProps } from "../XtermTerminal";
import { useTerminalGestures } from "@/lib/hooks/useTerminalGestures";
import { created, resetCreated } from "./xtermRefitMocks";
import { createViewportSettle, createRafScheduler, type ViewportSource } from "@/lib/terminal/viewportSettle";
import { mobileDebug } from "@/lib/terminal/mobileDebug";

type RO = { callback: (entries: Array<{ contentRect: { width: number; height: number } }>) => void };
let observers: RO[];
const size = { w: 320, h: 400 };

beforeEach(() => {
  jest.useFakeTimers();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
  (mobileDebug.log as jest.Mock).mockClear();
  observers = [];
  (global as any).ResizeObserver = class {
    constructor(cb: RO["callback"]) {
      observers.push({ callback: cb });
    }
    observe = jest.fn();
    unobserve = jest.fn();
    disconnect = jest.fn();
  };
  (global as any).WebGL2RenderingContext = class {};
  size.w = 320;
  size.h = 400;
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get: () => size.w });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", { configurable: true, get: () => size.h });
  resetCreated();
});

afterEach(() => {
  delete (HTMLElement.prototype as any).clientWidth;
  delete (HTMLElement.prototype as any).clientHeight;
  delete (window as any).visualViewport;
  jest.restoreAllMocks();
  jest.useRealTimers();
});

async function mount(props: Partial<XtermTerminalProps> = {}) {
  const ref = React.createRef<XtermTerminalHandle>();
  const utils = render(<XtermTerminal ref={ref} {...props} />);
  await act(async () => {
    jest.advanceTimersByTime(200);
    await Promise.resolve();
    await Promise.resolve();
  });
  const terminal = created.terminals[created.terminals.length - 1];
  const fit = created.fits[created.fits.length - 1];
  const ro = observers[observers.length - 1];
  fit.proposeDimensions.mockImplementation(() => ({ cols: terminal.cols, rows: terminal.rows }));
  fit.fit.mockImplementation(() => {
    const p = fit.proposeDimensions();
    if (p) Object.assign(terminal, p);
  });
  fit.fit.mockClear();
  fit.proposeDimensions.mockClear();
  terminal.refresh.mockClear();
  terminal.clearTextureAtlas.mockClear();
  return { ref, terminal, fit, ro, ...utils };
}

const advance = (ms: number) => act(() => void jest.advanceTimersByTime(ms));
const deliver = (ro: RO, width: number, height: number) => act(() => ro.callback([{ contentRect: { width, height } }]));

describe("sampler repaint branches", () => {
  it("sampler_should_RepaintOnce_When_ScheduleBranchFits", async () => {
    const { ref, terminal, fit } = await mount();
    fit.proposeDimensions.mockReturnValue({ cols: 100, rows: 30 });
    act(() => ref.current!.refit());
    advance(SAMPLE_INTERVAL_MS);
    expect(fit.fit).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledWith(0, 29);
  });

  it("refit_should_CallRefreshOnceAndNotFit_When_ProposedDimsEqualApplied", async () => {
    const { ref, terminal, fit } = await mount();
    act(() => ref.current!.refit());
    expect(fit.fit).not.toHaveBeenCalled();
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledWith(0, 23);
    advance(SAMPLE_INTERVAL_MS * 3); // sampler stopped: no further ticks or repaints
    expect(fit.proposeDimensions).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
  });

  it("refit_should_RepaintWhenSamplerAlreadyActive", async () => {
    const { ref, terminal, fit, ro } = await mount();
    fit.proposeDimensions.mockReturnValue({ cols: 90, rows: 30 });
    deliver(ro, 800, 480);
    advance(151); // RO debounce -> tick 1 registers pending; sampler active
    fit.proposeDimensions.mockReturnValue({ cols: 80, rows: 24 });
    act(() => ref.current!.refit()); // request must not be dropped
    advance(SAMPLE_INTERVAL_MS); // tick 2: at rest
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledWith(0, 23);
  });

  it("refit_should_RunFirstTickAtZeroMs_When_Called", async () => {
    const { ref, fit } = await mount();
    act(() => ref.current!.refit());
    expect(fit.proposeDimensions).toHaveBeenCalledTimes(1); // no 150 ms RO debounce
  });

  it("sampler_should_NotRepaint_When_RoDrivenAtRest", async () => {
    const { terminal, fit, ro } = await mount();
    deliver(ro, 800, 480);
    advance(151 + SAMPLE_INTERVAL_MS);
    expect(fit.proposeDimensions).toHaveBeenCalled();
    expect(terminal.refresh).not.toHaveBeenCalled();
  });

  it("sampler_should_RepaintOnGiveUp_When_RepaintRequested", async () => {
    const { ref, terminal, fit } = await mount();
    let n = 0;
    fit.proposeDimensions.mockImplementation(() => ({ cols: 90 + n++, rows: 30 })); // never repeats
    act(() => ref.current!.refit());
    advance(SAMPLE_INTERVAL_MS * (MAX_SAMPLES + 1));
    expect(fit.fit).not.toHaveBeenCalled();
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
  });
});

describe("refit onFitted completion callback (Story 2.1.3b)", () => {
  it("refit_should_InvokeOnFitted_AfterFitCompletes_WithFinalDims", async () => {
    const { ref, fit } = await mount();
    const onFitted = jest.fn();
    fit.proposeDimensions.mockReturnValue({ cols: 100, rows: 30 });
    act(() => ref.current!.refit({ reason: "manual-resize", onFitted }));
    expect(onFitted).not.toHaveBeenCalled(); // first sample only registers the pending dims
    advance(SAMPLE_INTERVAL_MS);
    expect(fit.fit).toHaveBeenCalledTimes(1);
    expect(onFitted).toHaveBeenCalledTimes(1);
    expect(onFitted).toHaveBeenCalledWith({ cols: 100, rows: 30 });
    advance(SAMPLE_INTERVAL_MS * 3);
    expect(onFitted).toHaveBeenCalledTimes(1);
  });

  it("refit_should_InvokeOnFitted_WithUnchangedDims_When_ProposedEqualsApplied", async () => {
    const { ref, fit } = await mount();
    const onFitted = jest.fn();
    act(() => ref.current!.refit({ onFitted }));
    expect(fit.fit).not.toHaveBeenCalled();
    expect(onFitted).toHaveBeenCalledTimes(1);
    expect(onFitted).toHaveBeenCalledWith({ cols: 80, rows: 24 });
  });

  it("refit_should_InvokeOnFittedOnce_When_SamplerGivesUp", async () => {
    const { ref, fit } = await mount();
    const onFitted = jest.fn();
    let n = 0;
    fit.proposeDimensions.mockImplementation(() => ({ cols: 90 + n++, rows: 30 }));
    act(() => ref.current!.refit({ onFitted }));
    expect(onFitted).not.toHaveBeenCalled();
    advance(SAMPLE_INTERVAL_MS * (MAX_SAMPLES + 1));
    expect(onFitted).toHaveBeenCalledTimes(1);
    expect(onFitted).toHaveBeenCalledWith({ cols: 80, rows: 24 });
  });

  it("refit_should_InvokeOnFittedOnce_When_ZeroSizeRetryExhausted", async () => {
    const { ref } = await mount();
    const onFitted = jest.fn();
    size.w = 0;
    size.h = 0;
    act(() => ref.current!.refit({ onFitted }));
    advance(50);
    expect(onFitted).not.toHaveBeenCalled();
    advance(1000);
    expect(onFitted).toHaveBeenCalledTimes(1);
    // stale: nothing was fitted, so these are the pre-hide dims and callers must not act on them as a fresh fit
    expect(onFitted).toHaveBeenCalledWith({ cols: 80, rows: 24, stale: true });
  });

  it("refit_should_InvokeEveryCoalescedOnFitted_When_RequestsOverlap", async () => {
    const { ref, fit, ro } = await mount();
    const first = jest.fn();
    const second = jest.fn();
    fit.proposeDimensions.mockReturnValue({ cols: 90, rows: 30 });
    deliver(ro, 800, 480);
    advance(151); // sampler active, pending registered
    act(() => ref.current!.refit({ onFitted: first }));
    act(() => ref.current!.refit({ onFitted: second }));
    advance(SAMPLE_INTERVAL_MS * 2);
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);
  });
});

describe("zero-size, restore, renderer and viewport handling", () => {
  it("refit_should_WarnRepaintAndSetPendingRefit_When_ContainerZeroFor20RafOr1000Ms", async () => {
    const { ref, terminal, fit, ro } = await mount();
    size.w = 0;
    size.h = 0;
    act(() => ref.current!.refit());
    advance(1000);
    expect((console.warn as jest.Mock).mock.calls.filter((c) => /stayed zero-size/.test(String(c[0])))).toHaveLength(1);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(fit.fit).not.toHaveBeenCalled();

    const rafSpy = jest.spyOn(window, "requestAnimationFrame");
    advance(500);
    expect(rafSpy).not.toHaveBeenCalled(); // no further rAF after exhaustion

    // pendingRefit: the observer's next non-zero delivery re-issues the request, once.
    size.w = 320;
    size.h = 400;
    deliver(ro, 320, 400);
    expect(terminal.refresh).toHaveBeenCalledTimes(2);
    advance(500);
    deliver(ro, 320, 400);
    expect(terminal.refresh).toHaveBeenCalledTimes(2);
  });

  it("refit_should_NotRenderErrorUi_When_RetryExhausted", async () => {
    const { ref, container } = await mount();
    size.w = 0;
    size.h = 0;
    act(() => ref.current!.refit());
    advance(1000);
    expect(document.querySelector('[role="alert"]')).toBeNull();
    expect(container.textContent ?? "").not.toMatch(/error|failed|retry/i);
  });

  it("refit_should_RepaintAndClearPending_When_ObserverRestoresIdenticalSize", async () => {
    const { terminal, ro } = await mount();
    deliver(ro, 320, 400);
    advance(151 + SAMPLE_INTERVAL_MS); // lastContainerSize = 320x400, sampler at rest
    terminal.refresh.mockClear();
    size.w = 0;
    size.h = 0;
    deliver(ro, 0, 0);
    size.w = 320;
    size.h = 400;
    deliver(ro, 320, 400); // identical to lastContainerSize: dedupe must not swallow the repaint
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledWith(0, 23);
  });

  it("onContextLoss_should_FallBackAndRepaint", async () => {
    const { terminal } = await mount();
    act(() => created.webgls[0].contextLossCb());
    expect(created.canvases).toHaveLength(1);
    advance(16);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.clearTextureAtlas).not.toHaveBeenCalled(); // renderer is canvas by now
    expect(mobileDebug.log).toHaveBeenCalledWith(
      "forced-refresh",
      expect.objectContaining({ renderer: "canvas", reason: "context-loss" })
    );
  });

  it("refit_should_RefreshOnce_When_CanvasRendererAndDimsUnchanged", async () => {
    delete (global as any).WebGL2RenderingContext;
    const { ref, terminal } = await mount();
    act(() => ref.current!.refit());
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledWith(0, 23);
    expect(terminal.clearTextureAtlas).not.toHaveBeenCalled();
  });

  it("refit_should_ClearAtlasOnce_When_WebglRendererAndDimsUnchanged", async () => {
    const { ref, terminal } = await mount();
    act(() => ref.current!.refit());
    expect(terminal.clearTextureAtlas).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
  });

  it("refit_should_LogViewportVsContainerMismatch_When_ContainerDoesNotTrackVisualViewport", async () => {
    const { ref } = await mount();
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: { height: 500, offsetTop: 0, addEventListener() {}, removeEventListener() {} },
    });
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      top: 0, bottom: 900, left: 0, right: 320, width: 320, height: 900, x: 0, y: 0, toJSON: () => ({}),
    });
    act(() => ref.current!.refit());
    expect((console.warn as jest.Mock).mock.calls.some((c) => /does not track visualViewport/.test(String(c[0])))).toBe(true);
    expect(mobileDebug.log).toHaveBeenCalledWith("refit-viewport", expect.objectContaining({ vvHeight: 500 }));
  });

  it("refit_should_FireOncePerSettle_When_NRapidViewportResizes", async () => {
    const { ref, terminal } = await mount();
    const listeners = new Set<() => void>();
    const vv = {
      height: 800,
      offsetTop: 0,
      addEventListener: (_t: string, l: () => void) => void listeners.add(l),
      removeEventListener: (_t: string, l: () => void) => void listeners.delete(l),
    } as unknown as ViewportSource;
    createViewportSettle(vv, createRafScheduler(), {
      stableFrames: 3,
      maxWaitMs: 600,
      onSettled: () => ref.current!.refit(),
    });
    act(() => {
      for (let i = 0; i < 10; i++) listeners.forEach((l) => l());
    });
    advance(16 * 6);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
  });

  it("refit_should_RefreshAfterEveryCycle_When_200AlternatingResizeRefitCycles", async () => {
    const { ref, terminal, fit } = await mount();
    let desired: { cols: number; rows: number } = { cols: 80, rows: 24 };
    fit.proposeDimensions.mockImplementation(() => desired);
    for (let i = 0; i < 200; i++) {
      desired = i % 2 === 0 ? { cols: terminal.cols, rows: terminal.rows } : { cols: 60 + (i % 40), rows: 20 + (i % 7) };
      const before = terminal.refresh.mock.calls.length;
      act(() => ref.current!.refit());
      advance(SAMPLE_INTERVAL_MS * 2);
      expect(terminal.refresh.mock.calls.length).toBe(before + 1);
      const last = terminal.refresh.mock.calls[before];
      expect(last).toEqual([0, terminal.rows - 1]);
      advance(20);
    }
  });
});

describe("former bare fit() call sites route through refit (Story 2.1.3c)", () => {
  it("fontSizeChange_should_RepaintViaRefit_When_FontSizePropChanges", async () => {
    const { rerender, terminal, fit } = await mount();
    fit.proposeDimensions.mockReturnValue({ cols: 100, rows: 30 });
    rerender(<XtermTerminal fontSize={20} />);
    expect(fit.fit).not.toHaveBeenCalled(); // deferred, not synchronous
    advance(0);
    advance(SAMPLE_INTERVAL_MS);
    expect(fit.fit).toHaveBeenCalledTimes(1);
    expect(terminal.refresh).toHaveBeenCalledTimes(1);
  });

  it("fitHandle_should_AliasRefit_When_Called", async () => {
    const { ref, terminal, fit } = await mount();
    act(() => ref.current!.fit());
    expect(terminal.refresh).toHaveBeenCalledTimes(1); // refit repaints even at unchanged dims
    expect(fit.fit).not.toHaveBeenCalled();
  });
});

describe("scroll settings wiring (Stories 1.2.5e / 1.2.5g)", () => {
  const fireWriteParsed = (terminal: any) => act(() => terminal.writeParsedCallbacks.forEach((cb: () => void) => cb()));
  const fireBufferChange = (terminal: any) => act(() => terminal.bufferChangeCallbacks.forEach((cb: () => void) => cb()));

  it("XtermTerminal_should_ReportScrollModeOnlyOnChange", async () => {
    const onScrollModeChange = jest.fn();
    const { terminal } = await mount({ onScrollModeChange });

    fireWriteParsed(terminal); // still normal / none: nothing to report
    expect(onScrollModeChange).not.toHaveBeenCalled();

    terminal.buffer.active.type = "alternate";
    fireBufferChange(terminal);
    expect(onScrollModeChange).toHaveBeenCalledTimes(1);
    expect(onScrollModeChange).toHaveBeenLastCalledWith({ bufferType: "alternate", mouseTrackingMode: "none" });

    fireWriteParsed(terminal); // unchanged: no repeat
    fireBufferChange(terminal);
    expect(onScrollModeChange).toHaveBeenCalledTimes(1);

    terminal.modes = { mouseTrackingMode: "any" }; // mode changes arrive through writes
    fireWriteParsed(terminal);
    expect(onScrollModeChange).toHaveBeenCalledTimes(2);
    expect(onScrollModeChange).toHaveBeenLastCalledWith({ bufferType: "alternate", mouseTrackingMode: "any" });
  });

  it("XtermTerminal_should_PassOverrideAndGestureFlagToHook", async () => {
    const hook = useTerminalGestures as jest.Mock;
    const scrollGesture = { scrollOverride: "tui", gestureScrollEnabled: false, tuiScrollPolicy: "wheel", connectionEpoch: 3 } as const;
    const { rerender } = await mount({ scrollGesture });
    expect(hook).toHaveBeenLastCalledWith(
      expect.objectContaining({ override: "tui", gestureScrollEnabled: false, tuiScrollPolicy: "wheel", connectionEpoch: 3 }),
    );

    const terminalBefore = created.terminals.length;
    rerender(<XtermTerminal scrollGesture={{ scrollOverride: "local", gestureScrollEnabled: true, connectionEpoch: 4 }} />);
    expect(hook).toHaveBeenLastCalledWith(expect.objectContaining({ override: "local", gestureScrollEnabled: true, connectionEpoch: 4 }));
    expect(created.terminals.length).toBe(terminalBefore); // no remount
  });

  it("XtermTerminal_should_PassGestureObservationCallbacksToHook", async () => {
    const hook = useTerminalGestures as jest.Mock;
    const cb = { onScrollStart: jest.fn(), onScrollGesture: jest.fn(), onPageKeysSent: jest.fn(), onGestureActiveChange: jest.fn() };
    await mount({ scrollGesture: cb });
    expect(hook).toHaveBeenLastCalledWith(expect.objectContaining(cb));
  });

  it("XtermTerminal_should_PassIsInputBusyToHook", async () => {
    const hook = useTerminalGestures as jest.Mock;
    const isInputBusy = jest.fn().mockReturnValue(true);
    await mount({ scrollGesture: { isInputBusy } });
    expect(hook).toHaveBeenLastCalledWith(expect.objectContaining({ isInputBusy }));
  });

  it("XtermTerminal_should_RouteHookSendsToProgrammaticSink_When_Provided", async () => {
    const hook = useTerminalGestures as jest.Mock;
    const onData = jest.fn();
    const onProgrammaticData = jest.fn();
    await mount({ onData, scrollGesture: { onProgrammaticData } });
    const { onSendData } = hook.mock.calls[hook.mock.calls.length - 1][0];
    onSendData("\x1b[5~");
    expect(onProgrammaticData).toHaveBeenCalledWith("\x1b[5~");
    expect(onData).not.toHaveBeenCalled();
  });

  it("XtermTerminal_should_RouteHookSendsToOnData_When_NoProgrammaticSink", async () => {
    const hook = useTerminalGestures as jest.Mock;
    const onData = jest.fn();
    await mount({ onData });
    hook.mock.calls[hook.mock.calls.length - 1][0].onSendData("x");
    expect(onData).toHaveBeenCalledWith("x");
  });

  it("XtermTerminal_should_SetDataGestureScrollAttribute", async () => {
    const { container, rerender } = await mount();
    const surface = () => container.querySelector("[data-gesture-scroll]");
    expect(surface()?.getAttribute("data-gesture-scroll")).toBe("on"); // default
    rerender(<XtermTerminal scrollGesture={{ gestureScrollEnabled: false }} />);
    expect(surface()?.getAttribute("data-gesture-scroll")).toBe("off");
  });
});
