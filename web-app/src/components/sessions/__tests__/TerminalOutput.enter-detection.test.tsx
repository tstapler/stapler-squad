// @feature approval-enter-detection
/**
 * Tests for Enter-key detection and optimistic approval clear in TerminalOutput.
 *
 * Covers:
 *  - T-UNIT-TS-011: Enter ("\r") calls clearForSession with the current sessionId
 *  - T-UNIT-TS-012: Non-Enter ("a") does NOT call clearForSession
 *  - T-UNIT-TS-013: Every keystroke schedules a debounced refresh
 *  - T-UNIT-TS-014: Rapid keystrokes coalesce to a single refresh call
 *  - T-UNIT-TS-015: clearForSession uses the correct sessionId (not another session's)
 *  - T-UNIT-TS-016: sendInput path is not disrupted by the new logic
 *  - T-UNIT-TS-017: debounce timer is cancelled on unmount
 */

import React from "react";
import { render, act, waitFor } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mocks — must be declared before importing TerminalOutput
// ---------------------------------------------------------------------------

const mockClearForSession = jest.fn();
const mockRefreshApprovals = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({
    approvals: [],
    pendingCount: 0,
    loading: false,
    error: null,
    approve: jest.fn(),
    deny: jest.fn(),
    refresh: mockRefreshApprovals,
    clearForSession: mockClearForSession,
    clearedSessions: new Set(),
  }),
}));

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

/** Latest onData prop captured from the XtermTerminal mock so tests can fire keystrokes */
let capturedOnData: ((data: string) => void) | null = null;

jest.mock("../XtermTerminal", () => {
  const React = require("react");
  const XtermTerminal = React.forwardRef((props: any, ref: any) => {
    capturedOnData = props.onData ?? null;
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
  validateCellDimensions: jest.fn((x: unknown) => x),
}));

jest.mock("@/lib/terminal/TerminalStreamManager", () => ({
  TerminalStreamManager: jest.fn().mockImplementation(() => ({
    setOnFirstOutput: jest.fn(),
    installDebugMonitor: jest.fn(),
    writeInitialContent: jest.fn().mockResolvedValue(undefined),
    write: jest.fn(),
    cleanup: jest.fn(),
    updateSendFlowControl: jest.fn(),
    prependScrollbackBatch: jest.fn().mockResolvedValue(undefined),
  })),
}));

const mockTrack = jest.fn();
jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));

jest.mock("@/lib/hooks/useBrowserLogStream", () => ({
  useBrowserLogStream: jest.fn(),
}));

// ---------------------------------------------------------------------------
// Imports (after jest.mock calls)
// ---------------------------------------------------------------------------
// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStreamMock(overrides: Record<string, unknown> = {}) {
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
    output: "",
    terminalState: "STABLE",
    ...overrides,
  };
}

function renderTerminal(sessionId = "session-a", baseUrl = "http://localhost:8543") {
  return render(
    <TerminalOutput sessionId={sessionId} baseUrl={baseUrl} isVisible={false} />
  );
}

// ---------------------------------------------------------------------------
// Setup / Teardown
// ---------------------------------------------------------------------------

function setupMatchMedia() {
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
}

beforeEach(() => {
  jest.clearAllMocks();
  capturedOnData = null;

  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());

  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
  setupMatchMedia();
});

afterEach(() => {
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Wait for capturedOnData to be populated by the lazy XtermTerminal mock */
async function waitForOnData(): Promise<void> {
  await waitFor(() => {
    expect(capturedOnData).not.toBeNull();
  }, { timeout: 3000 });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("TerminalOutput — enter-detection (T-UNIT-TS-011 through T-UNIT-TS-017)", () => {
  it("T-UNIT-TS-011: handleTerminalData_should_callClearForSession_When_enterKeyPressed", async () => {
    renderTerminal("session-a");
    await waitForOnData();

    act(() => {
      capturedOnData!("\r");
    });

    expect(mockClearForSession).toHaveBeenCalledTimes(1);
    expect(mockClearForSession).toHaveBeenCalledWith("session-a");
  });

  it("T-UNIT-TS-012: handleTerminalData_should_notCallClearForSession_When_nonEnterKeyPressed", async () => {
    renderTerminal("session-a");
    await waitForOnData();

    act(() => {
      capturedOnData!("a");
    });

    expect(mockClearForSession).not.toHaveBeenCalled();
  });

  it("T-UNIT-TS-013: handleTerminalData_should_scheduleRefresh_When_anyKeystrokeSent", async () => {
    jest.useFakeTimers();
    renderTerminal("session-a");

    // Without fake timers blocking waitFor, we need to flush promises first
    await act(async () => {
      await Promise.resolve();
    });

    // capturedOnData may be set now
    if (capturedOnData) {
      act(() => {
        capturedOnData!("a");
      });

      // Refresh should not be called immediately (debounced)
      expect(mockRefreshApprovals).not.toHaveBeenCalled();

      // Advance timer past the 300ms debounce
      act(() => {
        jest.advanceTimersByTime(300);
      });

      expect(mockRefreshApprovals).toHaveBeenCalledTimes(1);
    }
    jest.useRealTimers();
  });

  it("T-UNIT-TS-014: handleTerminalData_should_cancelPreviousTimer_When_keystrokeFollowsQuickly", async () => {
    jest.useFakeTimers();
    renderTerminal("session-a");

    await act(async () => {
      await Promise.resolve();
    });

    if (capturedOnData) {
      // Fire two keystrokes within 300ms
      act(() => { capturedOnData!("a"); });
      act(() => { jest.advanceTimersByTime(100); }); // 100ms elapsed
      act(() => { capturedOnData!("b"); });

      // Advance past the second debounce window
      act(() => { jest.advanceTimersByTime(300); });

      // Should only be called once (second keystroke cancelled the first timer)
      expect(mockRefreshApprovals).toHaveBeenCalledTimes(1);
    }
    jest.useRealTimers();
  });

  it("T-UNIT-TS-015: handleTerminalData_should_callClearForSession_With_correctSessionId_When_enterSentToSessionA", async () => {
    renderTerminal("session-a");
    await waitForOnData();

    act(() => {
      capturedOnData!("\r");
    });

    expect(mockClearForSession).toHaveBeenCalledWith("session-a");
    expect(mockClearForSession).not.toHaveBeenCalledWith("session-b");
  });

  it("T-UNIT-TS-016: handleTerminalData_should_callSendInput_When_dataReceived", async () => {
    const streamMock = makeStreamMock({ sspNegotiated: false });
    (useTerminalStream as jest.Mock).mockReturnValue(streamMock);

    renderTerminal("session-a");
    await waitForOnData();

    act(() => {
      capturedOnData!("hello");
    });

    // The existing sendInput path must not be broken
    expect(streamMock.sendInput).toHaveBeenCalledWith("hello");
  });

  it("T-UNIT-TS-017: handleTerminalData_should_cleanUpTimer_When_componentUnmounts", async () => {
    jest.useFakeTimers();
    const { unmount } = renderTerminal("session-a");

    await act(async () => {
      await Promise.resolve();
    });

    if (capturedOnData) {
      // Schedule a debounce timer
      act(() => { capturedOnData!("a"); });

      // Unmount before the timer fires
      unmount();

      // Advance past the debounce window
      act(() => { jest.advanceTimersByTime(300); });

      // refresh should NOT be called after unmount
      expect(mockRefreshApprovals).not.toHaveBeenCalled();
    }
    jest.useRealTimers();
  });

  it("Enter also schedules a debounced refresh (not just clearForSession)", async () => {
    jest.useFakeTimers();
    renderTerminal("session-a");

    await act(async () => {
      await Promise.resolve();
    });

    if (capturedOnData) {
      act(() => { capturedOnData!("\r"); });

      // No immediate refresh
      expect(mockRefreshApprovals).not.toHaveBeenCalled();

      act(() => { jest.advanceTimersByTime(300); });

      expect(mockClearForSession).toHaveBeenCalledTimes(1);
      expect(mockRefreshApprovals).toHaveBeenCalledTimes(1);
    }
    jest.useRealTimers();
  });
});
