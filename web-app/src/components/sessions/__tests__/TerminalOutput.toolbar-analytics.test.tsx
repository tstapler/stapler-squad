// @feature terminal-toolbar-analytics
/**
 * Tests for toolbar button analytics tracking in TerminalOutput.
 * Verifies every toolbar button click fires track() with the correct schema.
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

const mockTrack = jest.fn();
jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));

jest.mock("@/lib/hooks/useBrowserLogStream", () => ({
  useBrowserLogStream: jest.fn(),
}));

const mockUseViewport = jest.fn();
jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => mockUseViewport(),
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
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    // useVisibilityResync is wired unconditionally in TerminalOutput.tsx and
    // calls these on every unmount/session-id change.
    requestFullResync: jest.fn(),
    markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
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
  localStorage.setItem("stapler-squad-toolbar-expanded", "true");
  localStorage.setItem("stapler-squad-dev-toolbar", "true");
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());
  // Default: desktop-shaped viewport (not mobile, no fine pointer) — compactToolbar
  // stays false so existing tests keep exercising the expanded toolbar untouched.
  mockUseViewport.mockReturnValue({
    isMobile: false,
    isFoldable: false,
    isInnerScreen: true,
    hasFinePointer: false,
    isVirtualKeyboardOpen: false,
  });
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
  // JSDOM does not implement matchMedia — mock it so theme detection doesn't throw
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
});

afterEach(() => {
  jest.restoreAllMocks();
  localStorage.clear();
});

// ── Analytics tests (TC-A-*) ──────────────────────────────────────────────────

describe("toolbar analytics", () => {
  it("fires track with button:copy when Copy clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /copy terminal output to clipboard/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "copy" }),
    }));
  });

  it("fires track with button:paste when Paste clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /paste from clipboard/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "paste" }),
    }));
  });

  it("fires track with button:bottom when Bottom clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /scroll to bottom/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "bottom" }),
    }));
  });

  it("fires track with button:clear when Clear clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /clear terminal/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "clear" }),
    }));
  });

  it("fires track with button:gallery when Gallery clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /attach images from gallery/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "gallery" }),
    }));
  });

  it("fires track with button:files when Files clicked", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /attach files/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "files" }),
    }));
  });

  it("fires track with button:resize when Resize clicked (in dev panel)", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /resize terminal/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "resize" }),
    }));
  });

  it("fires track with button:mouse state:on when Mouse enabled", () => {
    // Mouse mode defaults to "any" (ON) on desktop viewports (isMobile: false, the
    // beforeEach default) and only starts "none" (OFF) on mobile — override to mobile
    // here so the button starts in the "Enable mouse mode" state this test expects.
    mockUseViewport.mockReturnValue({
      isMobile: true,
      isFoldable: false,
      isInnerScreen: true,
      hasFinePointer: false,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /enable mouse mode/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "mouse", state: "on" }),
    }));
  });

  it("fires track with button:debug state:on when Debug enabled", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /enable debug mode/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "debug", state: "on" }),
    }));
  });

  it("fires track with button:debug state:off when Debug disabled", () => {
    renderTerminal();
    // Click once to enable
    fireEvent.click(screen.getByRole("button", { name: /enable debug mode/i }));
    // Click again to disable
    fireEvent.click(screen.getByRole("button", { name: /disable debug mode/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "debug", state: "off" }),
    }));
  });

  it("fires track with button:log-stream state:on when Log Stream enabled", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /enable verbose debug log streaming/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "log-stream", state: "on" }),
    }));
  });

  it("fires track with button:record state:on when Record started", () => {
    renderTerminal();
    fireEvent.click(screen.getByRole("button", { name: /start recording terminal output/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "record", state: "on" }),
    }));
  });

  it("fires track with button:dev-panel state:closed when dev toggle clicked while open", () => {
    // beforeEach sets dev-toolbar=true so panel is open
    renderTerminal();
    fireEvent.click(screen.getByTestId("toolbar-dev-toggle"));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "dev-panel", state: "closed" }),
    }));
  });

  it("fires track with button:dev-panel state:open when dev toggle clicked while closed", () => {
    localStorage.removeItem("stapler-squad-dev-toolbar");
    renderTerminal();
    fireEvent.click(screen.getByTestId("toolbar-dev-toggle"));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      labels: expect.objectContaining({ button: "dev-panel", state: "open" }),
    }));
  });
});

// ── Dev panel behavior tests (TC-B-*) ─────────────────────────────────────────

describe("dev panel behavior", () => {
  it("dev panel defaults closed when localStorage not set", () => {
    localStorage.removeItem("stapler-squad-dev-toolbar");
    renderTerminal();
    expect(screen.queryByTestId("toolbar-dev-group")).not.toBeInTheDocument();
  });

  it("dev panel starts open when localStorage key is true", () => {
    // beforeEach sets this
    renderTerminal();
    expect(screen.getByTestId("toolbar-dev-group")).toBeInTheDocument();
  });

  it("dev panel opens inline when dev toggle is clicked", () => {
    localStorage.removeItem("stapler-squad-dev-toolbar");
    renderTerminal();
    fireEvent.click(screen.getByTestId("toolbar-dev-toggle"));
    expect(screen.getByTestId("toolbar-dev-group")).toBeInTheDocument();
  });

  it("dev panel collapses when dev toggle is clicked again", () => {
    // beforeEach: dev panel starts open
    renderTerminal();
    expect(screen.getByTestId("toolbar-dev-group")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("toolbar-dev-toggle"));
    expect(screen.queryByTestId("toolbar-dev-group")).not.toBeInTheDocument();
  });

  it("closing dev panel writes false to localStorage", () => {
    const setItemSpy = jest.spyOn(Storage.prototype, "setItem");
    renderTerminal();
    fireEvent.click(screen.getByTestId("toolbar-dev-toggle")); // close
    expect(setItemSpy).toHaveBeenCalledWith("stapler-squad-dev-toolbar", "false");
  });

  it("Debug button is not in DOM when dev panel is closed", () => {
    localStorage.removeItem("stapler-squad-dev-toolbar");
    renderTerminal();
    expect(screen.queryByRole("button", { name: /enable debug mode/i })).not.toBeInTheDocument();
  });

  it("dev toggle button has aria-expanded=false when panel closed", () => {
    localStorage.removeItem("stapler-squad-dev-toolbar");
    renderTerminal();
    const toggle = screen.getByTestId("toolbar-dev-toggle");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("dev toggle button has aria-expanded=true when panel open", () => {
    renderTerminal();
    const toggle = screen.getByTestId("toolbar-dev-toggle");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  it("dev group inner div has role=group and aria-label when panel open", () => {
    renderTerminal();
    const group = document.getElementById("toolbar-dev-group-inner");
    expect(group).toBeInTheDocument();
    expect(group).toHaveAttribute("role", "group");
    expect(group).toHaveAttribute("aria-label", "Developer tools");
  });

  it("Record button has aria-label for accessibility", () => {
    renderTerminal();
    const recordBtn = screen.getByRole("button", { name: /start recording terminal output/i });
    expect(recordBtn).toBeInTheDocument();
  });
});

// ── Compact toolbar auto-collapse tests (mouse+keyboard-on-mobile detection) ──

describe("compact toolbar auto-collapse", () => {
  it("collapses the toolbar and hides the mobile keyboard row when isMobile+hasFinePointer with default (auto) override", () => {
    mockUseViewport.mockReturnValue({
      isMobile: true,
      isFoldable: false,
      isInnerScreen: false,
      hasFinePointer: true,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    expect(screen.getByTestId("toolbar-toggle")).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryAllByTestId("mobile-key")).toHaveLength(0);
  });

  it("does not compact when isMobile but no fine pointer detected (touch-only phone)", () => {
    mockUseViewport.mockReturnValue({
      isMobile: true,
      isFoldable: false,
      isInnerScreen: false,
      hasFinePointer: false,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    expect(screen.getByTestId("toolbar-toggle")).toHaveAttribute("aria-expanded", "true");
    expect(screen.queryAllByTestId("mobile-key").length).toBeGreaterThan(0);
  });

  it("respects an explicit prior keyboard-visibility choice instead of forcing it hidden", () => {
    localStorage.setItem("stapler-squad-mobile-keyboard-visible-session-abc", "true");
    mockUseViewport.mockReturnValue({
      isMobile: true,
      isFoldable: false,
      isInnerScreen: false,
      hasFinePointer: true,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    // Toolbar still collapses (no such override guard exists for it)...
    expect(screen.getByTestId("toolbar-toggle")).toHaveAttribute("aria-expanded", "false");
    // ...but the user's explicit choice to keep the keyboard row visible is respected.
    expect(screen.queryAllByTestId("mobile-key").length).toBeGreaterThan(0);
  });

  it("forces compact mode with inputModeOverride:desktop even without a fine pointer", () => {
    localStorage.setItem("stapler-squad:input-mode-override", "desktop");
    mockUseViewport.mockReturnValue({
      isMobile: false,
      isFoldable: false,
      isInnerScreen: true,
      hasFinePointer: false,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    expect(screen.getByTestId("toolbar-toggle")).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryAllByTestId("mobile-key")).toHaveLength(0);
  });

  it("never compacts with inputModeOverride:touch even when isMobile+hasFinePointer", () => {
    localStorage.setItem("stapler-squad:input-mode-override", "touch");
    mockUseViewport.mockReturnValue({
      isMobile: true,
      isFoldable: false,
      isInnerScreen: false,
      hasFinePointer: true,
      isVirtualKeyboardOpen: false,
    });
    renderTerminal();
    expect(screen.getByTestId("toolbar-toggle")).toHaveAttribute("aria-expanded", "true");
    expect(screen.queryAllByTestId("mobile-key").length).toBeGreaterThan(0);
  });
});
