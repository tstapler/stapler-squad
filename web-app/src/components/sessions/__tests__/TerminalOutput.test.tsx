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

  // Story 1.4.0 — fake .xterm-viewport element with real
  // addEventListener/removeEventListener so TerminalOutput's wheel/scroll
  // listener effect can be exercised. Handlers are stored by type (single
  // slot per type, matching this component's own listener usage).
  const viewportHandlers: Record<string, EventListener> = {};
  const viewportEl = {
    addEventListener: (type: string, listener: EventListener) => {
      viewportHandlers[type] = listener;
    },
    removeEventListener: (type: string) => {
      delete viewportHandlers[type];
    },
  };
  const fireViewport = (type: string, event: unknown) => {
    viewportHandlers[type]?.(event as Event);
  };

  const state: {
    onResize: ((cols: number, rows: number) => void) | null;
    cols: number;
    rows: number;
    fit: jest.Mock;
    clear: jest.Mock;
    isAltScreenActive: (() => boolean) | null;
    onAltScreenScrollUp: ((lines: number) => void) | null;
  } = {
    onResize: null,
    cols: 80,
    rows: 24,
    fit: jest.fn(),
    clear: jest.fn(),
    isAltScreenActive: null,
    onAltScreenScrollUp: null,
  };

  const MockXtermTerminal = ReactLib.forwardRef((props: any, ref: any) => {
    state.onResize = props.onResize ?? null;
    state.isAltScreenActive = props.isAltScreenActive ?? null;
    state.onAltScreenScrollUp = props.onAltScreenScrollUp ?? null;

    ReactLib.useImperativeHandle(ref, () => ({
      get terminal() {
        return {
          get cols() {
            return state.cols;
          },
          get rows() {
            return state.rows;
          },
          buffer: { active: { viewportY: 0 } },
          scrollToBottom: jest.fn(),
          element: {
            querySelector: () => viewportEl,
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

  return { XtermTerminal: MockXtermTerminal, __mockXtermState: state, __fireViewportEvent: fireViewport };
});

const mockUseTerminalStream = jest.fn();
jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: (...args: any[]) => mockUseTerminalStream(...args),
}));

// ── Supporting mocks (origin/main scaffolding TerminalOutput now depends on;
// see TerminalOutput.reconnect.test.tsx for the canonical set) ──────────────

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

import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";
import { ScrollForwardOutcome, ScrollBlockedReason } from "@/gen/session/v1/events_pb";

// Story 3 — TerminalOutput now sources its xterm.js instance from
// TerminalPoolProvider (see TerminalPool.tsx); every render/rerender in this
// file must be wrapped in one, matching production's app-level provider.
function withPool(children: React.ReactNode) {
  return <TerminalPoolProvider>{children}</TerminalPoolProvider>;
}

const mockXtermState = jest.requireMock("../XtermTerminal").__mockXtermState as {
  onResize: ((cols: number, rows: number) => void) | null;
  cols: number;
  rows: number;
  fit: jest.Mock;
  clear: jest.Mock;
  isAltScreenActive: (() => boolean) | null;
  onAltScreenScrollUp: ((lines: number) => void) | null;
};
const fireViewportEvent = jest.requireMock("../XtermTerminal").__fireViewportEvent as (type: string, event: unknown) => void;
const mockManagerState = jest.requireMock("@/lib/terminal/TerminalStreamManager").__mockManagerState as { instance: any };

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
    rerender(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
  });
  return next;
}

// Flushes XtermTerminal's mount-time onResize microtask, then lets
// TerminalOutput's own size-stability debounce (50ms setTimeout + two nested
// requestAnimationFrame callbacks — see the "Event-driven size stability
// detection" block in TerminalOutput.tsx) elapse so isWaitingForStableSize
// clears. Needed before badge/overlay assertions that depend on text driven
// by isConnected/terminalState rather than the stabilizing state.
async function settleSizeStabilityWait() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  act(() => {
    jest.advanceTimersByTime(50);
  });
  act(() => {
    jest.advanceTimersByTime(32);
  });
}

// Shared by every describe block below (jscpd flagged the inlined version as
// a 3x clone): fake timers, silenced console, and a freshly reset mock xterm
// handle. Each describe still owns its own afterEach cleanup.
function resetSharedTerminalMocks() {
  jest.useFakeTimers();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
  mockXtermState.onResize = null;
  mockXtermState.cols = 80;
  mockXtermState.rows = 24;
  mockXtermState.fit.mockClear();
  mockXtermState.clear.mockClear();
}

describe("TerminalOutput resize call sites", () => {
  let streamState: ReturnType<typeof makeStreamMock>;

  beforeEach(() => {
    resetSharedTerminalMocks();
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
    const { rerender } = render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));

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
      rerender(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
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
    const { rerender, getByRole } = render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));

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
    const { rerender } = render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));

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
  beforeEach(resetSharedTerminalMocks);

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
    const { rerender } = render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));

    streamState = makeStreamMock({
      isConnected: false,
      isHardFailed: true,
      handleManualReconnect,
    });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
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

// Regression test: a brand-new session whose first connect attempt drops
// before any content has ever loaded must show a startup state, not the
// same bare "Disconnected" badge/overlay used for a real post-load drop.
describe("TerminalOutput startup state (never-connected content race)", () => {
  beforeEach(resetSharedTerminalMocks);

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it("TerminalOutput_should_showConnectingBadge_When_terminalStateIsConnecting", async () => {
    const streamState = makeStreamMock({ isConnected: false, terminalState: "CONNECTING" });
    mockUseTerminalStream.mockImplementation(() => streamState);
    render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
    await settleSizeStabilityWait();

    expect(screen.getByText("Connecting...")).toBeInTheDocument();
  });

  it("TerminalOutput_should_keepStartupOverlay_When_connectionDropsBeforeAnyContentLoaded", async () => {
    // Mount disconnected (as every real session does) and let the size-stability
    // wait clear before the first connect — mirrors production, where isConnected
    // never flips true until after that wait resolves.
    let streamState = makeStreamMock({ isConnected: false });
    mockUseTerminalStream.mockImplementation(() => streamState);
    const { rerender } = render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
    await settleSizeStabilityWait();

    // First connect lands...
    streamState = makeStreamMock({ isConnected: true });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
    });

    // ...then drops before the mocked TerminalStreamManager ever calls its
    // setOnFirstOutput callback, i.e. no content has loaded (the default —
    // see terminalOutputTestMocks.ts's terminalStreamManagerWithSerializeAddonMockModule).
    streamState = makeStreamMock({ isConnected: false, terminalState: "DISCONNECTED" });
    mockUseTerminalStream.mockImplementation(() => streamState);
    act(() => {
      rerender(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
    });

    // The startup overlay stays up with its own message instead of falling
    // through to the generic "Disconnected" badge/overlay treatment.
    expect(screen.getByText("Starting session...")).toBeInTheDocument();
  });
});

// Epic 1.4 — scroll-forward client rendering (Stories 1.4.0-1.4.5).
describe("TerminalOutput Epic 1.4 — scroll-forward client rendering", () => {
  beforeEach(() => {
    resetSharedTerminalMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  // Mounts, connects, and delivers the initial tmux-native scrollback so
  // isLoadingInitialContent flips false (gates the wheel/scroll listener
  // effect) and the mocked TerminalStreamManager instance exists (manager
  // creation is lazy — first triggered by handleScrollbackReceived).
  async function mountAndLoadInitialContent(overrides: Partial<Record<string, any>> = {}) {
    const streamState = makeStreamMock({ isConnected: true, ...overrides });
    mockUseTerminalStream.mockImplementation(() => streamState);
    render(withPool(<TerminalOutput sessionId="s1" baseUrl="http://x" />));
    await settleSizeStabilityWait();

    const callArgs = mockUseTerminalStream.mock.calls[mockUseTerminalStream.mock.calls.length - 1][0];
    await act(async () => {
      await callArgs.onScrollbackReceived("initial content", {
        hasMore: true, oldestSequence: 0, newestSequence: 10, totalLines: 10,
      });
    });

    return { streamState, callArgs };
  }

  // ---- Story 1.4.0 (REQ-8) — alt-screen wheel trigger ----
  describe("Story 1.4.0 — alt-screen wheel trigger", () => {
    it("calls requestScrollback on wheel scroll-up while altScreenActive is true", async () => {
      const { streamState } = await mountAndLoadInitialContent();
      act(() => { mockManagerState.instance.__onAltScreenChange?.(true); });

      act(() => { fireViewportEvent("wheel", { deltaY: -100 }); });

      expect(streamState.requestScrollback).toHaveBeenCalledWith(0, 500);
    });

    it("does not call requestScrollback on wheel scroll-up while altScreenActive is false — existing viewportY-gated trigger is unaffected", async () => {
      const { streamState } = await mountAndLoadInitialContent();

      act(() => { fireViewportEvent("wheel", { deltaY: -100 }); });

      expect(streamState.requestScrollback).not.toHaveBeenCalled();
    });

    it("does not call requestScrollback on a downward wheel (deltaY > 0) even when altScreenActive is true", async () => {
      const { streamState } = await mountAndLoadInitialContent();
      act(() => { mockManagerState.instance.__onAltScreenChange?.(true); });

      act(() => { fireViewportEvent("wheel", { deltaY: 100 }); });

      expect(streamState.requestScrollback).not.toHaveBeenCalled();
    });

    it("threads isAltScreenActive/onAltScreenScrollUp props to XtermTerminal for the touch trigger", async () => {
      await mountAndLoadInitialContent();
      act(() => { mockManagerState.instance.__onAltScreenChange?.(true); });

      expect(mockXtermState.isAltScreenActive?.()).toBe(true);
      expect(typeof mockXtermState.onAltScreenScrollUp).toBe("function");
    });
  });

  // ---- Story 1.4.5 (REQ-9b) — loading pill ----
  describe("Story 1.4.5 — loading pill", () => {
    async function triggerAltScreenScrollUp() {
      act(() => { mockManagerState.instance.__onAltScreenChange?.(true); });
      act(() => { fireViewportEvent("wheel", { deltaY: -100 }); });
    }

    it("does not show the pill before the 150ms delay elapses", async () => {
      await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();

      expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();
    });

    it("shows the pill once the 150ms delay elapses", async () => {
      await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(150); });

      expect(screen.getByTestId("scroll-loading-pill")).toBeInTheDocument();
    });

    it("shows stalled copy and a Cancel button after 8s with no response", async () => {
      await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(8000); });

      expect(screen.getByTestId("scroll-loading-pill")).toHaveTextContent(/Still trying/);
      expect(screen.getByTestId("scroll-loading-pill-cancel")).toBeInTheDocument();
    });

    it("Cancel clears the pill and resets fetch-in-flight state without cancelling the in-flight request", async () => {
      const { streamState } = await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(8000); });

      act(() => { fireEvent.click(screen.getByTestId("scroll-loading-pill-cancel")); });
      expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();

      // Fetch-in-flight was reset (not the original request cancelled) — a new
      // scroll-up gesture can fire again immediately.
      streamState.requestScrollback.mockClear();
      await triggerAltScreenScrollUp();
      expect(streamState.requestScrollback).toHaveBeenCalled();
    });

    it("clears the pill unconditionally when a DELIVERED AppScrollbackResponse arrives", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(150); });
      expect(screen.getByTestId("scroll-loading-pill")).toBeInTheDocument();

      await act(async () => {
        callArgs.onAppScrollback("content", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });

      expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();
    });

    it("clears the pill unconditionally when a BLOCKED AppScrollbackResponse arrives", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(150); });

      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.LEASE_CONTENTION);
      });

      expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();
    });

    it("clears the pill unconditionally when the plain tmux-native ScrollbackResponse arrives", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await triggerAltScreenScrollUp();
      act(() => { jest.advanceTimersByTime(150); });

      await act(async () => {
        await callArgs.onScrollbackReceived("more content", {
          hasMore: true, oldestSequence: 0, newestSequence: 20, totalLines: 20,
        });
      });

      expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();
    });
  });

  // ---- Story 1.4.2 (REQ-10) — ScrollSourceIndicator banner ----
  describe("Story 1.4.2 — ScrollSourceIndicator banner", () => {
    it("shows 'Viewing <Program>'s own history' on DELIVERED and hides on a genuinely different output frame", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      await act(async () => {
        callArgs.onAppScrollback("\x1b[2J\x1b[H1866 1867 1868", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      expect(screen.getByTestId("scroll-source-indicator")).toHaveTextContent("Viewing Claude Code's own history");

      // A frame that doesn't match the last-forwarded content is trusted as
      // a genuine live resume immediately, with no delay needed -- proves
      // this isn't a timing race (regression guard: an earlier count/time-
      // based implementation still swallowed a genuine live-resume that
      // happened to arrive first, found via a real failure in
      // scroll-forward-return-to-live.spec.ts).
      act(() => { callArgs.onOutput("claude> (resumed) continuing the live session..."); });
      expect(screen.queryByTestId("scroll-source-indicator")).not.toBeInTheDocument();
    });

    it("does NOT hide when the output frame is ForwardScroll's own PageUp echo (matches the last forwarded content)", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      await act(async () => {
        callArgs.onAppScrollback("\x1b[2J\x1b[H1866 1867 1868", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      expect(screen.getByTestId("scroll-source-indicator")).toHaveTextContent("Viewing Claude Code's own history");

      // A normal output frame whose content matches what ForwardScroll just
      // captured for this outcome is the redraw echo of its own keystroke
      // send, not a live resume -- it must not clear the banner (regression
      // guard for the real bug found via scroll-forward-outcome-signals-
      // distinct.spec.ts's AT_TOP case: the banner vanished right after
      // AT_TOP fired, because that outcome's own PageUp echo arrived as an
      // ordinary output frame and was misread as a genuine live resume).
      act(() => { callArgs.onOutput("\x1b[2J\x1b[H1866 1867 1868"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();

      // A later frame that genuinely differs still clears it -- the
      // signature is compared by content each time, not consumed/cleared
      // after the first match (a redraw can legitimately span more than one
      // output frame).
      act(() => { callArgs.onOutput("claude> (resumed) continuing the live session..."); });
      expect(screen.queryByTestId("scroll-source-indicator")).not.toBeInTheDocument();
    });

    it("does NOT hide on a SECOND frame that's still part of the same echoed redraw (e.g. a trailing prompt line)", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      await act(async () => {
        callArgs.onAppScrollback("\x1b[2J\x1b[H1866 1867 1868", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      // First echo fragment.
      act(() => { callArgs.onOutput("\x1b[2J\x1b[H1866 1867 1868"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();
      // A second fragment of the *same* redraw (e.g. the prompt line
      // arriving as its own batched frame) must not be misread as a
      // genuine resume just because the first fragment already matched
      // (regression guard: clearing the signature after one match did
      // exactly that -- found via a real, reproducible failure in
      // scroll-forward-outcome-signals-distinct.spec.ts's AT_TOP case).
      act(() => { callArgs.onOutput("\x1b[2J\x1b[H1866 1867 1868"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();
    });

    it("recognizes a self-echo even when it carries a DECSTR soft-reset (\\x1b[!p) the captured content didn't", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      // frame.content (the server's tmux capture-pane snapshot) has no
      // DECSTR prefix; the live PTY echo commonly does (regression guard: an
      // ANSI-stripping regex scoped to digits/semicolons only -- not the
      // full ECMA-48 parameter/intermediate-byte grammar -- silently failed
      // to strip `\x1b[!p` since `!` is an intermediate byte, not a
      // parameter byte, misaligning the two signatures and causing a real,
      // reproducible failure in scroll-forward-outcome-signals-distinct.
      // spec.ts's AT_TOP case).
      await act(async () => {
        callArgs.onAppScrollback("\x1b[2J\x1b[H1866 1867 1868", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      act(() => { callArgs.onOutput("\x1b[!p\x1b[2J\x1b[H1866 1867 1868"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();
    });

    it("recognizes a self-echo even when it line-wraps the same text differently than the captured content", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      // The server's capture-pane snapshot and the live PTY stream can wrap
      // identical text at different column positions (regression guard: a
      // real, reproducible failure where "1809 1810 ... 1824" in
      // frame.content lined up against "1809 1810 ... 1821\r\n1822 1823
      // 1824" in the echoed output -- same content, different wrap point,
      // so a signature comparison that didn't collapse whitespace treated
      // them as different).
      await act(async () => {
        callArgs.onAppScrollback("  1809 1810 1811 1812 1813 1814", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      act(() => { callArgs.onOutput("\x1b[2J\x1b[H  1809 1810 1811\r\n1812 1813 1814"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();
    });

    it("recognizes a self-echo when a long run of repeated characters wraps mid-run", async () => {
      const { callArgs } = await mountAndLoadInitialContent();

      // Collapsing whitespace to a single space (rather than stripping it
      // entirely) still fails when the wrap falls in the middle of a long
      // run of identical characters, e.g. a box-drawing horizontal rule:
      // the wrapped copy gains one space the unwrapped copy doesn't have,
      // so the two signatures differ by exactly that character even though
      // both represent the same unbroken rule (regression guard: a real,
      // reproducible failure in scroll-forward-outcome-signals-distinct.
      // spec.ts's DELIVERED case, where frame.content held one unbroken
      // "----------" separator line the live PTY echo wrapped into two).
      await act(async () => {
        callArgs.onAppScrollback("❯ ----------", ScrollForwardOutcome.DELIVERED, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      act(() => { callArgs.onOutput("\x1b[2J\x1b[H❯ -----\r\n-----"); });
      expect(screen.getByTestId("scroll-source-indicator")).toBeInTheDocument();
    });
  });

  // ---- Story 1.4.3 (REQ-11/11b) — Blocked outcome toast ----
  describe("Story 1.4.3 — Blocked outcome toast", () => {
    it("shows the MULTIPLE_VIEWERS toast with the canonical 'another viewer' copy", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.MULTIPLE_VIEWERS);
      });
      expect(screen.getByTestId("scroll-blocked-toast")).toHaveTextContent(
        "Can't browse Claude Code's history right now — another viewer is connected. Close other tabs or sessions viewing this session to enable it."
      );
    });

    it("shows the UNSUPPORTED_STREAMING_PATH toast without any 'another viewer' claim", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.UNSUPPORTED_STREAMING_PATH);
      });
      const toast = screen.getByTestId("scroll-blocked-toast");
      expect(toast).toHaveTextContent("Scroll-forwarding isn't available for this session yet");
      expect(toast).not.toHaveTextContent(/another viewer/i);
    });

    it("shows the LEASE_CONTENTION toast", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.LEASE_CONTENTION);
      });
      expect(screen.getByTestId("scroll-blocked-toast")).toHaveTextContent("Still loading — try again in a moment");
    });

    it("pulses ConnectionCountIndicator for MULTIPLE_VIEWERS", async () => {
      const { callArgs } = await mountAndLoadInitialContent({ connectionCount: 2 });
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.MULTIPLE_VIEWERS);
      });
      expect(screen.getByTestId("connection-count-indicator")).toHaveAttribute("data-pulse", "true");
    });

    it("does not pulse ConnectionCountIndicator for UNSUPPORTED_STREAMING_PATH", async () => {
      const { callArgs } = await mountAndLoadInitialContent({ connectionCount: 2 });
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.UNSUPPORTED_STREAMING_PATH);
      });
      expect(screen.getByTestId("connection-count-indicator")).not.toHaveAttribute("data-pulse", "true");
    });

    it("does not pulse ConnectionCountIndicator for LEASE_CONTENTION", async () => {
      const { callArgs } = await mountAndLoadInitialContent({ connectionCount: 2 });
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.BLOCKED, "Claude Code", "fwd-1", ScrollBlockedReason.LEASE_CONTENTION);
      });
      expect(screen.getByTestId("connection-count-indicator")).not.toHaveAttribute("data-pulse", "true");
    });
  });

  // ---- Story 1.4.4 (REQ-12) — AtTop / NoCapability outcome UX ----
  describe("Story 1.4.4 — AtTop / NoCapability outcome UX", () => {
    it("sets hasMoreAppScrollbackRef false and shows the no-more-history line on AT_TOP", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.AT_TOP, "Claude Code", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      expect(screen.getByTestId("no-more-app-history")).toHaveTextContent("No more history available");
    });

    it("falls back to a silent no-op without crashing for NO_CAPABILITY", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      await act(async () => {
        callArgs.onAppScrollback("", ScrollForwardOutcome.NO_CAPABILITY, "", "fwd-1", ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED);
      });
      expect(screen.queryByTestId("scroll-source-indicator")).not.toBeInTheDocument();
      expect(screen.queryByTestId("scroll-blocked-toast")).not.toBeInTheDocument();
      expect(screen.queryByTestId("no-more-app-history")).not.toBeInTheDocument();
    });

    it("leaves the pre-existing tmux-native exhaustion path (hasMoreScrollbackRef) unaffected by AT_TOP handling", async () => {
      const { callArgs } = await mountAndLoadInitialContent();
      // Tmux-native exhaustion (metadata.hasMore=false) — a path this feature
      // never touches; must render no AT_TOP-style affordance.
      await act(async () => {
        await callArgs.onScrollbackReceived("more content", {
          hasMore: false, oldestSequence: 0, newestSequence: 5, totalLines: 5,
        });
      });
      expect(screen.queryByTestId("no-more-app-history")).not.toBeInTheDocument();
    });
  });
});
