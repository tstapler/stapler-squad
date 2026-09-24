import { renderHook } from "@testing-library/react";
import { useWindowUrlSync } from "../useWindowUrlSync";
import type { NamedWindow } from "../windowTypes";
import type { LeafPane } from "@/lib/pane/paneTypes";

const LAST_FOCUSED_KEY = "cockpit.lastFocusedWindowId";

const mockReplace = jest.fn();
let mockSearchParams = new URLSearchParams();

jest.mock("next/navigation", () => ({
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
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-2"), { scroll: false });
  });

  it("useWindowUrlSync_should_selfHealToFirstWindowAndCorrectUrl_When_paramNamesWindowClosedByAnotherTab", () => {
    const windows = [makeWindow("win-1", "Window 1")];
    mockSearchParams = new URLSearchParams("window=win-2");

    const { result } = renderHook(() => useWindowUrlSync(windows, true));

    expect(result.current.currentWindowId).toBe("win-1");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-1"), { scroll: false });
  });

  it("useWindowUrlSync_should_navigateUrlOnlyWithoutDispatchingAction_When_switchToWindowIsCalled", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    mockSearchParams = new URLSearchParams("window=win-1&session=abc");
    const dispatchSpy = jest.fn();

    const { result } = renderHook(() => useWindowUrlSync(windows, true));
    mockReplace.mockClear();

    result.current.switchToWindow("win-2");

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const [url, opts] = mockReplace.mock.calls[0];
    expect(url).toContain("window=win-2");
    expect(url).toContain("session=abc");
    expect(opts).toEqual({ scroll: false });
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
