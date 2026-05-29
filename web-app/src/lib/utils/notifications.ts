"use client";

/**
 * Notification utilities for audio alerts and browser notifications
 */

import { NATIVE_MEDIUM_TTL_MS, NATIVE_ACTIONABLE_TTL_MS } from "@/lib/notification-policy";
import { NotificationPriority } from "@/gen/session/v1/types_pb";

/**
 * Notification sound types
 */
export enum NotificationSound {
  DING = "ding",
  CHIME = "chime",
  ALERT = "alert",
}

/**
 * Returns the shared AudioContext for the page lifetime, creating it on first use.
 * A singleton avoids accumulating OS-level audio handles — each `new AudioContext()`
 * allocates a native context that is never released unless explicitly `.close()`d.
 */
let _sharedAudioCtx: AudioContext | null = null;
function getAudioContext(): AudioContext | null {
  if (typeof window === "undefined") return null;
  try {
    if (!_sharedAudioCtx || _sharedAudioCtx.state === "closed") {
      _sharedAudioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
    }
    return _sharedAudioCtx;
  } catch {
    return null;
  }
}

/**
 * Plays a notification sound using the Web Audio API
 * Falls back to a simple beep if sound synthesis fails
 */
export function playNotificationSound(
  soundType: NotificationSound = NotificationSound.DING
): void {
  try {
    // Check if user has enabled notifications (localStorage preference)
    const notificationsEnabled = localStorage.getItem("notifications-enabled");
    if (notificationsEnabled === "false") {
      return;
    }

    const audioContext = getAudioContext();
    if (!audioContext) return;

    // Create oscillator for tone generation
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    // Configure sound based on type
    switch (soundType) {
      case NotificationSound.DING:
        // Pleasant ding sound (E note)
        oscillator.frequency.value = 659.25; // E5
        oscillator.type = "sine";
        gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(
          0.01,
          audioContext.currentTime + 0.5
        );
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.5);
        break;

      case NotificationSound.CHIME:
        // Two-tone chime (C -> E)
        oscillator.frequency.value = 523.25; // C5
        oscillator.type = "sine";
        gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
        oscillator.frequency.setValueAtTime(
          659.25,
          audioContext.currentTime + 0.15
        ); // E5
        gainNode.gain.exponentialRampToValueAtTime(
          0.01,
          audioContext.currentTime + 0.6
        );
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.6);
        break;

      case NotificationSound.ALERT:
        // Attention-grabbing alert sound
        oscillator.frequency.value = 800;
        oscillator.type = "square";
        gainNode.gain.setValueAtTime(0.2, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(
          0.01,
          audioContext.currentTime + 0.3
        );
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.3);
        break;
    }
  } catch (error) {
    console.warn("Failed to play notification sound:", error);
  }
}

/**
 * Plays a priority-mapped notification chime using the Web Audio API.
 * URGENT → 3 rapid beeps, HIGH → 2 beeps, MEDIUM → single chime, LOW → soft tone.
 */
export function playPriorityNotificationSound(priority: NotificationPriority): void {
  if (typeof window === "undefined" || !window.AudioContext) return;

  try {
    const audioCtx = getAudioContext();
    if (!audioCtx) return;

    const oscillator = audioCtx.createOscillator();
    const gainNode = audioCtx.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioCtx.destination);

    switch (priority) {
      case NotificationPriority.URGENT:
        // Rapid high-pitched alert (3 quick beeps)
        oscillator.type = "sine";
        oscillator.frequency.setValueAtTime(880, audioCtx.currentTime); // A5
        gainNode.gain.setValueAtTime(0.3, audioCtx.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.1);
        oscillator.start(audioCtx.currentTime);
        oscillator.stop(audioCtx.currentTime + 0.15);

        setTimeout(() => {
          const ctx = getAudioContext();
          if (!ctx) return;
          const osc2 = ctx.createOscillator();
          const gain2 = ctx.createGain();
          osc2.connect(gain2);
          gain2.connect(ctx.destination);
          osc2.type = "sine";
          osc2.frequency.setValueAtTime(880, ctx.currentTime);
          gain2.gain.setValueAtTime(0.3, ctx.currentTime);
          gain2.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.1);
          osc2.start();
          osc2.stop(ctx.currentTime + 0.15);
        }, 150);

        setTimeout(() => {
          const ctx = getAudioContext();
          if (!ctx) return;
          const osc3 = ctx.createOscillator();
          const gain3 = ctx.createGain();
          osc3.connect(gain3);
          gain3.connect(ctx.destination);
          osc3.type = "sine";
          osc3.frequency.setValueAtTime(880, ctx.currentTime);
          gain3.gain.setValueAtTime(0.3, ctx.currentTime);
          gain3.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.1);
          osc3.start();
          osc3.stop(ctx.currentTime + 0.15);
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
          const ctx = getAudioContext();
          if (!ctx) return;
          const osc2 = ctx.createOscillator();
          const gain2 = ctx.createGain();
          osc2.connect(gain2);
          gain2.connect(ctx.destination);
          osc2.type = "sine";
          osc2.frequency.setValueAtTime(784, ctx.currentTime); // G5
          gain2.gain.setValueAtTime(0.2, ctx.currentTime);
          gain2.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.15);
          osc2.start();
          osc2.stop(ctx.currentTime + 0.2);
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

/**
 * Extended options for showBrowserNotification — superset of NotificationOptions
 * with an optional auto-close override.
 */
export interface BrowserNotificationOptions extends NotificationOptions {
  /**
   * Override the auto-close delay in ms.
   * Defaults to NATIVE_ACTIONABLE_TTL_MS when requireInteraction is true,
   * otherwise NATIVE_MEDIUM_TTL_MS.
   */
  autoCloseMs?: number;
}

/**
 * Tracks open native Notification handles by tag for dedup and auto-close (FR-3, FR-4).
 *
 * macOS Notification Center limitation: notification.close() dismisses the banner
 * but cannot remove NC entries that the user has already swiped into the NC tray.
 * This is a known browser/OS limitation; no workaround is attempted here.
 */
const activeNativeNotifications = new Map<string, Notification>();

/**
 * Shows a browser notification if permission is granted.
 * Falls back to audio-only if notifications are not supported or denied.
 *
 * Implements:
 * - FR-3: Auto-close via setTimeout after autoCloseMs
 * - FR-4: Close-before-open dedup via activeNativeNotifications Map
 */
export async function showBrowserNotification(
  title: string,
  options?: BrowserNotificationOptions
): Promise<void> {
  // Check if notifications are enabled
  const notificationsEnabled = localStorage.getItem("notifications-enabled");
  if (notificationsEnabled === "false") {
    return;
  }

  // Check if browser supports notifications
  if (!("Notification" in window)) {
    console.warn("Browser does not support notifications");
    return;
  }

  // Request permission if needed
  if (Notification.permission === "default") {
    await Notification.requestPermission();
  }

  // Show notification if permission granted
  if (Notification.permission === "granted") {
    const tag = options?.tag ?? "__untagged__";

    // Close previous notification for this tag (FR-4)
    try {
      activeNativeNotifications.get(tag)?.close();
    } catch (e) {
      if (process.env.NODE_ENV === "development") {
        console.warn("[notifications] close() on prior notification failed:", e);
      }
    }

    // Strip autoCloseMs from native NotificationOptions before passing to constructor
    const { autoCloseMs: _autoCloseMs, ...notifOptions } = options ?? {};
    const autoCloseMs =
      _autoCloseMs ??
      (options?.requireInteraction ? NATIVE_ACTIONABLE_TTL_MS : NATIVE_MEDIUM_TTL_MS);

    const notif = new Notification(title, {
      icon: "/favicon.ico",
      badge: "/favicon.ico",
      ...notifOptions,
    });
    activeNativeNotifications.set(tag, notif);

    // Auto-close (FR-3)
    const timerId = setTimeout(() => {
      try {
        notif.close();
      } catch (e) {
        if (process.env.NODE_ENV === "development") {
          console.warn("[notifications] auto-close failed:", e);
        }
      }
      if (activeNativeNotifications.get(tag) === notif) {
        activeNativeNotifications.delete(tag);
      }
    }, autoCloseMs);

    // Clean up map entry when the OS closes the notification
    notif.onclose = () => {
      clearTimeout(timerId);
      if (activeNativeNotifications.get(tag) === notif) {
        activeNativeNotifications.delete(tag);
      }
    };
  }
}

/**
 * Gets the current notification preference from localStorage
 */
export function getNotificationPreference(): boolean {
  const stored = localStorage.getItem("notifications-enabled");
  // Default to enabled if not set
  return stored !== "false";
}

/**
 * Sets the notification preference in localStorage
 */
export function setNotificationPreference(enabled: boolean): void {
  localStorage.setItem("notifications-enabled", enabled.toString());
}

/**
 * Requests notification permission from the browser
 * Returns true if permission was granted
 */
export async function requestNotificationPermission(): Promise<boolean> {
  if (!("Notification" in window)) {
    return false;
  }

  if (Notification.permission === "granted") {
    return true;
  }

  const permission = await Notification.requestPermission();
  return permission === "granted";
}
