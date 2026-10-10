"use client";

import { createContext, useContext, useMemo } from "react";
import type { ReviewItem } from "@/gen/session/v1/types_pb";
import type { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";

/** Read-only notification state; changes whenever a toast, history row or panel flag does. */
export interface NotificationStateValue {
  notifications: NotificationData[];
  notificationHistory: NotificationHistoryItem[];
  isPanelOpen: boolean;
  historyLoading: boolean;
  historyHasMore: boolean;
  /** Set when the most recent history fetch failed; cleared on the next successful one. Last-known-good `notificationHistory` is left untouched either way (Task 3.1.2h, AC38). */
  historyError: Error | null;
  /** Date.now() of the last successful history fetch; null until the first one completes. */
  historyLastUpdatedAt: number | null;
  unreadCount: number;
  /** Set for the undo window after "Move all to tray"; toasts are off the deck, history untouched. */
  movedToTray: { count: number } | null;
}

/** Stable commands: identities never change, so command-only consumers never re-render on state. */
export interface NotificationCommandsValue {
  addNotification: (notification: Omit<NotificationData, "id" | "timestamp">) => void;
  /** Add to history panel only — no toast, no sound. For informational events like task_complete. */
  addToHistoryOnly: (notification: Omit<NotificationData, "id" | "timestamp">) => void;
  removeNotification: (id: string) => void;
  /**
   * Remove an active toast whose metadata.approval_id matches the given approvalId.
   * Used to preemptively clear approval toasts when an approval_response event arrives,
   * before refreshHistory() completes.
   */
  removeToastByApprovalId: (approvalId: string) => void;
  /**
   * Acknowledge one or more notifications: removes the active toast(s) and marks
   * them as read in the history panel. Use this for all user-triggered dismissals
   * so the two operations are always kept in sync.
   */
  acknowledgeNotification: (id: string | string[]) => void;
  /** Bulk clear that leaves pinned decisions in place. Never touches history. */
  clearAll: () => void;
  /** Takes one toast off the deck; its history row stays (a pinned toast is demoted, not deleted). */
  dismissToast: (id: string) => void;
  /** Demotes every toast to the tray without marking read or deleting anything. Returns the ids moved. */
  moveAllToTray: () => string[];
  undoMoveToTray: () => void;
  showSessionNotification: (
    item: ReviewItem,
    onView?: () => void,
    onAcknowledge?: () => void
  ) => void;
  togglePanel: () => void;
  markAsRead: (id: string | string[]) => void;
  markAsReadBySessionId: (sessionId: string | string[]) => void;
  /**
   * Remove active toast(s) for the given session ID(s).
   * Does NOT mark history as read — use acknowledgeNotification for that.
   * Used by useReviewQueueNotifications when a stale/queue item resolves,
   * so the toast disappears even if auto-minimize hasn't fired yet.
   */
  removeToastBySessionId: (sessionId: string | string[]) => void;
  removeFromHistory: (id: string) => void;
  clearHistory: () => void;
  loadMoreHistory: () => Promise<void>;
  /** Re-fetch the full notification history from the server (e.g. after a stream reconnect). */
  refreshHistory: () => Promise<void>;
  /**
   * Show an undo-variant toast. Returns the notification ID so the caller can
   * dismiss it when the undo window expires (e.g. via removeNotification).
   * Default duration is 5000ms (passed as durationMs for callers that want to
   * schedule their own dismissal; the toast itself auto-closes via the normal policy).
   */
  showUndoToast: (message: string, onUndo: () => void, durationMs?: number) => string;
  /**
   * Show a lightweight success/error toast for a routine action (e.g. a backlog
   * button click). Bypasses the history panel/audit log — for that, no toast is
   * the right call. `key` dedupes: a second call with the same key replaces the
   * existing toast instead of stacking a duplicate; toasts with different keys
   * (e.g. different items) never collide.
   */
  showActionToast: (message: string, type: "success" | "error", key: string) => string;
}

export type NotificationContextValue = NotificationStateValue &
  NotificationCommandsValue & { getUnreadCount: () => number };

export const NotificationStateContext = createContext<NotificationStateValue | null>(null);
export const NotificationCommandsContext = createContext<NotificationCommandsValue | null>(null);

const noop = () => {};
const OUTSIDE_PROVIDER_STATE: NotificationStateValue = {
  notifications: [],
  notificationHistory: [],
  isPanelOpen: false,
  historyLoading: false,
  historyHasMore: false,
  historyError: null,
  historyLastUpdatedAt: null,
  unreadCount: 0,
  movedToTray: null,
};

/**
 * Outside a NotificationProvider these return no-ops, so components render in
 * tests or embedded views without a full provider tree.
 */
const OUTSIDE_PROVIDER_COMMANDS = {
  addNotification: noop,
  addToHistoryOnly: noop,
  removeNotification: noop,
  removeToastByApprovalId: noop,
  acknowledgeNotification: noop,
  clearAll: noop,
  dismissToast: noop,
  moveAllToTray: () => [],
  undoMoveToTray: noop,
  showSessionNotification: noop,
  togglePanel: noop,
  markAsRead: noop,
  showActionToast: () => "",
} as unknown as NotificationCommandsValue;

/** Toast and history state, without the commands. */
export function useNotificationState(): NotificationStateValue {
  return useContext(NotificationStateContext) ?? OUTSIDE_PROVIDER_STATE;
}

/** Stable commands only; consumers using just these never re-render on notification state. */
export function useNotificationCommands(): NotificationCommandsValue {
  return useContext(NotificationCommandsContext) ?? OUTSIDE_PROVIDER_COMMANDS;
}

/** Both halves merged: the original `useNotifications()` surface. */
export function useNotifications(): NotificationContextValue {
  const state = useNotificationState();
  const commands = useNotificationCommands();
  return useMemo(
    () => ({ ...state, ...commands, getUnreadCount: () => state.unreadCount }),
    [state, commands],
  );
}

