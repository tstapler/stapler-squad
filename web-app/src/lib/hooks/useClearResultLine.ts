"use client";

import { useCallback, useState } from "react";
import { useAnnounce } from "@/lib/hooks/useAnnounce";

/** What a `ClearNotificationHistory{notification_ids}` call did: rows deleted and ids the server kept. */
export interface ClearByIdsResult {
  deleted: number;
  kept: string[];
}

/** The line shown (and announced once) when the server refused to delete rows; null when nothing was kept. */
export function keptLineText(keptCount: number): string | null {
  return keptCount > 0 ? `${keptCount} kept: still needs attention` : null;
}

/**
 * Shared by the tray and the Notifications page (Task 4.4f): the one place a
 * clear response's `kept` ids become a visible, once-announced line. The line
 * clears on the next user action via `clearKeptLine`; it carries no live role of
 * its own (the Announcer owns speech). The kept rows themselves never left local
 * history, so there is nothing to restore and no further RPC.
 */
export function useClearResultLine() {
  const { announce } = useAnnounce();
  const [keptLine, setKeptLine] = useState<string | null>(null);
  const applyClearResult = useCallback(
    (result: ClearByIdsResult | null | undefined) => {
      const line = keptLineText(result?.kept.length ?? 0);
      setKeptLine(line);
      if (line) announce(line, "polite", "clear-kept");
    },
    [announce],
  );
  const clearKeptLine = useCallback(() => setKeptLine(null), []);
  return { keptLine, applyClearResult, clearKeptLine };
}
