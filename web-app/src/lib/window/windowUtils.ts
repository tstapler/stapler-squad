import { generateSecureId } from "@/lib/pane/paneUtils";
import { initialPaneState } from "@/lib/pane/paneReducer";
import type { WindowId, WindowsState } from "./windowTypes";

export function generateWindowId(): WindowId {
  return generateSecureId().slice(0, 8);
}

export function initialWindowsState(): WindowsState {
  const id = generateWindowId();
  return { windows: [{ id, name: "Window 1", paneState: initialPaneState() }] };
}
