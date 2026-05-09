/**
 * Notification lifecycle policy.
 *
 * Centralises all rules about when toasts appear, minimize, and expire.
 * Components and the context import from here — business rules live in
 * exactly one place.
 */

import type { NotificationData } from "@/lib/types/notification";

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

/**
 * How long (ms) before a toast minimizes to a compact pill. 0 = never.
 * Actionable types never minimize because they need user interaction.
 */
export function toastAutoMinimizeMs(type: NotificationData["notificationType"]): number {
  if (isActionable(type)) return 0;
  if (type === "error" || type === "task_failed" || type === "warning") return 5_000;
  return 0;
}
