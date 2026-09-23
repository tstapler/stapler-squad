import { useState, useCallback } from "react";
import { useDatabases } from "./useDatabase";

export type SessionViewMode = "list" | "board";

function storageKey(workspaceId: string): string {
  return `ws-${workspaceId}.stapler-squad-session-view-mode`;
}

/**
 * Persists the user's List/Board view choice, scoped per workspace so switching
 * workspaces doesn't carry over an unrelated view preference. Falls back to an
 * in-memory "list" default if localStorage throws (e.g. private-browsing quota).
 */
export function useSessionViewMode(): [SessionViewMode, (m: SessionViewMode) => void] {
  const { currentId } = useDatabases();
  const [mode, setModeState] = useState<SessionViewMode>(() => {
    try {
      const raw = currentId ? localStorage.getItem(storageKey(currentId)) : null;
      return raw === "board" ? "board" : "list";
    } catch {
      return "list";
    }
  });
  const setMode = useCallback((m: SessionViewMode) => {
    setModeState(m);
    try {
      if (currentId) localStorage.setItem(storageKey(currentId), m);
    } catch {
      // localStorage unavailable — in-memory state still updates
    }
  }, [currentId]);
  return [mode, setMode];
}
