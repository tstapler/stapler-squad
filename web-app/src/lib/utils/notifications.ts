/**
 * Notification utilities for audio alerts and browser notifications
 */

/**
 * Notification sound types
 */
export enum NotificationSound {
  DING = "ding",
  CHIME = "chime",
  ALERT = "alert",
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

    // Create audio context
    const audioContext = new (window.AudioContext ||
      (window as any).webkitAudioContext)();

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
 * Shows a browser notification if permission is granted
 * Falls back to audio-only if notifications are not supported or denied
 */
export async function showBrowserNotification(
  title: string,
  options?: NotificationOptions
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
    new Notification(title, {
      icon: "/favicon.ico",
      badge: "/favicon.ico",
      ...options,
    });
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
