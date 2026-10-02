// @feature terminal-scroll-settings
/**
 * Stories 1.2.5b3 / 1.2.5d / 1.2.7b / 1.2.8: scrolling chip, picker, panel, hint, jump button,
 * netPagesUp wiring and the Redraw completion callback, mounted in TerminalOutput.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";

const mockXtermHandle: any = {
  terminal: null,
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

jest.mock("@/lib/hooks/useTerminalStream", () => ({ useTerminalStream: jest.fn() }));
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
jest.mock("@/lib/hooks/useHandedness", () => require("./terminalOutputTestMocks").handednessMockModule());
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
import { scrollSettings } from "@/lib/terminal/scrollOverride";
// eslint-disable-next-line import/first
import { HINT_TEXT_LOCAL, HINT_TEXT_TUI, SCROLL_HINT_SEEN_KEY } from "../ScrollHint";

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

let streamMock: ReturnType<typeof makeStreamMock>;

function installMatchMedia(coarse: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: coarse && query.includes("any-pointer: coarse"),
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

function makeTerminal(overrides: Record<string, unknown> = {}) {
  return {
    cols: 80,
    rows: 24,
    buffer: { active: { viewportY: 0, baseY: 0, length: 24, cursorY: 0 } },
    scrollToBottom: jest.fn(),
    scrollPages: jest.fn(),
    onScroll: jest.fn().mockReturnValue({ dispose: jest.fn() }),
    onWriteParsed: jest.fn().mockReturnValue({ dispose: jest.fn() }),
    options: {},
    focus: jest.fn(),
    ...overrides,
  };
}

async function renderTerminal(opts: { coarse?: boolean; terminal?: any } = {}) {
  installMatchMedia(opts.coarse ?? true);
  mockXtermHandle.terminal = opts.terminal ?? makeTerminal();
  const utils = render(<TerminalOutput sessionId="s1" baseUrl="/api" isVisible={false} />);
  await act(async () => {});
  // Terminal reports its size once laid out; this is what reveals rows to the chip.
  act(() => {
    capturedXtermProps.onResize(mockXtermHandle.terminal.cols, mockXtermHandle.terminal.rows);
  });
  return utils;
}

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  scrollSettings.setOverride("auto");
  scrollSettings.setGestureScroll(true);
  capturedXtermProps = null;
  mockXtermHandle.terminal = null;
  streamMock = makeStreamMock();
  (useTerminalStream as jest.Mock).mockReturnValue(streamMock);
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
  localStorage.clear();
});

const chip = () => screen.getByRole("button", { name: /^Scroll mode: .*Opens scroll settings$/ });

describe("TerminalOutput scrolling chip, picker and panel (Story 1.2.5b3)", () => {
  it("terminalOutput_should_RenderScrollChipWhenToolbarCollapsed", async () => {
    await renderTerminal();
    expect(screen.queryByTestId("toolbar-actions")).toBeNull();
    expect(chip().textContent).toBe("Terminal history");
  });

  it("terminalOutput_should_NotRenderChip_When_NoTouchCapability", async () => {
    await renderTerminal({ coarse: false });
    expect(screen.queryByRole("button", { name: /Opens scroll settings$/ })).toBeNull();
  });

  it("terminalOutput_should_OpenFullPanel_When_ScrollingEntrySelected", async () => {
    await renderTerminal();
    fireEvent.click(screen.getByTestId("toolbar-toggle"));
    fireEvent.click(screen.getByRole("button", { name: "Scrolling settings" }));
    expect(screen.getByTestId("scrolling-full")).toBeTruthy();
    expect(screen.queryByTestId("scrolling-picker")).toBeNull();
  });

  it("terminalOutput_should_OpenPickerAndStoreOverrideAndClose_When_ChipThenPageKeys", async () => {
    await renderTerminal();
    fireEvent.click(chip());
    expect(screen.getByTestId("scrolling-picker")).toBeTruthy();

    fireEvent.click(screen.getByRole("radio", { name: /^Page keys/ }));

    expect(localStorage.getItem("terminal-scroll-override")).toBe("tui");
    expect(screen.queryByTestId("scrolling-picker")).toBeNull();
  });

  it("terminalOutput_should_AnnounceOnce_When_PickerSelectionClosesPicker", async () => {
    await renderTerminal();
    fireEvent.click(chip());
    fireEvent.click(screen.getByRole("radio", { name: /^Page keys/ }));

    const announcements = screen
      .getAllByRole("status")
      .filter((el) => el.textContent === "Scroll mode: Page keys");
    expect(announcements).toHaveLength(1);
    // The chip stays silent
    expect(screen.getByTestId("scroll-chip-announcer").textContent).toBe("");
  });

  it("terminalOutput_should_SwitchToFullPanel_When_PickerMoreSettingsPressed", async () => {
    await renderTerminal();
    fireEvent.click(chip());
    fireEvent.click(screen.getByRole("button", { name: "More scrolling settings" }));
    expect(screen.getByTestId("scrolling-full")).toBeTruthy();
    expect(screen.queryByTestId("scrolling-picker")).toBeNull();
  });

  it("terminalOutput_should_RenderPanelAsOverlay_When_FewRowsWouldRemainInline", async () => {
    await renderTerminal({ terminal: makeTerminal({ rows: 12 }) });
    fireEvent.click(chip());
    expect(screen.getByTestId("scrolling-picker").getAttribute("data-overlay")).toBe("true");
  });

  it("terminalOutput_should_RenderPanelInline_When_PlentyOfRowsRemain", async () => {
    await renderTerminal({ terminal: makeTerminal({ rows: 60 }) });
    fireEvent.click(chip());
    expect(screen.getByTestId("scrolling-picker").getAttribute("data-overlay")).toBe("false");
  });

  it("terminalOutput_should_HighlightChip_When_MisrouteReported", async () => {
    await renderTerminal();
    expect(chip().getAttribute("data-highlighted")).toBe("false");
    act(() => {
      capturedXtermProps.scrollGesture.onScrollGesture({
        route: "xterm-local",
        postSlopLines: 3,
        viewportYChanged: false,
      });
    });
    expect(chip().getAttribute("data-highlighted")).toBe("true");
    expect(chip().textContent).toBe("! Terminal history");
  });
});

describe("TerminalOutput first-use scroll hint (Task 1.2.5d)", () => {
  const startScroll = (route: "xterm-local" | "tui-pgkeys") =>
    act(() => {
      capturedXtermProps.scrollGesture.onScrollStart(route);
    });

  it("hint_should_ShowRouteTextOnFirstScrollStart_And_SetSeenFlagOnGotIt", async () => {
    await renderTerminal();
    expect(screen.queryByTestId("scroll-hint")).toBeNull();

    startScroll("xterm-local");
    expect(screen.getByTestId("scroll-hint").textContent).toContain(HINT_TEXT_LOCAL);
    expect(localStorage.getItem(SCROLL_HINT_SEEN_KEY)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Got it" }));
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
    expect(localStorage.getItem(SCROLL_HINT_SEEN_KEY)).toBe("1");
  });

  it("hint_should_ShowTuiTextOnTuiRoute", async () => {
    await renderTerminal();
    startScroll("tui-pgkeys");
    expect(screen.getByTestId("scroll-hint").textContent).toContain(HINT_TEXT_TUI);
  });

  it("hint_should_NotShow_When_SeenFlagSet", async () => {
    localStorage.setItem(SCROLL_HINT_SEEN_KEY, "1");
    await renderTerminal();
    startScroll("xterm-local");
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
  });

  it("hint_should_DismissAndSetFlag_When_PickerOpens", async () => {
    await renderTerminal();
    startScroll("xterm-local");
    fireEvent.click(chip());
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
    expect(localStorage.getItem(SCROLL_HINT_SEEN_KEY)).toBe("1");
  });

  it("hint_should_DismissAndSetFlag_When_FullPanelOpens", async () => {
    await renderTerminal();
    startScroll("xterm-local");
    fireEvent.click(screen.getByTestId("toolbar-toggle"));
    fireEvent.click(screen.getByRole("button", { name: "Scrolling settings" }));
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
    expect(localStorage.getItem(SCROLL_HINT_SEEN_KEY)).toBe("1");
  });
});

describe("TerminalOutput jump button and netPagesUp wiring (Story 1.2.7b)", () => {
  const PG_UP = "\x1b[5~";
  const tuiLabel = { name: "Page down to latest" };

  async function renderTui() {
    scrollSettings.setOverride("tui");
    const utils = await renderTerminal();
    // The hook reports page keys right after sending them through the programmatic sink
    act(() => {
      capturedXtermProps.scrollGesture.onProgrammaticData(PG_UP);
      capturedXtermProps.scrollGesture.onPageKeysSent("up", 1);
    });
    return utils;
  }

  it("jumpToLatest_should_BeMountedInTerminalOutput_When_LocalBufferScrolledAway", async () => {
    const terminal = makeTerminal({ buffer: { active: { viewportY: 5, baseY: 20, length: 44, cursorY: 23 } } });
    await renderTerminal({ terminal });
    fireEvent.click(screen.getByRole("button", { name: "Jump to latest" }));
    expect(terminal.scrollToBottom).toHaveBeenCalled();
    expect(terminal.focus).not.toHaveBeenCalled();
  });

  it("isInputBusy_should_BeThreadedFromStreamHookToXtermScrollGesture", async () => {
    const isInputChunking = jest.fn().mockReturnValue(true);
    streamMock = makeStreamMock({ isInputChunking });
    (useTerminalStream as jest.Mock).mockReturnValue(streamMock);
    await renderTerminal();
    expect(capturedXtermProps.scrollGesture.isInputBusy()).toBe(true);
    expect(isInputChunking).toHaveBeenCalled();
  });

  it("jumpToLatest_should_BeAbsent_When_LocalBufferAtLive", async () => {
    await renderTerminal();
    expect(screen.queryByRole("button", { name: "Jump to latest" })).toBeNull();
  });

  it("netPagesUp_should_NotInvalidate_When_DragPageKeysSent_And_ShouldInvalidate_When_UserTypes", async () => {
    await renderTui();
    expect(screen.getByRole("button", tuiLabel)).toBeTruthy();
    expect(streamMock.sendInput).toHaveBeenCalledWith(PG_UP);

    act(() => {
      capturedXtermProps.scrollGesture.onProgrammaticData(PG_UP);
    });
    expect(screen.getByRole("button", tuiLabel)).toBeTruthy();

    act(() => {
      capturedXtermProps.onData("a");
    });
    expect(screen.queryByRole("button", tuiLabel)).toBeNull();
  });

  it("netPagesUp_should_NotInvalidate_When_ToolbarPageKeyPressed", async () => {
    scrollSettings.setOverride("tui");
    await renderTerminal();
    fireEvent.pointerDown(screen.getByRole("button", { name: "Page up" }));
    expect(screen.getByRole("button", tuiLabel)).toBeTruthy();
    expect(streamMock.sendInput).toHaveBeenCalledWith(PG_UP);
  });

  it("netPagesUp_should_Invalidate_When_ResizeOccurs", async () => {
    await renderTui();
    act(() => {
      capturedXtermProps.onResize(80, 20);
    });
    expect(screen.queryByRole("button", tuiLabel)).toBeNull();
  });

  it("netPagesUp_should_Invalidate_When_OverrideOrModeChanges", async () => {
    await renderTui();
    act(() => {
      scrollSettings.setOverride("local");
    });
    act(() => {
      scrollSettings.setOverride("tui");
    });
    expect(screen.queryByRole("button", tuiLabel)).toBeNull();

    act(() => {
      capturedXtermProps.scrollGesture.onPageKeysSent("up", 1);
    });
    expect(screen.getByRole("button", tuiLabel)).toBeTruthy();
    act(() => {
      capturedXtermProps.onScrollModeChange({ bufferType: "alternate", mouseTrackingMode: "none" });
    });
    expect(screen.queryByRole("button", tuiLabel)).toBeNull();
  });

  it("netPagesUp_should_CountEachPage_When_HookReportsMultiplePages", async () => {
    await renderTui();
    act(() => {
      capturedXtermProps.scrollGesture.onPageKeysSent("down", 1);
    });
    expect(screen.queryByRole("button", tuiLabel)).toBeNull(); // 1 up - 1 down = 0
  });
});

describe("Redraw waits for the fit to complete (Story 1.2.8 / 2.1.3b)", () => {
  const redraw = () => fireEvent.click(screen.getByRole("button", { name: "Redraw terminal (fixes a blank screen)" }));
  const fitOptions = () => mockXtermHandle.refit.mock.calls.at(-1)![0];

  it("redrawButton_should_SendForcedResizeWithPostFitDims_When_FitCompletes", async () => {
    await renderTerminal();
    streamMock.resize.mockClear();
    redraw();

    expect(mockXtermHandle.refit).toHaveBeenCalledWith(expect.objectContaining({ reason: "manual-resize" }));
    expect(streamMock.resize).not.toHaveBeenCalled(); // stale terminal dims must not go out

    act(() => {
      fitOptions().onFitted({ cols: 132, rows: 41 });
    });
    expect(streamMock.resize).toHaveBeenCalledTimes(1);
    expect(streamMock.resize).toHaveBeenCalledWith(132, 41, true);
  });

  it("redrawButton_should_NotSendResize_When_Disconnected", async () => {
    streamMock = makeStreamMock({ isConnected: false });
    (useTerminalStream as jest.Mock).mockReturnValue(streamMock);
    await renderTerminal();
    streamMock.resize.mockClear();
    redraw();
    act(() => {
      fitOptions().onFitted({ cols: 132, rows: 41 });
    });
    expect(streamMock.resize).not.toHaveBeenCalled();
  });
});
