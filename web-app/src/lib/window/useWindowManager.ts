"use client";

import { useReducer, useEffect, useRef, useCallback } from "react";
import type { PaneAction } from "@/lib/pane/paneTypes";
import { validateAndRepair } from "@/lib/pane/usePaneLayout";
import { getAllLeaves } from "@/lib/pane/paneReducer";
import { windowReducer } from "./windowReducer";
import { loadWindowLayout, saveWindowLayout } from "./windowPersistence";
import { generateWindowId, initialWindowsState } from "./windowUtils";
import type { NamedWindow, WindowId } from "./windowTypes";

interface SessionLike {
  id: string;
}

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";

/** The window id a tab viewing `closedId` should switch to, given the windows array *before* removal. */
function resolveReplacementViewId(
  windows: NamedWindow[],
  closedId: WindowId,
  replacementId: WindowId
): WindowId {
  const currentIndex = windows.findIndex((w) => w.id === closedId);
  const remaining = windows.filter((w) => w.id !== closedId);
  if (remaining.length === 0) return replacementId;
  if (currentIndex >= 0 && currentIndex < windows.length - 1) {
    return windows[currentIndex + 1].id;
  }
  return remaining[remaining.length - 1].id;
}

/** True if any leaf in the window's pane tree has a sessionId not present in validIds. */
function hasStaleSessionId(window: NamedWindow, validIds: Set<string>): boolean {
  return getAllLeaves(window.paneState.root).some(
    (l) => l.sessionId !== null && !validIds.has(l.sessionId)
  );
}

/**
 * Re-validate every window's pane tree against the current valid session ids.
 * Only windows that actually have a stale sessionId get a new object identity
 * — validateAndRepair itself always returns a fresh object, so gating on its
 * result by reference would treat every call as "changed".
 */
function revalidateWindows(
  windows: NamedWindow[],
  validIds: Set<string>
): { windows: NamedWindow[]; changed: boolean } {
  let changed = false;
  const nextWindows = windows.map((w) => {
    if (!hasStaleSessionId(w, validIds)) return w;
    changed = true;
    return { ...w, paneState: { ...w.paneState, root: validateAndRepair(w.paneState.root, validIds) } };
  });
  return { windows: nextWindows, changed };
}

/**
 * useWindowManager — owns windowReducer state, localStorage persistence, and
 * cross-tab reconciliation, mirroring usePaneReducer.ts's restore/debounce-save/
 * re-validate shape but across an array of windows (see plan.md Epic 1.4).
 *
 * Deliberately has no notion of "the active window" — that stays in
 * useWindowUrlSync (per-tab URL binding, ADR-001).
 */
export function useWindowManager(sessions: SessionLike[] | null) {
  const [state, dispatch] = useReducer(windowReducer, undefined, initialWindowsState);
  const restoredRef = useRef(false);
  const sessionsLoadedRef = useRef(false);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastKnownRevisionRef = useRef(0);

  // Track the first moment sessions arrive from the server (mirrors usePaneReducer.ts).
  useEffect(() => {
    if (sessions !== null && sessions.length > 0) {
      sessionsLoadedRef.current = true;
    }
  }, [sessions]);

  // Restore layout once sessions first become available (Task 1.4.1a).
  useEffect(() => {
    if (sessions === null) return;
    if (restoredRef.current) return;
    restoredRef.current = true;

    const layout = loadWindowLayout();
    if (layout) {
      lastKnownRevisionRef.current = layout.revision;
      dispatch({ type: "RESTORE_WINDOWS", windows: layout.windows });
    } else {
      lastKnownRevisionRef.current = 0;
      dispatch({ type: "RESTORE_WINDOWS", windows: initialWindowsState().windows });
    }
  }, [sessions]);

  // Debounced save (300ms, matches usePaneReducer.ts) through the revision guard (Task 1.4.1b).
  useEffect(() => {
    if (!restoredRef.current) return;
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current);
    }
    saveTimerRef.current = setTimeout(() => {
      const result = saveWindowLayout(state.windows, lastKnownRevisionRef.current);
      if (result.status === "ok") {
        lastKnownRevisionRef.current = result.revision;
      } else if (result.status === "conflict") {
        lastKnownRevisionRef.current = result.latest.revision;
        dispatch({ type: "RESTORE_WINDOWS", windows: result.latest.windows });
      } else {
        console.error("useWindowManager: saveWindowLayout failed");
      }
    }, 300);
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, [state]);

  // Cross-tab reconciliation via the storage event (Task 1.4.1c).
  useEffect(() => {
    function handleStorage(event: StorageEvent) {
      if (event.key !== WINDOW_LAYOUT_KEY) return;
      if (!event.newValue) return;
      try {
        const parsed = JSON.parse(event.newValue) as { revision?: unknown; windows?: unknown };
        if (typeof parsed.revision !== "number" || !Array.isArray(parsed.windows)) return;
        if (parsed.revision <= lastKnownRevisionRef.current) return;
        lastKnownRevisionRef.current = parsed.revision;
        dispatch({ type: "RESTORE_WINDOWS", windows: parsed.windows as NamedWindow[] });
      } catch {
        console.error("useWindowManager: failed to parse storage event newValue");
      }
    }
    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  // Re-validate every window (not just the currently-viewed one) when sessions
  // change (Task 1.4.1d) — background windows keep stale session ids around
  // otherwise (research/features.md).
  useEffect(() => {
    if (sessions === null) return;
    if (!restoredRef.current) return;
    if (!sessionsLoadedRef.current) return;

    const validIds = new Set(sessions.map((s) => s.id));
    const { windows: nextWindows, changed } = revalidateWindows(state.windows, validIds);

    if (!changed) return;
    dispatch({ type: "RESTORE_WINDOWS", windows: nextWindows });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessions]);

  const dispatchPane = useCallback((windowId: WindowId, action: PaneAction) => {
    dispatch({ type: "PANE_ACTION", windowId, action });
  }, []);

  const createWindow = useCallback(
    (name?: string): WindowId => {
      const id = generateWindowId();
      const resolvedName = name ?? `Window ${state.windows.length + 1}`;
      dispatch({ type: "CREATE_WINDOW", id, name: resolvedName });
      return id;
    },
    [state.windows.length]
  );

  const closeWindow = useCallback(
    (id: WindowId): WindowId => {
      const replacement = {
        id: generateWindowId(),
        name: `Window ${state.windows.length + 1}`,
      };
      const nextViewId = resolveReplacementViewId(state.windows, id, replacement.id);
      dispatch({ type: "CLOSE_WINDOW", id, replacement });
      return nextViewId;
    },
    [state.windows]
  );

  const renameWindow = useCallback((id: WindowId, name: string) => {
    dispatch({ type: "RENAME_WINDOW", id, name });
  }, []);

  return {
    windows: state.windows,
    dispatchPane,
    createWindow,
    closeWindow,
    renameWindow,
  };
}

export type UseWindowManagerResult = ReturnType<typeof useWindowManager>;
