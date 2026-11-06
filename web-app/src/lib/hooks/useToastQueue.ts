"use client";

import { useCallback, useRef, useState } from "react";
import type { NotificationData } from "@/lib/types/notification";
import {
  TOAST_STALE_MS,
  ACTIONABLE_TOAST_STALE_MS,
  hasLongToastLifetime,
  isPinned,
} from "@/lib/notification-policy";

/** The active (on-screen or overflowing) toasts, oldest first. */
export type ToastQueue = NotificationData[];

export type ToastQueueAction =
  | { type: "add"; notification: NotificationData }
  /** Ephemeral toast (undo, action receipt): appended; `replaceKey` swaps a same-keyed one. */
  | { type: "append"; notification: NotificationData; replaceKey?: string }
  | { type: "remove"; ids: ReadonlySet<string> }
  | { type: "removeByApprovalId"; approvalId: string }
  | { type: "removeBySessionIds"; sessionIds: ReadonlySet<string> }
  /** Bulk clear: drops every toast except a pinned decision, which only an explicit act removes. */
  | { type: "clearUnpinned" }
  /** Puts back toasts moved to the tray (undo), oldest first, skipping a session that has a newer toast. */
  | { type: "restore"; toasts: readonly NotificationData[] }
  /** Drop toasts past their staleness window; `keep` exempts a toast (a pinned decision). */
  | { type: "prune"; now: number; keep?: (n: NotificationData) => boolean };

function isApprovalToast(n: NotificationData): boolean {
  return Boolean(n.onApprove || n.onDeny);
}

export function toastQueueReducer(queue: ToastQueue, action: ToastQueueAction): ToastQueue {
  switch (action.type) {
    case "add": {
      const { notification } = action;
      // One toast per session: the latest replaces the earlier (history keeps both).
      // An approval toast is never displaced by a non-approval one — it needs resolving.
      const existing = queue.find((n) => n.sessionId === notification.sessionId);
      if (existing && isApprovalToast(existing) && !isApprovalToast(notification)) return queue;
      const repeated =
        existing && existing.notificationType === notification.notificationType
          ? { ...notification, repeatCount: (existing.repeatCount ?? 1) + 1 }
          : notification;
      return [...queue.filter((n) => n.sessionId !== notification.sessionId), repeated];
    }
    case "append": {
      const { notification, replaceKey } = action;
      const kept =
        replaceKey === undefined
          ? queue
          : queue.filter((n) => n.metadata?.actionToastKey !== replaceKey);
      return [...kept, notification];
    }
    case "remove":
      return queue.some((n) => action.ids.has(n.id)) ? queue.filter((n) => !action.ids.has(n.id)) : queue;
    case "removeByApprovalId":
      return queue.filter((n) => n.metadata?.approval_id !== action.approvalId);
    case "removeBySessionIds":
      return queue.filter((n) => !action.sessionIds.has(n.sessionId ?? ""));
    case "clearUnpinned": {
      const next = queue.filter(isPinned);
      return next.length === queue.length ? queue : next;
    }
    case "restore": {
      const present = new Set(queue.map((n) => n.id));
      const sessions = new Set(queue.map((n) => n.sessionId).filter(Boolean));
      const back = action.toasts.filter(
        (n) => !present.has(n.id) && !(n.sessionId && sessions.has(n.sessionId)),
      );
      return back.length === 0 ? queue : [...back, ...queue];
    }
    case "prune": {
      const next = queue.filter((n) => {
        if (action.keep?.(n)) return true;
        const window = hasLongToastLifetime(n.notificationType) ? ACTIONABLE_TOAST_STALE_MS : TOAST_STALE_MS;
        return action.now - n.timestamp < window;
      });
      return next.length === queue.length ? queue : next;
    }
  }
}

/**
 * Toast queue as a synchronously-readable reducer: `queueRef.current` is always the
 * latest state, so a command can compute from it without a state updater (which
 * must stay pure) and without waiting for a render.
 */
export function useToastQueue() {
  const queueRef = useRef<ToastQueue>([]);
  const [queue, setQueue] = useState<ToastQueue>([]);

  const dispatch = useCallback((action: ToastQueueAction) => {
    const next = toastQueueReducer(queueRef.current, action);
    if (next === queueRef.current) return;
    queueRef.current = next;
    setQueue(next);
  }, []);

  return { queue, queueRef, dispatch };
}
