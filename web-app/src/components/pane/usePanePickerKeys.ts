"use client";

import { useEffect, type Dispatch } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { getAllLeaves } from "@/lib/pane/paneReducer";
import type { PaneState, PaneAction } from "@/lib/pane/paneTypes";

interface PanePickerKeysArgs {
  pickerPendingSession: Session | null;
  state: PaneState;
  dispatch: Dispatch<PaneAction>;
  cancelPicker: () => void;
  openPeek: (session: Session) => void;
}

/**
 * Capture-phase keydown for the pane picker: Escape cancels, M peeks, V/H split, A-Z picks the nth
 * eligible pane. Inert unless `active` (the container's window is visible) so a hidden window with
 * a pending picker never swallows keys meant for the visible one.
 */
export function usePanePickerKeys(
  active: boolean,
  { pickerPendingSession, state, dispatch, cancelPicker, openPeek }: PanePickerKeysArgs,
): void {
  useEffect(() => {
    if (!active || !pickerPendingSession) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        cancelPicker();
        return;
      }
      const upper = e.key.toUpperCase();
      if (upper === "M") {
        e.stopPropagation();
        openPeek(pickerPendingSession);
        cancelPicker();
        return;
      }
      if (upper === "V" || upper === "H") {
        e.stopPropagation();
        dispatch({
          type: "SPLIT_AND_ASSIGN_SESSION",
          paneId: state.focusedPaneId,
          sessionId: pickerPendingSession.id,
          tab: "terminal",
          direction: upper === "V" ? "vertical" : "horizontal",
        });
        cancelPicker();
        return;
      }
      if (upper.length === 1 && upper >= "A" && upper <= "Z") {
        const eligiblePanes = getAllLeaves(state.root).filter((l) => l.viewKind !== "session-list");
        const target = eligiblePanes[upper.charCodeAt(0) - 65];
        if (target) {
          e.stopPropagation();
          dispatch({ type: "ASSIGN_SESSION", paneId: target.id, sessionId: pickerPendingSession.id });
          dispatch({ type: "ASSIGN_TAB", paneId: target.id, tab: "terminal" });
          cancelPicker();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown, { capture: true });
    return () => window.removeEventListener("keydown", handleKeyDown, { capture: true });
  }, [active, pickerPendingSession, state.root, state.focusedPaneId, dispatch, cancelPicker, openPeek]);
}
