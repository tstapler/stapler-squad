/**
 * @feature pane:picker
 *
 * Tests for PaneTilingContainer.triggerPicker — verifies the picker overlay is
 * shown when 2+ eligible panes exist, and bypassed correctly for 0 or 1 pane.
 */
import React from "react";
import { render, act } from "@testing-library/react";
import { PaneTilingContainer } from "../PaneTilingContainer";
import * as usePaneReducerModule from "@/lib/pane/usePaneReducer";
import type { PaneState, LeafPane, SplitPane } from "@/lib/pane/paneTypes";

// ─── Mock heavy dependencies ──────────────────────────────────────────────────

jest.mock("@/lib/pane/usePaneShortcuts", () => ({
  usePaneShortcuts: jest.fn(),
}));

// PaneSplitRenderer is a complex recursive component; replace with a stub.
jest.mock("../PaneSplitRenderer", () => ({
  PaneSplitRenderer: () => <div data-testid="pane-split-renderer" />,
}));

// PaneContext is consumed by PaneSplitRenderer (mocked) — no need to mock it.

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
  jest
    .spyOn(usePaneReducerModule, "usePaneReducer")
    .mockReturnValue([state, dispatch]);
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
    const { dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-X");

    const { rerender } = render(
      <PaneTilingContainer
        sessions={[session]}
        externalSessionAssign={null}
      />
    );

    // Trigger assignment by changing version
    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
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
    const { dispatch } = setupReducerMock(root, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
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
    const { dispatch } = setupReducerMock(listPane, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
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
    const { dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-B");

    const { rerender } = render(
      <PaneTilingContainer sessions={[session]} externalSessionAssign={null} />
    );

    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
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
    const { dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-X");

    const { rerender } = render(
      <PaneTilingContainer sessions={[session]} externalSessionAssign={null} />
    );

    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
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
