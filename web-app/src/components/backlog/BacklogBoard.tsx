"use client";
// +feature: backlog:board

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { filterBacklogItems, type BacklogFilterState } from "@/lib/hooks/useBacklogFilters";
import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { BacklogItemCard } from "./BacklogItemCard";
import { ConnectionIndicator } from "./ConnectionIndicator";
import { deriveStageDisplay, type Stage } from "./detail/StageTracker";
import * as styles from "./BacklogBoard.css";

// The board's 5 columns are exactly StageTracker's 5 stages — reuse the same
// pure status->stage mapping the item-detail pipeline stepper already uses,
// so a "queued"/"pr_pending"/"refining" item folds into its mapped column
// (In Progress / Review / Idea respectively) instead of silently matching no
// column at all (BUG-037: those items rendered nowhere on the board — no
// error, no count, no way to tell they existed short of opening item
// detail). Archived items are excluded from the board, same as before this
// fix (archived never matched any of the 5 literal statuses either). This is
// also the defined behavior for the shared `showArchived` filter (AC 7):
// toggling it changes what filterBacklogItems() lets through, but an
// archived item still resolves to no Stage here, so it never renders in any
// column regardless of the toggle — there is no archived column on the
// board, by design, not by oversight.
function stageOf(status: BacklogItemStatus): Stage | null {
  const { activeStage, archived } = deriveStageDisplay(status);
  return archived ? null : activeStage;
}

interface BacklogBoardProps {
  onAction: (action: string, itemId: string) => void;
  onItemClick: (itemId: string) => void;
  /** itemId -> action key currently in flight for that card. */
  pending?: Record<string, string>;
  /**
   * Resolved once (useStuckBacklogItems()) by the page-level caller and
   * distributed per card by itemId here — not re-fetched per card.
   */
  stuckItems?: StuckBacklogItem[];
  /**
   * Shared filter state from useBacklogFilters() (AC 0, 1, 2). Applied only
   * to each column's settled items — the raw useWatchBacklogItems() stream
   * stays unfiltered so the Epic 6.4 exit/enter animation tracking keeps
   * seeing every status transition. Omitted in existing tests to preserve
   * prior unfiltered behavior.
   */
  filters?: BacklogFilterState;
}

const COLUMNS: { status: BacklogItemStatus; label: string }[] = [
  { status: "idea", label: "Idea" },
  { status: "ready", label: "Ready" },
  { status: "in_progress", label: "In Progress" },
  { status: "review", label: "Review" },
  { status: "done", label: "Done" },
];

// Epic 6.4 (backlog-event-driven-updates): how long a card's exit fade plays
// in its origin column before it's removed from the DOM, and how long the
// "just changed" flash is forced on for a card that just entered a new
// column (ux.md §7 — "~200ms" fade, paired with the flash on entry). Under
// `prefers-reduced-motion: reduce` both collapse to 0ms at the call site.
const EXIT_TRANSITION_MS = 200;
const ENTER_FLASH_MS = 250;

function SkeletonCard() {
  return (
    <div className={styles.skeletonCard} aria-hidden="true">
      <div className={styles.skeletonLine} />
      <div className={`${styles.skeletonLine} ${styles.skeletonLineShort}`} />
    </div>
  );
}

function BoardColumn({
  column,
  items,
  exitingIds,
  enteringIds,
  onAction,
  onItemClick,
  isLoading,
  pending,
  stuckItemsById,
  isEmptyDueToFilter,
}: {
  column: { status: BacklogItemStatus; label: string };
  items: BacklogItem[];
  /** ids in `items` that are fading out of this column (Epic 6.4). */
  exitingIds: Set<string>;
  /** ids in `items` that just entered this column and should force-flash. */
  enteringIds: Set<string>;
  onAction: (action: string, itemId: string) => void;
  onItemClick: (itemId: string) => void;
  isLoading: boolean;
  pending: Record<string, string>;
  stuckItemsById: Map<string, StuckBacklogItem>;
  /** True when this column has items upstream but the active filter excluded all of them (AC 5). */
  isEmptyDueToFilter: boolean;
}) {
  // The column count badge should reflect genuinely present items, not a
  // still-fading departure that's only rendered for the exit transition.
  const settledCount = items.filter((item) => !exitingIds.has(item.id)).length;

  return (
    <section
      className={styles.column}
      aria-label={`${column.label} column`}
      data-testid={`backlog-column-${column.status}`}
    >
      <div className={styles.columnHeader}>
        <h3 className={styles.columnTitle}>{column.label}</h3>
        <span className={styles.columnCount} aria-label={`${settledCount} items`}>
          {settledCount}
        </span>
      </div>

      <div className={styles.columnCards} role="list" aria-label={`${column.label} items`}>
        {isLoading ? (
          <>
            <SkeletonCard />
            <SkeletonCard />
          </>
        ) : items.length === 0 && isEmptyDueToFilter ? (
          <p className={styles.emptyColumn} data-testid="backlog-column-empty-filtered">
            No items match filter
          </p>
        ) : items.length === 0 ? (
          <p className={styles.emptyColumn}>No items</p>
        ) : (
          items.map((item) => {
            const isExiting = exitingIds.has(item.id);
            const isEntering = !isExiting && enteringIds.has(item.id);
            return (
              <div
                key={item.id}
                role="listitem"
                className={isExiting ? styles.cardExiting : undefined}
                aria-hidden={isExiting || undefined}
                data-testid={isExiting ? "backlog-card-exiting" : undefined}
              >
                <BacklogItemCard
                  item={item}
                  onAction={onAction}
                  onClick={onItemClick}
                  pendingAction={pending[item.id] ?? null}
                  forceJustChanged={isEntering}
                  stuckItem={stuckItemsById.get(item.id)}
                />
              </div>
            );
          })
        )}
      </div>
    </section>
  );
}

export function BacklogBoard({
  onAction,
  onItemClick,
  pending = {},
  stuckItems = [],
  filters,
}: BacklogBoardProps) {
  // Epic 5.2 (backlog-event-driven-updates): the board subscribes to the
  // same live stream/normalized store as the list page (ux.md §2, "no
  // board-specific fetch") rather than receiving items as props — a status-
  // change event moves an item's column membership purely by this filter
  // re-evaluating on the updated item, no board-specific refetch involved.
  const { items, connectionState } = useWatchBacklogItems();
  // Only show the skeleton on a genuinely empty first paint — a disconnect/
  // reconnect must keep showing last-known state, not blank/spinner-out
  // (ux.md §1 "Error / edge cases", shared by this surface per §2).
  const isLoading = connectionState === "connecting" && items.length === 0;

  // Epic 6.4 (backlog-event-driven-updates): when a genuine live status
  // change (gated on `item.liveVersion` advancing, same signal as
  // BacklogItemCard's own flash and the list view's Epic 6.3 exit fade)
  // moves an item from one board column to another, briefly keep rendering
  // it in its origin column with a fade-out ("exiting") while the freshly
  // mounted card in its destination column force-flashes ("entering") — the
  // same event driving one continuous "moved from X to Y" (ux.md §7,
  // UX AC #8), not two independent, uncorrelated animations. A bulk
  // resnapshot on reconnect never advances `liveVersion` for its items (see
  // backlogItemsSlice.ts), so it falls straight through to an ordinary,
  // un-animated re-render — matching the list view's Epic 6.3 guard.
  const [exitingItems, setExitingItems] = useState<
    Map<string, { item: BacklogItem; fromStatus: BacklogItemStatus }>
  >(new Map());
  const [enteringIds, setEnteringIds] = useState<Set<string>>(new Set());
  const exitingMapRef = useRef<Map<string, { item: BacklogItem; fromStatus: BacklogItemStatus }>>(
    new Map()
  );
  const enteringSetRef = useRef<Set<string>>(new Set());
  const prevStatusRef = useRef<Map<string, BacklogItemStatus>>(new Map());
  const prevLiveVersionRef = useRef<Map<string, number | undefined>>(new Map());
  const exitTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const enterTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const reducedMotionRef = useRef(false);

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    reducedMotionRef.current = mq.matches;
    const onChange = () => {
      reducedMotionRef.current = mq.matches;
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  // useLayoutEffect (not useEffect): runs before the browser paints, so a
  // departing card is re-added to the exiting map within the same commit it
  // was excluded from its old column — no visible blank frame in between.
  useLayoutEffect(() => {
    const exitingMap = exitingMapRef.current;
    const enteringSet = enteringSetRef.current;
    let exitingChanged = false;
    let enteringChanged = false;

    // Flap protection first pass: if an item with a pending exit has
    // re-matched the column it was fading out of (before that fade
    // completed), cancel the exit rather than let it finish unmounting and
    // immediately remount (ux.md §7 "Error / edge cases"). This settling
    // move itself must not then be treated as a *new* transition to
    // animate (pass 2 below skips these ids) — otherwise a fast
    // review -> in_progress -> review flap would fade a card out of
    // "review" (cancelled here) only to immediately start a fresh
    // exit/enter cycle for the reverse hop, still flickering.
    const flappedIds = new Set<string>();
    for (const item of items) {
      const pendingExit = exitingMap.get(item.id);
      if (pendingExit && stageOf(pendingExit.fromStatus) === stageOf(item.status)) {
        const timer = exitTimersRef.current.get(item.id);
        if (timer) clearTimeout(timer);
        exitTimersRef.current.delete(item.id);
        exitingMap.delete(item.id);
        exitingChanged = true;
        flappedIds.add(item.id);

        const enterTimer = enterTimersRef.current.get(item.id);
        if (enterTimer) clearTimeout(enterTimer);
        enterTimersRef.current.delete(item.id);
        if (enteringSet.delete(item.id)) enteringChanged = true;
      }
    }

    for (const item of items) {
      const prevStatus = prevStatusRef.current.get(item.id);
      const prevVersion = prevLiveVersionRef.current.get(item.id);
      const isGenuineLiveChange =
        item.liveVersion !== undefined && item.liveVersion !== prevVersion;

      if (
        !flappedIds.has(item.id) &&
        isGenuineLiveChange &&
        prevStatus !== undefined &&
        stageOf(prevStatus) !== stageOf(item.status)
      ) {
        if (stageOf(prevStatus) !== null && !exitingMap.has(item.id)) {
          exitingMap.set(item.id, { item: { ...item, status: prevStatus }, fromStatus: prevStatus });
          exitingChanged = true;
          const duration = reducedMotionRef.current ? 0 : EXIT_TRANSITION_MS;
          const timer = setTimeout(() => {
            if (exitingMapRef.current.delete(item.id)) {
              setExitingItems(new Map(exitingMapRef.current));
            }
            exitTimersRef.current.delete(item.id);
          }, duration);
          exitTimersRef.current.set(item.id, timer);
        }

        if (stageOf(item.status) !== null) {
          enteringSet.add(item.id);
          enteringChanged = true;
          const existingTimer = enterTimersRef.current.get(item.id);
          if (existingTimer) clearTimeout(existingTimer);
          const duration = reducedMotionRef.current ? 0 : ENTER_FLASH_MS;
          const timer = setTimeout(() => {
            if (enteringSetRef.current.delete(item.id)) {
              setEnteringIds(new Set(enteringSetRef.current));
            }
            enterTimersRef.current.delete(item.id);
          }, duration);
          enterTimersRef.current.set(item.id, timer);
        }
      }

      prevStatusRef.current.set(item.id, item.status);
      prevLiveVersionRef.current.set(item.id, item.liveVersion);
    }

    if (exitingChanged) setExitingItems(new Map(exitingMap));
    if (enteringChanged) setEnteringIds(new Set(enteringSet));
  }, [items]);

  // Clear any in-flight timers on unmount.
  useEffect(() => {
    return () => {
      for (const timer of exitTimersRef.current.values()) clearTimeout(timer);
      for (const timer of enterTimersRef.current.values()) clearTimeout(timer);
    };
  }, []);

  const stuckItemsById = new Map(stuckItems.map((s) => [s.itemId, s]));

  return (
    <div className={styles.boardWrapper}>
      {/* Task 6.2.1c: one ConnectionIndicator per board, not per column
          (ux.md §2 "Interaction flow" #5, UX AC #9). */}
      <div className={styles.boardToolbar}>
        <ConnectionIndicator connectionState={connectionState} />
      </div>
      <div
        className={styles.board}
        role="region"
        aria-label="Backlog board"
        data-testid="backlog-board"
      >
        {COLUMNS.map((column) => {
          // Filtering (AC 0, 1, 2) applies only to this column's settled
          // items — `items` itself stays unfiltered so the exit/enter
          // animation tracking above keeps seeing every status transition
          // regardless of the active filter. Sort/group-by are list-only
          // (AC 6) and never reach the board.
          const unfilteredBaseItems = items.filter((i) => stageOf(i.status) === column.status);
          const baseItems = filters ? filterBacklogItems(unfilteredBaseItems, filters) : unfilteredBaseItems;
          const isEmptyDueToFilter = baseItems.length === 0 && unfilteredBaseItems.length > 0;
          const baseIds = new Set(baseItems.map((i) => i.id));
          const exitingForColumn = Array.from(exitingItems.values()).filter(
            (e) => stageOf(e.fromStatus) === column.status && !baseIds.has(e.item.id)
          );
          const displayItems =
            exitingForColumn.length === 0
              ? baseItems
              : [...baseItems, ...exitingForColumn.map((e) => e.item)];
          const exitingIdsForColumn = new Set(exitingForColumn.map((e) => e.item.id));

          return (
            <BoardColumn
              key={column.status}
              column={column}
              items={displayItems}
              exitingIds={exitingIdsForColumn}
              enteringIds={enteringIds}
              onAction={onAction}
              onItemClick={onItemClick}
              isLoading={isLoading}
              pending={pending}
              stuckItemsById={stuckItemsById}
              isEmptyDueToFilter={isEmptyDueToFilter}
            />
          );
        })}
      </div>
    </div>
  );
}
