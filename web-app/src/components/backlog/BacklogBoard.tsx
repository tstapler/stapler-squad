"use client";
// +feature: backlog:board

import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { BacklogItemCard } from "./BacklogItemCard";
import * as styles from "./BacklogBoard.css";

interface BacklogBoardProps {
  items: BacklogItem[];
  onAction: (action: string, itemId: string) => void;
  onItemClick: (itemId: string) => void;
  isLoading?: boolean;
  /** itemId -> action key currently in flight for that card. */
  pending?: Record<string, string>;
  /**
   * Resolved once (useStuckBacklogItems()) by the page-level caller and
   * distributed per card by itemId here — not re-fetched per card.
   */
  stuckItems?: StuckBacklogItem[];
}

const COLUMNS: { status: BacklogItemStatus; label: string }[] = [
  { status: "idea", label: "Idea" },
  { status: "ready", label: "Ready" },
  { status: "in_progress", label: "In Progress" },
  { status: "review", label: "Review" },
  { status: "done", label: "Done" },
];

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
  onAction,
  onItemClick,
  isLoading,
  pending,
  stuckItemsById,
}: {
  column: { status: BacklogItemStatus; label: string };
  items: BacklogItem[];
  onAction: (action: string, itemId: string) => void;
  onItemClick: (itemId: string) => void;
  isLoading: boolean;
  pending: Record<string, string>;
  stuckItemsById: Map<string, StuckBacklogItem>;
}) {
  return (
    <section
      className={styles.column}
      aria-label={`${column.label} column`}
      data-testid={`backlog-column-${column.status}`}
    >
      <div className={styles.columnHeader}>
        <h3 className={styles.columnTitle}>{column.label}</h3>
        <span className={styles.columnCount} aria-label={`${items.length} items`}>
          {items.length}
        </span>
      </div>

      <div className={styles.columnCards} role="list" aria-label={`${column.label} items`}>
        {isLoading ? (
          <>
            <SkeletonCard />
            <SkeletonCard />
          </>
        ) : items.length === 0 ? (
          <p className={styles.emptyColumn}>No items</p>
        ) : (
          items.map((item) => (
            <div key={item.id} role="listitem">
              <BacklogItemCard
                item={item}
                onAction={onAction}
                onClick={onItemClick}
                pendingAction={pending[item.id] ?? null}
                stuckItem={stuckItemsById.get(item.id)}
              />
            </div>
          ))
        )}
      </div>
    </section>
  );
}

export function BacklogBoard({
  items,
  onAction,
  onItemClick,
  isLoading = false,
  pending = {},
  stuckItems = [],
}: BacklogBoardProps) {
  const stuckItemsById = new Map(stuckItems.map((s) => [s.itemId, s]));

  return (
    <div
      className={styles.board}
      role="region"
      aria-label="Backlog board"
      data-testid="backlog-board"
    >
      {COLUMNS.map((column) => (
        <BoardColumn
          key={column.status}
          column={column}
          items={items.filter((i) => i.status === column.status)}
          onAction={onAction}
          onItemClick={onItemClick}
          isLoading={isLoading}
          pending={pending}
          stuckItemsById={stuckItemsById}
        />
      ))}
    </div>
  );
}
