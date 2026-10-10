// +feature: notification-toast-stack
"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { zIndex } from "@/styles/theme.css";
import { NotificationToast } from "@/components/ui/NotificationToast";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import type { ToastTimerRegistry } from "@/lib/hooks/useToastTimers";
import { toastAutoCloseMs, toastAutoMinimizeMs } from "@/lib/notification-policy";
import type { NotificationData } from "@/lib/types/notification";

const EXIT_ANIMATION_MS = 300;

interface ToastStackProps {
  toasts: NotificationData[];
  timers: ToastTimerRegistry;
  onRemove: (id: string) => void;
}

interface ToastSlotProps {
  notification: NotificationData;
  timers: ToastTimerRegistry;
  onRemove: (id: string) => void;
}

/**
 * One toast and its timers. Every timer goes through the registry so the stack,
 * and not the card, decides when a toast closes, minimizes or exits.
 */
function ToastSlot({ notification, timers, onRemove }: ToastSlotProps) {
  const { id } = notification;
  // The audit-log object is new every render; a ref keeps timer effects from restarting.
  const auditLog = useAuditLog();
  const auditLogRef = useRef(auditLog);
  auditLogRef.current = auditLog;
  const [presented, setPresented] = useState(false);
  const [exiting, setExiting] = useState(false);
  const [minimized, setMinimized] = useState(false);
  const closingRef = useRef(false);

  const requestClose = useCallback(
    ({ acknowledge = false }: { acknowledge?: boolean } = {}) => {
      if (closingRef.current) return;
      closingRef.current = true;
      setExiting(true);
      auditLogRef.current.logNotificationDismissed(id, notification.sessionId);
      timers.register(id, "exit", EXIT_ANIMATION_MS, () => {
        notification.onDismiss?.();
        if (acknowledge) notification.onAcknowledge?.();
        onRemove(id);
      });
    },
    [id, notification, onRemove, timers],
  );

  const onPresent = useCallback(() => {
    setPresented(true);
    return () => setPresented(false);
  }, []);

  useEffect(() => {
    if (!presented) return;
    const autoClose = toastAutoCloseMs(notification.notificationType);
    if (autoClose > 0) timers.register(id, "close", autoClose, () => requestClose());
    return () => timers.cancel(id, "close");
  }, [presented, id, notification.notificationType, requestClose, timers]);

  useEffect(() => {
    if (!presented || minimized) return;
    const autoMinimize = toastAutoMinimizeMs(notification.notificationType);
    if (autoMinimize > 0) timers.register(id, "minimize", autoMinimize, () => setMinimized(true));
    return () => timers.cancel(id, "minimize");
  }, [presented, minimized, id, notification.notificationType, timers]);

  useEffect(() => () => timers.cancel(id), [id, timers]);

  return (
    <NotificationToast
      notification={notification}
      exiting={exiting}
      minimized={minimized}
      onExpand={() => setMinimized(false)}
      onClose={requestClose}
      onPresent={onPresent}
    />
  );
}

/** Renders the active toasts. The provider owns the queue; this owns presentation and per-toast timers. */
export function ToastStack({ toasts, timers, onRemove }: ToastStackProps) {
  return (
    <div
      style={{
        position: "fixed",
        bottom: 0,
        right: 0,
        zIndex: zIndex.toast,
        pointerEvents: "none",
      }}
    >
      {toasts.map((notification) => (
        <div key={notification.id} style={{ pointerEvents: "auto" }}>
          <ToastSlot notification={notification} timers={timers} onRemove={onRemove} />
        </div>
      ))}
    </div>
  );
}
