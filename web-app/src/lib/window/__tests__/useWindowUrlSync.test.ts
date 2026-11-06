import { act, renderHook } from "@testing-library/react";
import { announce } from "@/lib/nav/announce";
import { useWindowUrlSync } from "../useWindowUrlSync";
import type { NamedWindow } from "../windowTypes";
import type { LeafPane } from "@/lib/pane/paneTypes";

const LAST_FOCUSED_KEY = "cockpit.lastFocusedWindowId";

const mockReplace = jest.fn();
let mockSearchParams = new URLSearchParams();

jest.mock("@/lib/nav/announce", () => ({
  announce: jest.fn(),
  formatWindowAnnouncement: jest.requireActual("@/lib/nav/announce").formatWindowAnnouncement,
}));

jest.mock("@/lib/nav", () => ({
  useSearchParams: () => mockSearchParams,
  useRouter: () => ({ replace: mockReplace }),
}));

function makeWindow(id: string, name: string): NamedWindow {
  const leaf: LeafPane = { type: "leaf", id: `${id}-leaf`, viewKind: "session-list", sessionId: null, activeTab: "terminal" };
  return {
    id,
    name,
    paneState: { root: leaf, focusedPaneId: leaf.id, zoomedPaneId: null },
  };
}

beforeEach(() => {
  jest.mocked(announce).mockClear();
  localStorage.clear();
  mockReplace.mockClear();
  mockSearchParams = new URLSearchParams();
});

describe("useWindowUrlSync", () => {
  it("useWindowUrlSync_should_honorExistingWindowIdParam_When_urlContainsValidWindowParam", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    mockSearchParams = new URLSearchParams("window=win-2");

    const { result } = renderHook(() => useWindowUrlSync(windows, true));

    expect(result.current.currentWindowId).toBe("win-2");
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("useWindowUrlSync_should_fallBackToLastFocusedWindowIdThenCorrectUrl_When_urlHasNoWindowParam", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    localStorage.setItem(LAST_FOCUSED_KEY, "win-2");
    mockSearchParams = new URLSearchParams();

    const { result } = renderHook(() => useWindowUrlSync(windows, true));

    expect(result.current.currentWindowId).toBe("win-2");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-2"), { resetScroll: false });
  });

  it("useWindowUrlSync_should_selfHealToFirstWindowAndCorrectUrl_When_paramNamesWindowClosedByAnotherTab", () => {
    const windows = [makeWindow("win-1", "Window 1")];
    mockSearchParams = new URLSearchParams("window=win-2");

    const { result } = renderHook(() => useWindowUrlSync(windows, true));

    expect(result.current.currentWindowId).toBe("win-1");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-1"), { resetScroll: false });
  });

  it("useWindowUrlSync_should_navigateUrlOnlyWithoutDispatchingAction_When_switchToWindowIsCalled", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    mockSearchParams = new URLSearchParams("window=win-1&session=abc");
    const dispatchSpy = jest.fn();

    const { result } = renderHook(() => useWindowUrlSync(windows, true));
    mockReplace.mockClear();

    act(() => result.current.switchToWindow("win-2"));

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const [url, opts] = mockReplace.mock.calls[0];
    expect(url).toContain("window=win-2");
    expect(url).toContain("session=abc");
    expect(opts).toEqual({ resetScroll: false });
    expect(localStorage.getItem(LAST_FOCUSED_KEY)).toBe("win-2");
    expect(dispatchSpy).not.toHaveBeenCalled();
  });

  it("useWindowUrlSync_should_notCorrectUrl_When_notYetRestored", () => {
    // Before useWindowManager's restore effect completes, `windows` is a
    // throwaway placeholder (a single window with a freshly generated random
    // id) — a real `?window=<id>` param naming an actual persisted window
    // will legitimately not match it. Without the `isRestored` guard, this
    // hook would "self-heal" by overwriting that valid param before the real
    // data even loads, permanently losing the window the URL originally named.
    const placeholder = [makeWindow("placeholder-1", "Window 1")];
    mockSearchParams = new URLSearchParams("window=win-real");

    const { result, rerender } = renderHook(
      ({ windows, isRestored }: { windows: NamedWindow[]; isRestored: boolean }) =>
        useWindowUrlSync(windows, isRestored),
      { initialProps: { windows: placeholder, isRestored: false } }
    );

    // Resolution still falls back for rendering purposes (nothing to show
    // otherwise), but the URL itself must be left alone.
    expect(result.current.currentWindowId).toBe("placeholder-1");
    expect(mockReplace).not.toHaveBeenCalled();

    // Once restoration completes with the real windows list (including the
    // window the URL always named), it must resolve correctly — no lingering
    // "correction" from the placeholder phase should have clobbered the URL.
    const real = [makeWindow("win-real", "Window 1"), makeWindow("win-other", "Window 2")];
    rerender({ windows: real, isRestored: true });

    expect(result.current.currentWindowId).toBe("win-real");
    expect(mockReplace).not.toHaveBeenCalled();
  });
});

describe("useWindowUrlSync pending switch", () => {
  const three = [makeWindow("w1", "one"), makeWindow("w2", "api"), makeWindow("w3", "")];
  type Props = { windows: NamedWindow[] };
  const setup = (windows = three, onCommit?: jest.Mock) => {
    mockSearchParams = new URLSearchParams("window=w1");
    return renderHook(({ windows: w }: Props) => useWindowUrlSync(w, true, onCommit), {
      initialProps: { windows },
    });
  };
  const commitUrl = (rerender: (p: Props) => void, id: string, windows = three) => {
    mockSearchParams = new URLSearchParams(`window=${id}`);
    rerender({ windows });
  };

  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("switchToWindow_should_HighlightTargetInSameRender_When_UrlHasNotCommitted", () => {
    const { result } = setup();
    act(() => result.current.switchToWindow("w3"));
    expect(result.current.highlightedWindowId).toBe("w3");
    expect(result.current.currentWindowId).toBe("w1");
  });

  it("pendingWindow_should_ClearAndAnnounceOnce_When_UrlCommitsToTarget", () => {
    const { result, rerender } = setup();
    act(() => result.current.switchToWindow("w2"));
    expect(announce).not.toHaveBeenCalled();
    act(() => commitUrl(rerender, "w2"));
    expect(result.current.highlightedWindowId).toBe("w2");
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith("Window 2 of 3: api");
    act(() => void jest.advanceTimersByTime(5000));
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it("switchToWindow_should_AnnounceWithoutName_When_WindowUnnamed", () => {
    const { result, rerender } = setup();
    act(() => result.current.switchToWindow("w3"));
    act(() => commitUrl(rerender, "w3"));
    expect(announce).toHaveBeenCalledWith("Window 3 of 3");
  });

  it("switchToWindow_should_BeNoop_When_TargetAlreadyActive", () => {
    const { result } = setup();
    mockReplace.mockClear();
    act(() => result.current.switchToWindow("w1"));
    expect(mockReplace).not.toHaveBeenCalled();
    expect(announce).not.toHaveBeenCalled();
    expect(result.current.highlightedWindowId).toBe("w1");
  });

  it("switchToWindow_should_LetLastSwitchWin_When_TwoSwitchesBeforeCommit", () => {
    const { result, rerender } = setup();
    act(() => result.current.switchToWindow("w2"));
    act(() => result.current.switchToWindow("w3"));
    expect(result.current.highlightedWindowId).toBe("w3");
    act(() => commitUrl(rerender, "w3"));
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith("Window 3 of 3");
  });

  it("pendingWindow_should_RevertAndAnnounceNothing_When_TargetClosedConcurrently", () => {
    const { result, rerender } = setup();
    act(() => result.current.switchToWindow("w3"));
    const withoutW3 = three.filter((w) => w.id !== "w3");
    act(() => rerender({ windows: withoutW3 }));
    expect(result.current.highlightedWindowId).toBe("w1");
    act(() => void jest.advanceTimersByTime(5000));
    expect(announce).not.toHaveBeenCalled();
  });

  it("pendingWindow_should_ClearAfter2sSafetyTimer_When_UrlNeverCommits", () => {
    const { result } = setup();
    act(() => result.current.switchToWindow("w2"));
    act(() => void jest.advanceTimersByTime(1999));
    expect(result.current.highlightedWindowId).toBe("w2");
    act(() => void jest.advanceTimersByTime(2));
    expect(result.current.highlightedWindowId).toBe("w1");
    expect(announce).not.toHaveBeenCalled();
  });

  it("onCommit_should_ReceiveTargetAndTrigger_When_UrlCommits", () => {
    const onCommit = jest.fn();
    const { result, rerender } = setup(three, onCommit);
    act(() => result.current.switchToWindow("w2", "touch"));
    expect(onCommit).not.toHaveBeenCalled();
    act(() => commitUrl(rerender, "w2"));
    expect(onCommit).toHaveBeenCalledWith("w2", "touch");
  });

  it("onCommit_should_DefaultToProgrammaticTrigger_When_NoTriggerGiven", () => {
    const onCommit = jest.fn();
    const { result, rerender } = setup(three, onCommit);
    act(() => result.current.switchToWindow("w2"));
    act(() => commitUrl(rerender, "w2"));
    expect(onCommit).toHaveBeenCalledWith("w2", "programmatic");
  });

  it("onCommit_should_NotFire_When_UrlChangesWithoutUserSwitch", () => {
    const onCommit = jest.fn();
    const { rerender } = setup(three, onCommit);
    act(() => commitUrl(rerender, "w2"));
    expect(onCommit).not.toHaveBeenCalled();
    expect(announce).not.toHaveBeenCalled();
  });
});
