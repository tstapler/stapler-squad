"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { NotificationHistoryItem } from "@/lib/types/notification";

export interface StableRowOrder {
  items: NotificationHistoryItem[];
  /** Rows that arrived while the order was held and are not shown yet. */
  heldCount: number;
  /** Show the held rows and let the order change. */
  release: () => void;
}

/**
 * Keeps rows from reordering under the user's finger (TR-5). While `hold` is
 * true, a row keeps the timestamp it had when the hold began (so a duplicate
 * that bumps a group's recency increments its count in place without moving
 * it). With `withholdNew`, rows that arrive are also held behind a "N new"
 * count; without it they appear at once (the list is at the top, so nothing
 * the user is reading shifts but a new row is not hidden). When the hold ends,
 * or the user releases it, the live order applies.
 */
export function useStableRowOrder(
  history: NotificationHistoryItem[],
  hold: boolean,
  withholdNew: boolean = hold,
): StableRowOrder {
  const [snapshot, setSnapshot] = useState<Map<string, number> | null>(null);
  const holdingRef = useRef(false);
  // Rows shown while not withholding must not vanish when the user scrolls and withholding starts.
  const admittedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (hold && !holdingRef.current) {
      holdingRef.current = true;
      admittedRef.current = new Set();
      setSnapshot(new Map(history.map((n) => [n.id, n.timestamp])));
    } else if (!hold && holdingRef.current) {
      holdingRef.current = false;
      setSnapshot(null);
    }
    // The snapshot is taken once per hold; later history changes are read through `history`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hold]);

  const release = useCallback(() => {
    holdingRef.current = false;
    setSnapshot(null);
  }, []);

  if (!hold || !snapshot) return { items: history, heldCount: 0, release };

  const items: NotificationHistoryItem[] = [];
  let heldCount = 0;
  for (const item of history) {
    const frozen = snapshot.get(item.id);
    if (frozen === undefined) {
      if (!withholdNew) admittedRef.current.add(item.id);
      if (withholdNew && !admittedRef.current.has(item.id)) {
        heldCount += 1;
      } else {
        items.push(item);
      }
      continue;
    }
    items.push(frozen === item.timestamp ? item : { ...item, timestamp: frozen });
  }
  return { items, heldCount, release };
}
