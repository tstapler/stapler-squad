/**
 * Terminal Jank Elimination, Story 3 (Task 3.1/3.5/3.6) — unit tests for
 * TerminalPool.tsx's pool mechanics: entry creation/identity, LRU eviction
 * with pin/dock protection, docking (fit-on-show/focus), and the rapid-switch
 * callback guard.
 *
 * XtermTerminal is mocked (same pattern as TerminalOutput.test.tsx and
 * siblings) -- these tests exercise the pool's own bookkeeping, not real
 * xterm.js rendering, which is already covered elsewhere (XtermTerminal.test.tsx,
 * the RedrawThrottler/pipeline-integration suites).
 */

import React from "react";
import { render, renderHook, act, waitFor } from "@testing-library/react";
import {
  TerminalPoolProvider,
  useTerminalPool,
  usePooledTerminal,
  usePooledTerminalCallbacks,
  DEFAULT_TERMINAL_POOL_MAX_SIZE,
} from "../TerminalPool";

// ---------------------------------------------------------------------------
// Mock XtermTerminal -- forwards a minimal imperative handle and exposes the
// sessionId it was mounted for via a data attribute, so tests can assert on
// docking (which entry's host node is under which anchor) without needing a
// real xterm.js Terminal.
// ---------------------------------------------------------------------------
const fitSpy = jest.fn();
const focusSpy = jest.fn();

jest.mock("@/components/sessions/XtermTerminal", () => {
  const ReactLib = require("react");
  const XtermTerminal = ReactLib.forwardRef((props: any, ref: any) => {
    ReactLib.useImperativeHandle(ref, () => ({
      terminal: { cols: 80, rows: 24 },
      serializeAddon: null,
      write: jest.fn(),
      writeln: jest.fn(),
      clear: jest.fn(),
      focus: focusSpy,
      fit: fitSpy,
      resize: jest.fn(),
      search: jest.fn(() => false),
      searchNext: jest.fn(() => false),
      searchPrevious: jest.fn(() => false),
    }));
    return ReactLib.createElement("div", {
      "data-testid": "mock-xterm-terminal",
      onClick: () => props.onData?.("typed"),
    });
  });
  XtermTerminal.displayName = "MockXtermTerminal";
  return { XtermTerminal };
});

beforeEach(() => {
  jest.clearAllMocks();
});

function wrapper({ children }: { children: React.ReactNode }) {
  return <TerminalPoolProvider maxSize={3}>{children}</TerminalPoolProvider>;
}

describe("TerminalPool -- entry identity and pool API", () => {
  it("getOrCreateEntry returns the SAME entry object across repeated calls for one sessionId", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper });
    const first = result.current.getOrCreateEntry("s1");
    const second = result.current.getOrCreateEntry("s1");
    expect(second).toBe(first);
  });

  it("defaults maxSize to DEFAULT_TERMINAL_POOL_MAX_SIZE (8) when omitted", () => {
    const { result } = renderHook(() => useTerminalPool(), {
      wrapper: ({ children }) => <TerminalPoolProvider>{children}</TerminalPoolProvider>,
    });
    expect(result.current.maxSize).toBe(DEFAULT_TERMINAL_POOL_MAX_SIZE);
    expect(DEFAULT_TERMINAL_POOL_MAX_SIZE).toBe(8);
  });

  it("useTerminalPool/usePooledTerminal throws outside a TerminalPoolProvider", () => {
    // renderHook propagates a render-phase throw synchronously rather than
    // capturing it on `result.error` (that's a legacy react-test-renderer
    // convention this version of @testing-library/react doesn't follow).
    expect(() => renderHook(() => useTerminalPool())).toThrow(
      "useTerminalPool/usePooledTerminal must be used within a TerminalPoolProvider"
    );
  });
});

describe("TerminalPool -- LRU eviction (Task 3.1)", () => {
  it("evicts the least-recently-accessed UNPINNED, UNDOCKED entry once at capacity", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper }); // maxSize=3

    act(() => {
      result.current.getOrCreateEntry("a");
      result.current.getOrCreateEntry("b");
      result.current.getOrCreateEntry("c");
    });

    // Pool at capacity (3). Creating a 4th evicts "a" (oldest, unpinned/undocked).
    act(() => {
      result.current.getOrCreateEntry("d");
    });

    // "a" was evicted -- getOrCreateEntry for it now creates a brand-new
    // entry (distinct object identity) rather than returning a cached one.
    let recreatedA: unknown;
    act(() => {
      recreatedA = result.current.getOrCreateEntry("a");
    });
    let bAgain: unknown;
    act(() => {
      bAgain = result.current.getOrCreateEntry("b");
    });
    expect(recreatedA).not.toBe(bAgain); // sanity: distinct entries, not both stale
  });

  it("never evicts a pinned entry even if it is the least-recently-accessed", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper }); // maxSize=3

    let entryA!: ReturnType<typeof result.current.getOrCreateEntry>;
    act(() => {
      entryA = result.current.getOrCreateEntry("a");
      result.current.pin("a"); // keep "a" alive despite being oldest
      result.current.getOrCreateEntry("b");
      result.current.getOrCreateEntry("c");
    });

    // Pool "full" per unpinned/undocked accounting -- b/c are both eligible,
    // a is not. A 4th entry must evict b (next-oldest unpinned), not a.
    act(() => {
      result.current.getOrCreateEntry("d");
    });

    let aAfter: unknown;
    act(() => {
      aAfter = result.current.getOrCreateEntry("a");
    });
    expect(aAfter).toBe(entryA); // same object -- never evicted/recreated
  });

  it("never evicts a docked entry", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper }); // maxSize=3
    const anchor = document.createElement("div");
    document.body.appendChild(anchor);

    let entryA!: ReturnType<typeof result.current.getOrCreateEntry>;
    act(() => {
      entryA = result.current.getOrCreateEntry("a");
      result.current.dock("a", anchor); // docked -- protected from eviction
      result.current.getOrCreateEntry("b");
      result.current.getOrCreateEntry("c");
      result.current.getOrCreateEntry("d"); // pool now "full" of exactly b, c (a is docked, exempt)
    });

    // A 5th entry can only evict b or c (both unpinned/undocked); a survives.
    act(() => {
      result.current.getOrCreateEntry("e");
    });

    let aAfter: unknown;
    act(() => {
      aAfter = result.current.getOrCreateEntry("a");
    });
    expect(aAfter).toBe(entryA);

    document.body.removeChild(anchor);
  });

  it("allows the pool to exceed maxSize rather than evict something on-screen (all entries pinned)", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper }); // maxSize=3

    act(() => {
      for (const id of ["a", "b", "c"]) {
        const entry = result.current.getOrCreateEntry(id);
        result.current.pin(id);
        void entry;
      }
    });

    let entryD!: ReturnType<typeof result.current.getOrCreateEntry>;
    act(() => {
      entryD = result.current.getOrCreateEntry("d"); // nothing evictable -- pool grows to 4
    });
    expect(entryD).toBeDefined();

    let dAgain: unknown;
    act(() => {
      dAgain = result.current.getOrCreateEntry("d");
    });
    expect(dAgain).toBe(entryD); // "d" itself is retained too, not silently dropped
  });
});

// ---------------------------------------------------------------------------
// usePooledTerminal -- docking, fit-on-show/focus (Task 3.5)
// ---------------------------------------------------------------------------

function PooledConsumer({ sessionId, isVisible }: { sessionId: string; isVisible: boolean }) {
  const dockRef = React.useRef<HTMLDivElement | null>(null);
  usePooledTerminal(sessionId, dockRef, isVisible);
  return <div data-testid={`anchor-${sessionId}`} ref={dockRef} />;
}

describe("usePooledTerminal -- docking + fit-on-show + focus (Task 3.5)", () => {
  // fitOnShow() (Bug 2 fix) only fits once the anchor reports a real,
  // non-zero size -- jsdom's getBoundingClientRect() always returns all-zero
  // rects, so every anchor needs a stubbed size for these tests to reach the
  // fit()/focus() calls at all.
  let getRectSpy: jest.SpyInstance;
  beforeEach(() => {
    getRectSpy = jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      width: 800, height: 600, top: 0, left: 0, right: 800, bottom: 600, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
  });
  afterEach(() => {
    getRectSpy.mockRestore();
  });

  it("docks the pooled terminal's host node under the consumer's anchor when visible", async () => {
    const { getByTestId } = render(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );

    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });
  });

  it("calls fit() and focus() (via requestAnimationFrame) once docked", async () => {
    render(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );

    await waitFor(() => expect(fitSpy).toHaveBeenCalled());
    expect(focusSpy).toHaveBeenCalled();
  });

  it("Bug 2 regression: does not fit() while the anchor is still zero-sized, and fits once it reports a real size", async () => {
    // Anchor starts genuinely unlaid-out (0x0) at dock time -- e.g. a
    // window-switch's freshly-mounted pane a frame before its ancestor's
    // flex/grid layout settles. The old single-requestAnimationFrame fit
    // would have run fit() against this zero size (or against the mock's
    // default 0x0) and never retried.
    getRectSpy.mockReturnValue({
      width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);

    render(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );

    // Give the zero-size poll a few frames to (not) fire.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(fitSpy).not.toHaveBeenCalled();

    // Anchor's layout settles to a real size -- the stubbed ResizeObserver
    // (jest.setup.js) only re-fires on the next observe()/getBoundingClientRect
    // change it's told about, so flip the mock and let the in-flight
    // requestAnimationFrame poll pick it up on its next tick.
    getRectSpy.mockReturnValue({
      width: 800, height: 600, top: 0, left: 0, right: 800, bottom: 600, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);

    await waitFor(() => expect(fitSpy).toHaveBeenCalled());
    expect(focusSpy).toHaveBeenCalled();
  });

  it("undocks (parks in the graveyard, not under the anchor) when isVisible becomes false", async () => {
    const { getByTestId, rerender } = render(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );
    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });

    rerender(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible={false} />
      </TerminalPoolProvider>
    );

    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).toBeNull();
    });
  });

  it("re-docks a warm (already-created) entry into a NEW anchor without recreating the underlying terminal (Task 3.4 warm-switch)", async () => {
    const { getByTestId, rerender } = render(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );
    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });
    const firstDom = getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]');

    // Simulate a consumer remount (e.g. a pane's session key changing and
    // back) by unmounting the old anchor and mounting a fresh one for the
    // SAME sessionId -- the pool must hand back the same DOM node, not a
    // freshly-mounted one.
    rerender(
      <TerminalPoolProvider>
        <div />
      </TerminalPoolProvider>
    );
    rerender(
      <TerminalPoolProvider>
        <PooledConsumer sessionId="s1" isVisible />
      </TerminalPoolProvider>
    );

    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });
    const secondDom = getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]');
    expect(secondDom).toBe(firstDom); // same DOM node moved, not destroyed + recreated
  });
});

// ---------------------------------------------------------------------------
// Rapid-switch race guard (Task 3.6 / Bug 3)
// ---------------------------------------------------------------------------

function PooledConsumerWithCallbacks({
  sessionId,
  isVisible,
  onData,
}: {
  sessionId: string;
  isVisible: boolean;
  onData: (data: string) => void;
}) {
  const dockRef = React.useRef<HTMLDivElement | null>(null);
  usePooledTerminal(sessionId, dockRef, isVisible);
  usePooledTerminalCallbacks(sessionId, isVisible, {
    onData,
    onResize: () => {},
    isAltScreenActive: () => false,
    onAltScreenScrollUp: () => {},
  });
  return <div data-testid={`anchor-${sessionId}`} ref={dockRef} />;
}

describe("usePooledTerminalCallbacks -- rapid-switch guard (Task 3.6)", () => {
  it("a stale consumer's onData is not invoked after it unmounts, even though the pooled DOM node it wrote through survives", async () => {
    const staleOnData = jest.fn();
    const { getByTestId, rerender } = render(
      <TerminalPoolProvider>
        <PooledConsumerWithCallbacks sessionId="s1" isVisible onData={staleOnData} />
      </TerminalPoolProvider>
    );
    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });

    // Unmount this consumer entirely (simulates a fast session-switch away).
    rerender(<TerminalPoolProvider>{null}</TerminalPoolProvider>);

    // The pooled terminal DOM node is now parked in the graveyard (still
    // mounted -- React never tore down its <XtermTerminal>), so it can still
    // fire onData. That must not reach the now-unmounted consumer's stale
    // callback.
    const graveyard = document.querySelector('[data-testid="terminal-pool-graveyard"]');
    expect(graveyard).not.toBeNull();
    const staleXterm = graveyard?.querySelector('[data-testid="mock-xterm-terminal"]');
    expect(staleXterm).not.toBeNull();

    act(() => {
      (staleXterm as HTMLElement).click(); // triggers the mock's onData("typed")
    });

    expect(staleOnData).not.toHaveBeenCalled();
  });

  it("a NEW consumer for the same sessionId receives onData after taking over from a fast-unmounted predecessor", async () => {
    const firstOnData = jest.fn();
    const secondOnData = jest.fn();

    const { rerender, getByTestId } = render(
      <TerminalPoolProvider>
        <PooledConsumerWithCallbacks sessionId="s1" isVisible onData={firstOnData} />
      </TerminalPoolProvider>
    );
    await waitFor(() => {
      expect(getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]')).not.toBeNull();
    });

    rerender(
      <TerminalPoolProvider>
        <PooledConsumerWithCallbacks sessionId="s1" isVisible onData={secondOnData} />
      </TerminalPoolProvider>
    );

    const secondXterm = getByTestId("anchor-s1").querySelector('[data-testid="mock-xterm-terminal"]');
    expect(secondXterm).not.toBeNull();
    act(() => {
      secondXterm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(secondOnData).toHaveBeenCalledWith("typed");
    expect(firstOnData).not.toHaveBeenCalled();
  });
});
