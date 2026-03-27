"use client";

import { useRouter } from "next/navigation";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useReviewQueueNotifications } from "@/lib/hooks/useReviewQueueNotifications";
import { NotificationSound } from "@/lib/utils/notifications";
import styles from "./ReviewQueueNavBadge.module.css";

interface ReviewQueueNavBadgeProps {
  inline?: boolean;
}

/**
 * Navigation badge that displays the count of items in the review queue.
 * Used in the header navigation to show queue status at a glance.
 */
export function ReviewQueueNavBadge({ inline = false }: ReviewQueueNavBadgeProps) {
  const router = useRouter();
  const { items } = useReviewQueueContext();

  // Play notification sound when new items are added to the queue
  useReviewQueueNotifications(items, {
    enabled: true,
    soundType: NotificationSound.DING,
    showBrowserNotification: true,
    showToastNotification: true,
    notificationTitle: "Session Needs Attention",
    onNavigateToSession: (sessionId) => {
      // Navigate directly to review queue with session pre-selected
      router.push(`/review-queue?session=${sessionId}`);
    },
  });

  const count = items.length;

  // Always show badge (even when count is 0) for test visibility
  // Badge will be styled to show 0 state
  const className = inline
    ? `${styles.badge} ${styles.inline} ${count === 0 ? styles.empty : ""}`
    : `${styles.badge} ${count === 0 ? styles.empty : ""}`;

  return (
    <span
      className={className}
      data-testid="review-queue-badge"
      aria-label={`${count} item${count !== 1 ? "s" : ""} in review queue`}
      title={`${count} session${count !== 1 ? "s" : ""} ${count > 0 ? "need attention" : "- queue empty"}`}
    >
      {count}
    </span>
  );
}
