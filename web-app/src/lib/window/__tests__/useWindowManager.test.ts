import { renderHook, act } from "@testing-library/react";
import { useWindowManager } from "../useWindowManager";
import * as windowPersistence from "../windowPersistence";
import type { NamedWindow } from "../windowTypes";
import type { LeafPane, SplitPane } from "@/lib/pane/paneTypes";

const LS_KEY = "cockpit.windowLayout";

function makeLeaf(id: string, sessionId: string | null): LeafPane {
  return { type: "leaf", id, viewKind: "session-detail", sessionId, activeTab: "terminal" };
}

function makeListLeaf(id: string): LeafPane {
  return { type: "leaf", id, viewKind: "session-list", sessionId: null, activeTab: "terminal" };
}

function makeSplit(id: string, first: LeafPane, second: LeafPane): SplitPane {
  return { type: "split", id, direction: "vertical", ratio: 0.4, first, second };
}

function makeWindow(id: string, name: string, sessionId: string | null): NamedWindow {
  return {
    id,
    name,
    paneState: {
      root: makeSplit(`${id}-split`, makeListLeaf(`${id}-list`), makeLeaf(`${id}-detail`, sessionId)),
      focusedPaneId: `${id}-detail`,
      zoomedPaneId: null,
    },
  };
}

function seedLayout(revision: number, windows: NamedWindow[]) {
  localStorage.setItem(LS_KEY, JSON.stringify({ version: 2, revision, windows }));
}

beforeEach(() => {
  localStorage.clear();
  jest.restoreAllMocks();
});

describe("useWindowManager", () => {
  it("useWindowManager_should_restoreWindowsFromLoadWindowLayoutExactlyOnce_When_sessionsBecomeAvailable", () => {
    const w1 = makeWindow("win-1", "Window 1", null);
    const w2 = makeWindow("win-2", "Window 2", null);
    seedLayout(2, [w1, w2]);
    const loadSpy = jest.spyOn(windowPersistence, "loadWindowLayout");

    const { result, rerender } = renderHook(
      ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
      { initialProps: { sessions: null as { id: string }[] | null } }
    );

    // Before sessions arrive the reducer holds its lazy-initial default window,
    // not yet the persisted layout.
    expect(result.current.windows).toHaveLength(1);

    rerender({ sessions: [{ id: "s1" }] });
    expect(result.current.windows).toEqual([w1, w2]);
    expect(loadSpy).toHaveBeenCalledTimes(1);

    // Re-render again (new sessions array reference, same content) — restore
    // must not fire a second time.
    rerender({ sessions: [{ id: "s1" }] });
    expect(loadSpy).toHaveBeenCalledTimes(1);
    expect(result.current.windows).toEqual([w1, w2]);
  });

  it("useWindowManager_should_saveOnceAfterDebounce_When_multipleDispatchesBurst", () => {
    jest.useFakeTimers();
    try {
      const w1 = makeWindow("win-1", "Window 1", null);
      seedLayout(1, [w1]);
      const setItemSpy = jest.spyOn(Storage.prototype, "setItem");

      const { result, rerender } = renderHook(
        ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
        { initialProps: { sessions: null as { id: string }[] | null } }
      );

      act(() => {
        rerender({ sessions: [{ id: "s1" }] });
      });

      setItemSpy.mockClear();

      act(() => {
        result.current.renameWindow("win-1", "First");
      });
      act(() => {
        result.current.renameWindow("win-1", "Second");
      });
      act(() => {
        result.current.renameWindow("win-1", "Third");
      });

      // No save should have landed yet — still inside the debounce window.
      expect(setItemSpy).not.toHaveBeenCalledWith(LS_KEY, expect.anything());

      act(() => {
        jest.advanceTimersByTime(300);
      });

      const windowLayoutCalls = setItemSpy.mock.calls.filter(([key]) => key === LS_KEY);
      expect(windowLayoutCalls).toHaveLength(1);
    } finally {
      jest.useRealTimers();
    }
  });

  it("useWindowManager_should_revalidateBackgroundWindow_When_sessionsChange", () => {
    const background = makeWindow("win-1", "Window 1", "deleted-1");
    const viewed = makeWindow("win-2", "Window 2", "s1");
    seedLayout(1, [background, viewed]);

    const { result, rerender } = renderHook(
      ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
      { initialProps: { sessions: null as { id: string }[] | null } }
    );

    act(() => {
      rerender({ sessions: [{ id: "deleted-1" }, { id: "s1" }] });
    });

    // deleted-1 still exists at this point, so nothing is stripped yet.
    const bgLeafBefore = (result.current.windows[0].paneState.root as SplitPane).second as LeafPane;
    expect(bgLeafBefore.sessionId).toBe("deleted-1");

    act(() => {
      rerender({ sessions: [{ id: "s1" }] });
    });

    const bgWindow = result.current.windows.find((w) => w.id === "win-1")!;
    const bgLeafAfter = (bgWindow.paneState.root as SplitPane).second as LeafPane;
    expect(bgLeafAfter.sessionId).toBeNull();

    // The currently-viewed window's valid session id is untouched.
    const viewedWindow = result.current.windows.find((w) => w.id === "win-2")!;
    const viewedLeaf = (viewedWindow.paneState.root as SplitPane).second as LeafPane;
    expect(viewedLeaf.sessionId).toBe("s1");
  });

  it("useWindowManager_should_replaceWithFreshWindow_When_closeWindowCalledOnLastRemainingWindow", () => {
    const only = makeWindow("win-1", "Window 1", null);
    seedLayout(1, [only]);

    const { result, rerender } = renderHook(
      ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
      { initialProps: { sessions: null as { id: string }[] | null } }
    );

    act(() => {
      rerender({ sessions: [{ id: "s1" }] });
    });

    expect(result.current.windows).toHaveLength(1);

    let returnedId: string | undefined;
    act(() => {
      returnedId = result.current.closeWindow("win-1");
    });

    expect(result.current.windows).toHaveLength(1);
    expect(result.current.windows[0].id).not.toBe("win-1");
    expect(returnedId).toBe(result.current.windows[0].id);
  });

  it("useWindowManager_should_dispatchRestoreWindowsAndUpdateRevisionRef_When_storageEventReportsNewerRevision", () => {
    jest.useFakeTimers();
    try {
      const w1 = makeWindow("win-1", "Window 1", null);
      seedLayout(5, [w1]);

      const { result, rerender } = renderHook(
        ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
        { initialProps: { sessions: null as { id: string }[] | null } }
      );

      act(() => {
        rerender({ sessions: [{ id: "s1" }] });
      });

      const w2 = makeWindow("win-2", "From other tab", null);
      const otherTabLayout = { version: 2 as const, revision: 6, windows: [w1, w2] };

      act(() => {
        // Simulate the other tab's write landing in localStorage, then firing
        // the storage event this tab listens for.
        localStorage.setItem(LS_KEY, JSON.stringify(otherTabLayout));
        window.dispatchEvent(
          new StorageEvent("storage", {
            key: LS_KEY,
            newValue: JSON.stringify(otherTabLayout),
          })
        );
      });

      expect(result.current.windows).toEqual([w1, w2]);

      // A subsequent save must build on the updated revision ref (6), not the
      // stale one this tab started with (5) — otherwise it would spuriously
      // conflict against the value it just adopted.
      const setItemSpy = jest.spyOn(Storage.prototype, "setItem");
      setItemSpy.mockClear();

      act(() => {
        result.current.renameWindow("win-1", "Renamed");
      });
      act(() => {
        jest.advanceTimersByTime(300);
      });

      const saved = JSON.parse(
        setItemSpy.mock.calls.find(([key]) => key === LS_KEY)![1] as string
      );
      expect(saved.revision).toBe(7);
    } finally {
      jest.useRealTimers();
    }
  });
});
