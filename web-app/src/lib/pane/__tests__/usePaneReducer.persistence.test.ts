/**
 * Regression tests for usePaneReducer localStorage persistence.
 *
 * Bug: session assignments were wiped on every page reload because the restore
 * effect fired immediately with sessions=[] (Redux initial state) and
 * validateAndRepair cleared all saved session IDs against an empty validIds set.
 */
import { renderHook, act } from "@testing-library/react";
import { usePaneReducer } from "../usePaneReducer";
import { savePaneLayout } from "../usePaneLayout";
import type { PaneState, SplitPane, LeafPane } from "../paneTypes";

const LS_KEY = "cockpit.paneLayout";

beforeEach(() => {
  localStorage.clear();
});

function makeSavedLayout(sessionId: string): PaneState {
  const listLeaf: LeafPane = {
    type: "leaf",
    id: "list-pane",
    viewKind: "session-list",
    sessionId: null,
    activeTab: "terminal",
  };
  const detailLeaf: LeafPane = {
    type: "leaf",
    id: "detail-pane",
    viewKind: "session-detail",
    sessionId,
    activeTab: "terminal",
  };
  const split: SplitPane = {
    type: "split",
    id: "split-root",
    direction: "vertical",
    ratio: 0.4,
    first: listLeaf,
    second: detailLeaf,
  };
  return { root: split, focusedPaneId: "detail-pane", zoomedPaneId: null };
}

describe("usePaneReducer persistence", () => {
  it("usePaneReducer_should_preserveSessionIds_When_sessionsNotYetLoaded", () => {
    // Save a layout with a session assignment to localStorage.
    const saved = makeSavedLayout("session-abc");
    savePaneLayout(saved);

    // Render the hook with an empty sessions array (server hasn't responded yet).
    const { result } = renderHook(() => usePaneReducer([]));

    const [state] = result.current;
    const root = state.root as SplitPane;
    const detailPane = root.second as LeafPane;

    // Session ID must be preserved — not cleared to null.
    expect(detailPane.sessionId).toBe("session-abc");
  });

  it("usePaneReducer_should_preserveSessionIds_When_sessionStillExists", async () => {
    const saved = makeSavedLayout("session-abc");
    savePaneLayout(saved);

    const { result, rerender } = renderHook(
      ({ sessions }: { sessions: { id: string }[] }) => usePaneReducer(sessions),
      { initialProps: { sessions: [] } }
    );

    // Load sessions — session-abc is still alive.
    await act(async () => {
      rerender({ sessions: [{ id: "session-abc" }] });
    });

    const [state] = result.current;
    const root = state.root as SplitPane;
    const detailPane = root.second as LeafPane;

    // Valid session ID must survive the re-validate pass.
    expect(detailPane.sessionId).toBe("session-abc");
  });

  it("usePaneReducer_should_clearSessionId_When_sessionDeletedAfterLoad", async () => {
    const saved = makeSavedLayout("session-abc");
    savePaneLayout(saved);

    const { result, rerender } = renderHook(
      ({ sessions }: { sessions: { id: string }[] }) => usePaneReducer(sessions),
      { initialProps: { sessions: [] } }
    );

    // First: session is alive → IDs preserved.
    await act(async () => {
      rerender({ sessions: [{ id: "session-abc" }] });
    });

    // Then: session is deleted → re-validate must clear the stale ID.
    await act(async () => {
      rerender({ sessions: [] });
    });

    const [state] = result.current;
    const root = state.root as SplitPane;
    const detailPane = root.second as LeafPane;

    expect(detailPane.sessionId).toBeNull();
  });

  it("usePaneReducer_should_restoreSplitRatio_When_layoutSaved", () => {
    const saved = makeSavedLayout("session-abc");
    savePaneLayout(saved);

    const { result } = renderHook(() => usePaneReducer([]));

    const [state] = result.current;
    const root = state.root as SplitPane;

    // Ratio of 0.4 must be restored from localStorage.
    expect(root.ratio).toBe(0.4);
  });
});
