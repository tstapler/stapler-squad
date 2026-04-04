"use client";

import { Priority, AttentionReason } from "@/gen/session/v1/types_pb";
import styles from "./ReviewQueueBadge.module.css";

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

  const getReasonText = (r: AttentionReason): string => {
    switch (r) {
      case AttentionReason.APPROVAL_PENDING:
        return "Approval Pending";
      case AttentionReason.INPUT_REQUIRED:
        return "Input Required";
      case AttentionReason.ERROR_STATE:
        return "Error";
      case AttentionReason.IDLE_TIMEOUT:
      case AttentionReason.IDLE:
        return "Idle";
      case AttentionReason.TASK_COMPLETE:
        return "Complete";
      case AttentionReason.UNCOMMITTED_CHANGES:
        return "Uncommitted Changes";
      case AttentionReason.STALE:
        return "Stale";
      case AttentionReason.WAITING_FOR_USER:
        return "Waiting";
      case AttentionReason.TESTS_FAILING:
        return "Tests Failing";
      default:
        return "Unknown";
    }
  };

  const getReasonClass = (r: AttentionReason): string => {
    switch (r) {
      case AttentionReason.APPROVAL_PENDING:
        return styles.reasonApproval;
      case AttentionReason.INPUT_REQUIRED:
        return styles.reasonInput;
      case AttentionReason.ERROR_STATE:
        return styles.reasonError;
      case AttentionReason.IDLE_TIMEOUT:
      case AttentionReason.IDLE:
        return styles.reasonIdle;
      case AttentionReason.TASK_COMPLETE:
        return styles.reasonComplete;
      case AttentionReason.UNCOMMITTED_CHANGES:
        return styles.reasonUnspecified;
      case AttentionReason.STALE:
        return styles.reasonUnspecified;
      case AttentionReason.WAITING_FOR_USER:
        return styles.reasonInput;
      case AttentionReason.TESTS_FAILING:
        return styles.reasonError;
      default:
        return styles.reasonUnspecified;
    }
  };

  if (compact) {
    return (
      <span
        className={`${styles.badgeCompact} ${getPriorityClass(priority)}`}
        title={`${getPriorityText(priority)}: ${getReasonText(reason)}`}
        aria-label={`${getPriorityText(priority)} priority: ${getReasonText(reason)}`}
      >
        {getPriorityEmoji(priority)}
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
      <span
        className={`${styles.reason} ${getReasonClass(reason)}`}
        aria-label={`Reason: ${getReasonText(reason)}`}
      >
        {getReasonText(reason)}
      </span>
    </div>
  );
}
