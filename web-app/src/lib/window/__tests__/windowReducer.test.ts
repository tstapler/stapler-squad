import { windowReducer } from "../windowReducer";
import { getAllLeaves, initialPaneState, paneReducer } from "@/lib/pane/paneReducer";
import type { NamedWindow, WindowsState } from "../windowTypes";

function namedWindow(id: string, name: string): NamedWindow {
  return { id, name, paneState: initialPaneState() };
}

// ─── CREATE_WINDOW ────────────────────────────────────────────────────────────

describe("CREATE_WINDOW", () => {
  it("windowReducer_should_appendNewWindowWithInitialPaneState_When_createWindowDispatched", () => {
    const state: WindowsState = { windows: [namedWindow("win-1", "Window 1")] };
    const next = windowReducer(state, { type: "CREATE_WINDOW", id: "win-2", name: "Window 2" });

    expect(next.windows).toHaveLength(2);
    expect(next.windows[0]).toBe(state.windows[0]);
    expect(next.windows[1].id).toBe("win-2");
    expect(next.windows[1].name).toBe("Window 2");
    expect(
      getAllLeaves(next.windows[1].paneState.root).some((l) => l.viewKind === "session-list")
    ).toBe(true);
  });
});

// ─── RENAME_WINDOW ────────────────────────────────────────────────────────────

describe("RENAME_WINDOW", () => {
  it("windowReducer_should_leavePreviousNameUnchanged_When_renameWindowWithEmptyString", () => {
    const state: WindowsState = { windows: [namedWindow("win-1", "Debugging session y")] };
    const next = windowReducer(state, { type: "RENAME_WINDOW", id: "win-1", name: "" });

    expect(next.windows[0].name).toBe("Debugging session y");
    expect(next).toBe(state);
  });

  it("windowReducer_should_renameOnlyTargetWindow_When_renameWindowWithNonEmptyString", () => {
    const state: WindowsState = {
      windows: [namedWindow("win-1", "Window 1"), namedWindow("win-2", "Window 2")],
    };
    const next = windowReducer(state, { type: "RENAME_WINDOW", id: "win-1", name: "Renamed" });

    expect(next.windows[0].name).toBe("Renamed");
    expect(next.windows[1]).toBe(state.windows[1]);
  });
});

// ─── CLOSE_WINDOW ─────────────────────────────────────────────────────────────

describe("CLOSE_WINDOW", () => {
  it("windowReducer_should_removeOnlyTargetWindow_When_closeWindowDispatchedWithOthersRemaining", () => {
    const state: WindowsState = {
      windows: [namedWindow("win-1", "Window 1"), namedWindow("win-2", "Window 2")],
    };
    const next = windowReducer(state, {
      type: "CLOSE_WINDOW",
      id: "win-2",
      replacement: { id: "win-4", name: "Window 3" },
    });

    expect(next.windows).toHaveLength(1);
    expect(next.windows[0].id).toBe("win-1");
    expect(next.windows.some((w) => w.id === "win-4")).toBe(false);
  });

  it("windowReducer_should_replaceWithFreshWindow_When_closeWindowDispatchedOnLastRemainingWindow", () => {
    const state: WindowsState = { windows: [namedWindow("win-1", "Window 1")] };
    const next = windowReducer(state, {
      type: "CLOSE_WINDOW",
      id: "win-1",
      replacement: { id: "win-3", name: "Window 1" },
    });

    expect(next.windows).toHaveLength(1);
    expect(next.windows[0].id).toBe("win-3");
    expect(next.windows[0].name).toBe("Window 1");
    expect(
      getAllLeaves(next.windows[0].paneState.root).some((l) => l.viewKind === "session-list")
    ).toBe(true);
    expect(next.windows.length).not.toBe(0);
  });
});

// ─── PANE_ACTION ──────────────────────────────────────────────────────────────

describe("PANE_ACTION", () => {
  it("windowReducer_should_delegateToUnmodifiedPaneReducer_When_paneActionAddressesOneWindow", () => {
    const window1 = namedWindow("win-1", "Window 1");
    const window2 = namedWindow("win-2", "Window 2");
    const state: WindowsState = { windows: [window1, window2] };

    const paneAction = { type: "ZOOM_PANE" as const, paneId: window2.paneState.focusedPaneId };
    const next = windowReducer(state, { type: "PANE_ACTION", windowId: "win-2", action: paneAction });

    expect(next.windows[0].paneState).toBe(window1.paneState);
    expect(next.windows[1].paneState).toEqual(paneReducer(window2.paneState, paneAction));
  });
});

// ─── RESTORE_WINDOWS ──────────────────────────────────────────────────────────

describe("RESTORE_WINDOWS", () => {
  it("windowReducer_should_replaceWindowsWholesale_When_restoreWindowsDispatched", () => {
    const state: WindowsState = { windows: [namedWindow("win-1", "Window 1")] };
    const w1 = namedWindow("win-a", "A");
    const w2 = namedWindow("win-b", "B");
    const next = windowReducer(state, { type: "RESTORE_WINDOWS", windows: [w1, w2] });

    expect(next).toEqual({ windows: [w1, w2] });
  });
});
