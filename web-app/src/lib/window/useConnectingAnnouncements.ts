import { useEffect, useRef } from "react";
import { announce } from "@/lib/nav/announce";
import type { TerminalConnectionStatus } from "@/lib/terminal/terminalConnectionStatus";

export const STILL_CONNECTING_DELAY_MS = 1000;

export interface ConnectingItem {
  id: string;
  title: string;
  status: TerminalConnectionStatus;
}

export function stillConnectingMessage(titles: readonly string[]): string {
  return titles.length === 1 ? `Still connecting ${titles[0]}` : `Still connecting ${titles.length} sessions`;
}

/**
 * Speaks once per activation: "Still connecting ..." if any pane is still connecting after 1 s,
 * and "Could not connect to <title>" once per failed pane. Silent while the window is hidden.
 */
export function useConnectingAnnouncements(active: boolean, items: readonly ConnectingItem[]): void {
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const announcedFailures = useRef(new Set<string>());

  useEffect(() => {
    if (!active) return;
    const timer = setTimeout(() => {
      const connecting = itemsRef.current.filter((i) => i.status === "connecting");
      if (connecting.length > 0) announce(stillConnectingMessage(connecting.map((i) => i.title)));
    }, STILL_CONNECTING_DELAY_MS);
    return () => clearTimeout(timer);
  }, [active]);

  useEffect(() => {
    if (!active) return;
    for (const item of items) {
      if (item.status === "failed" && !announcedFailures.current.has(item.id)) {
        announcedFailures.current.add(item.id);
        announce(`Could not connect to ${item.title}`);
      } else if (item.status !== "failed") {
        announcedFailures.current.delete(item.id);
      }
    }
  }, [active, items]);
}
