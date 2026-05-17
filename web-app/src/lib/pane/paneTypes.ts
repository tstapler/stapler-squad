export type PaneId = string;
export type SplitDirection = "horizontal" | "vertical";
export type PaneViewKind = "session-detail" | "session-list";

// Mirrors SessionDetail's tab union
export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser";

export interface LeafPane {
  type: "leaf";
  id: PaneId;
  viewKind: PaneViewKind;   // what this pane displays; defaults to "session-detail"
  sessionId: string | null; // used only when viewKind === "session-detail"
  activeTab: SessionDetailTab;
}

export interface SplitPane {
  type: "split";
  id: PaneId;
  // "vertical"   = children sit left | right (column split)
  // "horizontal" = children sit top | bottom (row split)
  direction: SplitDirection;
  ratio: number;  // [0.0, 1.0], fraction of space given to `first`
  first: PaneNode;
  second: PaneNode;
}

export type PaneNode = LeafPane | SplitPane;

export interface PaneState {
  root: PaneNode;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;  // Ctrl+Z zoom: null = normal view
}

// Persisted to localStorage as-is (no DOM refs, no functions)
export interface PersistedPaneLayout {
  version: 1;
  root: PaneNode;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;
}

export type PaneAction =
  | { type: "SPLIT_PANE";    paneId: PaneId; direction: SplitDirection }
  | { type: "CLOSE_PANE";    paneId: PaneId }
  | { type: "RESIZE_PANE";   splitId: PaneId; ratio: number }
  | { type: "FOCUS_PANE";    paneId: PaneId }
  | { type: "ASSIGN_SESSION"; paneId: PaneId; sessionId: string }
  | { type: "ASSIGN_TAB";    paneId: PaneId; tab: SessionDetailTab }
  | { type: "ZOOM_PANE";     paneId: PaneId | null }
  | { type: "NUDGE_RESIZE";  paneId: PaneId; direction: "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown"; amountPx: number; containerSizePx: number }
  | { type: "SET_PANE_VIEW"; paneId: PaneId; viewKind: PaneViewKind }
  | { type: "SWAP_PANES";    paneId: PaneId; targetPaneId: PaneId }
  | { type: "RESET_LAYOUT" }
  | { type: "RESTORE_LAYOUT"; state: PaneState }
  | { type: "SPLIT_AND_ASSIGN_SESSION"; paneId: PaneId; sessionId: string; tab: SessionDetailTab; direction?: SplitDirection };
