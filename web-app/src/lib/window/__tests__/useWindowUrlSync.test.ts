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

    const { result } = renderHook(() => useWindowUrlSync(windows));

    expect(result.current.currentWindowId).toBe("win-2");
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("useWindowUrlSync_should_fallBackToLastFocusedWindowIdThenCorrectUrl_When_urlHasNoWindowParam", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    localStorage.setItem(LAST_FOCUSED_KEY, "win-2");
    mockSearchParams = new URLSearchParams();

    const { result } = renderHook(() => useWindowUrlSync(windows));

    expect(result.current.currentWindowId).toBe("win-2");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-2"), { scroll: false });
  });

  it("useWindowUrlSync_should_selfHealToFirstWindowAndCorrectUrl_When_paramNamesWindowClosedByAnotherTab", () => {
    const windows = [makeWindow("win-1", "Window 1")];
    mockSearchParams = new URLSearchParams("window=win-2");

    const { result } = renderHook(() => useWindowUrlSync(windows));

    expect(result.current.currentWindowId).toBe("win-1");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledWith(expect.stringContaining("window=win-1"), { scroll: false });
  });

  it("useWindowUrlSync_should_navigateUrlOnlyWithoutDispatchingAction_When_switchToWindowIsCalled", () => {
    const windows = [makeWindow("win-1", "Window 1"), makeWindow("win-2", "Window 2")];
    mockSearchParams = new URLSearchParams("window=win-1&session=abc");
    const dispatchSpy = jest.fn();

    const { result } = renderHook(() => useWindowUrlSync(windows));
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
});
