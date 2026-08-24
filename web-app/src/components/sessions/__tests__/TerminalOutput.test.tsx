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

import { render, act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";

jest.mock("../XtermTerminal", () => {
  const ReactLib = require("react");
  const state: {
    onResize: ((cols: number, rows: number) => void) | null;
    cols: number;
    rows: number;
    fit: jest.Mock;
    clear: jest.Mock;
  } = {
    onResize: null,
    cols: 80,
    rows: 24,
    fit: jest.fn(),
    clear: jest.fn(),
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
      clear: state.clear,
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
  clear: jest.Mock;
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

// Shared by both stale-buffer-overlap regression tests below (jscpd flagged the
// inlined version as a clone — see reflect-and-fix's Level 0 consolidation gate).
// Asserts the local buffer was cleared exactly once, strictly before the resize
// RPC was sent: clearing after doesn't prevent the overlap the fix exists to close.
function expectClearedBeforeResize(streamState: ReturnType<typeof makeStreamMock>) {
  expect(mockXtermState.clear).toHaveBeenCalledTimes(1);
  expect(mockXtermState.clear.mock.invocationCallOrder[0]).toBeLessThan(
    streamState.resize.mock.invocationCallOrder[0]
  );
}

// Also shared by both regression tests (same jscpd flag): transitions the mocked
// stream to connected and rerenders, returning the new streamState for the caller
// to assign back to its `let streamState`.
function connectStream(rerender: (el: React.ReactElement) => void, base: ReturnType<typeof makeStreamMock>) {
  const next = makeStreamMock({ ...base, isConnected: true });
  mockUseTerminalStream.mockImplementation(() => next);
  act(() => {
    rerender(<TerminalOutput sessionId="s1" baseUrl="http://x" />);
  });
  return next;
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
    mockXtermState.clear.mockClear();
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

    // This resync path deliberately does NOT clear the local buffer: it fires on
    // every reconnect regardless of whether dimensions changed, and would blow
    // away the "--- reconnected ---" continuity banner written just above it in
    // TerminalOutput's connection-state effect. See reflect-and-fix (2026-08-23)
    // for why this path was deliberately excluded from the clear-before-resize fix.
    expect(mockXtermState.clear).not.toHaveBeenCalled();
  });

  // Task 4.2.3, AC4: manual Fit button click passes a literal force:true
  // third argument, using the mocked terminal's actual post-fit cols/rows.
  //
  // Also a regression test (reflect-and-fix, 2026-08-23): this was one of two
  // resize-sending call sites that silently bypassed the "clear the local buffer
  // before resizing" fix when it was first applied to only the automatic
  // handleTerminalResize path -- the manual Resize button kept showing the
  // pre-resize/post-resize overlap bug the fix was supposed to close. Asserting
  // clear() here (and that it happens strictly before resize()) pins the fix to
  // this call site so a future refactor can't silently drop it again.
  it("clears the local buffer before calling resize with a literal force:true third argument from the manual Fit button handler", () => {
    const { rerender, getByRole } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    streamState = connectStream(rerender, streamState);
    streamState.resize.mockClear();
    mockXtermState.clear.mockClear();

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
    expectClearedBeforeResize(streamState);
  });

  // Task 4.2.4, AC4 (negative): the automatic value-changed resize path
  // (handleTerminalResize) must NOT pass a truthy force argument, so it
  // keeps benefiting from useTerminalFlowControl's value-dedup.
  //
  // Also a regression test (reflect-and-fix, 2026-08-23) for the stale-buffer
  // overlap bug: see the manual Fit button test above for full context.
  it("clears the local buffer before calling resize without a truthy force argument from the automatic handleTerminalResize path", () => {
    const { rerender } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    streamState = connectStream(rerender, streamState);
    streamState.resize.mockClear();
    mockXtermState.clear.mockClear();

    // Simulate the child terminal reporting a genuinely new size while
    // already connected -- this is the automatic path (line ~327), not the
    // post-connection resync or the manual Fit button.
    act(() => {
      mockXtermState.onResize?.(90, 28);
    });

    expect(streamState.resize).toHaveBeenCalledTimes(1);
    expect(streamState.resize).toHaveBeenCalledWith(90, 28);
    expectClearedBeforeResize(streamState);
    // Exact-arity check: the 2-arg call must not have picked up a 3rd
    // truthy `force` argument.
    expect(streamState.resize.mock.calls[0]).toHaveLength(2);
  });
});

// Task 8.3.1.3 — hard-failed banner's Retry control must be reachable via
// normal tab order (no tabIndex={-1} skip) and activatable with Enter/Space,
// per design/ux.md §2 Accessibility "Keyboard" bullet.
describe("TerminalOutput hardFailedBanner Retry keyboard accessibility", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, "log").mockImplementation(() => {});
    jest.spyOn(console, "warn").mockImplementation(() => {});
    jest.spyOn(console, "error").mockImplementation(() => {});
    mockXtermState.onResize = null;
    mockXtermState.cols = 80;
    mockXtermState.rows = 24;
    mockXtermState.fit.mockClear();
    mockXtermState.clear.mockClear();
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it("TerminalOutput_should_ReachRetryViaNormalTabOrderAndActivateOnEnterAndSpace_When_HardFailedBannerShown", async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    const handleManualReconnect = jest.fn();

    // Start connected so hasEverConnectedRef becomes true, then transition to
    // hard-failed so the hardFailedBanner (and its Retry button) renders.
    let streamState = makeStreamMock({ isConnected: true });
    mockUseTerminalStream.mockImplementation(() => streamState);
    const { rerender } = render(<TerminalOutput sessionId="s1" baseUrl="http://x" />);

    streamState = makeStreamMock({
      isConnected: false,
      isHardFailed: true,
      handleManualReconnect,
    });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(<TerminalOutput sessionId="s1" baseUrl="http://x" />);
    });
    act(() => {
      jest.advanceTimersByTime(2100);
    });

    const button = screen.getByRole("button", { name: /Retry/i });

    // Not explicitly removed from tab order.
    expect(button).not.toHaveAttribute("tabindex", "-1");

    // Reachable via normal tab order: focus starts at document.body, and
    // repeatedly tabbing forward must land on the Retry button without
    // requiring any out-of-band focus() call.
    let reached = false;
    for (let i = 0; i < 25; i++) {
      // eslint-disable-next-line no-await-in-loop
      await user.tab();
      if (document.activeElement === button) {
        reached = true;
        break;
      }
    }
    expect(reached).toBe(true);

    // Enter activates it (native button semantics).
    await user.keyboard("{Enter}");
    expect(handleManualReconnect).toHaveBeenCalledTimes(1);

    // Space activates it too.
    button.focus();
    await user.keyboard(" ");
    expect(handleManualReconnect).toHaveBeenCalledTimes(2);
  });
});
