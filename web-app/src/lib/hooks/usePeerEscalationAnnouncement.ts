"use client";

import { useEffect, useRef } from "react";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { AnnounceApi } from "./useAnnounce";

/** Metadata the server stamps on a peer-help escalation (services.MetaPeerTitle). */
export const PEER_TITLE_KEY = "peer_title";
/** An item already in history this long ago was not "just" raised, so it is not announced. */
const RECENT_MS = 60_000;

const isPeerEscalation = (n: NotificationHistoryItem): boolean =>
  n.isPendingDecision === true && n.metadata?.[PEER_TITLE_KEY] !== undefined;

/**
 * A peer-help escalation is one more decision for the operator (UX-AC80): it speaks once,
 * politely, in the shared region and never moves focus. The row itself waits behind the
 * engaged-card freeze like any other new Needs-you item.
 */
export function usePeerEscalationAnnouncement(history: readonly NotificationHistoryItem[], announce: AnnounceApi["announce"]): void {
  const seen = useRef<Set<string> | null>(null);
  useEffect(() => {
    const current = history.filter(isPeerEscalation);
    if (seen.current === null) {
      seen.current = new Set(current.map((n) => n.id));
      return;
    }
    const known = seen.current;
    const fresh = current.filter((n) => !known.has(n.id) && n.timestamp > Date.now() - RECENT_MS);
    current.forEach((n) => known.add(n.id));
    if (fresh.length > 0) {
      announce(fresh.length === 1 ? "1 item needs a decision" : `${fresh.length} items need a decision`, "polite", "peer-help-escalation");
    }
  }, [history, announce]);
}
