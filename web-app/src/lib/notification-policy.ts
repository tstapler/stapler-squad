/**
 * Notification lifecycle policy.
 *
 * Centralises all rules about when toasts appear, minimize, and expire.
 * Components and the context import from here — business rules live in
 * exactly one place.
 */

import type { NotificationData } from "@/lib/types/notification";
import { NotificationPriority } from "@/gen/session/v1/types_pb";

/** Non-actionable toasts are swept from the active list after 5 minutes. */
export const TOAST_STALE_MS = 5 * 60 * 1000;

/**
 * Deduplication window for toasts: suppress a second toast for the same
 * (sessionId, notificationType) if one was shown within this period.
 * Actionable types (approval_needed, question) are never suppressed.
 */
export const TOAST_DEDUP_WINDOW_MS = 10_000;

/**
 * Actionable toasts (approval_needed, question) are swept after 6 minutes.
 * They stay in the notification history panel regardless.
 */
export const ACTIONABLE_TOAST_STALE_MS = 6 * 60 * 1000;

/** Returns true for notification types that require explicit user action before closing. */
export function isActionable(type: NotificationData["notificationType"]): boolean {
  return type === "approval_needed" || type === "question";
}

/**
 * How long (ms) before a toast auto-closes via the component timer.
 * Actionable types use the full ACTIONABLE_TOAST_STALE_MS so they remain
 * visible until resolved, or until the 6-minute fallback fires.
 */
export function toastAutoCloseMs(type: NotificationData["notificationType"]): number {
  if (isActionable(type)) return ACTIONABLE_TOAST_STALE_MS;
  if (type === "error" || type === "task_failed") return 12_000;
  return 8_000;
}

/** Auto-close delay for high/urgent native (OS) notifications (FR-3). */
export const NATIVE_HIGH_TTL_MS = 30_000;
/** Auto-close delay for medium/low native (OS) notifications (FR-3). */
export const NATIVE_MEDIUM_TTL_MS = 15_000;
/** Auto-close delay for actionable native (OS) notifications (approval_needed, input_required) (FR-3). */
export const NATIVE_ACTIONABLE_TTL_MS = 5 * 60 * 1000;

/** Maps a NotificationPriority to the native notification auto-close TTL. */
export function nativeAutoCloseMs(priority: NotificationPriority): number {
  switch (priority) {
    case NotificationPriority.URGENT:
    case NotificationPriority.HIGH:
      return NATIVE_HIGH_TTL_MS;
    case NotificationPriority.MEDIUM:
    case NotificationPriority.LOW:
    case NotificationPriority.UNSPECIFIED:
    default:
      return NATIVE_MEDIUM_TTL_MS;
  }
}

/**
 * How long (ms) before a toast minimizes to a compact pill. 0 = never.
 * Actionable types never minimize because they need user interaction.
 */
export function toastAutoMinimizeMs(type: NotificationData["notificationType"]): number {
  if (isActionable(type)) return 0;
  if (type === "error" || type === "task_failed") return 5_000;
  if (type === "warning") return 5_000;
  return 3_000;
}
