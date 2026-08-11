/**
 * Regression tests for TerminalOutput's resize() call sites (Epic 2/4,
 * Story 4.2): the post-connection resync effect and the manual Fit button
 * must pass a literal `force: true` third argument, while the automatic
 * value-changed resize path must not, so it keeps benefiting from
 * useTerminalFlowControl's value-dedup.
 *
 * useTerminalStream is mocked so the component renders without a real
 * ConnectRPC connection (Task 4.2.1). XtermTerminal is mocked at the module
 * boundary too, exposing a minimal imperative handle (fit/terminal) and
 * capturing the onResize prop so tests can simulate the child firing resize
 * events without mounting real xterm.js.
 */

import { render, act, fireEvent } from "@testing-library/react";
import React from "react";

jest.mock("../XtermTerminal", () => {
  const ReactLib = require("react");
  const state: {
    onResize: ((cols: number, rows: number) => void) | null;
    cols: number;
    rows: number;
    fit: jest.Mock;
  } = {
    onResize: null,
    cols: 80,
    rows: 24,
    fit: jest.fn(),
  };

  const MockXtermTerminal = ReactLib.forwardRef((props: any, ref: any) => {
    state.onResize = props.onResize ?? null;

    ReactLib.useImperativeHandle(ref, () => ({
      get terminal() {
        return {
          get cols() {
            return state.cols;
          },
          get rows() {
            return state.rows;
          },
        };
      },
      write: jest.fn(),
      writeln: jest.fn(),
      clear: jest.fn(),
      focus: jest.fn(),
      fit: () => {
        state.cols = 100;
        state.rows = 30;
        state.fit();
      },
      search: jest.fn(() => false),
      searchNext: jest.fn(() => false),
      searchPrevious: jest.fn(() => false),
    }));

    // Simulate XtermTerminal's real behavior of firing an initial resize
    // callback once the terminal is ready.
    ReactLib.useEffect(() => {
      props.onResize?.(state.cols, state.rows);
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return null;
  });
  MockXtermTerminal.displayName = "MockXtermTerminal";

  return { XtermTerminal: MockXtermTerminal, __mockXtermState: state };
});

const mockUseTerminalStream = jest.fn();
jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: (...args: any[]) => mockUseTerminalStream(...args),
}));

// ── Supporting mocks (origin/main scaffolding TerminalOutput now depends on;
// see TerminalOutput.reconnect.test.tsx for the canonical set) ──────────────

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

import { TerminalOutput } from "../TerminalOutput";

const mockXtermState = jest.requireMock("../XtermTerminal").__mockXtermState as {
  onResize: ((cols: number, rows: number) => void) | null;
  cols: number;
  rows: number;
  fit: jest.Mock;
};

function makeStreamMock(overrides: Partial<Record<string, any>> = {}) {
  return {
    isConnected: false,
    error: null,
    output: "",
    sendInput: jest.fn(),
    resize: jest.fn(),
    connect: jest.fn(),
    disconnect: jest.fn(),
    scrollbackLoaded: true,
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

describe("TerminalOutput resize call sites", () => {
  let streamState: ReturnType<typeof makeStreamMock>;

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, "log").mockImplementation(() => {});
    jest.spyOn(console, "warn").mockImplementation(() => {});
    jest.spyOn(console, "error").mockImplementation(() => {});
    mockXtermState.onResize = null;
    mockXtermState.cols = 80;
    mockXtermState.rows = 24;
    mockXtermState.fit.mockClear();
    streamState = makeStreamMock();
    mockUseTerminalStream.mockImplementation(() => streamState);
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  // Task 4.2.2, AC4: post-connection resync effect passes a literal
  // force:true third argument.
  it("calls resize with a literal force:true third argument from the post-connection resync effect", async () => {
    const { rerender } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    // XtermTerminal is lazy-loaded (React.lazy + Suspense) in TerminalOutput, so its
    // mock's mount-time onResize(80,24) fires on a later microtask, not synchronously
    // within the initial render()'s act(). Flush that before asserting anything that
    // depends on lastResizeRef having been populated.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Initial mount: XtermTerminal's simulated onResize(80,24) fires while
    // disconnected, populating lastResizeRef without calling resize().
    expect(streamState.resize).not.toHaveBeenCalled();

    // Transition disconnected -> connected.
    streamState = makeStreamMock({ ...streamState, isConnected: true, resize: streamState.resize });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(<TerminalOutput sessionId="s1" baseUrl="http://x" />);
    });

    // The post-connection resize sync is deliberately delayed 250ms so the
    // container can settle before it fires (see TerminalOutput's connection-state
    // effect) -- flush that timer before asserting.
    act(() => {
      jest.advanceTimersByTime(250);
    });

    expect(streamState.resize).toHaveBeenCalledTimes(1);
    expect(streamState.resize).toHaveBeenCalledWith(expect.any(Number), expect.any(Number), true);
    expect(streamState.resize).toHaveBeenCalledWith(80, 24, true);
  });

  // Task 4.2.3, AC4: manual Fit button click passes a literal force:true
  // third argument, using the mocked terminal's actual post-fit cols/rows.
  it("calls resize with a literal force:true third argument from the manual Fit button handler", () => {
    const { rerender, getByRole } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    streamState = makeStreamMock({ ...streamState, isConnected: true });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(<TerminalOutput sessionId="s1" baseUrl="http://x" />);
    });
    streamState.resize.mockClear();

    // The toolbar (which holds the Resize button) starts collapsed; expand it first.
    const toolbarToggle = getByRole("button", { name: "Toggle toolbar" });
    act(() => {
      fireEvent.click(toolbarToggle);
    });

    const fitButton = getByRole("button", { name: "Resize terminal to fit container" });
    act(() => {
      fireEvent.click(fitButton);
    });

    expect(mockXtermState.fit).toHaveBeenCalledTimes(1);
    expect(streamState.resize).toHaveBeenCalledTimes(1);
    expect(streamState.resize).toHaveBeenCalledWith(100, 30, true);
  });

  // Task 4.2.4, AC4 (negative): the automatic value-changed resize path
  // (handleTerminalResize) must NOT pass a truthy force argument, so it
  // keeps benefiting from useTerminalFlowControl's value-dedup.
  it("calls resize without a truthy force argument from the automatic handleTerminalResize path", () => {
    const { rerender } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    streamState = makeStreamMock({ ...streamState, isConnected: true });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(<TerminalOutput sessionId="s1" baseUrl="http://x" />);
    });
    streamState.resize.mockClear();

    // Simulate the child terminal reporting a genuinely new size while
    // already connected -- this is the automatic path (line ~327), not the
    // post-connection resync or the manual Fit button.
    act(() => {
      mockXtermState.onResize?.(90, 28);
    });

    expect(streamState.resize).toHaveBeenCalledTimes(1);
    expect(streamState.resize).toHaveBeenCalledWith(90, 28);
    // Exact-arity check: the 2-arg call must not have picked up a 3rd
    // truthy `force` argument.
    expect(streamState.resize.mock.calls[0]).toHaveLength(2);
  });
});
