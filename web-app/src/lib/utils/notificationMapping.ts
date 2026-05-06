/**
 * Canonical notification mappings.
 *
 * - Proto → UI type: mapNotificationType, mapPriority
 * - UI type → display: notificationTypeIcon, notificationTypeLabel, priorityColor,
 *   notificationTypeFilter
 *
 * Components import from here. Never duplicate these switch statements in UI files.
 */

import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import type { NotificationData } from "@/lib/types/notification";

type UIType = NotificationData["notificationType"];
type UIPriority = NotificationData["priority"];

export function mapNotificationType(protoType: number): NotificationData["notificationType"] {
  switch (protoType) {
    case NotificationType.APPROVAL_NEEDED:
    case NotificationType.CONFIRMATION_NEEDED:
      return "approval_needed";
    case NotificationType.INPUT_REQUIRED:
      return "question";
    case NotificationType.ERROR:
    case NotificationType.FAILURE:
      return "error";
    case NotificationType.WARNING:
      return "warning";
    case NotificationType.TASK_COMPLETE:
    case NotificationType.PROCESS_FINISHED:
      return "task_complete";
    case NotificationType.PROCESS_STARTED:
      return "progress";
    case NotificationType.AUTO_APPROVED:
      return "auto_approved";
    case NotificationType.INFO:
    case NotificationType.DEBUG:
    case NotificationType.STATUS_CHANGE:
      return "info";
    case NotificationType.CUSTOM:
      return "custom";
    default:
      return "info";
  }
}

export function mapPriority(protoPriority: number): "urgent" | "high" | "medium" | "low" {
  switch (protoPriority) {
    case NotificationPriority.URGENT: return "urgent";
    case NotificationPriority.HIGH:   return "high";
    case NotificationPriority.MEDIUM: return "medium";
    case NotificationPriority.LOW:    return "low";
    default:                          return "medium";
  }
}

// ---------------------------------------------------------------------------
// UI display helpers — used by both NotificationToast and NotificationPanel
// ---------------------------------------------------------------------------

export function notificationTypeIcon(type: UIType): string {
  switch (type) {
    case "approval_needed": return "⚠️";
    case "auto_approved":   return "✅";
    case "error":           return "❌";
    case "warning":         return "⚠️";
    case "task_complete":   return "✅";
    case "task_failed":     return "💥";
    case "progress":        return "⏳";
    case "question":        return "❓";
    case "reminder":        return "⏰";
    case "system":          return "⚙️";
    default:                return "🔔";
  }
}

export function notificationTypeLabel(type: UIType): string {
  switch (type) {
    case "approval_needed": return "Approval Needed";
    case "auto_approved":   return "Auto-handled";
    case "error":           return "Error";
    case "warning":         return "Warning";
    case "task_complete":   return "Task Complete";
    case "task_failed":     return "Task Failed";
    case "progress":        return "Progress";
    case "question":        return "Question";
    case "reminder":        return "Reminder";
    case "system":          return "System";
    case "custom":          return "Custom";
    default:                return "Info";
  }
}

export function priorityColor(priority: UIPriority): string {
  switch (priority) {
    case "urgent": return "var(--color-error, #f44336)";
    case "high":   return "var(--color-warning, #ff9800)";
    case "medium": return "var(--color-info, #2196f3)";
    case "low":    return "var(--color-success, #4caf50)";
    default:       return "var(--color-primary, #0070f3)";
  }
}

/**
 * Returns the set of UI notification types that belong to a given filter category.
 * The "error" pill covers task_failed and warning; "info" covers everything else.
 */
export function notificationTypeFilter(
  category: "all" | "approval_needed" | "error" | "task_complete" | "info",
  types: UIType[]
): UIType[] {
  switch (category) {
    case "approval_needed":
      return types.filter((t) => t === "approval_needed" || t === "question");
    case "error":
      return types.filter((t) => t === "error" || t === "task_failed" || t === "warning");
    case "task_complete":
      return types.filter((t) => t === "task_complete");
    case "info":
      return types.filter(
        (t) =>
          t !== "approval_needed" &&
          t !== "auto_approved" &&
          t !== "error" &&
          t !== "task_failed" &&
          t !== "warning" &&
          t !== "task_complete"
      );
    default:
      return types;
  }
}
