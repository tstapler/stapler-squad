/**
 * @feature pane:picker
 *
 * Tests for PaneTilingContainer.triggerPicker — verifies the picker overlay is
 * shown when 2+ eligible panes exist, and bypassed correctly for 0 or 1 pane.
 */
import React from "react";
import { render, act, screen, fireEvent } from "@testing-library/react";
import { PaneTilingContainer } from "../PaneTilingContainer";
import type { Session } from "@/gen/session/v1/types_pb";
import type { PaneState, LeafPane, SplitPane } from "@/lib/pane/paneTypes";

// ─── Mock heavy dependencies ──────────────────────────────────────────────────

jest.mock("@/lib/pane/usePaneShortcuts", () => ({
  usePaneShortcuts: jest.fn(),
}));

// PaneSplitRenderer is a complex recursive component; replace with a stub.
jest.mock("../PaneSplitRenderer", () => ({
  PaneSplitRenderer: () => <div data-testid="pane-split-renderer" />,
}));

// Desktop viewport: the provider-less default is mobile, which uses the bottom sheet instead of the key picker.
jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false, isFoldable: false, isInnerScreen: true, isLandscape: false }),
}));

// PaneContext is consumed by PaneSplitRenderer (mocked) — no need to mock it.

// Avoid a real RPC call (and its noisy console.error in jsdom) from the
// backlog-origin-badge index lookup this container now fetches once on mount.
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogSessionIndex: () => ({ index: new Map(), loading: false }),
}));

// ─── Fixtures ─────────────────────────────────────────────────────────────────

function makeLeaf(
  id: string,
  viewKind: "session-detail" | "session-list",
  sessionId: string | null = null
): LeafPane {
  return { type: "leaf", id, viewKind, sessionId, activeTab: "terminal" };
}

function makeSplit(id: string, first: LeafPane | SplitPane, second: LeafPane | SplitPane): SplitPane {
  return { type: "split", id, direction: "vertical", ratio: 0.5, first, second };
}

function makeSession(id: string) {
  // Minimal Session-like object; only `id` is accessed in triggerPicker.
  return { id } as Session;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function setupReducerMock(root: PaneState["root"], focusedPaneId: string) {
  const dispatch = jest.fn();
  const state: PaneState = { root, focusedPaneId, zoomedPaneId: null };
  return { state, dispatch };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
});

describe("triggerPicker", () => {
  it("triggerPicker_should_showPickerOverlay_When_twoDetailPanesExist", async () => {
    // Two session-detail panes; detail pane-1 is focused (the bypass scenario).
    const pane1 = makeLeaf("pane-1", "session-detail");
    const pane2 = makeLeaf("pane-2", "session-detail");
    const root = makeSplit("split-1", pane1, pane2);
    const { state, dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-X");

    const { rerender } = render(
      <PaneTilingContainer
        sessions={[session]}
        paneState={state}
        dispatch={dispatch}
        externalSessionAssign={null}
      />
    );

    // Trigger assignment by changing version
    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
          paneState={state}
          dispatch={dispatch}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // ASSIGN_SESSION must NOT have been dispatched (picker should show instead)
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION" })
    );
  });

  it("triggerPicker_should_assignDirectly_When_oneDetailPane", async () => {
    // One session-list pane + one session-detail pane.
    const listPane = makeLeaf("pane-list", "session-list");
    const detailPane = makeLeaf("pane-detail", "session-detail");
    const root = makeSplit("split-1", listPane, detailPane);
    // List pane is focused (user clicked session from the list)
    const { state, dispatch } = setupReducerMock(root, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
          paneState={state}
          dispatch={dispatch}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // Should dispatch ASSIGN_SESSION directly to the single detail pane
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION", paneId: "pane-detail", sessionId: "session-X" })
    );
    // Should also dispatch ASSIGN_TAB and FOCUS_PANE
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_TAB", paneId: "pane-detail" })
    );
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "FOCUS_PANE", paneId: "pane-detail" })
    );
  });

  it("triggerPicker_should_autoSplit_When_noDetailPanes", async () => {
    // Only a session-list pane — no detail panes.
    const listPane = makeLeaf("pane-list", "session-list");
    const { state, dispatch } = setupReducerMock(listPane, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
          paneState={state}
          dispatch={dispatch}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // Should dispatch ASSIGN_SESSION to focusedPaneId (reducer handles auto-split)
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION", paneId: "pane-list", sessionId: "session-X" })
    );
  });

  it("triggerPicker_should_showPicker_When_detailPaneFocusedAndTwoPanesExist", async () => {
    // Regression guard for the specific Bug 1 scenario:
    // detail pane IS focused, but 2 detail panes exist → picker must still show.
    const pane1 = makeLeaf("pane-1", "session-detail", "session-A");
    const pane2 = makeLeaf("pane-2", "session-detail", null);
    const root = makeSplit("split-1", pane1, pane2);
    // Detail pane-1 is focused — this was the bypass condition before the fix
    const { state, dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-B");

    const { rerender } = render(
      <PaneTilingContainer
        sessions={[session]}
        paneState={state}
        dispatch={dispatch}
        externalSessionAssign={null}
      />
    );

    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
          paneState={state}
          dispatch={dispatch}
          externalSessionAssign={{ sessionId: "session-B", version: 1 }}
        />
      );
    });

    // With the fix, ASSIGN_SESSION must NOT be dispatched — picker overlay shows instead
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION" })
    );
  });

  // T-015: cancelPicker must not fire during triggerPicker — overlay stays open
  it("triggerPicker_should_notDispatchCancelPicker_When_twoDetailPanes", async () => {
    const pane1 = makeLeaf("pane-1", "session-detail");
    const pane2 = makeLeaf("pane-2", "session-detail");
    const root = makeSplit("split-1", pane1, pane2);
    const { state, dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-X");

    const { rerender } = render(
      <PaneTilingContainer
        sessions={[session]}
        paneState={state}
        dispatch={dispatch}
        externalSessionAssign={null}
      />
    );

    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
          paneState={state}
          dispatch={dispatch}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // Picker must remain open — cancelPicker must not have been dispatched
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: "CANCEL_PICKER" })
    );
  });
});

describe("windowActive gating (Story 4.1.3a)", () => {
  async function renderWithPendingPicker(windowActive: boolean) {
    const root = makeSplit("split-1", makeLeaf("pane-1", "session-detail"), makeLeaf("pane-2", "session-detail"));
    const { state, dispatch } = setupReducerMock(root, "pane-1");
    const session = makeSession("session-X");
    const ui = (active: boolean, assign: { sessionId: string; version: number } | null) => (
      <PaneTilingContainer
        sessions={[session]}
        paneState={state}
        dispatch={dispatch}
        externalSessionAssign={assign}
        windowActive={active}
      />
    );
    const view = render(ui(true, null));
    await act(async () => {
      view.rerender(ui(true, { sessionId: "session-X", version: 1 }));
    });
    if (!windowActive) {
      await act(async () => {
        view.rerender(ui(false, { sessionId: "session-X", version: 1 }));
      });
    }
    return { dispatch, ...view };
  }

  it("pickerKeys_should_NotStopPropagation_When_ContainerInHiddenWindow", async () => {
    const { dispatch } = await renderWithPendingPicker(false);
    const bubbled = jest.fn();
    document.addEventListener("keydown", bubbled);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "v", bubbles: true, cancelable: true }));
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "v", bubbles: true, cancelable: true }));
    document.removeEventListener("keydown", bubbled);
    expect(bubbled).toHaveBeenCalledTimes(1);
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("pickerKeys_should_HandleKey_When_ContainerInActiveWindow", async () => {
    const { dispatch } = await renderWithPendingPicker(true);
    const bubbled = jest.fn();
    document.addEventListener("keydown", bubbled);
    act(() => {
      document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "v", bubbles: true, cancelable: true }));
    });
    document.removeEventListener("keydown", bubbled);
    expect(bubbled).not.toHaveBeenCalled();
    expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ type: "SPLIT_AND_ASSIGN_SESSION", direction: "vertical" }));
  });

  it("externalAssign_should_NotBeConsumed_When_WindowHidden", async () => {
    const listPane = makeLeaf("pane-list", "session-list");
    const detailPane = makeLeaf("pane-detail", "session-detail");
    const { state, dispatch } = setupReducerMock(makeSplit("s", listPane, detailPane), "pane-list");
    const session = makeSession("session-X");
    render(
      <PaneTilingContainer
        sessions={[session]}
        paneState={state}
        dispatch={dispatch}
        externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        windowActive={false}
      />,
    );
    expect(dispatch).not.toHaveBeenCalled();
  });
});

describe("open existing pane", () => {
  async function open(panes: LeafPane[], sessionPane: string | null) {
    const [a, b] = panes;
    const root = panes.length === 1 ? a : makeSplit("split-1", a, b);
    const { state, dispatch } = setupReducerMock(root, a.id);
    const session = makeSession("session-X");
    const ui = (assign: { sessionId: string; version: number } | null) => (
      <PaneTilingContainer sessions={[session]} paneState={state} dispatch={dispatch} externalSessionAssign={assign} />
    );
    const view = render(ui(null));
    await act(async () => {
      view.rerender(ui({ sessionId: "session-X", version: 1 }));
    });
    return { dispatch, sessionPane };
  }

  it("shows_Open_existing_pane_first_and_focuses_it_on_click", async () => {
    const { dispatch } = await open([makeLeaf("p1", "session-detail"), makeLeaf("p2", "session-detail", "session-X")], "p2");
    const first = screen.getByRole("toolbar").querySelector("button");
    expect(first?.textContent).toContain("Open existing pane");
    fireEvent.click(screen.getByTestId("picker-open-existing"));
    expect(dispatch).toHaveBeenCalledWith({ type: "FOCUS_PANE", paneId: "p2" });
    expect(screen.queryByRole("toolbar")).toBeNull();
  });

  it("omits_Open_existing_pane_when_session_not_in_a_pane", async () => {
    await open([makeLeaf("p1", "session-detail"), makeLeaf("p2", "session-detail")], null);
    expect(screen.queryByTestId("picker-open-existing")).toBeNull();
  });

  it("O_key_focuses_existing_pane_and_closes_picker", async () => {
    const { dispatch } = await open([makeLeaf("p1", "session-detail"), makeLeaf("p2", "session-detail", "session-X")], "p2");
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "o", bubbles: true, cancelable: true }));
    });
    expect(dispatch).toHaveBeenCalledWith({ type: "FOCUS_PANE", paneId: "p2" });
    expect(screen.queryByRole("toolbar")).toBeNull();
  });

  it("O_key_falls_through_to_pane_letter_when_session_not_open", async () => {
    const panes = Array.from({ length: 15 }, (_, i) => makeLeaf(`p${i}`, "session-detail"));
    const root = panes.slice(1).reduce<PaneState["root"]>((acc, p, i) => makeSplit(`s${i}`, acc, p), panes[0]);
    const { state, dispatch } = setupReducerMock(root, "p0");
    const session = makeSession("session-X");
    const ui = (assign: { sessionId: string; version: number } | null) => (
      <PaneTilingContainer sessions={[session]} paneState={state} dispatch={dispatch} externalSessionAssign={assign} />
    );
    const view = render(ui(null));
    await act(async () => {
      view.rerender(ui({ sessionId: "session-X", version: 1 }));
    });
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "o", bubbles: true, cancelable: true }));
    });
    expect(dispatch).toHaveBeenCalledWith({ type: "ASSIGN_SESSION", paneId: "p14", sessionId: "session-X" });
    expect(dispatch).not.toHaveBeenCalledWith(expect.objectContaining({ type: "FOCUS_PANE" }));
  });

  it("single_pane_already_holding_session_only_dispatches_FOCUS_PANE", async () => {
    const { dispatch } = await open([makeLeaf("p1", "session-detail", "session-X")], "p1");
    expect(dispatch).toHaveBeenCalledWith({ type: "FOCUS_PANE", paneId: "p1" });
    expect(dispatch).not.toHaveBeenCalledWith(expect.objectContaining({ type: "ASSIGN_SESSION" }));
  });
});
