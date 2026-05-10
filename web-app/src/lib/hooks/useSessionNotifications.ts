"use client";

import { useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { NotificationEvent } from "@/gen/session/v1/events_pb";
import { NotificationPriority, NotificationType } from "@/gen/session/v1/types_pb";
import { SessionService } from "@/gen/session/v1/session_pb";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { NotificationData } from "@/lib/types/notification";
import { mapNotificationType, mapPriority } from "@/lib/utils/notificationMapping";
import { TOAST_DEDUP_WINDOW_MS } from "@/lib/notification-policy";
import { getConnectTransport } from "@/lib/api/transport";

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

/**
 * Plays notification audio based on priority level
 */
function playNotificationSound(priority: NotificationPriority): void {
  // Use Web Audio API for chimes
  if (typeof window === "undefined" || !window.AudioContext) return;

  try {
    const audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
    const oscillator = audioCtx.createOscillator();
    const gainNode = audioCtx.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioCtx.destination);

    // Different frequencies and durations for different priorities
    switch (priority) {
      case NotificationPriority.URGENT:
        // Rapid high-pitched alert (3 quick beeps)
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(880, audioCtx.currentTime); // A5
        gainNode.gain.setValueAtTime(0.3, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.1);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.15);

        // Second beep
        setTimeout(() => {
          const osc2 = audioCtx.createOscillator();
          const gain2 = audioCtx.createGain();
          osc2.connect(gain2);
          gain2.connect(audioCtx.destination);
          osc2.type = "sine";
          osc2.frequency.setValueAtTime(880, audioCtx.currentTime);
          gain2.gain.setValueAtTime(0.3, audioCtx.currentTime);
          gain2.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.1);
          osc2.start();
          osc2.stop(audioCtx.currentTime + 0.15);
        }, 150);

        // Third beep
        setTimeout(() => {
          const osc3 = audioCtx.createOscillator();
          const gain3 = audioCtx.createGain();
          osc3.connect(gain3);
          gain3.connect(audioCtx.destination);
          osc3.type = "sine";
          osc3.frequency.setValueAtTime(880, audioCtx.currentTime);
          gain3.gain.setValueAtTime(0.3, audioCtx.currentTime);
          gain3.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.1);
          osc3.start();
          osc3.stop(audioCtx.currentTime + 0.15);
        }, 300);
        break;

      case NotificationPriority.HIGH:
        // Double beep
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(659, audioCtx.currentTime); // E5
        gainNode.gain.setValueAtTime(0.2, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.15);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.2);

        setTimeout(() => {
          const osc2 = audioCtx.createOscillator();
          const gain2 = audioCtx.createGain();
          osc2.connect(gain2);
          gain2.connect(audioCtx.destination);
          osc2.type = "sine";
          osc2.frequency.setValueAtTime(784, audioCtx.currentTime); // G5
          gain2.gain.setValueAtTime(0.2, audioCtx.currentTime);
          gain2.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.15);
          osc2.start();
          osc2.stop(audioCtx.currentTime + 0.2);
        }, 200);
        break;

      case NotificationPriority.MEDIUM:
        // Single soft chime
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(523, audioCtx.currentTime); // C5
        gainNode.gain.setValueAtTime(0.15, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.4);
        break;

      case NotificationPriority.LOW:
        // Very soft, low tone
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(392, audioCtx.currentTime); // G4
        gainNode.gain.setValueAtTime(0.08, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.2);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.25);
        break;

      default:
        // Default medium chime
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(523, audioCtx.currentTime);
        gainNode.gain.setValueAtTime(0.1, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.4);
    }
  } catch (e) {
    console.warn("Failed to play notification sound:", e);
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
    // Suppress duplicate toasts for the same (sessionId, notificationType)
    // within a 10-second window. The server handles history-store dedup
    // independently; this only prevents redundant UI toasts.
    const DEDUP_WINDOW_MS = TOAST_DEDUP_WINDOW_MS;
    const dedupKey = `${event.sessionId}:${event.notificationType}`;
    const now = Date.now();
    const lastShown = recentToastKeys.current.get(dedupKey);

    // Prune stale entries to prevent unbounded map growth
    for (const [key, ts] of recentToastKeys.current) {
      if (now - ts >= DEDUP_WINDOW_MS) {
        recentToastKeys.current.delete(key);
      }
    }

    // Never suppress approval_needed or question notifications — each one blocks Claude and requires a response.
    const isApproval = event.notificationType === NotificationType.APPROVAL_NEEDED ||
      event.notificationType === NotificationType.INPUT_REQUIRED;
    if (!isApproval && lastShown && now - lastShown < DEDUP_WINDOW_MS) {
      // Duplicate toast suppressed — event still reaches history store via server
      return;
    }
    recentToastKeys.current.set(dedupKey, now);

    // History-only types: no toast, no sound — just record in the history panel
    if (HISTORY_ONLY_TYPES.has(event.notificationType)) {
      const notificationData: Omit<NotificationData, "id" | "timestamp"> = {
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
      };
      addToHistoryOnly(notificationData);
      return;
    }

    // Play audio chime based on priority
    if (enableAudioRef.current) {
      playNotificationSound(event.priority);
    }

    // Extract source app metadata from the event
    const sourceApp = event.metadata?.["source_app"];
    const sourceBundleId = event.metadata?.["source_bundle"];
    const sourceWorkingDir = event.metadata?.["cwd"];
    const sourceProject = event.metadata?.["source_project"];

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
      onApprove: event.metadata?.["approval_id"] ? () => resolveApproval(event.metadata?.["approval_id"]!, "allow") : undefined,
      onDeny: event.metadata?.["approval_id"] ? () => resolveApproval(event.metadata?.["approval_id"]!, "deny") : undefined,
    };

    // Add visual notification
    addNotification(notificationData);
  }, [addNotification, addToHistoryOnly]);

  return handleNotification;
}
