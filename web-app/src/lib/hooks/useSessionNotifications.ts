"use client";

import { useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { NotificationEvent } from "@/gen/session/v1/events_pb";
import { NotificationType } from "@/gen/session/v1/types_pb";
import { SessionService } from "@/gen/session/v1/session_pb";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { NotificationData } from "@/lib/types/notification";
import { mapNotificationType, mapPriority } from "@/lib/utils/notificationMapping";
import { TOAST_DEDUP_WINDOW_MS, nativeAutoCloseMs, NATIVE_ACTIONABLE_TTL_MS } from "@/lib/notification-policy";
import { getConnectTransport } from "@/lib/api/transport";
import { showBrowserNotification, playPriorityNotificationSound } from "@/lib/utils/notifications";

/**
 * Notification types that should only appear in history — no toast, no sound.
 * These are informational events where interrupting the user adds no value.
 */
const HISTORY_ONLY_TYPES = new Set([
  NotificationType.TASK_COMPLETE,
  NotificationType.PROCESS_FINISHED,
  NotificationType.PROCESS_STARTED,
  NotificationType.STATUS_CHANGE,
  NotificationType.INFO,
  NotificationType.DEBUG,
]);

/**
 * Calls resolveApproval RPC to allow or deny a pending tool use.
 */
async function resolveApproval(approvalId: string, decision: "allow" | "deny"): Promise<void> {
  try {
    const client = createClient(SessionService, getConnectTransport());
    await client.resolveApproval({ approvalId, decision });
  } catch (error) {
    console.error(`[resolveApproval] Failed to resolve approval ${approvalId}:`, error);
  }
}

/**
 * Calls the FocusWindow API to bring an application window to front
 */
async function focusWindow(bundleId?: string, appName?: string): Promise<void> {
  if (!bundleId && !appName) return;

  try {
    const client = createClient(SessionService, getConnectTransport());
    const response = await client.focusWindow({
      bundleId: bundleId,
      appName: appName,
    });

    // Check if the server reported a permissions issue
    if (!response.success && response.message) {
      console.warn("Focus window failed:", response.message);
    }
  } catch (error) {
    console.warn("Failed to focus window:", error);
  }
}

interface UseSessionNotificationsOptions {
  /** Enable audio chimes (default: true) */
  enableAudio?: boolean;
  /** Callback when user clicks "View" on a notification */
  onViewSession?: (sessionId: string) => void;
}

/**
 * Hook that handles session notification events from the server.
 * Creates notification toasts and plays audio chimes based on priority.
 *
 * @returns A callback to handle NotificationEvent from useSessionService
 */
export function useSessionNotifications(options: UseSessionNotificationsOptions = {}) {
  const { enableAudio = true, onViewSession } = options;
  const { addNotification, addToHistoryOnly } = useNotifications();

  // Use refs to avoid recreating callback when dependencies change
  const enableAudioRef = useRef(enableAudio);
  const onViewSessionRef = useRef(onViewSession);

  // Dedup cache: maps "sessionId:notificationType" -> timestamp of last shown toast
  const recentToastKeys = useRef<Map<string, number>>(new Map());

  useEffect(() => {
    enableAudioRef.current = enableAudio;
  }, [enableAudio]);

  useEffect(() => {
    onViewSessionRef.current = onViewSession;
  }, [onViewSession]);

  const handleNotification = useCallback((event: NotificationEvent) => {
    // --- Toast deduplication ---
    // Within a 10-second window, suppress audio and native notifications for
    // repeat (sessionId, notificationType) events. However, visible toasts are
    // still refreshed with the latest content so that rapidly-updating system
    // alerts (e.g. fork-pressure) replace their stale toast rather than going
    // silent until the window expires.
    const dedupKey = `${event.sessionId}:${event.notificationType}`;
    const now = Date.now();

    // Prune stale entries before reading — ensures lastShown reflects the
    // post-prune state, so a boundary-case entry doesn't linger in the map.
    for (const [key, ts] of recentToastKeys.current) {
      if (now - ts >= TOAST_DEDUP_WINDOW_MS) {
        recentToastKeys.current.delete(key);
      }
    }

    const lastShown = recentToastKeys.current.get(dedupKey);

    // Never suppress approval_needed or question notifications — each one blocks Claude and requires a response.
    const isApproval = event.notificationType === NotificationType.APPROVAL_NEEDED ||
      event.notificationType === NotificationType.INPUT_REQUIRED;
    const isDuplicate = !isApproval && !!lastShown && now - lastShown < TOAST_DEDUP_WINDOW_MS;

    // History-only types: no toast, no sound — just record in the history panel.
    // Duplicates are fully suppressed (no visible toast to refresh).
    if (HISTORY_ONLY_TYPES.has(event.notificationType)) {
      if (isDuplicate) return;
      addToHistoryOnly({
        sessionId: event.sessionId,
        sessionName: event.sessionName || "Unknown Session",
        title: event.title,
        message: event.message,
        priority: mapPriority(event.priority),
        notificationType: mapNotificationType(event.notificationType),
        metadata: event.metadata,
        onView: onViewSessionRef.current
          ? () => onViewSessionRef.current?.(event.sessionId)
          : undefined,
      });
      return;
    }

    // Extract source app metadata from the event
    const sourceApp = event.metadata?.["source_app"];
    const sourceBundleId = event.metadata?.["source_bundle"];
    const sourceWorkingDir = event.metadata?.["cwd"];
    const sourceProject = event.metadata?.["source_project"];
    const approvalId = event.metadata?.["approval_id"];

    // Build the notification data with all available fields
    const notificationData: Omit<NotificationData, "id" | "timestamp"> = {
      sessionId: event.sessionId,
      sessionName: event.sessionName || "Unknown Session",
      title: event.title,
      message: event.message,
      priority: mapPriority(event.priority),
      notificationType: mapNotificationType(event.notificationType),
      sourceApp: sourceApp,
      sourceBundleId: sourceBundleId,
      sourceWorkingDir: sourceWorkingDir,
      sourceProject: sourceProject,
      metadata: event.metadata,
      onView: onViewSessionRef.current
        ? () => onViewSessionRef.current?.(event.sessionId)
        : undefined,
      // Add focus window handler if we have source app info
      onFocusWindow: (sourceApp || sourceBundleId)
        ? () => focusWindow(sourceBundleId, sourceApp)
        : undefined,
      // Attach approve/deny callbacks for tool-use approval requests
      onApprove: approvalId ? () => resolveApproval(approvalId, "allow") : undefined,
      onDeny:    approvalId ? () => resolveApproval(approvalId, "deny")  : undefined,
    };

    // Duplicate visible-toast event: refresh the existing toast with updated
    // content (addNotification replaces by sessionId, not stacks) but skip
    // audio and native notification to avoid spamming the user.
    if (isDuplicate) {
      addNotification(notificationData);
      return;
    }

    recentToastKeys.current.set(dedupKey, now);

    // Play audio chime based on priority
    if (enableAudioRef.current) {
      playPriorityNotificationSound(event.priority);
    }

    // Add visual notification
    addNotification(notificationData);

    // Native notification (FR-6): fire alongside toast for non-history types.
    // Guard: only when already granted — we do NOT call requestPermission() here.
    // Requesting permission mid-session triggers a disruptive OS prompt; permission
    // is requested proactively via requestNotificationPermission() in the settings flow.
    if (typeof window !== "undefined" && "Notification" in window && Notification.permission === "granted") {
      const nativeTag = isApproval && event.metadata?.["approval_id"]
        ? `approval:${event.metadata["approval_id"]}`
        : `${event.sessionId}:${mapNotificationType(event.notificationType)}`;

      // Approval body is redacted in the OS tray: shell commands and file paths from
      // tool inputs must not appear in notification center, screen recordings, or
      // lock-screen previews. The in-app toast shows the full content.
      const nativeBody = isApproval
        ? "Open Stapler Squad to review and approve"
        : event.message ?? undefined;

      void showBrowserNotification(event.title, {
        body: nativeBody,
        tag: nativeTag,
        autoCloseMs: isApproval ? NATIVE_ACTIONABLE_TTL_MS : nativeAutoCloseMs(event.priority),
        requireInteraction: isApproval,
      });
    }
  }, [addNotification, addToHistoryOnly]);

  return handleNotification;
}
