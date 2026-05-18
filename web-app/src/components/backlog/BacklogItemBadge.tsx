"use client";
// +feature: backlog:item-badge

import type { BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./BacklogItemBadge.css";

interface BacklogItemBadgeProps {
  itemTitle: string;
  status: BacklogItemStatus;
  acTotal: number;
  acDone: number;
}

const STATUS_LABELS: Record<BacklogItemStatus, string> = {
  idea: "Idea",
  ready: "Ready",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
  archived: "Archived",
};

const STATUS_CLASS: Record<BacklogItemStatus, string> = {
  idea: styles.statusIdea,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

export function BacklogItemBadge({
  itemTitle,
  status,
  acTotal,
  acDone,
}: BacklogItemBadgeProps) {
  return (
    <span className={styles.badge} data-testid="backlog-item-badge">
      <span
        className={`${styles.statusChip} ${STATUS_CLASS[status]}`}
        aria-label={`Status: ${STATUS_LABELS[status]}`}
      >
        {STATUS_LABELS[status]}
      </span>
      {acTotal > 0 && (
        <span className={styles.acCount} aria-label={`${acDone} of ${acTotal} criteria done`}>
          {acDone}/{acTotal} ✓
        </span>
      )}
      <span className={styles.itemTitle} title={itemTitle}>
        {truncate(itemTitle, 40)}
      </span>
    </span>
  );
}
