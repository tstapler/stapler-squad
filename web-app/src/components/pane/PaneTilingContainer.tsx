"use client";

import { useRef, useEffect, useState, useCallback } from "react";
import { Columns2, Rows2, LayoutList, X } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import { usePaneReducer } from "@/lib/pane/usePaneReducer";
import { usePaneShortcuts } from "@/lib/pane/usePaneShortcuts";
import { getAllLeaves } from "@/lib/pane/paneReducer";
import type { SplitDirection } from "@/lib/pane/paneTypes";
import { PaneSplitRenderer } from "./PaneSplitRenderer";
import { PaneContext } from "./PaneContext";
import {
  pickerActionBar,
  pickerActionButton,
  pickerActionKbd,
} from "@/styles/pane/panePickerOverlay.css";

interface PaneTilingContainerProps {
  sessions: Session[];
  /**
   * When set, the session with this id is assigned to the currently focused pane.
   * The `version` field must change each time an assignment should fire (even for
   * the same session), so callers should increment a counter.
   */
  externalSessionAssign?: {
    sessionId: string;
    tab?: "terminal" | "diff" | "vcs" | "logs" | "info" | "files";
    forceNewPane?: boolean;
    version: number;
  } | null;
}

/**
 * PaneTilingContainer — top-level tiling layout component.
 *
 * Holds the pane reducer state, wires keyboard shortcuts, persists layout,
 * and renders the recursive pane tree via PaneSplitRenderer.
 */
export function PaneTilingContainer({
  sessions,
  externalSessionAssign,
}: PaneTilingContainerProps) {
  const [state, dispatch] = usePaneReducer(sessions);
  const containerRef = useRef<HTMLDivElement>(null);
  const prevVersionRef = useRef<number | null>(null);

  const [pickerPendingSession, setPickerPendingSession] = useState<Session | null>(null);

  const cancelPicker = useCallback(() => {
    setPickerPendingSession(null);
  }, []);

  const triggerPickerForceNew = useCallback(
    (session: Session, tab?: string) => {
      const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

      // Choose split direction that keeps terminal width >= 120 columns.
      // At ~8px per character, 120 cols ≈ 960px per pane → need container >= 1920px for vertical split.
      // If the container is narrower, use horizontal split (stacked) to preserve full width.
      const containerWidth = containerRef.current?.getBoundingClientRect().width ?? 0;
      const direction: SplitDirection = containerWidth / 2 >= 960 ? "vertical" : "horizontal";

      dispatch({
        type: "SPLIT_AND_ASSIGN_SESSION",
        paneId: state.focusedPaneId,
        sessionId: session.id,
        tab: resolvedTab,
        direction,
      });
    },
    [state.focusedPaneId, dispatch, containerRef],
  );

  const triggerPicker = useCallback(
    (session: Session, tab?: string) => {
      const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

      const allLeaves = getAllLeaves(state.root);
      const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");

      if (eligiblePanes.length === 0) {
        // Auto-split: reducer creates a new detail pane beside the session-list pane.
        dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
      } else if (eligiblePanes.length === 1) {
        // Single detail pane: assign directly. If focused pane is the detail pane,
        // eligiblePanes[0] === focusedLeaf, so the intent shortcut still fires — just
        // via this branch rather than the removed early-return block.
        dispatch({ type: "ASSIGN_SESSION", paneId: eligiblePanes[0].id, sessionId: session.id });
        dispatch({ type: "ASSIGN_TAB", paneId: eligiblePanes[0].id, tab: resolvedTab });
        // On mobile, vertical splits show only the focused pane. Move focus to the detail
        // pane so the user sees the session they just opened instead of the session list.
        dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
      } else {
        // 2+ eligible panes: always show the picker overlay, even if a detail pane
        // is focused. The user must choose which pane receives the session.
        setPickerPendingSession(session);
      }
    },
    [state.root, state.focusedPaneId, dispatch],
  );

  // Register keyboard shortcuts
  usePaneShortcuts(state, dispatch, containerRef);

  // Keyboard handler for the pane picker: Escape cancels, A–Z selects the nth eligible pane.
  useEffect(() => {
    if (!pickerPendingSession) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        cancelPicker();
        return;
      }
      const upper = e.key.toUpperCase();
      if (upper === "V") {
        e.stopPropagation();
        dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "vertical" });
        cancelPicker();
        return;
      }
      if (upper === "H") {
        e.stopPropagation();
        dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "horizontal" });
        cancelPicker();
        return;
      }
      if (upper.length === 1 && upper >= "A" && upper <= "Z") {
        const allLeaves = getAllLeaves(state.root);
        const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");
        const idx = upper.charCodeAt(0) - 65;
        const target = eligiblePanes[idx];
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
  }, [pickerPendingSession, state.root, dispatch, cancelPicker]);

  // When externalSessionAssign fires (omnibar, URL nav, keyboard), route through
  // triggerPicker so the user gets the same pane-picker UX as session-list clicks.
  useEffect(() => {
    if (!externalSessionAssign) return;
    if (externalSessionAssign.version === prevVersionRef.current) return;
    prevVersionRef.current = externalSessionAssign.version;

    const session = sessions.find((s) => s.id === externalSessionAssign.sessionId);
    if (!session) return;
    if (externalSessionAssign.forceNewPane) {
      triggerPickerForceNew(session, externalSessionAssign.tab);
    } else {
      triggerPicker(session, externalSessionAssign.tab);
    }
  }, [externalSessionAssign, sessions, triggerPicker, triggerPickerForceNew]);

  const allLeaves = pickerPendingSession ? getAllLeaves(state.root) : [];
  const listPane = allLeaves.find((l) => l.viewKind === "session-list") ?? null;

  return (
    <PaneContext.Provider value={{ state, dispatch, sessions, pickerPendingSession, triggerPicker, triggerPickerForceNew, cancelPicker }}>
      <div
        ref={containerRef}
        style={{ display: "flex", flex: 1, overflow: "hidden", minHeight: 0, position: "relative" }}
        data-context="cockpit"
      >
        <PaneSplitRenderer
          state={state}
          dispatch={dispatch}
          sessions={sessions}
        />
        {pickerPendingSession && (
          <div className={pickerActionBar}>
            <button
              className={pickerActionButton}
              onClick={() => {
                dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "vertical" });
                cancelPicker();
              }}
              title="Open in a new pane side by side"
            >
              <Columns2 size={14} />
              Side by side
              <span className={pickerActionKbd}>V</span>
            </button>
            <button
              className={pickerActionButton}
              onClick={() => {
                dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "horizontal" });
                cancelPicker();
              }}
              title="Open in a new pane stacked top/bottom"
            >
              <Rows2 size={14} />
              Top / Bottom
              <span className={pickerActionKbd}>H</span>
            </button>
            {listPane && (
              <button
                className={pickerActionButton}
                onClick={() => {
                  dispatch({ type: "SET_PANE_VIEW", paneId: listPane.id, viewKind: "session-detail" });
                  dispatch({ type: "ASSIGN_SESSION", paneId: listPane.id, sessionId: pickerPendingSession.id });
                  cancelPicker();
                }}
                title="Replace the session list pane with this session"
              >
                <LayoutList size={14} />
                Replace list
              </button>
            )}
            <button
              className={pickerActionButton}
              onClick={cancelPicker}
              title="Cancel (Esc)"
            >
              <X size={14} />
              Cancel
              <span className={pickerActionKbd}>Esc</span>
            </button>
          </div>
        )}
      </div>
    </PaneContext.Provider>
  );
}
