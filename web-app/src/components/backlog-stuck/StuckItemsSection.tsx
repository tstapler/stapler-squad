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
//
// IMPORTANT: this list must be kept in sync with every StuckReason value
// (stuckReason.ts's STUCK_REASON_LABELS/ICONS/CLASS maps are `Record<StuckReason,
// T>` and so are compile-checked exhaustive, but this array is not — a
// reason present in `grouped` (below) yet absent here is silently never
// rendered even though it still counts toward the total/badge, which is
// exactly the kind of count-vs-list mismatch this feature exists to avoid).
// plan_not_approved, spawn_failed, pr_pending_no_pr, rework_blocked_stale,
// pr_needs_fix, and respawn_blocked_active were all previously missing here
// (backlog/plan-approval-flicker fix, 2026-08) — found via the e2e test for
// the plan-approval flicker fix never being able to find a seeded
// plan_not_approved item's card despite the section's own count showing 1.
const GROUP_ORDER: StuckReason[] = [
  StuckReason.PR_READY_UNMERGED,
  StuckReason.PLAN_NOT_APPROVED,
  StuckReason.ABANDONED_REVIEW,
  StuckReason.STALE_WORK,
  StuckReason.ORPHANED_TRIAGE,
  StuckReason.REWORK_CAP,
  StuckReason.AUTONOMOUS_STUCK,
  StuckReason.BOUNCING,
  StuckReason.PUSH_FAILED,
  StuckReason.SPAWN_FAILED,
  StuckReason.PR_PENDING_NO_PR,
  StuckReason.REWORK_BLOCKED_STALE,
  StuckReason.PR_NEEDS_FIX,
  StuckReason.RESPAWN_BLOCKED_ACTIVE,
  StuckReason.LIKELY_FLAKY,
  StuckReason.BLOCKED_BY_DEPENDENCY,
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
/** Mirrors StuckItem.tsx's MAX_REMEDIATION_ATTEMPTS — see that constant's doc comment. */
const MAX_REMEDIATION_ATTEMPTS = 5;

export function StuckItemsSection() {
  const {
    items,
    isLoading,
    error,
    lastFetched,
    refetch,
    snooze,
    bulkResetParkedRemediation,
    triggerRemediationNow,
  } = useStuckBacklogItems();
  const { updateBacklogItem, transitionStatus, spawnSessionFromItem, approvePlan } = useBacklogService();
  const [filter, setFilter] = useState<FilterValue>("all");
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [resolvedGhosts, setResolvedGhosts] = useState<Map<string, ResolvedGhost>>(new Map());
  const [bulkResetState, setBulkResetState] = useState<"idle" | "pending" | "error">("idle");
  const [bulkResetMessage, setBulkResetMessage] = useState<string | null>(null);

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

  // BUG-038 follow-up: the only "Approve Plan" UI action lived inside the
  // item-detail page's `status === "ready"` block, but items this reason
  // flags are stuck in `status === "queued"` — so that button was never
  // reachable. This is the fix: approve directly from the stuck-item card.
  //
  // Deliberately NOT try/catch-swallowed (unlike the other handlers in this
  // file): useBacklogService's approvePlan rethrows the backend's
  // FailedPrecondition message verbatim (e.g. "no plan artifacts found — run
  // TriggerTriage first"), and StuckItemDetail needs that specific message
  // rather than a generic failure, in case the hasPlan gate is ever stale.
  const handleApprovePlan = useCallback(
    async (itemId: string): Promise<void> => {
      // approvePlan resolves null (without throwing) if the RPC client isn't
      // ready yet — must not let that silently read as success, which would
      // reintroduce the exact "looks approved but isn't" flicker this PR
      // fixes elsewhere.
      const updated = await approvePlan(itemId);
      if (!updated) throw new Error("Approve plan did not return an updated item.");
      await refetch();
    },
    [approvePlan, refetch]
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
  const parkedCount = useMemo(
    () => items.filter((i) => i.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS).length,
    [items]
  );

  const handleBulkResetParked = useCallback(async () => {
    setBulkResetState("pending");
    setBulkResetMessage(null);
    try {
      const n = await bulkResetParkedRemediation();
      setBulkResetState("idle");
      setBulkResetMessage(n > 0 ? `Reset ${n} parked item${n !== 1 ? "s" : ""}.` : "No parked items to reset.");
    } catch (err) {
      setBulkResetState("error");
      setBulkResetMessage(err instanceof Error ? err.message : "Bulk reset failed");
    }
  }, [bulkResetParkedRemediation]);

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
                      onTriggerRemediationNow={triggerRemediationNow}
                      onApprovePlan={handleApprovePlan}
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
        {parkedCount > 0 && (
          <button
            type="button"
            className={styles.resetParkedBtn}
            onClick={handleBulkResetParked}
            disabled={bulkResetState === "pending"}
            title="Clear the automated-retry counters on every item that has exhausted its 5 automated attempts, so they get a fresh shot"
            data-testid="stuck-items-reset-parked"
          >
            {bulkResetState === "pending" ? "Resetting…" : `Reset all parked (${parkedCount})`}
          </button>
        )}
      </div>
      {bulkResetMessage && (
        <div
          className={bulkResetState === "error" ? styles.resetParkedMessageError : styles.resetParkedMessage}
          aria-live="polite"
          data-testid="stuck-items-reset-parked-message"
        >
          {bulkResetMessage}
        </div>
      )}

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
