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
    case NotificationType.UNSPECIFIED:
    case NotificationType.INFO:
    case NotificationType.DEBUG:
    case NotificationType.STATUS_CHANGE:
      return "info";
    case NotificationType.CUSTOM:
      return "custom";
    default:
      // Fail-safe: an unmapped backend NotificationType must not silently
      // land in the informational bucket — that's the exact bug Task 3.1.1a
      // fixes one stage downstream. Warn for triage and default to an
      // actionable type so it surfaces in NeedsDecisionSection instead of
      // disappearing into collapsed informational activity. "warning" is
      // used (not "approval_needed") since it carries no approve/deny
      // button assumptions elsewhere in the rendering pipeline.
      console.warn(
        `[notificationMapping] Unmapped NotificationType ${protoType}; defaulting to "warning" (actionable fail-safe)`
      );
      return "warning";
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
 * Notification types that represent something the user must actively decide on
 * (approve/deny a tool-use request, answer a question, or triage a failure) —
 * the single source of truth for "needs a decision" membership. `"info"`'s
 * allow-list below is defined as this set's literal complement so a type can
 * never fall into neither bucket (see Epic 3.1.1's regression: a second,
 * independently-maintained exclusion list let "task_complete" fall through
 * both).
 */
const ACTIONABLE_TYPES = new Set<UIType>([
  "approval_needed",
  "question",
  "error",
  "task_failed",
  "warning",
]);

export function isActionableNotification(type: UIType): boolean {
  return ACTIONABLE_TYPES.has(type);
}

/**
 * Returns the unread notification IDs that are safe for a bulk "mark read"
 * action to touch — i.e. everything except an unread actionable item, which
 * must only leave "needs a decision" by being resolved. Shared by
 * NotificationsPage's "Mark activity read" and NotificationPanel's bulk-read
 * button so the scoping rule is defined once (Task 3.1.2e / 3.1.5a).
 */
export function computeScopedMarkReadIds(
  notifications: Array<{ id: string; isRead: boolean; notificationType?: UIType }>
): string[] {
  return notifications
    .filter((n) => !n.isRead && !isActionableNotification(n.notificationType))
    .map((n) => n.id);
}

/**
 * Caps a raw badge count at "99+", matching NavBadge.tsx's convention.
 */
export function capBadgeCount(n: number): string {
  return n > 99 ? "99+" : String(n);
}

/**
 * Returns the set of UI notification types that belong to a given filter category.
 * The "error" pill covers task_failed and warning; "info" is the allow-list
 * complement of ACTIONABLE_TYPES so a new UI type always lands somewhere.
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
      return types.filter((t) => !isActionableNotification(t));
    default:
      return types;
  }
}

/**
 * ApprovalService appends a trailing " https://.../checks" URL to the CI-block error
 * message (AC5/AC6) — split it out so approval UIs can render it as a "View CI run"
 * link instead of a raw URL in the warning text.
 */
export function splitCIBlockMessage(message: string): { text: string; checksUrl?: string } {
  const match = message.match(/\s(https?:\/\/\S+)$/);
  if (!match) {
    return { text: message };
  }
  return { text: message.slice(0, match.index), checksUrl: match[1] };
}

/**
 * Whether a notification's resolution came from rule-reconciliation rather than a
 * live human decision (Epic 2.1's `IsReconciled()` on the Go side — this is the
 * single shared TS accessor for the same `metadata.reconciled` check, used by every
 * UI branch that needs to distinguish the two instead of each inlining its own
 * `metadata?.reconciled === "true"` comparison.
 */
export function isReconciledNotification(n: { metadata?: Record<string, string> }): boolean {
  return n.metadata?.reconciled === "true";
}
