// +feature: backlog-stuck-items
"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { getStuckReasonLabel } from "./stuckReason";
import { StuckItem } from "./StuckItem";
import * as styles from "./StuckItemsSection.css";

type FilterValue = "all" | StuckReason;

// Fixed, deliberate display order — by typical actionability, NOT severity.
// pr_ready_unmerged leads (one known next step: merge it); the remaining
// reasons that need a decision or investigation follow. This must never be
// read as a danger/severity ranking (design/ux.md Surface 2).
const GROUP_ORDER: StuckReason[] = [
  StuckReason.PR_READY_UNMERGED,
  StuckReason.ABANDONED_REVIEW,
  StuckReason.STALE_WORK,
  StuckReason.ORPHANED_TRIAGE,
  StuckReason.REWORK_CAP,
  StuckReason.AUTONOMOUS_STUCK,
  StuckReason.BOUNCING,
  StuckReason.PUSH_FAILED,
];

function itemKey(item: Pick<StuckBacklogItem, "itemId" | "reason">): string {
  return `${item.itemId}::${item.reason}`;
}

function firstDetectedMs(item: StuckBacklogItem): number {
  const ts = item.firstDetectedAt;
  if (!ts) return 0;
  return Number(ts.seconds) * 1000;
}

interface ResolvedGhost {
  item: StuckBacklogItem;
  message: string;
}

/**
 * "Stuck Backlog Items" section — grouped-by-reason, filterable list mounted
 * on /unfinished, directly below the existing filter-chip row and above
 * GitHubPRsSection (design/ux.md Surface 2).
 */
export function StuckItemsSection() {
  const { items, isLoading, error, lastFetched, refetch, snooze } = useStuckBacklogItems();
  const { updateBacklogItem, transitionStatus, spawnSessionFromItem } = useBacklogService();
  const [filter, setFilter] = useState<FilterValue>("all");
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [resolvedGhosts, setResolvedGhosts] = useState<Map<string, ResolvedGhost>>(new Map());

  const prevItemsRef = useRef<StuckBacklogItem[]>([]);
  const ghostTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // Surface 12: an item that resolves while its card is expanded gets a brief
  // "was just resolved" confirmation instead of being yanked out immediately.
  useEffect(() => {
    const prevItems = prevItemsRef.current;
    const nextKeys = new Set(items.map(itemKey));
    const newlyMissingExpanded = prevItems.filter(
      (p) => expandedKeys.has(itemKey(p)) && !nextKeys.has(itemKey(p))
    );

    if (newlyMissingExpanded.length > 0) {
      setResolvedGhosts((prev) => {
        const next = new Map(prev);
        for (const item of newlyMissingExpanded) {
          const key = itemKey(item);
          if (next.has(key)) continue;
          const message =
            item.reason === StuckReason.PR_READY_UNMERGED && item.prNumber > 0
              ? `PR #${item.prNumber} was merged.`
              : "This item was just resolved.";
          next.set(key, { item, message });

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
          }, 2800);
          ghostTimersRef.current.set(key, timer);
        }
        return next;
      });
    }

    prevItemsRef.current = items;
  }, [items, expandedKeys]);

  useEffect(() => {
    const timers = ghostTimersRef.current;
    return () => {
      for (const t of timers.values()) clearTimeout(t);
    };
  }, []);

  const toggleExpand = useCallback((key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const handleClearFilter = useCallback(() => setFilter("all"), []);

  // rework_cap "continue automatically" action: sets the item's per-item
  // override then immediately reopens it — mirrors BacklogItemDetail.tsx's
  // handleGateReopen exactly (transition to in_progress, spawn a fresh work
  // session), so the item starts working again in the same click instead of
  // requiring a separate "Reopen for Revision" click elsewhere. On success,
  // future automatic rework/re-review rounds for this item use the raised
  // cap instead of the global default (see effectiveReworkCap on the backend).
  const handleReworkCapOverride = useCallback(
    async (itemId: string, override: number): Promise<boolean> => {
      try {
        const updated = await updateBacklogItem(itemId, { reworkCapOverride: override });
        if (!updated) return false;
        await transitionStatus(itemId, "in_progress");
        await spawnSessionFromItem(itemId);
        await refetch();
        return true;
      } catch (err) {
        console.error("[StuckItemsSection] reworkCapOverride reopen failed:", err);
        return false;
      }
    },
    [updateBacklogItem, transitionStatus, spawnSessionFromItem, refetch]
  );

  // Visible items: the filtered set actually rendered. Cross-reference badges
  // are computed from this set so they auto-suppress once a filter narrows an
  // item to a single visible card.
  const visibleItems = useMemo(
    () => (filter === "all" ? items : items.filter((i) => i.reason === filter)),
    [items, filter]
  );

  const itemIdCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of visibleItems) {
      counts.set(item.itemId, (counts.get(item.itemId) ?? 0) + 1);
    }
    return counts;
  }, [visibleItems]);

  const otherReasonLabelsFor = useCallback(
    (item: StuckBacklogItem): string[] =>
      visibleItems
        .filter((i) => i.itemId === item.itemId && i.reason !== item.reason)
        .map((i) => getStuckReasonLabel(i.reason)),
    [visibleItems]
  );

  const grouped = useMemo(() => {
    const map = new Map<StuckReason, StuckBacklogItem[]>();
    for (const reason of GROUP_ORDER) map.set(reason, []);
    for (const item of visibleItems) {
      if (!map.has(item.reason)) map.set(item.reason, []);
      map.get(item.reason)!.push(item);
    }
    for (const list of map.values()) {
      list.sort((a, b) => firstDetectedMs(a) - firstDetectedMs(b));
    }
    return map;
  }, [visibleItems]);

  const countsByReason = useMemo(() => {
    const counts = new Map<StuckReason, number>();
    for (const item of items) {
      counts.set(item.reason, (counts.get(item.reason) ?? 0) + 1);
    }
    return counts;
  }, [items]);

  const totalCount = items.length;
  const activeFilterLabel = filter === "all" ? null : getStuckReasonLabel(filter);
  const filteredCount = visibleItems.length;

  const chips: { value: FilterValue; label: string; count: number }[] = [
    { value: "all", label: "All", count: totalCount },
    ...GROUP_ORDER.map((reason) => ({
      value: reason,
      label: getStuckReasonLabel(reason),
      count: countsByReason.get(reason) ?? 0,
    })),
  ];

  const showFirstLoadError = error !== null && lastFetched === null;
  const showStaleBanner = error !== null && lastFetched !== null;
  const showInitialLoading = isLoading && lastFetched === null && !error;

  let body: React.ReactNode;
  if (showFirstLoadError) {
    body = (
      <div className={styles.errorBannerFullBody} data-testid="stuck-items-error-full">
        <span>⚠ Couldn&apos;t check for stuck items right now.</span>
        <button className={styles.retryBtn} onClick={refetch} data-testid="stuck-items-retry">
          Retry
        </button>
      </div>
    );
  } else if (showInitialLoading) {
    body = (
      <div className={styles.loading} data-testid="stuck-items-loading">
        <span className={styles.spinner} aria-label="Checking" />
        Checking for stuck items…
      </div>
    );
  } else if (totalCount === 0) {
    body = (
      <div className={styles.empty} data-testid="stuck-items-empty">
        ✓ Nothing stuck — all backlog items are progressing.
      </div>
    );
  } else if (filteredCount === 0) {
    body = (
      <div className={styles.filteredEmpty} data-testid="stuck-items-filtered-empty">
        <span>No stuck items match &quot;{activeFilterLabel}&quot;.</span>
        <button
          className={styles.clearFilterBtn}
          onClick={handleClearFilter}
          data-testid="stuck-items-clear-filter"
        >
          Clear filter
        </button>
      </div>
    );
  } else {
    body = (
      <>
        {GROUP_ORDER.filter((reason) => (grouped.get(reason)?.length ?? 0) > 0).map((reason) => {
          const groupItems = grouped.get(reason) ?? [];
          return (
            <div className={styles.group} key={reason} data-testid={`stuck-group-${reason}`}>
              <h3 className={styles.groupHeading}>
                {getStuckReasonLabel(reason)} ({groupItems.length})
              </h3>
              <div className={styles.itemList}>
                {groupItems.map((item) => {
                  const key = itemKey(item);
                  const otherCount = (itemIdCounts.get(item.itemId) ?? 1) - 1;
                  const ghost = resolvedGhosts.get(key);
                  return (
                    <StuckItem
                      key={key}
                      item={item}
                      isExpanded={expandedKeys.has(key)}
                      onToggleExpand={() => toggleExpand(key)}
                      otherReasonsCount={otherCount}
                      otherReasonLabels={otherReasonLabelsFor(item)}
                      justResolved={ghost !== undefined}
                      resolvedMessage={ghost?.message}
                      onSnooze={snooze}
                      onReworkCapOverride={handleReworkCapOverride}
                    />
                  );
                })}
              </div>
            </div>
          );
        })}
      </>
    );
  }

  return (
    <section className={styles.section} aria-label="Stuck Backlog Items" data-testid="stuck-items-section">
      <div className={styles.sectionHeader}>
        <h2 className={styles.sectionTitle}>Stuck Backlog Items</h2>
        <span className={styles.countRegion} aria-live="polite" data-testid="stuck-items-count">
          {totalCount} stuck
        </span>
      </div>

      {showStaleBanner && lastFetched && (
        <div className={styles.errorBanner} data-testid="stuck-items-stale-banner">
          <span>
            Couldn&apos;t refresh stuck items (last updated{" "}
            {Math.max(0, Math.floor((Date.now() - lastFetched.getTime()) / 60000))}m ago).
          </span>
          <button className={styles.retryBtn} onClick={refetch} data-testid="stuck-items-retry">
            Retry
          </button>
        </div>
      )}

      {!showFirstLoadError && !showInitialLoading && totalCount > 0 && (
        <div className={styles.filterRow} role="group" aria-label="Filter stuck items by reason">
          {chips.map(({ value, label, count }) => (
            <button
              key={String(value)}
              className={`${styles.chip} ${filter === value ? styles.chipActive : ""}`}
              onClick={() => setFilter(value)}
              aria-pressed={filter === value}
              data-testid={`stuck-filter-chip-${value}`}
            >
              {label} ({count})
            </button>
          ))}
        </div>
      )}

      {body}
    </section>
  );
}
