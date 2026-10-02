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
