// +feature: backlog-stuck-items
"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useDiagnoseAction } from "@/lib/hooks/useDiagnoseAction";
import { getStuckReasonLabel } from "./stuckReason";
import { StuckItem } from "./StuckItem";
import { itemKey, isEscalationReason } from "./stuckItemsSectionShared";
import { useResolvedGhosts } from "./useResolvedGhosts";
import { useReworkCapOverrides } from "./useReworkCapOverrides";
import { useBulkResetParked } from "./useBulkResetParked";
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
  StuckReason.MULTIPLE_REASONS,
  StuckReason.BOUNCE_CAP_EXHAUSTED,
  StuckReason.PUSH_FAILED,
  StuckReason.SPAWN_FAILED,
  StuckReason.STEER_FAILED,
  StuckReason.PR_PENDING_NO_PR,
  StuckReason.REWORK_BLOCKED_STALE,
  StuckReason.PR_NEEDS_FIX,
  StuckReason.RESPAWN_BLOCKED_ACTIVE,
  StuckReason.LIKELY_FLAKY,
  StuckReason.BLOCKED_BY_DEPENDENCY,
  StuckReason.BLOCKED_BY_CLAIM,
  StuckReason.WORKTREE_INCONSISTENT,
  StuckReason.REPEATED_NOOP_DISPATCH,
  StuckReason.MERGED_PR_UNVERIFIED,
];

function firstDetectedMs(item: StuckBacklogItem): number {
  const ts = item.firstDetectedAt;
  if (!ts) return 0;
  return Number(ts.seconds) * 1000;
}

/**
 * "Stuck Backlog Items" section — grouped-by-reason, filterable list mounted
 * on /unfinished, directly below the existing filter-chip row and above
 * GitHubPRsSection (design/ux.md Surface 2).
 */
/** Mirrors StuckItem.tsx's MAX_REMEDIATION_ATTEMPTS — see that constant's doc comment. */
const MAX_REMEDIATION_ATTEMPTS = 5;

export interface StuckItemsSectionProps {
  /**
   * itemId from the `/unfinished?item=<itemId>` deep link (routes.unfinishedItem).
   * When set, pre-expands every card matching this itemId (a bare itemId can
   * match multiple composite keys — one item can appear under several
   * reasons) and, if the active reason filter would hide all of them,
   * resets the filter to "all" so the deep link always surfaces a match.
   */
  focusItemId?: string;
}

export function StuckItemsSection({ focusItemId }: StuckItemsSectionProps = {}) {
  const {
    items,
    isLoading,
    error,
    lastFetched,
    refetch,
    snooze,
    bulkResetParkedRemediation,
    triggerRemediationNow,
    overrideClaimBlock,
  } = useStuckBacklogItems();
  const { updateBacklogItem, transitionStatus, spawnSessionFromItem, approvePlan, getBacklogItem } =
    useBacklogService();
  const { dispatchDiagnose } = useDiagnoseAction();
  const [filter, setFilter] = useState<FilterValue>("all");
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const resolvedGhosts = useResolvedGhosts(items, expandedKeys, setExpandedKeys);
  // Populated lazily on expand for REWORK_CAP items only — StuckBacklogItem
  // (the list-fetch shape) doesn't carry reworkCapOverride, so the current
  // value has to be fetched via getBacklogItem (full BacklogItem) on demand.
  const reworkCapOverrides = useReworkCapOverrides(items, expandedKeys, getBacklogItem);
  const { bulkResetState, bulkResetMessage, resettingReason, anyResetPending, handleBulkResetParked } =
    useBulkResetParked(bulkResetParkedRemediation);
  const appliedFocusItemIdRef = useRef<string | undefined>(undefined);

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

  // /unfinished?item=<itemId> deep link (routes.unfinishedItem): pre-expand
  // every card matching this itemId (a bare itemId can match multiple
  // composite keys, since one item can appear under several reasons), and
  // fall back to the "all" filter if the currently active reason filter
  // would otherwise hide every matching card.
  //
  // Applied at most once per focusItemId (tracked via appliedFocusItemIdRef),
  // not on every `items` change: useStuckBacklogItems() hands back a fresh
  // array reference on every ~60s poll tick regardless of whether the data
  // actually changed, and focusItemId never clears itself from the URL. An
  // unguarded effect would re-run on each poll and silently re-expand a card
  // the user had just manually collapsed, or re-reset a filter they'd since
  // changed away from "all".
  useEffect(() => {
    if (!focusItemId || appliedFocusItemIdRef.current === focusItemId) return;
    const matches = items.filter((i) => i.itemId === focusItemId);
    if (matches.length === 0) return;
    appliedFocusItemIdRef.current = focusItemId;

    setExpandedKeys((prev) => {
      const next = new Set(prev);
      for (const item of matches) next.add(itemKey(item));
      return next;
    });

    setFilter((prevFilter) => {
      if (prevFilter === "all") return prevFilter;
      const stillVisible = matches.some((i) => i.reason === prevFilter);
      return stillVisible ? prevFilter : "all";
    });
  }, [focusItemId, items]);

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
  // reachable from here. Approve directly from the stuck-item card instead.
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
      // reintroduce the "looks approved but isn't" flicker (see
      // backlog/plan-approval-flicker).
      const updated = await approvePlan(itemId);
      if (!updated) throw new Error("Approve plan did not return an updated item.");
      await refetch();
    },
    [approvePlan, refetch]
  );

  // Diagnose & Nudge (backlog item 68964304): dispatches a diagnostic agent
  // scoped to this card's own StuckReason. Deliberately NOT
  // try/catch-swallowed, same rationale as handleApprovePlan above — the
  // card needs the specific dispatch failure, not a generic one. Does not
  // refetch(): dispatching a diagnostic session doesn't change this item's
  // stuck-state row by itself (a nudge, if the agent performs one, is what
  // eventually resolves the condition on a later poll tick). Returns the
  // dispatched session's UUID so StuckItemDetail can link straight to it —
  // see onDiagnose's doc comment (StuckItem.tsx) for why that link matters.
  const handleDiagnose = useCallback(
    async (itemId: string, reason: StuckReason): Promise<string> => {
      const resp = await dispatchDiagnose(itemId, reason);
      return resp.diagnosticSessionUuid;
    },
    [dispatchDiagnose]
  );

  // Visible items: the filtered set actually rendered. Cross-reference badges
  // are computed from this set so they auto-suppress once a filter narrows an
  // item to a single visible card.
  //
  // A row that just resolved (tracked in resolvedGhosts) has, by definition,
  // already disappeared from `items` — grouping/rendering purely off `items`
  // would make its ghost/confirmation card (justResolved banner) never
  // actually appear, since it wouldn't be in any group to render at all. Keep
  // it visible for its fade-out window by re-including it here (deduped
  // against `items`, and still respecting the active filter) until its ghost
  // timer clears it from resolvedGhosts.
  const visibleItems = useMemo(() => {
    const base = filter === "all" ? items : items.filter((i) => i.reason === filter);
    if (resolvedGhosts.size === 0) return base;
    const baseKeys = new Set(base.map(itemKey));
    const ghostOnlyItems = Array.from(resolvedGhosts.values())
      .map((g) => g.item)
      .filter((item) => !baseKeys.has(itemKey(item)) && (filter === "all" || item.reason === filter));
    return ghostOnlyItems.length > 0 ? [...base, ...ghostOnlyItems] : base;
  }, [items, filter, resolvedGhosts]);

  // Counts only non-escalation reasons per item — the two escalation reasons
  // are aggregate summaries of the other rows, not additional "other reasons"
  // in their own right (see isEscalationReason's doc comment).
  const itemIdCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of visibleItems) {
      if (isEscalationReason(item.reason)) continue;
      counts.set(item.itemId, (counts.get(item.itemId) ?? 0) + 1);
    }
    return counts;
  }, [visibleItems]);

  const otherReasonLabelsFor = useCallback(
    (item: StuckBacklogItem): string[] =>
      visibleItems
        .filter(
          (i) => i.itemId === item.itemId && i.reason !== item.reason && !isEscalationReason(i.reason)
        )
        .map((i) => getStuckReasonLabel(i.reason)),
    [visibleItems]
  );

  const grouped = useMemo(() => {
    const map = new Map<StuckReason, StuckBacklogItem[]>();
    for (const reason of GROUP_ORDER) map.set(reason, []);
    for (const item of visibleItems) {
      const list = map.get(item.reason);
      if (list) {
        list.push(item);
      } else {
        map.set(item.reason, [item]);
      }
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
  const parkedCountByReason = useMemo(() => {
    const counts = new Map<StuckReason, number>();
    for (const item of items) {
      if (item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS) {
        counts.set(item.reason, (counts.get(item.reason) ?? 0) + 1);
      }
    }
    return counts;
  }, [items]);

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
        {
          // analytics-exempt
          <button className={styles.retryBtn} onClick={refetch} data-testid="stuck-items-retry">
            Retry
          </button>
        }
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
        {
          // analytics-exempt
          <button
            className={styles.clearFilterBtn}
            onClick={handleClearFilter}
            data-testid="stuck-items-clear-filter"
          >
            Clear filter
          </button>
        }
      </div>
    );
  } else {
    body = (
      <>
        {GROUP_ORDER.filter((reason) => (grouped.get(reason)?.length ?? 0) > 0).map((reason) => {
          const groupItems = grouped.get(reason) ?? [];
          const parkedInGroup = parkedCountByReason.get(reason) ?? 0;
          return (
            <div className={styles.group} key={reason} data-testid={`stuck-group-${reason}`}>
              <div className={styles.groupHeadingRow}>
                <h3 className={styles.groupHeading}>
                  {getStuckReasonLabel(reason)} ({groupItems.length})
                </h3>
                {parkedInGroup > 0 && (
                  // analytics-exempt
                  <button
                    type="button"
                    className={styles.resetParkedReasonBtn}
                    onClick={() => handleBulkResetParked(reason)}
                    disabled={anyResetPending}
                    title={`Clear the automated-retry counters on every ${getStuckReasonLabel(reason)} item that has exhausted its 5 automated attempts, so they get a fresh shot`}
                    data-testid={`stuck-group-reset-parked-${reason}`}
                  >
                    {resettingReason === reason ? "Resetting…" : `Reset parked (${parkedInGroup})`}
                  </button>
                )}
              </div>
              <div className={styles.itemList}>
                {groupItems.map((item) => {
                  const key = itemKey(item);
                  // Escalation-reason cards (multiple_reasons/bounce_cap_exhausted) aren't
                  // themselves counted in itemIdCounts (see isEscalationReason), so they don't
                  // subtract 1 for "self" the way an ordinary reason card does.
                  const otherCount = isEscalationReason(item.reason)
                    ? itemIdCounts.get(item.itemId) ?? 0
                    : (itemIdCounts.get(item.itemId) ?? 1) - 1;
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
                      resolvedTrailingMessage={ghost?.trailingMessage}
                      onSnooze={snooze}
                      onReworkCapOverride={handleReworkCapOverride}
                      currentReworkCapOverride={reworkCapOverrides.get(item.itemId)}
                      reworkCapOverrideLoaded={reworkCapOverrides.has(item.itemId)}
                      onTriggerRemediationNow={triggerRemediationNow}
                      onOverrideClaimBlock={overrideClaimBlock}
                      onApprovePlan={handleApprovePlan}
                      onDiagnose={handleDiagnose}
                      focusItemId={focusItemId}
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
          // analytics-exempt
          <button
            type="button"
            className={styles.resetParkedBtn}
            onClick={() => handleBulkResetParked()}
            disabled={anyResetPending}
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
          {
            // analytics-exempt
            <button className={styles.retryBtn} onClick={refetch} data-testid="stuck-items-retry">
              Retry
            </button>
          }
        </div>
      )}

      {!showFirstLoadError && !showInitialLoading && totalCount > 0 && (
        <div className={styles.filterRow} role="group" aria-label="Filter stuck items by reason">
          {chips.map(({ value, label, count }) => (
            // analytics-exempt
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
