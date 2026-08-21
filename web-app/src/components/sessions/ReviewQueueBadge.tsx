"use client";

import { Priority, AttentionReason } from "@/gen/session/v1/types_pb";
import { StatusBadge, getAttentionReasonInfo } from "./StatusBadge";
import * as styles from "./ReviewQueueBadge.css";

interface ReviewQueueBadgeProps {
  priority: Priority;
  reason: AttentionReason;
  compact?: boolean;
}

/**
 * Badge component that displays priority and attention reason for review queue items.
 *
 * Shows visual indicators using emojis and colors to communicate urgency and context.
 */
export function ReviewQueueBadge({
  priority,
  reason,
  compact = false,
}: ReviewQueueBadgeProps) {
  const getPriorityEmoji = (p: Priority): string => {
    switch (p) {
      case Priority.URGENT:
        return "🔴";
      case Priority.HIGH:
        return "🟡";
      case Priority.MEDIUM:
        return "🔵";
      case Priority.LOW:
        return "⚪";
      default:
        return "⚫";
    }
  };

  const getPriorityAbbr = (p: Priority): string => {
    switch (p) {
      case Priority.URGENT:
        return "URG";
      case Priority.HIGH:
        return "HIGH";
      case Priority.MEDIUM:
        return "MED";
      case Priority.LOW:
        return "LOW";
      default:
        return "";
    }
  };

  const getPriorityClass = (p: Priority): string => {
    switch (p) {
      case Priority.URGENT:
        return styles.priorityUrgent;
      case Priority.HIGH:
        return styles.priorityHigh;
      case Priority.MEDIUM:
        return styles.priorityMedium;
      case Priority.LOW:
        return styles.priorityLow;
      default:
        return styles.priorityUnspecified;
    }
  };

  const getPriorityText = (p: Priority): string => {
    switch (p) {
      case Priority.URGENT:
        return "Urgent";
      case Priority.HIGH:
        return "High";
      case Priority.MEDIUM:
        return "Medium";
      case Priority.LOW:
        return "Low";
      default:
        return "Unknown";
    }
  };

  const reasonLabel = getAttentionReasonInfo(reason).label;

  if (compact) {
    const abbr = getPriorityAbbr(priority);
    return (
      <span
        className={`${styles.badgeCompact} ${getPriorityClass(priority)}`}
        title={`${getPriorityText(priority)}: ${reasonLabel}`}
        aria-label={`${getPriorityText(priority)} priority: ${reasonLabel}`}
      >
        <span aria-hidden="true">{getPriorityEmoji(priority)}</span>
        {abbr && <span className={styles.priorityAbbr} aria-hidden="true">{abbr}</span>}
      </span>
    );
  }

  return (
    <div className={styles.badge}>
      <span
        className={`${styles.priority} ${getPriorityClass(priority)}`}
        aria-label={`Priority: ${getPriorityText(priority)}`}
      >
        {getPriorityEmoji(priority)} {getPriorityText(priority)}
      </span>
      <StatusBadge reason={reason} />
    </div>
  );
}
