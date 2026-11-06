import type { PaneAction, PaneState } from "@/lib/pane/paneTypes";

export type WindowId = string;

// Deliberately not named bare `Window` — that would shadow the global DOM
// `Window` interface in every file that also calls window.location/addEventListener.
export interface NamedWindow {
  id: WindowId;
  name: string;
  paneState: PaneState;
}

export interface WindowsState {
  windows: NamedWindow[];
}

export type WindowRevision = number;

// Persisted to localStorage as-is (no DOM refs, no functions)
export interface PersistedWindowLayoutV2 {
  version: 2;
  revision: WindowRevision;
  windows: NamedWindow[];
}

export type WindowAction =
  | { type: "CREATE_WINDOW"; id: WindowId; name: string }
  // `replacement` is required (not optional): every CLOSE_WINDOW carries one
  // regardless of windows.length, so an empty windows array is unrepresentable.
  | { type: "CLOSE_WINDOW"; id: WindowId; replacement: { id: WindowId; name: string } }
  | { type: "RENAME_WINDOW"; id: WindowId; name: string }
  | { type: "PANE_ACTION"; windowId: WindowId; action: PaneAction }
  | { type: "RESTORE_WINDOWS"; windows: NamedWindow[] };
