import { type Dispatch, type SetStateAction, useEffect, useRef, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { itemKey, isEscalationReason, type ResolvedGhost } from "./stuckItemsSectionShared";

const GHOST_LIFETIME_MS = 2800;

/**
 * Builds the "just resolved" copy for one ghost row.
 *
 * Only MULTIPLE_REASONS gets de-escalation copy, not BOUNCE_CAP_EXHAUSTED —
 * intentional, not an oversight: bounce_cap_exhausted can only ever coexist
 * with an open bouncing row (backend invariant, see reconcileBouncingItems'
 * paired resolve), so it never resolves independently while the item still
 * has open non-escalation reasons. If that invariant ever changes, this
 * condition needs the same OR as isEscalationReason.
 */
function buildGhostCopy(
  item: StuckBacklogItem,
  items: StuckBacklogItem[],
  nextItemIds: Set<string>
): { message: string; trailingMessage?: string } {
  const isDeescalation = item.reason === StuckReason.MULTIPLE_REASONS && nextItemIds.has(item.itemId);
  if (!isDeescalation) {
    const message =
      item.reason === StuckReason.PR_READY_UNMERGED && item.prNumber > 0
        ? `PR #${item.prNumber} was merged.`
        : "This item was just resolved.";
    return { message };
  }

  const remainingReasons = items.filter(
    (i) => i.itemId === item.itemId && !isEscalationReason(i.reason)
  ).length;
  return {
    message: `No longer critical — down to ${remainingReasons} open reason${remainingReasons !== 1 ? "s" : ""}.`,
    trailingMessage: "This card will be removed shortly; the item itself is still open elsewhere in the list.",
  };
}

/** Adds one ghost entry per newly-missing item that isn't already tracked. */
function addGhostEntries(
  prev: Map<string, ResolvedGhost>,
  newlyMissingExpanded: StuckBacklogItem[],
  items: StuckBacklogItem[],
  nextItemIds: Set<string>,
  scheduleGhostExpiry: (key: string) => void
): Map<string, ResolvedGhost> {
  const next = new Map(prev);
  const toAdd = newlyMissingExpanded.filter((item) => !next.has(itemKey(item)));
  for (const item of toAdd) {
    const key = itemKey(item);
    next.set(key, { item, ...buildGhostCopy(item, items, nextItemIds) });
    scheduleGhostExpiry(key);
  }
  return next;
}

/**
 * Surface 12: an item that resolves while its card is expanded gets a brief
 * "was just resolved" confirmation instead of being yanked out immediately.
 * Extracted from StuckItemsSection.tsx — same behavior, isolated as its own concern.
 *
 * Task 2.1.4a: `itemKey` is per-(itemId, reason), so a `multiple_reasons` row
 * de-escalating (resolving while the item itself remains open under other
 * reasons) is *already* caught by this same "row disappeared while expanded"
 * comparison — each reason is its own list entry. The only extension needed
 * is distinguishing that case (item still present under another reason) from
 * true full-item resolution, so the copy doesn't falsely claim the whole item
 * is going away.
 */
export function useResolvedGhosts(
  items: StuckBacklogItem[],
  expandedKeys: Set<string>,
  setExpandedKeys: Dispatch<SetStateAction<Set<string>>>
) {
  const [resolvedGhosts, setResolvedGhosts] = useState<Map<string, ResolvedGhost>>(new Map());
  const prevItemsRef = useRef<StuckBacklogItem[]>([]);
  const ghostTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // Expires one ghost row after GHOST_LIFETIME_MS: drops it from resolvedGhosts
  // and finally collapses the (now-gone) card out of expandedKeys.
  const scheduleGhostExpiry = (key: string) => {
    const timer = setTimeout(() => {
      setResolvedGhosts((p) => {
        const n = new Map(p);
        n.delete(key);
        return n;
      });
      setExpandedKeys((p) => {
        const n = new Set(p);
        n.delete(key);
        return n;
      });
      ghostTimersRef.current.delete(key);
    }, GHOST_LIFETIME_MS);
    ghostTimersRef.current.set(key, timer);
  };

  useEffect(() => {
    const prevItems = prevItemsRef.current;
    prevItemsRef.current = items;

    const nextKeys = new Set(items.map(itemKey));
    const newlyMissingExpanded = prevItems.filter(
      (p) => expandedKeys.has(itemKey(p)) && !nextKeys.has(itemKey(p))
    );
    if (newlyMissingExpanded.length === 0) return;

    const nextItemIds = new Set(items.map((i) => i.itemId));
    setResolvedGhosts((prev) =>
      addGhostEntries(prev, newlyMissingExpanded, items, nextItemIds, scheduleGhostExpiry)
    );
    // scheduleGhostExpiry is recreated every render but only reads from refs/setters
    // that are themselves stable, so omitting it keeps this effect's dependency set
    // focused on the data that actually drives it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, expandedKeys, setExpandedKeys]);

  useEffect(() => {
    const timers = ghostTimersRef.current;
    return () => {
      for (const t of timers.values()) clearTimeout(t);
    };
  }, []);

  return resolvedGhosts;
}
