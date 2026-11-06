"use client";

import type { ReactNode } from "react";
import { createContext, useContext } from "react";
import type { SessionVcsState } from "@/lib/hooks/useSessionVcs";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";
import type { PollMode } from "@/lib/hooks/pollMode";

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
  pollMode,
  children,
}: {
  sessionId: string;
  baseUrl: string;
  /** Forwarded to useSessionVcs — see its doc comment. Defaults to "fast". */
  pollMode?: PollMode;
  children: ReactNode;
}) {
  const value = useSessionVcs(sessionId, baseUrl, pollMode);
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
