"use client";
// +feature: backlog:duplicate-pending-badge

import * as styles from "./DuplicatePendingBadge.css";

interface DuplicatePendingBadgeProps {
  duplicateRef: string;
}

/** Marks an item in review whose report_duplicate claim awaits operator confirmation, so it reads differently from a real in-review PR. */
export function DuplicatePendingBadge({ duplicateRef }: DuplicatePendingBadgeProps) {
  return (
    <span
      className={styles.badge}
      data-testid="duplicate-pending-badge"
      title={duplicateRef ? `Claimed duplicate of ${duplicateRef} — awaiting confirmation` : "Duplicate claimed — awaiting confirmation"}
    >
      Duplicate? awaiting confirmation
    </span>
  );
}
