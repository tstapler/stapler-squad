// @feature terminal-reconnect
/**
 * Story 1.2.9: the mobile-toolbar PgUp/PgDn follow the same effective route as the drag.
 * Local route scrolls xterm history; a TUI route sends the exact bytes it always did.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";

const scrollPages = jest.fn();
const terminal = {
  cols: 80,
  rows: 24,
  options: { disableStdin: false },
  scrollPages,
  getSelection: jest.fn(() => "selected text"),
  buffer: { active: { viewportY: 50, baseY: 100, type: "normal" } },
};
const mockXtermHandle = {
  terminal,
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
jest.mock("@/lib/contexts/AnalyticsContext", () => require("./terminalOutputTestMocks").analyticsContextPlainMockModule());
jest.mock("@/lib/contexts/ApprovalsContext", () => require("./terminalOutputTestMocks").approvalsContextMockModule());
jest.mock("@/lib/hooks/useHandedness", () => require("./terminalOutputTestMocks").handednessMockModule());
jest.mock("@/lib/hooks/useSplitContainerSize", () => require("./terminalOutputTestMocks").splitContainerSizeMockModule());
jest.mock("@/lib/hooks/useBrowserLogStream", () => require("./terminalOutputTestMocks").browserLogStreamMockModule());
jest.mock("@/components/providers/ViewportProvider", () => require("./terminalOutputTestMocks").viewportProviderDesktopMockModule());
jest.mock("@/lib/terminal/mobileDebug", () => ({
  mobileDebug: { log: jest.fn(), toolbarKey: jest.fn(), overrideChange: jest.fn(), routeDecision: jest.fn() },
}));

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";

// TerminalOutput sources its xterm instance from the pool (see TerminalPool.tsx); renders need the provider.
function withPool(children: React.ReactNode) {
  return <TerminalPoolProvider>{children}</TerminalPoolProvider>;
}
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
// eslint-disable-next-line import/first
import { mobileDebug } from "@/lib/terminal/mobileDebug";

const PGUP = "\x1b[5~";
const PGDN = "\x1b[6~";
const CTRL_PGUP = "\x1b[5;5~";
const sendInput = jest.fn();
let isInputChunking: jest.Mock;

async function renderTerminal(readOnly = false, announce?: jest.Mock) {
  const terminalUi = () => withPool(<TerminalOutput sessionId="s1" baseUrl="/api" isVisible={false} readOnly={readOnly} />);
  const ui = () =>
    announce ? (
      <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>{terminalUi()}</AnnouncerContext.Provider>
    ) : (
      terminalUi()
    );
  const view = render(ui());
  await act(async () => {});
  // The pooled xterm handle attaches after the first commit; a later render applies disableStdin.
  view.rerender(ui());
  await act(async () => {});
}

const tap = (name: string) => fireEvent.pointerDown(screen.getByRole("button", { name }));
const setMode = (mode: { bufferType: string; mouseTrackingMode: string }) =>
  act(() => capturedXtermProps.onScrollModeChange(mode));

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  terminal.buffer.active = { viewportY: 50, baseY: 100, type: "normal" };
  terminal.options.disableStdin = false;
  isInputChunking = jest.fn(() => false);
  (useTerminalStream as jest.Mock).mockReturnValue({
    isInputChunking,
    isConnected: true, error: null, output: "", connect: jest.fn(), disconnect: jest.fn(), sendInput,
    resize: jest.fn(), scrollbackLoaded: false, requestScrollback: jest.fn(), sendFlowControl: jest.fn(),
    startRecording: jest.fn(), stopRecording: jest.fn(), terminalState: "CONNECTED", isHardFailed: false,
    handleManualReconnect: jest.fn(), requestFullResync: jest.fn(), markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
  });
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: false, media: query, addEventListener: jest.fn(), removeEventListener: jest.fn(),
      addListener: jest.fn(), removeListener: jest.fn(), dispatchEvent: jest.fn(),
    })),
  });
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
});

afterEach(() => jest.restoreAllMocks());

describe("toolbar PgUp/PgDn route awareness", () => {
  it("toolbarKeys_should_ScrollPages_When_LocalRoute", async () => {
    await renderTerminal(); // normal buffer, tracking none, auto => xterm-local
    tap("Page up");
    expect(scrollPages).toHaveBeenCalledWith(-1);
    tap("Page down");
    expect(scrollPages).toHaveBeenLastCalledWith(1);
    expect(sendInput).not.toHaveBeenCalled();
    expect(mobileDebug.toolbarKey).toHaveBeenCalledWith("PageUp");
    expect(mobileDebug.toolbarKey).toHaveBeenCalledWith("PageDown");
  });

  it("toolbarKeys_should_SendExactBytes_When_TuiRoute", async () => {
    await renderTerminal();
    await setMode({ bufferType: "alternate", mouseTrackingMode: "none" });
    tap("Page up");
    tap("Page down");
    expect(sendInput.mock.calls.map((c) => c[0])).toEqual([PGUP, PGDN]);
    expect(scrollPages).not.toHaveBeenCalled();
  });

  it("toolbarKeys_should_DropPageKeys_When_ChunkedPasteInFlightOnTuiRoute", async () => {
    await renderTerminal();
    await setMode({ bufferType: "alternate", mouseTrackingMode: "none" });
    isInputChunking.mockReturnValue(true);
    tap("Page up");
    tap("Page down");
    expect(sendInput).not.toHaveBeenCalled();
    isInputChunking.mockReturnValue(false);
    tap("Page up");
    expect(sendInput).toHaveBeenCalledWith(PGUP);
  });

  it("toolbarKeys_should_StillScrollLocalHistory_When_ChunkedPasteInFlight", async () => {
    await renderTerminal(); // xterm-local route
    isInputChunking.mockReturnValue(true);
    tap("Page up");
    expect(scrollPages).toHaveBeenCalledWith(-1);
    expect(sendInput).not.toHaveBeenCalled();
  });

  it("toolbarKeys_should_SendBytes_When_TuiOverrideChosenOnNormalBuffer", async () => {
    localStorage.setItem("terminal-scroll-override", "tui");
    await renderTerminal();
    tap("Page up");
    expect(sendInput).toHaveBeenCalledWith(PGUP);
    expect(scrollPages).not.toHaveBeenCalled();
  });

  it("toolbarKeys_should_SendModifiedBytesNotScroll_When_CtrlArmed", async () => {
    await renderTerminal();
    tap("Control modifier");
    tap("Page up");
    expect(sendInput).toHaveBeenCalledWith(CTRL_PGUP);
    expect(scrollPages).not.toHaveBeenCalled();
  });

  it("toolbarKeys_should_FallThroughToBytes_When_AutoLocalAtEdge", async () => {
    terminal.buffer.active = { viewportY: 0, baseY: 100, type: "normal" }; // already at top
    await renderTerminal();
    tap("Page up");
    expect(sendInput).toHaveBeenCalledWith(PGUP);
    expect(scrollPages).not.toHaveBeenCalled();
  });

  it("toolbarKeys_should_NotFallThrough_When_ExplicitLocalOverrideAtEdge", async () => {
    localStorage.setItem("terminal-scroll-override", "local");
    terminal.buffer.active = { viewportY: 0, baseY: 100, type: "normal" };
    await renderTerminal();
    await setMode({ bufferType: "alternate", mouseTrackingMode: "none" }); // override still wins
    tap("Page up");
    expect(scrollPages).toHaveBeenCalledWith(-1);
    expect(sendInput).not.toHaveBeenCalled();
  });
});

// Story 5.3 (RO-1, RO-2 client half): a hidden session's terminal is read-only.
describe("TerminalOutput readOnly (hidden session)", () => {
  it("readonly_should_disable_stdin_and_pass_readOnly_to_the_stream_hook", async () => {
    await renderTerminal(true);
    expect(terminal.options.disableStdin).toBe(true);
    const lastCall = (useTerminalStream as jest.Mock).mock.calls.at(-1);
    expect(lastCall?.[0].readOnly).toBe(true);
  });

  it("readonly_should_drop_typed_input_before_the_stream", async () => {
    await renderTerminal(true);
    act(() => capturedXtermProps.onData("rm -rf"));
    expect(sendInput).not.toHaveBeenCalled();
  });

  it("readonly_should_remove_the_keyboard_toggle", async () => {
    await renderTerminal(true);
    expect(screen.queryByRole("button", { name: /mobile keyboard/i })).toBeNull();
  });

  it("writable_should_keep_stdin_input_and_the_keyboard_toggle", async () => {
    await renderTerminal(false);
    expect(terminal.options.disableStdin).toBe(false);
    act(() => capturedXtermProps.onData("ls"));
    expect(sendInput).toHaveBeenCalledWith("ls");
    expect(screen.getByRole("button", { name: /mobile keyboard/i })).toBeInTheDocument();
  });

  it("read_only_terminal_should_send_0_bytes_and_announce_hint_at_most_once_per_10s_when_keys_typed", async () => {
    const announce = jest.fn();
    let now = 1_000_000;
    jest.spyOn(Date, "now").mockImplementation(() => now);
    await renderTerminal(true, announce);

    act(() => capturedXtermProps.onData("a"));
    act(() => capturedXtermProps.onData("b"));
    now += 9_999;
    act(() => capturedXtermProps.onData("c"));
    expect(sendInput).not.toHaveBeenCalled();
    const hints = () => announce.mock.calls.filter((c) => c[0] === "Read-only session");
    expect(hints()).toHaveLength(1);

    now += 1;
    act(() => capturedXtermProps.onData("d"));
    expect(hints()).toHaveLength(2);
    expect(hints()[0].slice(1)).toEqual(["polite", "terminal-read-only-hint"]);
    expect(sendInput).not.toHaveBeenCalled();
  });

  it("ro6_read_only_view_should_keep_copy_and_drop_paste_and_uploads", async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    await renderTerminal(true);

    fireEvent.click(screen.getByTestId("toolbar-toggle"));
    fireEvent.click(screen.getAllByRole("button", { name: "Copy terminal output to clipboard" })[0]);
    expect(writeText).toHaveBeenCalledWith("selected text");
    expect(screen.queryByRole("button", { name: "Paste from clipboard" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Attach images from gallery/ })).toBeNull();
  });
});
