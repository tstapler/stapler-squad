// @feature terminal-browser-log-stream
/**
 * Tests for the Remote Log Stream toggle button in TerminalOutput.
 * These tests verify the button's presence, localStorage persistence,
 * active styling, and hook wiring.
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

// ── Mocks ─────────────────────────────────────────────────────────────────────

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

jest.mock("../XtermTerminal", () => {
  const React = require("react");
  const XtermTerminal = React.forwardRef((props: any, ref: any) => {
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return React.createElement("div", { "data-testid": "mock-xterm" });
  });
  XtermTerminal.displayName = "XtermTerminal";
  return { XtermTerminal };
});

jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: jest.fn(),
}));

jest.mock("@/lib/terminal/TerminalDimensionCache", () => ({
  getCachedDimensions: jest.fn().mockReturnValue(null),
  saveDimensions: jest.fn(),
}));

jest.mock("@/lib/terminal/TerminalStreamManager", () => ({
  TerminalStreamManager: jest.fn().mockImplementation(() => ({
    setOnFirstOutput: jest.fn(),
    installDebugMonitor: jest.fn(),
    writeInitialContent: jest.fn().mockResolvedValue(undefined),
    write: jest.fn(),
    cleanup: jest.fn(),
    updateSendFlowControl: jest.fn(),
  })),
}));

jest.mock("@/lib/telemetry", () => ({ track: jest.fn() }));

// Mock useBrowserLogStream so hook side-effects (console patching) don't bleed
// into the test environment.
const mockUseBrowserLogStream = jest.fn();
jest.mock("@/lib/hooks/useBrowserLogStream", () => ({
  useBrowserLogStream: (...args: any[]) => mockUseBrowserLogStream(...args),
}));

// ── Imports (after mocks) ──────────────────────────────────────────────────────

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeStreamMock(overrides = {}) {
  return {
    isConnected: false,
    error: null,
    connect: jest.fn(),
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
    ...overrides,
  };
}

function renderTerminal(sessionId = "session-abc", baseUrl = "/api") {
  return render(
    <TerminalOutput sessionId={sessionId} baseUrl={baseUrl} isVisible={false} />
  );
}

// ── Setup / Teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());
  mockUseBrowserLogStream.mockReset();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
  localStorage.clear();
});

// ── UT-UI-01: Button present ─────────────────────────────────────────────────

describe("UT-UI-01: renders log stream button in expanded toolbar", () => {
  it("renders_log_stream_button_in_expanded_toolbar", () => {
    renderTerminal();
    const btn = screen.getByRole("button", { name: /enable remote log streaming/i });
    expect(btn).toBeInTheDocument();
  });
});

// ── UT-UI-02: devOnly class applied ──────────────────────────────────────────

describe("UT-UI-02: log stream button has devOnly class", () => {
  it("log_stream_button_has_devOnly_class", () => {
    renderTerminal();
    const btn = screen.getByRole("button", { name: /enable remote log streaming/i });
    // The button should have the devOnly class (controls mobile visibility via CSS)
    expect(btn.className).toContain("devOnly");
  });
});

// ── UT-UI-03: Toggle on sets localStorage ─────────────────────────────────────

describe("UT-UI-03: toggle on calls localStorage.setItem with correct key", () => {
  it("toggle_on_calls_localStorage_setItem_with_correct_key", () => {
    const setItemSpy = jest.spyOn(Storage.prototype, "setItem");
    renderTerminal();

    const btn = screen.getByRole("button", { name: /enable remote log streaming/i });
    fireEvent.click(btn);

    expect(setItemSpy).toHaveBeenCalledWith("stapler-squad-remote-debug", "true");
  });
});

// ── UT-UI-04: Toggle off removes localStorage ─────────────────────────────────

describe("UT-UI-04: toggle off calls localStorage.removeItem", () => {
  it("toggle_off_calls_localStorage_removeItem", () => {
    // Start with the button ON
    localStorage.setItem("stapler-squad-remote-debug", "true");
    const removeItemSpy = jest.spyOn(Storage.prototype, "removeItem");

    renderTerminal();

    const btn = screen.getByRole("button", { name: /disable remote log streaming/i });
    fireEvent.click(btn);

    expect(removeItemSpy).toHaveBeenCalledWith("stapler-squad-remote-debug");
  });
});

// ── UT-UI-05: Default state is off ───────────────────────────────────────────

describe("UT-UI-05: default state is off when localStorage empty", () => {
  it("default_state_is_off_when_localStorage_empty", () => {
    renderTerminal();
    // When off, aria-label should say "Enable..."
    const btn = screen.getByRole("button", { name: /enable remote log streaming/i });
    expect(btn).toBeInTheDocument();
    // Should NOT show "Log Stream ON"
    expect(btn.textContent).not.toContain("Log Stream ON");
  });
});

// ── UT-UI-06: Active state shows ON label and green style ────────────────────

describe("UT-UI-06: active state shows ON label and green style", () => {
  it("active_state_shows_ON_label_and_green_style", () => {
    renderTerminal();

    // Click to activate
    const btn = screen.getByRole("button", { name: /enable remote log streaming/i });
    fireEvent.click(btn);

    // Now it should show the ON label
    const activeBtn = screen.getByRole("button", { name: /disable remote log streaming/i });
    expect(activeBtn.textContent).toContain("Log Stream ON");
    expect(activeBtn).toHaveStyle({ backgroundColor: "#2a4" });
  });
});

// ── UT-UI-07: Hook receives sessionId prop ────────────────────────────────────

describe("UT-UI-07: hook called with sessionId prop", () => {
  it("hook_called_with_sessionId_prop", () => {
    renderTerminal("my-session-id");

    expect(mockUseBrowserLogStream).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "my-session-id" })
    );
  });
});

// ── UT-UI-08: Initializes from localStorage true ──────────────────────────────

describe("UT-UI-08: initializes from localStorage true", () => {
  it("initializes_from_localStorage_true", () => {
    localStorage.setItem("stapler-squad-remote-debug", "true");

    renderTerminal();

    // When seeded as ON, button should show "disable" aria-label
    const btn = screen.getByRole("button", { name: /disable remote log streaming/i });
    expect(btn).toBeInTheDocument();
    expect(btn.textContent).toContain("Log Stream ON");
  });
});
