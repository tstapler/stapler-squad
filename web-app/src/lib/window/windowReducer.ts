import { paneReducer, initialPaneState } from "@/lib/pane/paneReducer";
import type { NamedWindow, WindowAction, WindowsState } from "./windowTypes";

function createWindow(state: WindowsState, action: Extract<WindowAction, { type: "CREATE_WINDOW" }>): WindowsState {
  const newWindow: NamedWindow = { id: action.id, name: action.name, paneState: initialPaneState() };
  return { windows: [...state.windows, newWindow] };
}

function renameWindow(state: WindowsState, action: Extract<WindowAction, { type: "RENAME_WINDOW" }>): WindowsState {
  if (action.name.trim() === "") return state;
  return {
    windows: state.windows.map((w) => (w.id === action.id ? { ...w, name: action.name } : w)),
  };
}

function closeWindow(state: WindowsState, action: Extract<WindowAction, { type: "CLOSE_WINDOW" }>): WindowsState {
  const remaining = state.windows.filter((w) => w.id !== action.id);
  if (remaining.length > 0) return { windows: remaining };

  // Every CLOSE_WINDOW carries a `replacement`, so an empty `windows` array
  // is unreachable — the reducer only branches on the length check above.
  const replacement: NamedWindow = {
    id: action.replacement.id,
    name: action.replacement.name,
    paneState: initialPaneState(),
  };
  return { windows: [replacement] };
}

function delegatePaneAction(state: WindowsState, action: Extract<WindowAction, { type: "PANE_ACTION" }>): WindowsState {
  return {
    windows: state.windows.map((w) =>
      w.id === action.windowId ? { ...w, paneState: paneReducer(w.paneState, action.action) } : w
    ),
  };
}

export function windowReducer(state: WindowsState, action: WindowAction): WindowsState {
  switch (action.type) {
    case "CREATE_WINDOW":
      return createWindow(state, action);
    case "RENAME_WINDOW":
      return renameWindow(state, action);
    case "CLOSE_WINDOW":
      return closeWindow(state, action);
    case "PANE_ACTION":
      return delegatePaneAction(state, action);
    case "RESTORE_WINDOWS":
      return { windows: action.windows };
    default: {
      const _exhaustive: never = action;
      throw new Error(`Unhandled WindowAction: ${JSON.stringify(_exhaustive)}`);
    }
  }
}
