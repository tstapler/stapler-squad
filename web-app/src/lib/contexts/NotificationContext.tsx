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
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { mapPriority } from "@/lib/utils/notificationMapping";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { NOTIFICATION_TRAY_V2_FLAG, isPinned } from "@/lib/notification-policy";
import { isSessionViewed } from "@/lib/utils/viewedSessions";
import { mergeBackendHistory, recordToHistoryItem } from "@/lib/utils/notificationHistoryMerge";
import { createNotificationSyncChannel } from "@/lib/utils/broadcastChannel";
import { markAcknowledged } from "@/lib/utils/notificationStorage";
import type { FailureReason } from "@/lib/utils/sessionFailure";

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
  const { queue: notifications, queueRef, dispatch } = useToastQueue();
  const timers = useToastTimers();
  const [notificationHistory, setNotificationHistory] = useState<NotificationHistoryItem[]>([]);
  const [isPanelOpen, setIsPanelOpen] = useState(false);

  const auditLog = useAuditLog();
  const history = useNotificationHistory();
  // Commands read these through refs so their identities stay stable across renders.
  const auditLogRef = useRef(auditLog);
  auditLogRef.current = auditLog;
  const historyRef = useRef(history);
  historyRef.current = history;
  const trayV2Ref = useRef(false);
  trayV2Ref.current = useFeatureFlag(NOTIFICATION_TRAY_V2_FLAG);

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

    const appendEphemeral = (notification: NotificationData, lifetimeMs: number, replaceKey?: string) => {
      dispatch({ type: "append", notification, replaceKey });
      timers.register(notification.id, "expire", lifetimeMs, () => removeToast(notification.id));
      return notification.id;
    };

    const addToHistory = (notification: NotificationData) =>
      setNotificationHistory((prev) =>
        prev.some((n) => n.id === notification.id) ? prev : [{ ...notification, isRead: false }, ...prev],
      );

    const addNotification: NotificationCommandsValue["addNotification"] = (notification) => {
      const next: NotificationData = { ...notification, id: newNotificationId(), timestamp: Date.now() };
      // Under the capped deck a non-pinned toast for the session already on screen
      // adds nothing; the history row is still recorded.
      const suppressed = trayV2Ref.current && !isPinned(next) && isSessionViewed(next.sessionId);
      if (!suppressed) dispatch({ type: "add", notification: next });
      addToHistory(next);
    };

    const addToHistoryOnly: NotificationCommandsValue["addToHistoryOnly"] = (notification) =>
      addToHistory({ ...notification, id: newNotificationId(), timestamp: Date.now() });

    const markAsRead = (id: string | string[]) => {
      const ids = Array.isArray(id) ? id : [id];
      const idSet = new Set(ids);
      setNotificationHistory((prev) => {
        for (const n of prev) {
          if (idSet.has(n.id)) auditLogRef.current.logNotificationMarkedRead(n.id, n.sessionId);
        }
        return prev.map((n) => (idSet.has(n.id) ? { ...n, isRead: true, isPendingDecision: false } : n));
      });
      historyRef.current.markAsRead(ids);
    };

    return {
      addNotification,
      addToHistoryOnly,
      removeNotification: removeToast,
      removeToastByApprovalId: (approvalId) => dispatch({ type: "removeByApprovalId", approvalId }),
      clearAll: () => {
        timers.cancelAll();
        dispatch({ type: "clear" });
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
      togglePanel: () =>
        setIsPanelOpen((prev) => {
          if (!prev) auditLogRef.current.logNotificationPanelOpened();
          else auditLogRef.current.logNotificationPanelClosed();
          return !prev;
        }),
      markAsRead,
      // Removes the toast(s) AND marks them read in one operation; prefer it to
      // calling removeNotification + markAsRead separately.
      acknowledgeNotification: (id) => {
        const ids = Array.isArray(id) ? id : [id];
        const idSet = new Set(ids);
        const syncChannel = createNotificationSyncChannel();
        for (const n of queueRef.current) {
          if (!idSet.has(n.id)) continue;
          syncChannel.broadcast({ type: "NOTIFICATION_DISMISSED", notificationId: n.id });
          if (n.sessionId) markAcknowledged(n.sessionId);
          timers.cancel(n.id);
        }
        dispatch({ type: "remove", ids: idSet });
        markAsRead(ids);
      },
      markAsReadBySessionId: (sessionId) => {
        const sessionIds = toIdSet(sessionId);
        setNotificationHistory((prev) => {
          const idsToMark: string[] = [];
          const updated = prev.map((n) => {
            if (!n.isRead && n.sessionId != null && sessionIds.has(n.sessionId)) {
              idsToMark.push(n.id);
              return { ...n, isRead: true, isPendingDecision: false };
            }
            return n;
          });
          if (idsToMark.length > 0) historyRef.current.markAsRead(idsToMark);
          return updated;
        });
      },
      removeToastBySessionId: (sessionId) => {
        const sessionIds = toIdSet(sessionId);
        sessionIds.delete(""); // never match notifications without a sessionId
        if (sessionIds.size > 0) dispatch({ type: "removeBySessionIds", sessionIds });
      },
      removeFromHistory: (id) =>
        setNotificationHistory((prev) => {
          const notification = prev.find((n) => n.id === id);
          if (notification) auditLogRef.current.logNotificationRemoved(notification.id, notification.sessionId);
          return prev.filter((n) => n.id !== id);
        }),
      clearHistory: () => {
        setNotificationHistory((prev) => {
          if (prev.length > 0) auditLogRef.current.logNotificationHistoryCleared(prev.length);
          return [];
        });
        historyRef.current.clearHistory();
      },
      loadMoreHistory: () => historyRef.current.loadMore(),
      refreshHistory: () => historyRef.current.refresh(),
    };
  }, [dispatch, queueRef, timers]);

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
      // NOTIFICATION_ACKNOWLEDGED is intentionally not handled here.
      // Cross-tab session acknowledgement is driven by the sessionAcknowledged
      // event from the server stream (useSessionService), not BroadcastChannel.
    });
    return unsubscribe;
  }, [dispatch, timers]);

  const unreadCount = useMemo(
    () => groupNotifications(notificationHistory.filter((n) => !n.isRead)).length,
    [notificationHistory],
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
    }),
    [notifications, notificationHistory, isPanelOpen, history.loading, history.hasMore, history.error, history.lastUpdatedAt, unreadCount],
  );

  return (
    <AnnouncerProvider>
      <NotificationCommandsContext.Provider value={commands}>
        <NotificationStateContext.Provider value={state}>
          {children}
          <ToastStack timers={timers} />
        </NotificationStateContext.Provider>
      </NotificationCommandsContext.Provider>
    </AnnouncerProvider>
  );
}
