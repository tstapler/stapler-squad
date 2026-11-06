import { useEffect, useRef, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { itemKey } from "./stuckItemsSectionShared";

/**
 * Fetches the current reworkCapOverride for newly-expanded REWORK_CAP items
 * so StuckItemDetail can show it instead of guessing blind. Extracted from
 * StuckItemsSection.tsx — same behavior, isolated as its own concern. Skips
 * items already present in the map (including a resolved `undefined`) so
 * re-expanding doesn't re-fetch.
 *
 * reworkCapOverrides is deliberately NOT a dependency of the effect below
 * even though it's read inside (via reworkCapFetchStartedRef.current
 * instead) — it used to be, but every successful per-item fetch changed the
 * map reference and re-fired the effect while the previous invocation's
 * for-loop was still mid-flight on later items, causing O(N^2) duplicate
 * getBacklogItem calls when several REWORK_CAP items were expanded at once.
 * reworkCapFetchStartedRef (a ref, so mutating it can't trigger a
 * re-render/re-run) tracks "already fetched or in-flight" instead, updated
 * synchronously before the async fetch starts so an overlapping effect
 * invocation (from items/expandedKeys changing mid-batch) can't double-fetch
 * the same item either.
 */
export function useReworkCapOverrides(
  items: StuckBacklogItem[],
  expandedKeys: Set<string>,
  getBacklogItem: (itemId: string) => Promise<BacklogItem | null>
) {
  const [reworkCapOverrides, setReworkCapOverrides] = useState<Map<string, number | undefined>>(new Map());
  // Mirrors reworkCapOverrides synchronously so the effect's cleanup can check
  // "was this item actually committed?" without depending on a stale closure
  // over the state value — the effect deliberately excludes reworkCapOverrides
  // from its dependency array, so its own closure only ever sees the map as of
  // when the effect instance started, not later commits made mid-batch.
  const reworkCapOverridesRef = useRef<Map<string, number | undefined>>(new Map());
  // Tracks itemIds already fetched-or-in-flight. Deliberately NOT derived from
  // reworkCapOverrides state — see the doc comment above.
  const reworkCapFetchStartedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    // Captured once per effect instance (not re-read from the ref inside the
    // cleanup) purely to satisfy react-hooks/exhaustive-deps's "ref value may
    // have changed by cleanup time" check — reworkCapFetchStartedRef.current
    // is a single long-lived Set that's only ever mutated in place, never
    // reassigned, so this alias and `.current` always point at the same Set.
    const fetchStarted = reworkCapFetchStartedRef.current;
    const toFetch = items.filter(
      (item) =>
        item.reason === StuckReason.REWORK_CAP &&
        expandedKeys.has(itemKey(item)) &&
        !reworkCapOverrides.has(item.itemId) &&
        !fetchStarted.has(item.itemId)
    );
    if (toFetch.length === 0) return;
    for (const item of toFetch) fetchStarted.add(item.itemId);
    let cancelled = false;
    void (async () => {
      for (const item of toFetch) {
        const full = await getBacklogItem(item.itemId);
        if (cancelled) return;
        if (full === null) {
          // getBacklogItem swallows RPC errors (and "client not ready yet")
          // as null (see useBacklogService.ts's getBacklogItem) — caching
          // that as a resolved `undefined` would be indistinguishable from a
          // genuinely-unset override and StuckItemDetail would confidently
          // (and wrongly) render "No override set" after a transient
          // network failure. Leave it out of the map entirely so it's
          // absent from `reworkCapOverrides`, and release the started-marker
          // so a later effect invocation (e.g. the item being re-expanded)
          // retries instead of being stuck permanently unfetched.
          fetchStarted.delete(item.itemId);
          continue;
        }
        setReworkCapOverrides((prev) => {
          if (prev.has(item.itemId)) return prev;
          const next = new Map(prev);
          next.set(item.itemId, full.reworkCapOverride);
          reworkCapOverridesRef.current = next;
          return next;
        });
      }
    })();
    return () => {
      cancelled = true;
      // This invocation was interrupted (items/expandedKeys changed) before
      // finishing its whole batch — release the started-marker for anything
      // it didn't get to commit, so a later invocation can still fetch it
      // instead of leaving it stuck marked "in flight" forever.
      // reworkCapOverridesRef (not the possibly-stale `reworkCapOverrides`
      // closed over by this effect instance) reflects every commit made up
      // to this exact moment, including ones from this same batch.
      for (const item of toFetch) {
        if (!reworkCapOverridesRef.current.has(item.itemId)) {
          fetchStarted.delete(item.itemId);
        }
      }
    };
    // reworkCapOverrides is deliberately excluded; see the doc comment above this hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, expandedKeys, getBacklogItem]);

  return reworkCapOverrides;
}
