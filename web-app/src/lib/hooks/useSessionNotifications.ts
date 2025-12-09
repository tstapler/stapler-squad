"use client";

import { useCallback, useEffect, useRef } from "react";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { NotificationEvent } from "@/gen/session/v1/events_pb";
import { NotificationPriority } from "@/gen/session/v1/types_pb";

/**
 * Maps protobuf NotificationPriority enum to UI priority string
 */
function mapPriority(priority: NotificationPriority): "urgent" | "high" | "medium" | "low" {
  switch (priority) {
    case NotificationPriority.URGENT:
      return "urgent";
    case NotificationPriority.HIGH:
      return "high";
    case NotificationPriority.MEDIUM:
      return "medium";
    case NotificationPriority.LOW:
      return "low";
    default:
      return "medium";
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
  const { addNotification } = useNotifications();

  // Use refs to avoid recreating callback when dependencies change
  const enableAudioRef = useRef(enableAudio);
  const onViewSessionRef = useRef(onViewSession);

  useEffect(() => {
    enableAudioRef.current = enableAudio;
  }, [enableAudio]);

  useEffect(() => {
    onViewSessionRef.current = onViewSession;
  }, [onViewSession]);

  const handleNotification = useCallback((event: NotificationEvent) => {
    // Play audio chime based on priority
    if (enableAudioRef.current) {
      playNotificationSound(event.priority);
    }

    // Add visual notification
    addNotification({
      sessionId: event.sessionId,
      sessionName: event.sessionName || "Unknown Session",
      message: event.message || event.title,
      priority: mapPriority(event.priority),
      onView: onViewSessionRef.current
        ? () => onViewSessionRef.current?.(event.sessionId)
        : undefined,
    });
  }, [addNotification]);

  return handleNotification;
}
