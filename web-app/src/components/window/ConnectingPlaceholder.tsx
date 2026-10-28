"use client";

import { useMemo } from "react";
import { retryTerminalConnection, useTerminalConnectionStatuses } from "@/lib/terminal/terminalConnectionStatus";
import { useConnectingAnnouncements, type ConnectingItem } from "@/lib/window/useConnectingAnnouncements";
import { placeholderItem, placeholderRoot } from "./ConnectingPlaceholder.css";

export interface ConnectingSession {
  id: string;
  title: string;
}

interface ConnectingPlaceholderProps {
  sessions: readonly ConnectingSession[];
  /** The window is the current one; a hidden window never announces. */
  active: boolean;
}

/**
 * Per-window overlay shown while a window mounted cold has terminals connecting. Not a live region
 * (silent by design, ux.md S2); only `announce()` speaks, through `useConnectingAnnouncements`.
 * Renders nothing once every terminal is connected.
 */
export function ConnectingPlaceholder({ sessions, active }: ConnectingPlaceholderProps) {
  const statuses = useTerminalConnectionStatuses(sessions.map((s) => s.id));
  const items = useMemo<ConnectingItem[]>(
    () => sessions.map((s, i) => ({ id: s.id, title: s.title, status: statuses[i] })),
    // statuses is a fresh array per render; its content key is what matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sessions, statuses.join(",")],
  );
  useConnectingAnnouncements(active, items);

  const pending = items.filter((i) => i.status !== "connected");
  if (pending.length === 0) return null;
  return (
    <div className={placeholderRoot} data-testid="connecting-placeholder">
      {pending.map((item) =>
        item.status === "failed" ? (
          <div key={item.id} className={placeholderItem} data-testid="connecting-placeholder-failed" style={{ pointerEvents: "auto" }}>
            <span>Could not connect to {item.title}</span>
            <RetryButton sessionId={item.id} />
          </div>
        ) : (
          <div key={item.id} className={placeholderItem} data-testid="connecting-placeholder-item" aria-busy="true">
            Connecting {item.title}...
          </div>
        ),
      )}
    </div>
  );
}

function RetryButton({ sessionId }: { sessionId: string }) {
  return (
    // analytics-exempt
    <button type="button" onClick={() => retryTerminalConnection(sessionId)}>
      Retry
    </button>
  );
}
