import { PaneState, PaneNode, LeafPane, PersistedPaneLayout, PaneViewKind } from "./paneTypes";

const STORAGE_KEY = "cockpit.paneLayout";

/**
 * Walk the pane tree and nullify any sessionId that is not in the validIds set.
 * Returns a new tree (immutable).
 */
export function validateAndRepair(tree: PaneNode, validIds: Set<string>): PaneNode {
  if (tree.type === "leaf") {
    const leaf: LeafPane = {
      ...tree,
      // Backward compat: old saved layouts pre-dating viewKind default to session-detail
      viewKind: (tree.viewKind ?? "session-detail") as PaneViewKind,
      sessionId:
        tree.sessionId !== null && validIds.has(tree.sessionId)
          ? tree.sessionId
          : null,
    };
    return leaf;
  }
  return {
    ...tree,
    first: validateAndRepair(tree.first, validIds),
    second: validateAndRepair(tree.second, validIds),
  };
}

/** Serialize and save the pane state to localStorage */
export function savePaneLayout(state: PaneState): void {
  try {
    const layout: PersistedPaneLayout = {
      version: 1,
      root: state.root,
      focusedPaneId: state.focusedPaneId,
      zoomedPaneId: state.zoomedPaneId,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
  } catch {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
  }
}

/** Clear the saved pane layout from localStorage */
export function clearPaneLayout(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

/**
 * Parse and validate a PersistedPaneLayout from localStorage.
 * Returns null on any parse/validation error.
 */
export function loadPaneLayout(): PersistedPaneLayout | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return null;

    const parsed = JSON.parse(stored) as Partial<PersistedPaneLayout>;

    if (parsed.version !== 1) return null;
    if (!parsed.root) return null;
    if (typeof parsed.focusedPaneId !== "string") return null;
    if (!("zoomedPaneId" in parsed)) return null;

    return parsed as PersistedPaneLayout;
  } catch {
    return null;
  }
}
