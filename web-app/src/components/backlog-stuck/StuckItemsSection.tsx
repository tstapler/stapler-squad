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
  StuckReason.MULTIPLE_REASONS,
  StuckReason.BOUNCE_CAP_EXHAUSTED,
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

// multiple_reasons / bounce_cap_exhausted are synthetic aggregate rows over an
// item's *other* stuck reasons, not independent reasons themselves — they
// must be excluded from "other reasons" counting/labeling, mirroring the
// backend's own self-exclusion in reconcileMultiReasonEscalation. Without
// this, an item with 2 real reasons plus its own multiple_reasons escalation
// row would show "+2 other reasons" instead of "+1" (plan.md Task 2.1.1c).
function isEscalationReason(reason: StuckReason): boolean {
  return reason === StuckReason.MULTIPLE_REASONS || reason === StuckReason.BOUNCE_CAP_EXHAUSTED;
}

function firstDetectedMs(item: StuckBacklogItem): number {
  const ts = item.firstDetectedAt;
  if (!ts) return 0;
  return Number(ts.seconds) * 1000;
}

interface ResolvedGhost {
  item: StuckBacklogItem;
  message: string;
  /** Overrides StuckItem's default "It will be removed from this list shortly." trailing copy — used for de-escalation, where only this card (not the whole item) is going away. */
  trailingMessage?: string;
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
  } = useStuckBacklogItems();
  const { updateBacklogItem, transitionStatus, spawnSessionFromItem, approvePlan, getBacklogItem } =
    useBacklogService();
  const [filter, setFilter] = useState<FilterValue>("all");
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [resolvedGhosts, setResolvedGhosts] = useState<Map<string, ResolvedGhost>>(new Map());
  // Populated lazily on expand for REWORK_CAP items only — StuckBacklogItem
  // (the list-fetch shape) doesn't carry reworkCapOverride, so the current
  // value has to be fetched via getBacklogItem (full BacklogItem) on demand.
  const [reworkCapOverrides, setReworkCapOverrides] = useState<Map<string, number | undefined>>(new Map());
  const appliedFocusItemIdRef = useRef<string | undefined>(undefined);
  // Mirrors reworkCapOverrides synchronously so the fetch-on-expand effect's
  // cleanup (below) can check "was this item actually committed?" without
  // depending on a stale closure over the state value — the effect
  // deliberately excludes reworkCapOverrides from its dependency array (see
  // that effect's comment), so its own closure only ever sees the map as of
  // when the effect instance started, not later commits made mid-batch.
  const reworkCapOverridesRef = useRef<Map<string, number | undefined>>(new Map());
  const [bulkResetState, setBulkResetState] = useState<"idle" | "pending" | "error">("idle");
  const [bulkResetMessage, setBulkResetMessage] = useState<string | null>(null);
  // Which single reason's "Reset parked (N)" button is mid-flight — distinct
  // from bulkResetState (the global "Reset all parked" button's own pending
  // flag) so clicking one doesn't visually disable the other.
  const [resettingReason, setResettingReason] = useState<StuckReason | null>(null);

  const prevItemsRef = useRef<StuckBacklogItem[]>([]);
  const ghostTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  // Tracks itemIds already fetched-or-in-flight for the reworkCapOverrides
  // fetch-on-expand effect below. Deliberately NOT derived from
  // reworkCapOverrides state — see that effect's comment for why.
  const reworkCapFetchStartedRef = useRef<Set<string>>(new Set());

  // Surface 12: an item that resolves while its card is expanded gets a brief
  // "was just resolved" confirmation instead of being yanked out immediately.
  //
  // Task 2.1.4a: `itemKey` is per-(itemId, reason), so a `multiple_reasons`
  // row de-escalating (resolving while the item itself remains open under
  // other reasons) is *already* caught by this same "row disappeared while
  // expanded" comparison below — each reason is its own list entry. The only
  // extension needed is distinguishing that case (item still present under
  // another reason) from true full-item resolution, so the copy doesn't
  // falsely claim the whole item is going away.
  useEffect(() => {
    const prevItems = prevItemsRef.current;
    const nextKeys = new Set(items.map(itemKey));
    const nextItemIds = new Set(items.map((i) => i.itemId));
    const newlyMissingExpanded = prevItems.filter(
      (p) => expandedKeys.has(itemKey(p)) && !nextKeys.has(itemKey(p))
    );

    if (newlyMissingExpanded.length > 0) {
      setResolvedGhosts((prev) => {
        const next = new Map(prev);
        for (const item of newlyMissingExpanded) {
          const key = itemKey(item);
          if (next.has(key)) continue;

          // Only MULTIPLE_REASONS gets de-escalation copy, not
          // BOUNCE_CAP_EXHAUSTED — intentional, not an oversight:
          // bounce_cap_exhausted can only ever coexist with an open bouncing
          // row (backend invariant, see reconcileBouncingItems' paired
          // resolve), so it never resolves independently while the item
          // still has open non-escalation reasons. If that invariant ever
          // changes, this condition needs the same OR as isEscalationReason.
          const isDeescalation =
            item.reason === StuckReason.MULTIPLE_REASONS && nextItemIds.has(item.itemId);

          let message: string;
          let trailingMessage: string | undefined;
          if (isDeescalation) {
            const remainingReasons = items.filter(
              (i) => i.itemId === item.itemId && !isEscalationReason(i.reason)
            ).length;
            message = `No longer critical — down to ${remainingReasons} open reason${
              remainingReasons !== 1 ? "s" : ""
            }.`;
            trailingMessage =
              "This card will be removed shortly; the item itself is still open elsewhere in the list.";
          } else {
            message =
              item.reason === StuckReason.PR_READY_UNMERGED && item.prNumber > 0
                ? `PR #${item.prNumber} was merged.`
                : "This item was just resolved.";
          }
          next.set(key, { item, message, trailingMessage });

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

  // Fetches the current reworkCapOverride for newly-expanded REWORK_CAP items
  // so StuckItemDetail can show it instead of guessing blind. Skips items
  // already present in the map (including a resolved `undefined`) so
  // re-expanding doesn't re-fetch.
  //
  // reworkCapOverrides is deliberately NOT a dependency here even though it's
  // read below (via reworkCapFetchStartedRef.current instead) — it used to
  // be, but every successful per-item fetch changed the map reference and
  // re-fired this effect while the previous invocation's for-loop was still
  // mid-flight on later items, causing O(N^2) duplicate getBacklogItem calls
  // when several REWORK_CAP items were expanded at once. reworkCapFetchStartedRef
  // (a ref, so mutating it can't trigger a re-render/re-run) tracks "already
  // fetched or in-flight" instead, updated synchronously before the async
  // fetch starts so an overlapping effect invocation (from items/expandedKeys
  // changing mid-batch) can't double-fetch the same item either.
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
    // reworkCapOverrides is deliberately excluded; see the doc comment above this effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, expandedKeys, getBacklogItem]);

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
  const parkedCountByReason = useMemo(() => {
    const counts = new Map<StuckReason, number>();
    for (const item of items) {
      if (item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS) {
        counts.set(item.reason, (counts.get(item.reason) ?? 0) + 1);
      }
    }
    return counts;
  }, [items]);

  // Shared by both the global "Reset all parked" button and each
  // per-reason-group "Reset parked (N)" button — reason omitted resets
  // across every reason (unchanged existing behavior), reason set scopes
  // the reset (and the resulting message) to just that bucket.
  const handleBulkResetParked = useCallback(
    async (reason?: StuckReason) => {
      if (reason !== undefined) {
        setResettingReason(reason);
      } else {
        setBulkResetState("pending");
      }
      setBulkResetMessage(null);
      try {
        const n = await bulkResetParkedRemediation(reason);
        setBulkResetState("idle");
        const scope = reason !== undefined ? ` in ${getStuckReasonLabel(reason)}` : "";
        setBulkResetMessage(
          n > 0 ? `Reset ${n} parked item${n !== 1 ? "s" : ""}${scope}.` : "No parked items to reset."
        );
      } catch (err) {
        setBulkResetState("error");
        setBulkResetMessage(err instanceof Error ? err.message : "Bulk reset failed");
      } finally {
        setResettingReason(null);
      }
    },
    [bulkResetParkedRemediation]
  );

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
          const parkedInGroup = parkedCountByReason.get(reason) ?? 0;
          return (
            <div className={styles.group} key={reason} data-testid={`stuck-group-${reason}`}>
              <div className={styles.groupHeadingRow}>
                <h3 className={styles.groupHeading}>
                  {getStuckReasonLabel(reason)} ({groupItems.length})
                </h3>
                {parkedInGroup > 0 && (
                  <button
                    type="button"
                    className={styles.resetParkedReasonBtn}
                    onClick={() => handleBulkResetParked(reason)}
                    disabled={resettingReason === reason}
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
                      onApprovePlan={handleApprovePlan}
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
          <button
            type="button"
            className={styles.resetParkedBtn}
            onClick={() => handleBulkResetParked()}
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
