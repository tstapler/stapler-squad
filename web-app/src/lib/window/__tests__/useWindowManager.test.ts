import { renderHook, act } from "@testing-library/react";
import { useWindowManager } from "../useWindowManager";
import * as windowPersistence from "../windowPersistence";
import type { NamedWindow } from "../windowTypes";
import type { LeafPane, SplitPane } from "@/lib/pane/paneTypes";
import { getAllLeaves } from "@/lib/pane/paneReducer";

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

  it("useWindowManager_should_repairStaleSessionIdAndFocusedPaneId_When_restoringWithSessionsAlreadyLoaded", () => {
    // Regression test: usePaneReducer.ts's restore effect used to run
    // validateAndRepair (plus focusedPaneId/zoomedPaneId repointing) inline
    // before dispatching. When the reducer moved up a level into this hook,
    // only the separate sessions-changed effect kept re-running
    // validateAndRepair — the restore path itself dispatched the raw,
    // unrepaired persisted layout. This reproduces the case that gap missed:
    // sessions are already loaded (non-null, non-empty) on the very first
    // render, so there is no later "sessions changed" transition to trigger
    // the other effect at all.
    const stale = makeWindow("win-1", "Window 1", "deleted-session");
    seedLayout(1, [stale]);
    // A stable reference across renders, unlike an inline array literal in
    // the renderHook callback (which would get a fresh identity on every
    // internal re-render and inadvertently re-trigger the *other* repair
    // effect below, masking this exact gap).
    const stableSessions = [{ id: "s1" }];

    const { result } = renderHook(
      ({ sessions }: { sessions: { id: string }[] }) => useWindowManager(sessions),
      { initialProps: { sessions: stableSessions } }
    );

    const repairedLeaves = getAllLeaves(result.current.windows[0].paneState.root);
    const detailLeaf = repairedLeaves.find((l) => l.viewKind === "session-detail");
    expect(detailLeaf?.sessionId).toBeNull();
  });

  it("useWindowManager_should_resetToDefaultSplit_When_restoredWindowHasNoSessionListLeaf", () => {
    // Pre-tiling / corrupt layouts have no session-list leaf at all — restore
    // must reset that window to the default split rather than keeping a
    // permanently broken pane tree with no way to browse sessions.
    const noListPane: NamedWindow = {
      id: "win-1",
      name: "Window 1",
      paneState: {
        root: makeLeaf("only-leaf", "s1"),
        focusedPaneId: "only-leaf",
        zoomedPaneId: null,
      },
    };
    seedLayout(1, [noListPane]);

    const { result } = renderHook(() => useWindowManager([{ id: "s1" }]));

    const leaves = getAllLeaves(result.current.windows[0].paneState.root);
    expect(leaves.some((l) => l.viewKind === "session-list")).toBe(true);
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

  it("useWindowManager_should_notSaveVacuously_When_stateChangesFromRestoreOrStorageSync", () => {
    // Regression test: saveWindowLayout unconditionally bumps the revision on
    // every call, even for data that's already persisted byte-for-byte. If
    // the debounced-save effect fired for the RESTORE_WINDOWS dispatch that
    // *loads* that same data (on mount, or after a cross-tab storage-event
    // sync), it would vacuously bump the shared revision counter. Two tabs
    // each doing this on their own mount race to bump the same counter, and
    // whichever tab's *real* edit (e.g. a rename) lands after the other
    // tab's vacuous bump gets rejected as a stale write and silently
    // discarded via RESTORE_WINDOWS — this is exactly what made the
    // multi-window-cross-tab.spec.ts e2e rename-propagation case fail.
    jest.useFakeTimers();
    try {
      const w1 = makeWindow("win-1", "Window 1", null);
      seedLayout(3, [w1]);

      const { result, rerender } = renderHook(
        ({ sessions }: { sessions: { id: string }[] | null }) => useWindowManager(sessions),
        { initialProps: { sessions: null as { id: string }[] | null } }
      );

      const setItemSpy = jest.spyOn(Storage.prototype, "setItem");

      act(() => {
        rerender({ sessions: [{ id: "s1" }] });
      });
      act(() => {
        jest.advanceTimersByTime(300);
      });

      // No save from the restore-triggered RESTORE_WINDOWS dispatch alone.
      expect(setItemSpy.mock.calls.some(([key]) => key === LS_KEY)).toBe(false);

      const w2 = makeWindow("win-2", "From other tab", null);
      const otherTabLayout = { version: 2 as const, revision: 4, windows: [w1, w2] };
      act(() => {
        // Simulates the other tab's own write landing — not something this
        // hook does, so clear the spy right after it, before checking
        // whether *this* hook's own effect vacuously re-saves in response.
        localStorage.setItem(LS_KEY, JSON.stringify(otherTabLayout));
        setItemSpy.mockClear();
        window.dispatchEvent(
          new StorageEvent("storage", { key: LS_KEY, newValue: JSON.stringify(otherTabLayout) })
        );
      });
      act(() => {
        jest.advanceTimersByTime(300);
      });

      // No save from the storage-event-triggered RESTORE_WINDOWS dispatch either.
      expect(setItemSpy.mock.calls.some(([key]) => key === LS_KEY)).toBe(false);

      // A genuine subsequent edit still saves normally, building on revision 4.
      act(() => {
        result.current.renameWindow("win-1", "Renamed");
      });
      act(() => {
        jest.advanceTimersByTime(300);
      });

      const saved = JSON.parse(
        setItemSpy.mock.calls.find(([key]) => key === LS_KEY)![1] as string
      );
      expect(saved.revision).toBe(5);
    } finally {
      jest.useRealTimers();
    }
  });
});
