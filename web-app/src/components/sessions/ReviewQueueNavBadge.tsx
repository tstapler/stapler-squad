"use client";

import { useReviewQueue } from "@/lib/hooks/useReviewQueue";
import styles from "./ReviewQueueNavBadge.module.css";

interface ReviewQueueNavBadgeProps {
  inline?: boolean;
}

/**
 * Navigation badge that displays the count of items in the review queue.
 * Used in the header navigation to show queue status at a glance.
 */
export function ReviewQueueNavBadge({ inline = false }: ReviewQueueNavBadgeProps) {
  const { items, loading } = useReviewQueue({
    baseUrl: "http://localhost:8543",
    autoRefresh: true,
    refreshInterval: 5000,
  });

  const count = items.length;

  if (loading && count === 0) {
    return null; // Don't show badge while initially loading
  }

  if (count === 0) {
    return null; // Don't show badge when queue is empty
  }

  const className = inline
    ? `${styles.badge} ${styles.inline}`
    : styles.badge;

  return (
    <span
      className={className}
      aria-label={`${count} item${count !== 1 ? "s" : ""} in review queue`}
      title={`${count} session${count !== 1 ? "s" : ""} need attention`}
    >
      {count}
    </span>
  );
}
