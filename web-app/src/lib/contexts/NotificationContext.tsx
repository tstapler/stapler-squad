"use client";

import React, { useState, useEffect, useMemo, useRef } from "react";
import { ToastStack } from "@/components/ui/ToastStack";
import { AnnouncerProvider } from "@/components/ui/Announcer";
import { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";
import {
  NotificationStateContext,
  NotificationCommandsContext,
  useNotificationState,
  useNotificationCommands,
  useNotifications,
  type NotificationStateValue,
  type NotificationCommandsValue,
  type NotificationContextValue,
} from "@/lib/contexts/notificationContexts";
import { ReviewItem, AttentionReason } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";
import { useToastQueue } from "@/lib/hooks/useToastQueue";
import { useToastTimers } from "@/lib/hooks/useToastTimers";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { createToastTrayCommands, MOVE_UNDO_TIMER_ID, type MovedBatch } from "@/lib/contexts/toastTray";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { mapPriority } from "@/lib/utils/notificationMapping";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { NOTIFICATION_TRAY_V2_FLAG, isPinned } from "@/lib/notification-policy";
import { isSessionViewed } from "@/lib/utils/viewedSessions";
import { mergeBackendHistory, recordToHistoryItem } from "@/lib/utils/notificationHistoryMerge";
import { createNotificationSyncChannel } from "@/lib/utils/broadcastChannel";
import { markAcknowledged } from "@/lib/utils/notificationStorage";
import { readQuietMode, writeQuietMode } from "@/lib/utils/deckSettings";
import type { FailureReason } from "@/lib/utils/sessionFailure";

// One sender per tab; a channel per broadcast was opened and never closed.
let broadcastSender: ReturnType<typeof createNotificationSyncChannel> | null = null;
function sendToOtherTabs(message: Parameters<ReturnType<typeof createNotificationSyncChannel>["broadcast"]>[0]) {
  if (!broadcastSender) broadcastSender = createNotificationSyncChannel();
  broadcastSender.broadcast(message);
}

export type { NotificationData, NotificationHistoryItem, NotificationContextValue };
export { useNotificationState, useNotificationCommands, useNotifications };

/**
 * Reason-specific toast copy for a session that transitioned to
 * SESSION_STATUS_FAILED (async-session-creation Epic 5.3, Surface 4). Mirrors
 * the FailureReason taxonomy set server-side by the Background Resolution
 * Pipeline (session.Instance.FailureReason) — "GitHubResolutionError",
 * "StartupError", "Stale", "Cancelled", "WorktreeResolutionFailed", or
 * "DirectoryCollision". A reasonable default per
 * plan.md's Unresolved Questions (exact copy is a pending product decision).
 *
 * Deliberately different wording than SessionCard.tsx/SessionRow.tsx's
 * persistent card/row copy (lib/utils/sessionFailure.ts's getFailureMessage)
 * — toast copy is transient and can afford to be more conversational, while
 * the card/row copy is a durable record. The `FailureReason` type import
 * (rather than a bare `string` param) only guarantees the *set* of reasons
 * this switch handles stays in sync with the card/row's; it does not force
 * the wording to match.
 */
export function getFailureReasonToastMessage(failureReason: string): string {
  switch (failureReason as FailureReason) {
    case "GitHubResolutionError":
      return "Couldn't resolve the GitHub repository. Check the URL and try again.";
    case "StartupError":
      return "The session failed to start. See the session card for details.";
    case "Stale":
      return "Session creation timed out and was marked as failed.";
    case "Cancelled":
      return "Session creation was cancelled.";
    case "WorktreeResolutionFailed":
      return "Couldn't set up an isolated workspace for this session. It was not started.";
    case "DirectoryCollision":
      return "Another session is already running in that directory, so this one wasn't started.";
    default:
      return "Session creation failed.";
  }
}

function reviewItemToNotificationType(reason: AttentionReason): NotificationData["notificationType"] {
  switch (reason) {
    case AttentionReason.APPROVAL_PENDING:
    case AttentionReason.WAITING_FOR_USER:
      return "approval_needed";
    case AttentionReason.INPUT_REQUIRED:
      return "question";
    case AttentionReason.ERROR_STATE:
    case AttentionReason.TESTS_FAILING:
      return "error";
    case AttentionReason.STALE:
      return "warning";
    case AttentionReason.TASK_COMPLETE:
      return "task_complete";
    default:
      return "info";
  }
}

const newNotificationId = () => `notification-${Date.now()}-${Math.random()}`;
const toIdSet = (id: string | string[]) => new Set(Array.isArray(id) ? id : [id]);

const SWEEP_INTERVAL_MS = 60_000;
const UNDO_TOAST_DEFAULT_MS = 5_000;
const ACTION_TOAST_SUCCESS_MS = 5_000;
const ACTION_TOAST_ERROR_MS = 10_000;

export function NotificationProvider({ children }: { children: React.ReactNode }) {
  // The Announcer wraps the provider's own logic so commands can announce receipts.
  return (
    <AnnouncerProvider>
      <NotificationProviderInner>{children}</NotificationProviderInner>
    </AnnouncerProvider>
  );
}

function NotificationProviderInner({ children }: { children: React.ReactNode }) {
  const { queue: notifications, queueRef, dispatch } = useToastQueue();
  const timers = useToastTimers();
  const [notificationHistory, setNotificationHistory] = useState<NotificationHistoryItem[]>([]);
  const [isPanelOpen, setIsPanelOpen] = useState(false);
  // Commands read these for side effects (audit, RPC) so React updater functions stay pure.
  const historyStateRef = useRef<NotificationHistoryItem[]>([]);
  historyStateRef.current = notificationHistory;
  const isPanelOpenRef = useRef(false);
  isPanelOpenRef.current = isPanelOpen;
  const [moved, setMovedState] = useState<MovedBatch | null>(null);
  const movedRef = useRef<MovedBatch | null>(null);
  const { announce } = useAnnounce();
  const announceRef = useRef(announce);
  announceRef.current = announce;

  const auditLog = useAuditLog();
  const history = useNotificationHistory();
  // Commands read these through refs so their identities stay stable across renders.
  const auditLogRef = useRef(auditLog);
  auditLogRef.current = auditLog;
  const historyRef = useRef(history);
  historyRef.current = history;
  const trayV2Ref = useRef(false);
  trayV2Ref.current = useFeatureFlag(NOTIFICATION_TRAY_V2_FLAG);
  const [quietMode, setQuietModeState] = useState(false);
  const quietRef = useRef(false);
  // Read after mount so server and client render the same first frame.
  useEffect(() => {
    quietRef.current = readQuietMode();
    setQuietModeState(quietRef.current);
  }, []);

  // Backend data is authoritative: runs on initial load and whenever refreshHistory()
  // is called (reconnect, approval_response), updating local items with the server version.
  useEffect(() => {
    if (history.notifications.length === 0) return;
    const backendItems = history.notifications.map(recordToHistoryItem);
    setNotificationHistory((prev) => mergeBackendHistory(prev, backendItems));
  }, [history.notifications]);

  const commands = useMemo<NotificationCommandsValue>(() => {
    const removeToast = (id: string) => {
      timers.cancel(id);
      dispatch({ type: "remove", ids: new Set([id]) });
    };

    const tray = createToastTrayCommands({
      queueRef,
      dispatch,
      timers,
      movedRef,
      setMoved: (batch) => {
        movedRef.current = batch;
        setMovedState(batch);
      },
      announce: (message) => announceRef.current(message),
      notifyOtherTabs: (ids) =>
        sendToOtherTabs({ type: "NOTIFICATIONS_BULK_DISMISSED", kind: "moved", ids }),
    });

    const appendEphemeral = (notification: NotificationData, lifetimeMs: number, replaceKey?: string) => {
      dispatch({ type: "append", notification, replaceKey });
      timers.register(notification.id, "expire", lifetimeMs, () => removeToast(notification.id));
      return notification.id;
    };

    // Pure transform applied to the state and, eagerly, to the ref a second command in the same tick reads.
    const mutateHistory = (fn: (prev: NotificationHistoryItem[]) => NotificationHistoryItem[]) => {
      historyStateRef.current = fn(historyStateRef.current);
      setNotificationHistory(fn);
    };

    const addToHistory = (notification: NotificationData) =>
      setNotificationHistory((prev) =>
        prev.some((n) => n.id === notification.id) ? prev : [{ ...notification, isRead: false }, ...prev],
      );

    const addNotification: NotificationCommandsValue["addNotification"] = (notification) => {
      const next: NotificationData = { ...notification, id: notification.id ?? newNotificationId(), timestamp: Date.now() };
      // Under the capped deck a non-pinned toast for the session already on screen adds
      // nothing, and Quiet mode demotes every non-pinned toast; a pending decision always
      // toasts. The history row is recorded either way.
      const suppressed = trayV2Ref.current && !isPinned(next) && (quietRef.current || isSessionViewed(next.sessionId));
      if (!suppressed) dispatch({ type: "add", notification: next });
      addToHistory(next);
    };

    const addToHistoryOnly: NotificationCommandsValue["addToHistoryOnly"] = (notification) =>
      addToHistory({ ...notification, id: notification.id ?? newNotificationId(), timestamp: Date.now() });

    const markAsRead = (id: string | string[]) => {
      const ids = Array.isArray(id) ? id : [id];
      const idSet = new Set(ids);
      for (const n of historyStateRef.current) {
        if (idSet.has(n.id)) auditLogRef.current.logNotificationMarkedRead(n.id, n.sessionId);
      }
      mutateHistory((prev) => prev.map((n) => (idSet.has(n.id) ? { ...n, isRead: true, isPendingDecision: false } : n)));
      return historyRef.current.markAsRead(ids);
    };

    return {
      addNotification,
      addToHistoryOnly,
      removeNotification: removeToast,
      dismissToast: removeToast,
      moveAllToTray: tray.moveAllToTray,
      undoMoveToTray: tray.undoMoveToTray,
      removeToastByApprovalId: (approvalId) => dispatch({ type: "removeByApprovalId", approvalId }),
      clearAll: () => {
        queueRef.current.filter((n) => !isPinned(n)).forEach((n) => timers.cancel(n.id));
        dispatch({ type: "clearUnpinned" });
      },
      showSessionNotification: (item, onView, onAcknowledge) =>
        addNotification({
          sessionId: item.sessionId,
          sessionName: item.sessionName || "Unnamed Session",
          message: item.context || "This session is waiting for your input",
          priority: mapPriority(item.priority),
          notificationType: reviewItemToNotificationType(item.reason),
          onView,
          onAcknowledge,
        }),
      showUndoToast: (message, onUndo, durationMs = UNDO_TOAST_DEFAULT_MS) =>
        appendEphemeral(
          {
            id: newNotificationId(),
            sessionId: "",
            sessionName: "",
            message,
            timestamp: Date.now(),
            notificationType: "undo",
            onUndo,
          },
          durationMs,
        ),
      showActionToast: (message, type, key) =>
        appendEphemeral(
          {
            id: newNotificationId(),
            sessionId: "",
            sessionName: "",
            message,
            timestamp: Date.now(),
            notificationType: type === "success" ? "task_complete" : "error",
            metadata: { actionToastKey: key },
          },
          type === "success" ? ACTION_TOAST_SUCCESS_MS : ACTION_TOAST_ERROR_MS,
          key,
        ),
      setQuietMode: (on) => {
        quietRef.current = on;
        writeQuietMode(on);
        setQuietModeState(on);
      },
      togglePanel: () => {
        const opening = !isPanelOpenRef.current;
        isPanelOpenRef.current = opening;
        if (opening) auditLogRef.current.logNotificationPanelOpened();
        else auditLogRef.current.logNotificationPanelClosed();
        setIsPanelOpen(opening);
      },
      markAsRead,
      // Removes the toast(s) AND marks them read in one operation; prefer it to
      // calling removeNotification + markAsRead separately.
      acknowledgeNotification: (id) => {
        const ids = Array.isArray(id) ? id : [id];
        const idSet = new Set(ids);
        for (const n of queueRef.current) {
          if (!idSet.has(n.id)) continue;
          sendToOtherTabs({ type: "NOTIFICATION_DISMISSED", notificationId: n.id });
          if (n.sessionId) markAcknowledged(n.sessionId);
          timers.cancel(n.id);
        }
        dispatch({ type: "remove", ids: idSet });
        markAsRead(ids);
      },
      markAsReadBySessionId: (sessionId) => {
        const sessionIds = toIdSet(sessionId);
        const idsToMark = historyStateRef.current
          .filter((n) => !n.isRead && n.sessionId != null && sessionIds.has(n.sessionId))
          .map((n) => n.id);
        if (idsToMark.length === 0) return;
        const marked = new Set(idsToMark);
        mutateHistory((prev) =>
          prev.map((n) => (marked.has(n.id) ? { ...n, isRead: true, isPendingDecision: false } : n)),
        );
        historyRef.current.markAsRead(idsToMark);
      },
      removeToastBySessionId: (sessionId) => {
        const sessionIds = toIdSet(sessionId);
        sessionIds.delete(""); // never match notifications without a sessionId
        if (sessionIds.size > 0) dispatch({ type: "removeBySessionIds", sessionIds });
      },
      removeFromHistory: (id) => {
        const notification = historyStateRef.current.find((n) => n.id === id);
        if (notification) auditLogRef.current.logNotificationRemoved(notification.id, notification.sessionId);
        mutateHistory((prev) => prev.filter((n) => n.id !== id));
      },
      clearHistory: () => {
        const count = historyStateRef.current.length;
        if (count > 0) auditLogRef.current.logNotificationHistoryCleared(count);
        mutateHistory(() => []);
        historyRef.current.clearHistory();
      },
      clearHistoryByIds: async (ids, options) => {
        const result = await historyRef.current.clearByIds(ids, options);
        const gone = ids.filter((id) => !result.kept.includes(id));
        if (gone.length > 0) {
          const removedCount = historyStateRef.current.filter((n) => gone.includes(n.id)).length;
          if (removedCount > 0) auditLogRef.current.logNotificationHistoryCleared(removedCount);
          mutateHistory((prev) => prev.filter((n) => !gone.includes(n.id)));
          sendToOtherTabs({ type: "NOTIFICATIONS_BULK_DISMISSED", kind: "dismissed", ids: gone });
        }
        return result;
      },
      loadMoreHistory: () => historyRef.current.loadMore(),
      refreshHistory: () => historyRef.current.refresh(),
    };
  }, [dispatch, queueRef, timers]);

  // Opening the tray ends the undo window: the moved toasts are now simply tray rows.
  useEffect(() => {
    if (isPanelOpen && movedRef.current) {
      timers.cancel(MOVE_UNDO_TIMER_ID);
      movedRef.current = null;
      setMovedState(null);
    }
  }, [isPanelOpen, timers]);

  // Remove stale toasts every minute (history keeps them): plain toasts after
  // TOAST_STALE_MS, approval/question toasts after ACTIONABLE_TOAST_STALE_MS.
  useEffect(() => {
    const interval = setInterval(
      () => dispatch({ type: "prune", now: Date.now(), keep: trayV2Ref.current ? isPinned : undefined }),
      SWEEP_INTERVAL_MS,
    );
    return () => clearInterval(interval);
  }, [dispatch]);

  // Cross-tab sync: when another tab dismisses a notification, reflect it locally.
  useEffect(() => {
    const syncChannel = createNotificationSyncChannel();
    const unsubscribe = syncChannel.subscribe((message) => {
      if (message.type === "NOTIFICATION_DISMISSED") {
        const { notificationId } = message;
        timers.cancel(notificationId);
        dispatch({ type: "remove", ids: new Set([notificationId]) });
        setNotificationHistory((prev) =>
          prev.map((n) => (n.id === notificationId ? { ...n, isRead: true, isPendingDecision: false } : n))
        );
      }
      if (message.type === "NOTIFICATIONS_BULK_DISMISSED") {
        const ids = new Set(message.ids);
        ids.forEach((id) => timers.cancel(id));
        dispatch({ type: "remove", ids });
        // "moved" only leaves the deck; "dismissed" also drops the rows the other tab cleared.
        if (message.kind === "dismissed") {
          setNotificationHistory((prev) => prev.filter((n) => !ids.has(n.id)));
        }
      }
      // NOTIFICATION_ACKNOWLEDGED is intentionally not handled here.
      // Cross-tab session acknowledgement is driven by the sessionAcknowledged
      // event from the server stream (useSessionService), not BroadcastChannel.
    });
    return () => {
      unsubscribe();
      broadcastSender = null; // the provider is gone; the next one opens its own sender
    };
  }, [dispatch, timers]);

  // Only one page of history is loaded, so the server's unread total is the floor: a badge
  // for 120 unread must not read 49 just because that is what has been paged in.
  const serverUnread = history.unreadCount ?? 0;
  const unreadCount = useMemo(
    () => Math.max(groupNotifications(notificationHistory.filter((n) => !n.isRead)).length, serverUnread),
    [notificationHistory, serverUnread],
  );

  const state = useMemo<NotificationStateValue>(
    () => ({
      notifications,
      notificationHistory,
      isPanelOpen,
      historyLoading: history.loading,
      historyHasMore: history.hasMore,
      historyError: history.error,
      historyLastUpdatedAt: history.lastUpdatedAt,
      unreadCount,
      movedToTray: moved ? { count: moved.toasts.length } : null,
      quietMode,
    }),
    [notifications, notificationHistory, isPanelOpen, moved, quietMode, history.loading, history.hasMore, history.error, history.lastUpdatedAt, unreadCount],
  );

  return (
    <NotificationCommandsContext.Provider value={commands}>
      <NotificationStateContext.Provider value={state}>
        {children}
        <ToastStack timers={timers} />
      </NotificationStateContext.Provider>
    </NotificationCommandsContext.Provider>
  );
}
