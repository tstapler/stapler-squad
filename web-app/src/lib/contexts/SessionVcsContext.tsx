"use client";

import { createContext, useContext, ReactNode } from "react";
import { useSessionVcs, SessionVcsState } from "@/lib/hooks/useSessionVcs";

const SessionVcsContext = createContext<SessionVcsState | null>(null);

/**
 * Provides a single shared VCS state (status + diff) for a SessionDetail and
 * all of its tabs (VcsPanel, FilesTab, DiffViewer). Without this provider all
 * three tabs fetched independently with no shared cache.
 *
 * Place this around the tab content in SessionDetail, not at the app root —
 * the data is scoped to a single session and should start/stop with it.
 */
export function SessionVcsProvider({
  sessionId,
  baseUrl,
  isActive,
  children,
}: {
  sessionId: string;
  baseUrl: string;
  /** Forwarded to useSessionVcs — see its doc comment. Defaults to true. */
  isActive?: boolean;
  children: ReactNode;
}) {
  const value = useSessionVcs(sessionId, baseUrl, isActive);
  return (
    <SessionVcsContext.Provider value={value}>
      {children}
    </SessionVcsContext.Provider>
  );
}

export function useSessionVcsContext(): SessionVcsState {
  const ctx = useContext(SessionVcsContext);
  if (!ctx) throw new Error("useSessionVcsContext must be used within SessionVcsProvider");
  return ctx;
}
