"use client";
// +feature: backlog:item-badge

import { getStatusLabel } from "@/lib/backlog/status";
import * as styles from "./BacklogItemBadge.css";

interface BacklogItemBadgeProps {
  itemTitle: string;
  status: string;
  acTotal: number;
  acDone: number;
}

const STATUS_CLASS: Record<string, string> = {
  idea: styles.statusIdea,
  refining: styles.statusRefining,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const getStatusClass = (s: string): string => STATUS_CLASS[s] ?? styles.statusArchived;

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

export function BacklogItemBadge({
  itemTitle,
  status,
  acTotal,
  acDone,
}: BacklogItemBadgeProps) {
  // Story 5.1.2 decision (plan.md Task 5.1.2a): DEFERRED — no compact
  // BlockerChip added here. This badge's container is capped at 260px
  // (BacklogItemBadge.css.ts's `badge` style), single-line (`whiteSpace:
  // "nowrap"`, `overflow: "hidden"`), and already renders 3 inline elements
  // (status chip, AC count, title truncated to 40 chars). A 4th element —
  // even the compact icon+label-only BlockerChip variant, whose longest
  // label ("Autonomous mode stopped without finishing") is ~40 chars — would
  // either force wrapping (breaking this badge's intentional single-line
  // design) or truncate the title further, with no free width budget to
  // absorb it. Per design/ux.md Surface 7's explicit fallback for the
  // deferred branch: the stuck reason is not hidden entirely — it's one
  // click away via the detail panel's full-variant BlockerChip
  // (LifecycleSummary), and /unfinished remains the primary stuck-item
  // triage surface, not this list badge.
  return (
    <span className={styles.badge} data-testid="backlog-item-badge">
      <span
        className={`${styles.statusChip} ${getStatusClass(status)}`}
        aria-label={`Status: ${getStatusLabel(status)}`}
      >
        {getStatusLabel(status)}
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
