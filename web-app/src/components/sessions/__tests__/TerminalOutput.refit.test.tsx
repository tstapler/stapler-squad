// @feature terminal-reconnect
/**
 * Stories 2.1.3 / 2.1.4c / 2.1.5c: TerminalOutput routes every resize-driven fit through
 * xtermRef.refit(); the old 400 ms onVpResize/isFittingRef pipeline and the 50 ms
 * visibility setTimeout(fit) are gone. Fake timers only.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";

const mockXtermHandle = {
  terminal: null as null,
  fit: jest.fn(),
  refit: jest.fn(),
  write: jest.fn(),
  writeln: jest.fn(),
  clear: jest.fn(),
  focus: jest.fn(),
  serializeAddon: null,
  search: jest.fn().mockReturnValue(false),
  searchNext: jest.fn().mockReturnValue(false),
  searchPrevious: jest.fn().mockReturnValue(false),
};

let capturedXtermProps: any = null;

jest.mock("../XtermTerminal", () => {
  const React = require("react");
  const XtermTerminal = React.forwardRef((props: any, ref: any) => {
    capturedXtermProps = props;
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return React.createElement("div", { "data-testid": "mock-xterm" });
  });
  XtermTerminal.displayName = "XtermTerminal";
  return { XtermTerminal };
});

jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: jest.fn(),
}));

jest.mock("@/lib/terminal/TerminalDimensionCache", () =>
  require("./terminalOutputTestMocks").terminalDimensionCacheWithValidationMockModule()
);
jest.mock("@/lib/terminal/TerminalStreamManager", () =>
  require("./terminalOutputTestMocks").terminalStreamManagerWithSerializeAddonMockModule()
);
jest.mock("@/lib/contexts/AnalyticsContext", () =>
  require("./terminalOutputTestMocks").analyticsContextPlainMockModule()
);
jest.mock("@/lib/contexts/ApprovalsContext", () =>
  require("./terminalOutputTestMocks").approvalsContextMockModule()
);
jest.mock("@/lib/hooks/useHandedness", () =>
  require("./terminalOutputTestMocks").handednessMockModule()
);
jest.mock("@/lib/hooks/useSplitContainerSize", () =>
  require("./terminalOutputTestMocks").splitContainerSizeMockModule()
);
jest.mock("@/lib/hooks/useBrowserLogStream", () =>
  require("./terminalOutputTestMocks").browserLogStreamMockModule()
);
jest.mock("@/components/providers/ViewportProvider", () =>
  require("./terminalOutputTestMocks").viewportProviderDesktopMockModule()
);

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
// eslint-disable-next-line import/first
import { TerminalStreamManager } from "@/lib/terminal/TerminalStreamManager";

function makeStreamMock(overrides: Record<string, unknown> = {}) {
  return {
    isConnected: true,
    error: null,
    output: "",
    connect: jest.fn(),
    disconnect: jest.fn(),
    sendInput: jest.fn(),
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    terminalState: "CONNECTED",
    isHardFailed: false,
    handleManualReconnect: jest.fn(),
    requestFullResync: jest.fn(),
    markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
    ...overrides,
  };
}

/** Minimal visualViewport double that supports resize/scroll listeners. */
function installVisualViewport() {
  const target = new EventTarget();
  const vv = {
    height: 800,
    offsetTop: 0,
    addEventListener: (t: string, l: () => void) => target.addEventListener(t, l),
    removeEventListener: (t: string, l: () => void) => target.removeEventListener(t, l),
    emit: (t: "resize" | "scroll") => target.dispatchEvent(new Event(t)),
  };
  Object.defineProperty(window, "visualViewport", { configurable: true, writable: true, value: vv });
  return vv;
}

const SETTLE_MS = 100; // 3 stable frames at 16 ms plus slack; well under maxWaitMs

let streamMock: ReturnType<typeof makeStreamMock>;

// XtermTerminal is lazy-loaded; flush the Suspense boundary so xtermRef is populated.
async function renderTerminal(isVisible = false) {
  const utils = render(<TerminalOutput sessionId="s1" baseUrl="/api" isVisible={isVisible} />);
  await act(async () => {});
  return utils;
}

beforeEach(() => {
  jest.clearAllMocks();
  jest.useFakeTimers();
  localStorage.clear();
  capturedXtermProps = null;
  streamMock = makeStreamMock();
  (useTerminalStream as jest.Mock).mockReturnValue(streamMock);
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
  delete (window as any).visualViewport;
  localStorage.clear();
});

describe("TerminalOutput refit wiring", () => {
  it("viewportResize_should_CallRefitOncePerSettle_And_NotAfterUnmount", async () => {
    const vv = installVisualViewport();
    const { unmount } = await renderTerminal();

    act(() => {
      vv.emit("resize");
      vv.height = 480;
      vv.emit("resize");
    });
    expect(mockXtermHandle.refit).not.toHaveBeenCalled(); // trailing, not leading-edge
    act(() => { jest.advanceTimersByTime(SETTLE_MS); });

    expect(mockXtermHandle.refit).toHaveBeenCalledTimes(1);
    expect(mockXtermHandle.refit).toHaveBeenCalledWith({ reason: "viewport-settle", onFitted: expect.any(Function) });
    // The old 300/400 ms pipeline must not also fit.
    act(() => { jest.advanceTimersByTime(1000); });
    expect(mockXtermHandle.fit).not.toHaveBeenCalled();
    expect(mockXtermHandle.refit).toHaveBeenCalledTimes(1);

    unmount();
    mockXtermHandle.refit.mockClear();
    act(() => {
      vv.emit("resize");
      jest.advanceTimersByTime(1000);
    });
    expect(mockXtermHandle.refit).not.toHaveBeenCalled();
  });

  it("viewportScroll_should_ArmSettle_When_OffsetTopChanges", async () => {
    const vv = installVisualViewport();
    await renderTerminal();

    act(() => {
      vv.offsetTop = 40;
      vv.emit("scroll");
      jest.advanceTimersByTime(SETTLE_MS);
    });
    expect(mockXtermHandle.refit).toHaveBeenCalledTimes(1);
  });

  it("visibilityTrue_should_CallRefit_NotSetTimeoutFit", async () => {
    const { rerender } = await renderTerminal();
    expect(mockXtermHandle.refit).not.toHaveBeenCalled();

    rerender(<TerminalOutput sessionId="s1" baseUrl="/api" isVisible={true} />);
    act(() => { jest.advanceTimersByTime(200); });

    expect(mockXtermHandle.refit).toHaveBeenCalledWith({ reason: "visibility" });
    expect(mockXtermHandle.fit).not.toHaveBeenCalled();
  });

  it("handleManualResize_should_CallRefit", async () => {
    await renderTerminal();

    fireEvent.click(screen.getByLabelText("Redraw terminal (fixes a blank screen)"));

    expect(mockXtermHandle.refit).toHaveBeenCalledWith(expect.objectContaining({ reason: "manual-resize" }));
    expect(mockXtermHandle.fit).not.toHaveBeenCalled();
  });
});

describe("TerminalOutput settle-driven bounce-hold bypass", () => {
  function settleOnce(vv: ReturnType<typeof installVisualViewport>) {
    act(() => {
      vv.emit("resize");
      jest.advanceTimersByTime(SETTLE_MS);
    });
  }

  const lastRefitOptions = () => mockXtermHandle.refit.mock.calls.at(-1)![0];

  it("handleTerminalResize_should_PassBypass_When_ResizeFollowsSettleRefit", async () => {
    const vv = installVisualViewport();
    await renderTerminal();
    act(() => { capturedXtermProps.onResize(80, 24); }); // seed lastResizeRef
    settleOnce(vv);
    expect(lastRefitOptions()).toEqual(expect.objectContaining({ reason: "viewport-settle" }));

    streamMock.resize.mockClear();
    act(() => { capturedXtermProps.onResize(80, 20); });

    expect(streamMock.resize).toHaveBeenCalledTimes(1);
    expect(streamMock.resize).toHaveBeenCalledWith(80, 20, false, { bypassBounceHold: true });
  });

  it("handleTerminalResize_should_BypassOnlyOnce_When_OscillatingBackWithinSeconds", async () => {
    const vv = installVisualViewport();
    await renderTerminal();
    act(() => { capturedXtermProps.onResize(80, 24); });
    settleOnce(vv);
    streamMock.resize.mockClear();

    act(() => { capturedXtermProps.onResize(80, 20); }); // A -> B, produced by the settle fit
    act(() => { jest.advanceTimersByTime(200); });
    act(() => { capturedXtermProps.onResize(80, 24); }); // B -> A, a real bounce: must be held

    const unforced = streamMock.resize.mock.calls.filter((c) => !c[2]);
    expect(unforced).toHaveLength(2);
    expect(unforced[0][3]).toEqual({ bypassBounceHold: true });
    expect(unforced[1][3]).toBeUndefined();
  });

  it("handleTerminalResize_should_NotBypassAgain_When_SettleRearmsContinuouslyButFitProducedNoResize", async () => {
    const vv = installVisualViewport();
    await renderTerminal();
    act(() => { capturedXtermProps.onResize(80, 24); });
    settleOnce(vv);
    // The settle fit finished without a size change.
    act(() => { lastRefitOptions().onFitted({ cols: 80, rows: 24 }); });

    streamMock.resize.mockClear();
    act(() => { jest.advanceTimersByTime(100); });
    act(() => { capturedXtermProps.onResize(80, 20); });

    expect(streamMock.resize).toHaveBeenCalledTimes(1);
    expect(streamMock.resize.mock.calls[0][3]).toBeUndefined();
  });

  it("handleTerminalResize_should_NotPassBypass_When_NoSettleHasHappened", async () => {
    installVisualViewport();
    await renderTerminal();
    act(() => { capturedXtermProps.onResize(80, 24); });
    act(() => { capturedXtermProps.onResize(80, 20); });

    expect(streamMock.resize.mock.calls[0][3]).toBeUndefined();
  });
});

describe("TerminalOutput manual resize with stale dims", () => {
  it("handleManualResize_should_NotForceResizeOrClearBuffer_When_FitReportsStaleDims", async () => {
    await renderTerminal();
    fireEvent.click(screen.getByLabelText("Redraw terminal (fixes a blank screen)"));
    const { onFitted } = mockXtermHandle.refit.mock.calls.at(-1)![0];

    streamMock.resize.mockClear();
    mockXtermHandle.clear.mockClear();
    act(() => onFitted({ cols: 80, rows: 24, stale: true }));
    expect(streamMock.resize).not.toHaveBeenCalled();
    expect(mockXtermHandle.clear).not.toHaveBeenCalled();

    act(() => onFitted({ cols: 80, rows: 24 }));
    expect(streamMock.resize).toHaveBeenCalledWith(80, 24, true);
  });
});

describe("TerminalOutput Redraw button (Story 1.2.8)", () => {
  it("redrawButton_should_CallRefitWithManualResize_When_Tapped", async () => {
    await renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: "Redraw terminal (fixes a blank screen)" }));
    expect(mockXtermHandle.refit).toHaveBeenCalledTimes(1);
    expect(mockXtermHandle.refit).toHaveBeenCalledWith(expect.objectContaining({ reason: "manual-resize" }));
  });

  it("redrawButton_should_BeRenderedWhenToolbarCollapsed", async () => {
    await renderTerminal();
    expect(screen.queryByTestId("toolbar-actions")).toBeNull(); // collapsed by default
    const button = screen.getByRole("button", { name: "Redraw terminal (fixes a blank screen)" });
    expect(button.textContent).toContain("Redraw");
    expect(button.getAttribute("title")).toBe("Redraw terminal (fixes a blank screen)");
  });
});

describe("TerminalOutput scroll settings plumbing (Stories 1.2.5e-g)", () => {
  const epoch = () => capturedXtermProps.scrollGesture.connectionEpoch;

  afterEach(() => {
    (mockXtermHandle as any).terminal = null;
  });

  /** Gives TerminalOutput a live terminal and forces the stream manager into existence. */
  async function attachTerminalAndOutput() {
    (mockXtermHandle as any).terminal = { cols: 80, rows: 24 };
    await act(async () => {
      (useTerminalStream as jest.Mock).mock.calls.at(-1)![0].onOutput("hello");
    });
    const manager = (TerminalStreamManager as unknown as jest.Mock).mock.results.at(-1)!.value;
    return manager.setOnFullSnapshot.mock.calls[0][0] as () => void;
  }

  it("connectionEpoch_should_Increment_When_ReconnectOrFullSnapshot", async () => {
    const { rerender } = await renderTerminal();
    expect(epoch()).toBe(0);

    const onFullSnapshot = await attachTerminalAndOutput();
    act(() => onFullSnapshot());
    expect(epoch()).toBe(1);

    // Hard failure: the banner's Retry goes through the hook reconnect.
    streamMock = makeStreamMock({ isConnected: false, isHardFailed: true });
    (useTerminalStream as jest.Mock).mockReturnValue(streamMock);
    rerender(<TerminalOutput sessionId="s1" baseUrl="/api" isVisible={false} />);
    act(() => { jest.advanceTimersByTime(2100); });
    fireEvent.click(screen.getByRole("button", { name: /Retry/i }));
    expect(epoch()).toBe(2);
    expect(streamMock.handleManualReconnect).toHaveBeenCalledTimes(1);
  });

  it("TerminalOutput_should_ThreadSettingsToXterm_And_FollowScrollModeChanges", async () => {
    localStorage.setItem("terminal-scroll-override", "tui");
    await renderTerminal();
    expect(capturedXtermProps.scrollGesture).toEqual(
      expect.objectContaining({ scrollOverride: "tui", gestureScrollEnabled: true, connectionEpoch: 0 }),
    );
    expect(typeof capturedXtermProps.onScrollModeChange).toBe("function");
    act(() => capturedXtermProps.onScrollModeChange({ bufferType: "alternate", mouseTrackingMode: "any" }));
    expect(capturedXtermProps.scrollGesture.scrollOverride).toBe("tui");
  });
});
