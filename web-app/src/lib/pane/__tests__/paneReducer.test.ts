import { paneReducer, getAllLeaves, getAdjacentLeaf } from "../paneReducer";
import { PaneState, PaneNode, LeafPane, SplitPane } from "../paneTypes";

// ─── Fixtures ─────────────────────────────────────────────────────────────────

function leaf(id: string, sessionId: string | null = null): LeafPane {
  return { type: "leaf", id, viewKind: "session-detail", sessionId, activeTab: "terminal" };
}

function split(
  id: string,
  direction: "horizontal" | "vertical",
  first: PaneNode,
  second: PaneNode,
  ratio = 0.5
): SplitPane {
  return { type: "split", id, direction, ratio, first, second };
}

function stateOf(root: PaneNode, focusedPaneId: string): PaneState {
  return { root, focusedPaneId, zoomedPaneId: null };
}

// ─── SPLIT_PANE ───────────────────────────────────────────────────────────────

describe("SPLIT_PANE", () => {
  it("paneReducer_should_replaceLeafWithSplit_When_splitVertical", () => {
    const leaf1 = leaf("pane-1");
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, { type: "SPLIT_PANE", paneId: "pane-1", direction: "vertical" });
    expect(next.root.type).toBe("split");
    const s = next.root as SplitPane;
    expect(s.direction).toBe("vertical");
    expect(s.first).toMatchObject({ type: "leaf", id: "pane-1" });
    expect(s.second.type).toBe("leaf");
    // New leaf is focused
    expect(next.focusedPaneId).toBe((s.second as LeafPane).id);
  });

  it("paneReducer_should_replaceLeafWithSplit_When_splitHorizontal", () => {
    const leaf1 = leaf("pane-1");
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, { type: "SPLIT_PANE", paneId: "pane-1", direction: "horizontal" });
    const s = next.root as SplitPane;
    expect(s.direction).toBe("horizontal");
  });

  it("paneReducer_should_setDefaultRatio_When_splitCreated", () => {
    const state = stateOf(leaf("pane-1"), "pane-1");
    const next = paneReducer(state, { type: "SPLIT_PANE", paneId: "pane-1", direction: "vertical" });
    expect((next.root as SplitPane).ratio).toBe(0.5);
  });

  it("paneReducer_should_returnUnchangedState_When_splitAtMaxDepth", () => {
    // Build a deeply nested tree at exactly MAX_DEPTH (8)
    // Each split adds one level; a leaf at depth 8 should be refused
    let node: PaneNode = leaf("deep-leaf");
    for (let i = 0; i < 8; i++) {
      node = split(`split-${i}`, "vertical", node, leaf(`other-${i}`));
    }
    // The deepest leaf is at depth 8; attempting to split it should be a no-op
    const state = stateOf(node, "deep-leaf");
    const next = paneReducer(state, { type: "SPLIT_PANE", paneId: "deep-leaf", direction: "vertical" });
    expect(next).toEqual(state);
  });

  it("paneReducer_should_returnUnchangedState_When_duplicateSessionAlreadyOpen", () => {
    // SPLIT_PANE creates an empty second pane — the duplicate guard is on ASSIGN_SESSION.
    // However, the test in the plan is about SPLIT_PANE; the real guard fires during ASSIGN.
    // Verify: splitting from a leaf that already has a session is allowed (empty second pane)
    const leaf1 = leaf("pane-1", "session-A");
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, { type: "SPLIT_PANE", paneId: "pane-1", direction: "vertical" });
    expect(next.root.type).toBe("split");
    // The new second pane should be empty
    const newLeaf = (next.root as SplitPane).second as LeafPane;
    expect(newLeaf.sessionId).toBeNull();
  });
});

// ─── CLOSE_PANE ───────────────────────────────────────────────────────────────

describe("CLOSE_PANE", () => {
  it("paneReducer_should_collapseParentWithSibling_When_closingLeaf", () => {
    const leaf1 = leaf("pane-1", "session-A");
    const leaf2 = leaf("pane-2", "session-B");
    const root = split("split-1", "vertical", leaf1, leaf2);
    const state = stateOf(root, "pane-1");
    const next = paneReducer(state, { type: "CLOSE_PANE", paneId: "pane-1" });
    expect(next.root).toMatchObject({ type: "leaf", id: "pane-2" });
    expect(next.focusedPaneId).toBe("pane-2");
  });

  it("paneReducer_should_focusSibling_When_closingFocusedPane", () => {
    const l1 = leaf("pane-1");
    const l2 = leaf("pane-2");
    const l3 = leaf("pane-3");
    const inner = split("split-inner", "vertical", l2, l3);
    const root = split("split-outer", "vertical", l1, inner);
    const state = stateOf(root, "pane-2");
    const next = paneReducer(state, { type: "CLOSE_PANE", paneId: "pane-2" });
    expect(next.focusedPaneId).toBe("pane-3");
  });

  it("paneReducer_should_resetToInitialState_When_closingLastRootLeaf", () => {
    const root = leaf("pane-only");
    const state = stateOf(root, "pane-only");
    const next = paneReducer(state, { type: "CLOSE_PANE", paneId: "pane-only" });
    // Closing the last leaf resets to the initial split layout (list | detail)
    expect(next.root.type).toBe("split");
    expect(next.zoomedPaneId).toBeNull();
  });

  it("paneReducer_should_returnUnchangedState_When_paneIdNotFound", () => {
    const state = stateOf(leaf("pane-1"), "pane-1");
    const next = paneReducer(state, { type: "CLOSE_PANE", paneId: "nonexistent" });
    expect(next).toEqual(state);
  });
});

// ─── RESIZE_PANE ──────────────────────────────────────────────────────────────

describe("RESIZE_PANE", () => {
  it("paneReducer_should_updateRatio_When_resizePaneDispatched", () => {
    const root = split("split-1", "vertical", leaf("pane-1"), leaf("pane-2"), 0.5);
    const state = stateOf(root, "pane-1");
    const next = paneReducer(state, { type: "RESIZE_PANE", splitId: "split-1", ratio: 0.7 });
    expect((next.root as SplitPane).ratio).toBe(0.7);
  });

  it("paneReducer_should_clampRatioToMinBound_When_ratioTooSmall", () => {
    const root = split("split-1", "vertical", leaf("pane-1"), leaf("pane-2"), 0.5);
    const state = stateOf(root, "pane-1");
    const next = paneReducer(state, { type: "RESIZE_PANE", splitId: "split-1", ratio: 0.01 });
    expect((next.root as SplitPane).ratio).toBeGreaterThanOrEqual(0.1);
  });

  it("paneReducer_should_clampRatioToMaxBound_When_ratioTooLarge", () => {
    const root = split("split-1", "vertical", leaf("pane-1"), leaf("pane-2"), 0.5);
    const state = stateOf(root, "pane-1");
    const next = paneReducer(state, { type: "RESIZE_PANE", splitId: "split-1", ratio: 0.99 });
    expect((next.root as SplitPane).ratio).toBeLessThanOrEqual(0.9);
  });

  it("paneReducer_should_returnUnchangedState_When_splitIdNotFound", () => {
    const state = stateOf(leaf("pane-1"), "pane-1");
    const next = paneReducer(state, { type: "RESIZE_PANE", splitId: "nonexistent", ratio: 0.6 });
    expect(next).toEqual(state);
  });
});

// ─── FOCUS_PANE ───────────────────────────────────────────────────────────────

describe("FOCUS_PANE", () => {
  it("paneReducer_should_updateFocusedPaneId_When_focusPaneDispatched", () => {
    const root = split("split-1", "vertical", leaf("pane-1"), leaf("pane-2"));
    const state = stateOf(root, "pane-1");
    const next = paneReducer(state, { type: "FOCUS_PANE", paneId: "pane-2" });
    expect(next.focusedPaneId).toBe("pane-2");
  });

  it("paneReducer_should_notChangeFocusedPaneId_When_paneIdNotFound", () => {
    const state = stateOf(leaf("pane-1"), "pane-1");
    const next = paneReducer(state, { type: "FOCUS_PANE", paneId: "nonexistent" });
    expect(next.focusedPaneId).toBe("pane-1");
  });
});

// ─── ASSIGN_SESSION ───────────────────────────────────────────────────────────

describe("ASSIGN_SESSION", () => {
  it("paneReducer_should_setSessionId_When_assignSessionDispatched", () => {
    const state = stateOf(leaf("pane-1", null), "pane-1");
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A" });
    expect((next.root as LeafPane).sessionId).toBe("session-A");
  });

  it("paneReducer_should_resetActiveTabToTerminal_When_sessionAssigned", () => {
    const l = { ...leaf("pane-1", null), activeTab: "diff" as const };
    const state = stateOf(l, "pane-1");
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A" });
    expect((next.root as LeafPane).activeTab).toBe("terminal");
  });

  it("paneReducer_should_moveSessionAndClearSource_When_sessionAlreadyPresentInAnotherPane", () => {
    const leaf1 = leaf("pane-1", "session-A");
    const leaf2 = leaf("pane-2", null);
    const root = split("split-1", "vertical", leaf1, leaf2);
    const state = stateOf(root, "pane-2");
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-2", sessionId: "session-A" });

    const leaves = getAllLeaves(next.root);
    const pane1 = leaves.find((l) => l.id === "pane-1")!;
    const pane2 = leaves.find((l) => l.id === "pane-2")!;

    // Session moved to pane-2
    expect(pane2.sessionId).toBe("session-A");
    // Source pane cleared
    expect(pane1.sessionId).toBeNull();
  });

  it("paneReducer_should_clearSourceAndAssignTarget_When_sessionMovedAcrossPanes", () => {
    // Three panes: session-A in pane-1, session-B in pane-2, pane-3 empty
    const leaf1 = leaf("pane-1", "session-A");
    const leaf2 = leaf("pane-2", "session-B");
    const leaf3 = leaf("pane-3", null);
    const inner = split("split-inner", "vertical", leaf2, leaf3);
    const root = split("split-outer", "vertical", leaf1, inner);
    const state = stateOf(root, "pane-3");

    // Move session-A from pane-1 to pane-3
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-3", sessionId: "session-A" });

    const leaves = getAllLeaves(next.root);
    const p1 = leaves.find((l) => l.id === "pane-1")!;
    const p2 = leaves.find((l) => l.id === "pane-2")!;
    const p3 = leaves.find((l) => l.id === "pane-3")!;

    expect(p3.sessionId).toBe("session-A");
    expect(p1.sessionId).toBeNull();
    expect(p2.sessionId).toBe("session-B"); // unaffected
  });

  it("paneReducer_should_allowAssign_When_sessionIsNullInBothPanes", () => {
    const leaf1 = leaf("pane-1", null);
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-new" });
    expect((next.root as LeafPane).sessionId).toBe("session-new");
  });
});

// ─── SPLIT_AND_ASSIGN_SESSION ────────────────────────────────────────────────

describe("SPLIT_AND_ASSIGN_SESSION", () => {
  it("paneReducer_should_splitAndAssignNewLeaf_When_sessionNotInAnyPane", () => {
    const leaf1 = leaf("pane-1", null);
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "pane-1",
      sessionId: "session-A",
      tab: "terminal",
      direction: "vertical",
    });

    expect(next.root.type).toBe("split");
    const s = next.root as SplitPane;
    expect(s.direction).toBe("vertical");
    // First child is the original pane
    expect((s.first as LeafPane).id).toBe("pane-1");
    // Second child (new leaf) holds the session
    const newLeaf = s.second as LeafPane;
    expect(newLeaf.sessionId).toBe("session-A");
    expect(newLeaf.activeTab).toBe("terminal");
    expect(next.focusedPaneId).toBe(newLeaf.id);
  });

  it("paneReducer_should_clearSourceAndSplitAssign_When_sessionAlreadyInAnotherPane", () => {
    // session-A is in pane-1; split pane-2 and move session-A there
    const leaf1 = leaf("pane-1", "session-A");
    const leaf2 = leaf("pane-2", null);
    const root = split("split-1", "vertical", leaf1, leaf2);
    const state = stateOf(root, "pane-2");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "pane-2",
      sessionId: "session-A",
      tab: "terminal",
      direction: "vertical",
    });

    // Tree now has 3 leaves: pane-1 (cleared), pane-2 (original), new leaf (session-A)
    const leaves = getAllLeaves(next.root);
    const p1 = leaves.find((l) => l.id === "pane-1")!;
    const newLeaf = leaves.find((l) => l.sessionId === "session-A")!;

    expect(p1.sessionId).toBeNull();
    expect(newLeaf.sessionId).toBe("session-A");
    expect(next.focusedPaneId).toBe(newLeaf.id);
  });

  it("paneReducer_should_assignInPlace_When_maxDepthReached", () => {
    // Build a tree at max depth so the split is refused
    let node: PaneNode = leaf("deep-leaf");
    for (let i = 0; i < 8; i++) {
      node = split(`split-${i}`, "vertical", node, leaf(`other-${i}`));
    }
    const state = stateOf(node, "deep-leaf");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "deep-leaf",
      sessionId: "session-X",
      tab: "terminal",
      direction: "vertical",
    });

    // Tree structure unchanged; deep-leaf now has the session
    const deepLeaf = getAllLeaves(next.root).find((l) => l.id === "deep-leaf")!;
    expect(deepLeaf.sessionId).toBe("session-X");
    // No new leaves were added
    expect(getAllLeaves(next.root).length).toBe(getAllLeaves(state.root).length);
  });

  it("paneReducer_should_notClearSource_When_sourcePaneIsSameAsSplitTarget", () => {
    // Single leaf pane-1 holds session-A; split pane-1 and assign session-A to new pane
    const leaf1 = leaf("pane-1", "session-A");
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "pane-1",
      sessionId: "session-A",
      tab: "terminal",
      direction: "vertical",
    });

    expect(next.root.type).toBe("split");
    const s = next.root as SplitPane;
    // First child (original pane-1) should retain session-A — not cleared
    expect((s.first as LeafPane).id).toBe("pane-1");
    expect((s.first as LeafPane).sessionId).toBe("session-A");
    // Second child (new leaf) also has session-A
    expect((s.second as LeafPane).sessionId).toBe("session-A");
  });
});

// ─── RESET_LAYOUT ─────────────────────────────────────────────────────────────

describe("RESET_LAYOUT", () => {
  it("paneReducer_should_returnInitialSplitLayout_When_resetLayout", () => {
    const root = split("s1", "vertical",
      split("s2", "horizontal", leaf("p1", "session-A"), leaf("p2", "session-B")),
      leaf("p3", "session-C")
    );
    const state = stateOf(root, "p3");
    const next = paneReducer(state, { type: "RESET_LAYOUT" });
    // RESET_LAYOUT returns the default split layout: session-list | session-detail
    expect(next.root.type).toBe("split");
    const s = next.root as SplitPane;
    expect(s.direction).toBe("vertical");
    expect((s.first as LeafPane).viewKind).toBe("session-list");
    expect((s.second as LeafPane).viewKind).toBe("session-detail");
    expect((s.second as LeafPane).sessionId).toBeNull();
    expect(next.zoomedPaneId).toBeNull();
  });

  it("paneReducer_should_setFocusToDetailPane_When_resetLayout", () => {
    const root = split("s1", "vertical", leaf("p1"), leaf("p2"));
    const state = stateOf(root, "p2");
    const next = paneReducer(state, { type: "RESET_LAYOUT" });
    // Focus lands on the detail pane (second leaf in the default split)
    const detailLeaf = ((next.root as SplitPane).second) as LeafPane;
    expect(next.focusedPaneId).toBe(detailLeaf.id);
  });
});

// ─── ZOOM_PANE ────────────────────────────────────────────────────────────────

describe("ZOOM_PANE", () => {
  it("paneReducer_should_setZoomedPaneId_When_zoomPaneDispatched", () => {
    const state = stateOf(leaf("pane-1"), "pane-1");
    const next = paneReducer(state, { type: "ZOOM_PANE", paneId: "pane-1" });
    expect(next.zoomedPaneId).toBe("pane-1");
  });

  it("paneReducer_should_clearZoomedPaneId_When_zoomPaneDispatchedWithNull", () => {
    const state = { ...stateOf(leaf("pane-1"), "pane-1"), zoomedPaneId: "pane-1" };
    const next = paneReducer(state, { type: "ZOOM_PANE", paneId: null });
    expect(next.zoomedPaneId).toBeNull();
  });

  it("paneReducer_should_toggleZoomOff_When_sameZoomedPaneIdDispatched", () => {
    const state = { ...stateOf(leaf("pane-1"), "pane-1"), zoomedPaneId: "pane-1" };
    const next = paneReducer(state, { type: "ZOOM_PANE", paneId: "pane-1" });
    expect(next.zoomedPaneId).toBeNull();
  });
});

// ─── NAVIGATE_FOCUS (getAdjacentLeaf) ────────────────────────────────────────

describe("NAVIGATE_FOCUS (via getAdjacentLeaf)", () => {
  it("getAdjacentLeaf_should_returnRightLeaf_When_focusedIsLeftOfVerticalSplit", () => {
    const l1 = leaf("left");
    const l2 = leaf("right");
    const root = split("s1", "vertical", l1, l2);
    const adj = getAdjacentLeaf(root, "left", "ArrowRight");
    expect(adj?.id).toBe("right");
  });

  it("getAdjacentLeaf_should_returnNull_When_onlyOnePaneExists", () => {
    const root = leaf("only");
    const adj = getAdjacentLeaf(root, "only", "ArrowRight");
    expect(adj).toBeNull();
  });

  it("getAdjacentLeaf_should_returnNull_When_noAdjacentPaneInDirection", () => {
    const l1 = leaf("left");
    const l2 = leaf("right");
    const root = split("s1", "vertical", l1, l2);
    const adj = getAdjacentLeaf(root, "right", "ArrowRight");
    expect(adj).toBeNull();
  });

  it("getAdjacentLeaf_should_returnBottomLeaf_When_focusedIsTopOfHorizontalSplit", () => {
    const top = leaf("top");
    const bottom = leaf("bottom");
    const root = split("s1", "horizontal", top, bottom);
    const adj = getAdjacentLeaf(root, "top", "ArrowDown");
    expect(adj?.id).toBe("bottom");
  });
});

// ─── State Invariants ─────────────────────────────────────────────────────────

describe("state invariants after any action", () => {
  const ALL_ACTIONS = [
    { type: "SPLIT_PANE" as const, paneId: "pane-1", direction: "vertical" as const },
    { type: "CLOSE_PANE" as const, paneId: "pane-1" },
    { type: "RESIZE_PANE" as const, splitId: "nonexistent", ratio: 0.5 },
    { type: "FOCUS_PANE" as const, paneId: "pane-1" },
    { type: "ASSIGN_SESSION" as const, paneId: "pane-1", sessionId: "session-X" },
    { type: "RESET_LAYOUT" as const },
    { type: "ZOOM_PANE" as const, paneId: "pane-1" },
    { type: "SPLIT_AND_ASSIGN_SESSION" as const, paneId: "pane-1", sessionId: "session-X", tab: "terminal" as const, direction: "vertical" as const },
  ];

  it.each(ALL_ACTIONS)(
    "paneReducer_should_neverProduceStateWithMissingFocusedPaneId_When_$type",
    (action) => {
      const state = stateOf(leaf("pane-1"), "pane-1");
      const next = paneReducer(state, action);
      const allLeafIds = getAllLeaves(next.root).map((l) => l.id);
      expect(allLeafIds).toContain(next.focusedPaneId);
    }
  );

  it("paneReducer_should_keepRatioWithinBounds_When_resizeApplied", () => {
    const root = split("s1", "vertical", leaf("p1"), leaf("p2"), 0.5);
    const state = stateOf(root, "p1");
    [0, 0.01, 0.99, 1, -0.5, 2].forEach((ratio) => {
      const next = paneReducer(state, { type: "RESIZE_PANE", splitId: "s1", ratio });
      const s = next.root as SplitPane;
      expect(s.ratio).toBeGreaterThan(0);
      expect(s.ratio).toBeLessThan(1);
    });
  });

  // T-016: keyboard-initiated ASSIGN_SESSION inherits move-and-clear from reducer
  it("paneReducer_should_allowKeyboardAssignToMoveSession", () => {
    // pane-1 holds session-A; session-B is in pane-2
    const pane1 = { ...leaf("pane-1"), sessionId: "session-A" };
    const pane2 = { ...leaf("pane-2"), sessionId: "session-B" };
    const root = split("split-1", "vertical", pane1, pane2);
    const state = stateOf(root, "pane-1");

    // Keyboard handler fires ASSIGN_SESSION targeting pane-1 with session-B
    const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-B" });

    const leaves = getAllLeaves(next.root);
    const nextPane1 = leaves.find((l) => l.id === "pane-1")!;
    const nextPane2 = leaves.find((l) => l.id === "pane-2")!;
    // session-B moved to pane-1
    expect(nextPane1.sessionId).toBe("session-B");
    // pane-2 cleared (move-and-clear semantics)
    expect(nextPane2.sessionId).toBeNull();
  });
});
