import { useCallback, useMemo, useState } from "react";

function storageKey(itemId: string, sectionKey: string): string {
  return `backlog-detail-showmore-${itemId}-${sectionKey}`;
}

export interface UseShowMoreResult<T> {
  /** The items to render — capped to `cap` unless "show all" has been triggered. */
  visible: T[];
  /** True when `items.length` exceeds `cap` and the list is still capped. */
  hasMore: boolean;
  /** `items.length - cap`, clamped to 0 — the count for the "Show N more" label. */
  remaining: number;
  /** Reveals the rest of the list and persists that choice for this item/section. */
  showAll: () => void;
}

/**
 * Blocker C fix (research/features.md finding #5) + pre-mortem finding #2:
 * caps a long list's default rendering to the most recent `cap` entries,
 * revealing the rest via `showAll()`. The "show all" choice is
 * `localStorage`-backed (key `backlog-detail-showmore-${itemId}-${sectionKey}`,
 * same pattern/convention as `useSectionExpandState`) — NOT a plain
 * `useState` that resets to the capped view on every mount, since that
 * would make a heavily-cycled, chronically-stuck item (exactly the case
 * this project exists to make inspectable) re-pay the "Show N more" click
 * on every single re-open (Story 3.1.4, Task 3.1.4c2).
 *
 * Reads/writes are wrapped in try/catch, matching
 * `useSectionExpandState`'s defensive localStorage pattern.
 */
export function useShowMore<T>(
  itemId: string,
  sectionKey: string,
  items: T[],
  cap: number
): UseShowMoreResult<T> {
  const [showingAll, setShowingAll] = useState<boolean>(() => {
    try {
      return localStorage.getItem(storageKey(itemId, sectionKey)) === "true";
    } catch {
      return false;
    }
  });

  const showAll = useCallback(() => {
    setShowingAll(true);
    try {
      localStorage.setItem(storageKey(itemId, sectionKey), "true");
    } catch {
      // Persistence is best-effort only — in-memory state still updates.
    }
  }, [itemId, sectionKey]);

  const hasMore = items.length > cap && !showingAll;
  const remaining = Math.max(items.length - cap, 0);
  const visible = useMemo(
    () => (showingAll || items.length <= cap ? items : items.slice(-cap)),
    [items, cap, showingAll]
  );

  return { visible, hasMore, remaining, showAll };
}
