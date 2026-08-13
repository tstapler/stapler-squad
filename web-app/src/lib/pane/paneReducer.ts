import {
  PaneState,
  PaneAction,
  PaneNode,
  LeafPane,
  SplitPane,
} from "./paneTypes";
import {
  MIN_RATIO,
  MAX_RATIO,
  createLeaf,
  generatePaneId,
  initialPaneState,
  getAllLeaves,
  findLeaf,
  findSplit,
  findParentSplit,
  findNearestAncestorSplit,
  containsPaneId,
  replaceNode,
  wouldExceedMaxDepth,
  swapPanes,
} from "./paneUtils";

function clampRatio(ratio: number): number {
  return Math.max(MIN_RATIO, Math.min(MAX_RATIO, ratio));
}

/** Get the first leaf of a subtree (for focus fallback) */
function getFirstLeaf(node: PaneNode): LeafPane {
  if (node.type === "leaf") return node;
  return getFirstLeaf(node.first);
}

export function paneReducer(state: PaneState, action: PaneAction): PaneState {
  switch (action.type) {
    case "SPLIT_PANE": {
      const { paneId, direction } = action;
      const target = findLeaf(state.root, paneId);
      if (!target) return state;

      // Guard: don't exceed max depth
      if (wouldExceedMaxDepth(state.root, paneId)) return state;

      const newLeaf = createLeaf();
      const splitNode: SplitPane = {
        type: "split",
        id: generatePaneId(),
        direction,
        ratio: 0.5,
        first: target,
        second: newLeaf,
      };
      const newRoot = replaceNode(state.root, paneId, splitNode);
      return { ...state, root: newRoot, focusedPaneId: newLeaf.id };
    }

    case "CLOSE_PANE": {
      const { paneId } = action;
      // Special case: closing the root leaf
      if (state.root.type === "leaf") {
        if (state.root.id === paneId) {
          return initialPaneState();
        }
        return state;
      }

      const parentSplit = findParentSplit(state.root, paneId);
      if (!parentSplit) return state;

      const sibling = parentSplit.first.id === paneId ? parentSplit.second : parentSplit.first;
      const newRoot = replaceNode(state.root, parentSplit.id, sibling);

      // Focus should move to the sibling — find the first leaf of sibling
      const siblingFocus = getFirstLeaf(sibling);
      return { ...state, root: newRoot, focusedPaneId: siblingFocus.id };
    }

    case "RESIZE_PANE": {
      const { splitId, ratio } = action;
      const target = findSplit(state.root, splitId);
      if (!target) return state;
      const clamped = clampRatio(ratio);
      const updated: SplitPane = { ...target, ratio: clamped };
      const newRoot = replaceNode(state.root, splitId, updated);
      return { ...state, root: newRoot };
    }

    case "FOCUS_PANE": {
      const { paneId } = action;
      // Only update if the pane actually exists
      const leaf = findLeaf(state.root, paneId);
      if (!leaf) return state;
      return { ...state, focusedPaneId: paneId };
    }

    case "ASSIGN_SESSION": {
      const { paneId, sessionId } = action;
      const allLeaves = getAllLeaves(state.root);

      // Move-and-clear: if this session is already in another pane, clear it from there
      // first so the same session is never visible in two panes simultaneously.
      const sourcePaneId =
        allLeaves.find((l) => l.id !== paneId && l.sessionId === sessionId)?.id ?? null;

      const target = findLeaf(state.root, paneId);
      if (!target) return state;

      // Step 1: clear the source pane (atomic — uses the immutable replaceNode chain)
      let newRoot = state.root;
      if (sourcePaneId) {
        const sourceLeaf = findLeaf(newRoot, sourcePaneId)!;
        newRoot = replaceNode(newRoot, sourcePaneId, { ...sourceLeaf, sessionId: null });
      }

      // Step 2: assign to the target pane.
      // If the target is a session-list pane, auto-split to create a detail pane.
      if (target.viewKind === "session-list" && !wouldExceedMaxDepth(newRoot, paneId)) {
        const newDetail = createLeaf(undefined, "terminal", "session-detail");
        const detailWithSession: LeafPane = { ...newDetail, sessionId };
        const splitNode: SplitPane = {
          type: "split",
          id: generatePaneId(),
          direction: "vertical",
          ratio: 0.35,
          first: target,
          second: detailWithSession,
        };
        newRoot = replaceNode(newRoot, paneId, splitNode);
        return { ...state, root: newRoot, focusedPaneId: detailWithSession.id };
      }

      const updated: LeafPane = { ...target, sessionId, activeTab: "terminal" };
      newRoot = replaceNode(newRoot, paneId, updated);
      return { ...state, root: newRoot };
    }

    case "ASSIGN_TAB": {
      const { paneId, tab } = action;
      const target = findLeaf(state.root, paneId);
      if (!target) return state;
      const updated: LeafPane = { ...target, activeTab: tab };
      const newRoot = replaceNode(state.root, paneId, updated);
      return { ...state, root: newRoot };
    }

    case "ZOOM_PANE": {
      const { paneId } = action;
      if (paneId === null) {
        return { ...state, zoomedPaneId: null };
      }
      // Toggle: if same pane is already zoomed, clear it
      if (state.zoomedPaneId === paneId) {
        return { ...state, zoomedPaneId: null };
      }
      return { ...state, zoomedPaneId: paneId };
    }

    case "NUDGE_RESIZE": {
      const { paneId, direction, amountPx, containerSizePx } = action;
      if (containerSizePx <= 0) return state;
      const ancestorSplit = findNearestAncestorSplit(state.root, paneId, direction);
      if (!ancestorSplit) return state;

      const delta = amountPx / containerSizePx;
      // If focused pane is in the first subtree and going right/down, increase ratio
      const focusedInFirst = containsPaneId(ancestorSplit.first, paneId);
      const increase = direction === "ArrowRight" || direction === "ArrowDown";
      const sign = (focusedInFirst && increase) || (!focusedInFirst && !increase) ? 1 : -1;

      const newRatio = clampRatio(ancestorSplit.ratio + sign * delta);
      const updated: SplitPane = { ...ancestorSplit, ratio: newRatio };
      const newRoot = replaceNode(state.root, ancestorSplit.id, updated);
      return { ...state, root: newRoot };
    }

    case "SET_PANE_VIEW": {
      const { paneId, viewKind } = action;
      const target = findLeaf(state.root, paneId);
      if (!target) return state;
      const updated: LeafPane = {
        ...target,
        viewKind,
        // session-list panes don't display a specific session
        sessionId: viewKind === "session-list" ? null : target.sessionId,
      };
      const newRoot = replaceNode(state.root, paneId, updated);
      return { ...state, root: newRoot };
    }

    case "SWAP_PANES": {
      const { paneId, targetPaneId } = action;
      if (paneId === targetPaneId) return state;
      const newRoot = swapPanes(state.root, paneId, targetPaneId);
      if (newRoot === state.root) return state;
      return { ...state, root: newRoot };
    }

    case "RESET_LAYOUT": {
      return initialPaneState();
    }

    case "RESTORE_LAYOUT": {
      return action.state;
    }

    case "SPLIT_AND_ASSIGN_SESSION": {
      const { paneId, sessionId, tab, direction = "vertical" } = action;
      const target = findLeaf(state.root, paneId);
      if (!target) return state;

      const allLeaves = getAllLeaves(state.root);

      // Move-and-clear: find the pane currently holding this session (if any).
      // Skip clearing if the source is the same pane being split (splitting the
      // pane that already holds the session is valid — the new child gets it).
      const sourceLeaf = allLeaves.find((l) => l.sessionId === sessionId) ?? null;
      const sourcePaneId = sourceLeaf?.id ?? null;

      let newRoot = state.root;
      if (sourcePaneId && sourcePaneId !== paneId) {
        const src = findLeaf(newRoot, sourcePaneId)!;
        newRoot = replaceNode(newRoot, sourcePaneId, { ...src, sessionId: null });
      }

      // If max depth reached, fall back to assigning in place
      if (wouldExceedMaxDepth(newRoot, paneId)) {
        const updated: LeafPane = { ...target, sessionId, activeTab: tab, viewKind: "session-detail" };
        newRoot = replaceNode(newRoot, paneId, updated);
        return { ...state, root: newRoot };
      }

      const newLeaf: LeafPane = { type: "leaf", id: generatePaneId(), sessionId, activeTab: tab, viewKind: "session-detail" };
      const splitNode: SplitPane = {
        type: "split",
        id: generatePaneId(),
        direction,
        ratio: 0.5,
        first: target,
        second: newLeaf,
      };
      newRoot = replaceNode(newRoot, paneId, splitNode);
      return { ...state, root: newRoot, focusedPaneId: newLeaf.id };
    }

    default: {
      const _exhaustive: never = action;
      return state;
    }
  }
}

// Re-export helpers needed by tests and hooks
export {
  initialPaneState,
  getAllLeaves,
  findLeaf,
  findSplit,
  getAdjacentLeaf,
  generatePaneId,
  createLeaf,
  swapPanes,
  MIN_RATIO,
  MAX_RATIO,
  containsPaneId,
} from "./paneUtils";
