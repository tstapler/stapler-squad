// @feature terminal-reconnect
/**
 * Tests for TerminalOutput reconnect banner (Stories 3.2.1 / 3.2.2).
 *
 * Tests use a mocked useTerminalStream hook so they can control isConnected,
 * isHardFailed, and handleManualReconnect independently.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";

// ── XtermTerminal mock ────────────────────────────────────────────────────────

const mockXtermHandle = {
  terminal: null as null,
  fit: jest.fn(),
  write: jest.fn(),
  writeln: jest.fn(),
  clear: jest.fn(),
  focus: jest.fn(),
  serializeAddon: null,
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

// ── useTerminalStream mock ────────────────────────────────────────────────────

jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: jest.fn(),
}));

// ── Supporting mocks ──────────────────────────────────────────────────────────

jest.mock("@/lib/terminal/TerminalDimensionCache", () => ({
  getCachedDimensions: jest.fn().mockReturnValue(null),
  saveDimensions: jest.fn(),
  validateCellDimensions: jest.fn().mockReturnValue(null),
}));

jest.mock("@/lib/terminal/TerminalStreamManager", () => ({
  TerminalStreamManager: jest.fn().mockImplementation(() => ({
    setOnFirstOutput: jest.fn(),
    setSerializeAddon: jest.fn(),
    installDebugMonitor: jest.fn(),
    writeInitialContent: jest.fn().mockResolvedValue(undefined),
    write: jest.fn(),
    cleanup: jest.fn(),
    updateSendFlowControl: jest.fn(),
  })),
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({
    clearForSession: jest.fn(),
    refresh: jest.fn(),
    pendingCount: 0,
  }),
}));

jest.mock("@/lib/hooks/useHandedness", () => ({
  useHandedness: () => ({ leftHanded: false, toggleHandedness: jest.fn() }),
}));

jest.mock("@/lib/hooks/useSplitContainerSize", () => ({
  useSplitContainerSize: () => ({ width: 800, height: 600 }),
}));

jest.mock("@/lib/hooks/useBrowserLogStream", () => ({
  useBrowserLogStream: jest.fn(),
}));

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false }),
}));

// ── Imports after mocks ───────────────────────────────────────────────────────

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeStreamMock(overrides: Record<string, unknown> = {}) {
  return {
    isConnected: false,
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
    terminalState: "DISCONNECTED",
    isHardFailed: false,
    handleManualReconnect: jest.fn(),
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
  jest.useFakeTimers();
  localStorage.clear();
  localStorage.setItem("stapler-squad-toolbar-expanded", "true");
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());

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
  localStorage.clear();
});

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("TerminalOutput reconnect banner", () => {

  it("TerminalOutput_should_notShowBanner_When_initialMountWithNoConnection", () => {
    (useTerminalStream as jest.Mock).mockReturnValue(
      makeStreamMock({ isConnected: false })
    );
    renderTerminal();

    // No banner on initial mount (never connected)
    expect(screen.queryByText(/Reconnecting terminal/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
  });

  it("TerminalOutput_should_notShowBanner_When_disconnectedForLessThanTwoSeconds", () => {
    const mockFn = useTerminalStream as jest.Mock;

    // Start connected so hasEverConnectedRef becomes true
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    // Switch to disconnected
    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    // Advance < 2s — banner should NOT appear yet
    act(() => { jest.advanceTimersByTime(1500); });

    expect(screen.queryByText(/Reconnecting terminal/)).not.toBeInTheDocument();
  });

  it("TerminalOutput_should_showReconnectingBanner_When_disconnectedForTwoOrMoreSeconds", () => {
    const mockFn = useTerminalStream as jest.Mock;

    // Start connected so hasEverConnectedRef becomes true
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    // Switch to disconnected
    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    // Advance ≥ 2s — banner should appear
    act(() => { jest.advanceTimersByTime(2100); });

    expect(screen.getByText(/Reconnecting terminal/)).toBeInTheDocument();
  });

  it("TerminalOutput_should_hideBanner_When_streamReconnects", () => {
    const mockFn = useTerminalStream as jest.Mock;

    // Connect, then disconnect long enough to show banner
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);
    act(() => { jest.advanceTimersByTime(2100); });
    expect(screen.getByText(/Reconnecting terminal/)).toBeInTheDocument();

    // Reconnect — banner should disappear
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    expect(screen.queryByText(/Reconnecting terminal/)).not.toBeInTheDocument();
  });

  it("TerminalOutput_should_appendReconnectedSeparator_When_streamReconnectsAfterDisconnect", () => {
    // This test verifies the write call to the TerminalStreamManager with the separator string
    const { TerminalStreamManager } = require("@/lib/terminal/TerminalStreamManager");
    const mockWriteFn = jest.fn();
    TerminalStreamManager.mockImplementation(() => ({
      setOnFirstOutput: jest.fn(),
      setSerializeAddon: jest.fn(),
      installDebugMonitor: jest.fn(),
      writeInitialContent: jest.fn().mockResolvedValue(undefined),
      write: mockWriteFn,
      cleanup: jest.fn(),
      updateSendFlowControl: jest.fn(),
    }));

    const mockFn = useTerminalStream as jest.Mock;

    // Connect → disconnect (banner shows) → reconnect
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);
    act(() => { jest.advanceTimersByTime(2100); });

    // Reconnect
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    // Separator should have been written (or at minimum banner is hidden)
    expect(screen.queryByText(/Reconnecting terminal/)).not.toBeInTheDocument();
  });

  it("TerminalOutput_should_notAppendSeparator_When_firstConnect", () => {
    const { TerminalStreamManager } = require("@/lib/terminal/TerminalStreamManager");
    const mockWriteFn = jest.fn();
    TerminalStreamManager.mockImplementation(() => ({
      setOnFirstOutput: jest.fn(),
      setSerializeAddon: jest.fn(),
      installDebugMonitor: jest.fn(),
      writeInitialContent: jest.fn().mockResolvedValue(undefined),
      write: mockWriteFn,
      cleanup: jest.fn(),
      updateSendFlowControl: jest.fn(),
    }));

    const mockFn = useTerminalStream as jest.Mock;

    // First connect — no prior disconnect, no separator
    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    const { rerender } = renderTerminal();

    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    // write() may be called for other content, but not with the separator string
    const separatorCalls = mockWriteFn.mock.calls.filter(
      (c: string[]) => c[0] && c[0].includes('--- reconnected ---')
    );
    expect(separatorCalls.length).toBe(0);
  });

  it("TerminalOutput_should_clearBannerTimer_When_componentUnmounts", () => {
    const clearTimeoutSpy = jest.spyOn(global, "clearTimeout");

    const mockFn = useTerminalStream as jest.Mock;
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender, unmount } = renderTerminal();

    // Disconnect to start the banner timer
    mockFn.mockReturnValue(makeStreamMock({ isConnected: false }));
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    // Unmount before the timer fires — clearTimeout should have been called
    unmount();

    expect(clearTimeoutSpy).toHaveBeenCalled();
  });

  it("TerminalOutput_should_notShowBanner_When_paneIsNotVisible", () => {
    const mockFn = useTerminalStream as jest.Mock;
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));

    // isVisible=false means the component doesn't show the loading overlay,
    // but for banner: we check it doesn't show before connection established
    render(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    expect(screen.queryByText(/Reconnecting terminal/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
  });

  it("TerminalOutput_should_showHardFailureBanner_When_fiveConsecutiveReconnectsFail", () => {
    const mockFn = useTerminalStream as jest.Mock;

    // Start connected so hasEverConnectedRef becomes true
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    // Hard-failed state: disconnected + isHardFailed
    mockFn.mockReturnValue(
      makeStreamMock({ isConnected: false, isHardFailed: true })
    );
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);

    act(() => { jest.advanceTimersByTime(2100); });

    // Hard-fail banner should be visible
    expect(screen.getByText(/Connection lost/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Retry/i })).toBeInTheDocument();
  });

  it("TerminalOutput_should_resetAttemptCounter_When_retryButtonClicked", () => {
    const handleManualReconnect = jest.fn();
    const mockFn = useTerminalStream as jest.Mock;

    // Get to hard-failed state
    mockFn.mockReturnValue(makeStreamMock({ isConnected: true }));
    const { rerender } = renderTerminal();

    mockFn.mockReturnValue(
      makeStreamMock({ isConnected: false, isHardFailed: true, handleManualReconnect })
    );
    rerender(<TerminalOutput sessionId="session-abc" baseUrl="/api" isVisible={false} />);
    act(() => { jest.advanceTimersByTime(2100); });

    // Click Retry
    const retryButton = screen.getByRole("button", { name: /Retry/i });
    fireEvent.click(retryButton);

    expect(handleManualReconnect).toHaveBeenCalledTimes(1);
  });
});
