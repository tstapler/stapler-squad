"use client";

import { useReducer, useEffect, useRef } from "react";
import { PaneState, PaneAction } from "./paneTypes";
import { paneReducer, initialPaneState, getAllLeaves, findLeaf } from "./paneReducer";
import { savePaneLayout, clearPaneLayout, loadPaneLayout, validateAndRepair } from "./usePaneLayout";

interface SessionLike {
  id: string;
}

/**
 * usePaneReducer — wraps useReducer(paneReducer) with localStorage persistence.
 *
 * On first render with non-null sessions: loads and restores the saved layout.
 * On every state change: debounces a save to localStorage.
 * On RESET_LAYOUT: clears localStorage.
 */
export function usePaneReducer(
  sessions: SessionLike[] | null
): [PaneState, React.Dispatch<PaneAction>] {
  const [state, dispatch] = useReducer(paneReducer, undefined, initialPaneState);
  const restoredRef = useRef(false);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // True once sessions have been seen as non-empty — distinguishes "server hasn't responded
  // yet" (sessions=[]) from "user genuinely has no sessions" (sessions=[]).
  const sessionsLoadedRef = useRef(false);

  // Track the first moment sessions arrive from the server.
  // Must be defined before the re-validate effect so the ref is updated first.
  useEffect(() => {
    if (sessions !== null && sessions.length > 0) {
      sessionsLoadedRef.current = true;
    }
  }, [sessions]);

  // Restore layout once sessions first become available.
  // Sessions start as [] in the Redux store before the server responds, so we
  // restore the tree structure immediately but skip session-ID validation until
  // sessions actually load — otherwise validateAndRepair would clear every saved
  // session ID against an empty validIds set.
  useEffect(() => {
    if (sessions === null) return;
    if (restoredRef.current) return;
    restoredRef.current = true;

    const layout = loadPaneLayout();
    if (!layout) return;

    // If sessions haven't loaded from the server yet, restore the layout as-is
    // (preserving saved session IDs). The re-validate effect below will clean up
    // genuinely stale IDs once the real session list arrives.
    const repairedRoot = sessionsLoadedRef.current
      ? validateAndRepair(layout.root, new Set(sessions.map((s) => s.id)))
      : layout.root;

    // Pre-tiling layouts have no session-list pane. Discard them so the user
    // gets the default split layout rather than a grid of empty detail panes.
    const allLeaves = getAllLeaves(repairedRoot);
    const hasListPane = allLeaves.some((l) => l.viewKind === "session-list");
    if (!hasListPane) {
      clearPaneLayout();
      return;
    }

    // Ensure focusedPaneId still exists in the repaired tree
    const focusedStillExists = allLeaves.some((l) => l.id === layout.focusedPaneId);
    const focusedPaneId = focusedStillExists
      ? layout.focusedPaneId
      : allLeaves[0]?.id ?? layout.focusedPaneId;

    // Ensure zoomedPaneId still exists
    const zoomedPaneId = layout.zoomedPaneId !== null && findLeaf(repairedRoot, layout.zoomedPaneId)
      ? layout.zoomedPaneId
      : null;

    dispatch({
      type: "RESTORE_LAYOUT",
      state: { root: repairedRoot, focusedPaneId, zoomedPaneId },
    });
  }, [sessions]);

  // Re-validate when sessions change (sessions deleted externally).
  // Skipped while sessions haven't loaded yet to avoid clearing saved IDs
  // against an empty validIds set on the very first render.
  useEffect(() => {
    if (sessions === null) return;
    if (!restoredRef.current) return;
    if (!sessionsLoadedRef.current) return;

    const validIds = new Set(sessions.map((s) => s.id));
    const allLeaves = getAllLeaves(state.root);
    const hasStale = allLeaves.some(
      (l) => l.sessionId !== null && !validIds.has(l.sessionId)
    );
    if (!hasStale) return;

    const repairedRoot = validateAndRepair(state.root, validIds);
    dispatch({
      type: "RESTORE_LAYOUT",
      state: { ...state, root: repairedRoot },
    });
  }, [sessions]); // eslint-disable-line react-hooks/exhaustive-deps

  // Save to localStorage on state change (debounced 300ms).
  // Guarded by restoredRef so we never overwrite a valid saved layout with the
  // default initialPaneState before the restore effect has had a chance to fire.
  useEffect(() => {
    if (!restoredRef.current) return;
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current);
    }
    saveTimerRef.current = setTimeout(() => {
      savePaneLayout(state);
    }, 300);
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, [state]);

  // Intercept RESET_LAYOUT to also clear localStorage
  const wrappedDispatch: React.Dispatch<PaneAction> = (action) => {
    if (action.type === "RESET_LAYOUT") {
      clearPaneLayout();
    }
    dispatch(action);
  };

  return [state, wrappedDispatch];
}

/** Derived selector: get the currently-focused leaf pane */
export function getFocusedLeaf(state: PaneState) {
  return findLeaf(state.root, state.focusedPaneId) ?? null;
}
