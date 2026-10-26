/**
 * @feature pane:picker
 *
 * Tests for PaneTilingContainer.triggerPicker — verifies the picker overlay is
 * shown when 2+ eligible panes exist, and bypassed correctly for 0 or 1 pane.
 */
import React from "react";
import { render, act } from "@testing-library/react";
import { PaneTilingContainer } from "../PaneTilingContainer";
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
  return { id } as import("@/gen/session/v1/types_pb").Session;
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
