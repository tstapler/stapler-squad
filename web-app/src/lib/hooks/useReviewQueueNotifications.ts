"use client";

import { useEffect, useRef } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import {
  playNotificationSound,
  showBrowserNotification,
  NotificationSound,
} from "@/lib/utils/notifications";
import { useNotifications } from "@/lib/contexts/NotificationContext";

interface UseReviewQueueNotificationsOptions {
  /**
   * Enable/disable notifications
   * @default true
   */
  enabled?: boolean;

  /**
   * Sound type to play
   * @default NotificationSound.DING
   */
  soundType?: NotificationSound;

  /**
   * Show browser notification in addition to sound
   * @default true
   */
  showBrowserNotification?: boolean;

  /**
   * Show in-app toast notification
   * @default true
   */
  showToastNotification?: boolean;

  /**
   * Custom notification title
   */
  notificationTitle?: string;

  /**
   * Callback to navigate to a session when clicked
   */
  onNavigateToSession?: (sessionId: string) => void;

  /**
   * Callback when new items are detected
   */
  onNewItems?: (items: ReviewItem[]) => void;
}

/**
 * Hook that monitors review queue items and plays notification sounds
 * when new sessions are added that need user attention.
 *
 * @example
 * ```tsx
 * const { items } = useReviewQueue();
 * useReviewQueueNotifications(items, {
 *   enabled: true,
 *   soundType: NotificationSound.DING,
 *   showBrowserNotification: true,
 * });
 * ```
 */
export function useReviewQueueNotifications(
  items: ReviewItem[],
  options: UseReviewQueueNotificationsOptions = {}
) {
  const {
    enabled = true,
    soundType = NotificationSound.DING,
    showBrowserNotification: showBrowser = true,
    showToastNotification: showToast = true,
    notificationTitle = "Session Needs Attention",
    onNewItems,
    onNavigateToSession,
  } = options;

  const { showSessionNotification } = useNotifications();

  // Track previous items to detect new additions
  const previousItemsRef = useRef<Set<string>>(new Set());
  const isInitialLoadRef = useRef(true);

  useEffect(() => {
    if (!enabled) return;

    // Build current item set
    const currentItemIds = new Set(items.map((item) => item.sessionId));

    // Skip notification on initial load to avoid spurious alerts
    if (isInitialLoadRef.current) {
      isInitialLoadRef.current = false;
      previousItemsRef.current = currentItemIds;
      return;
    }

    // Find new items that weren't in previous set
    const newItemIds = Array.from(currentItemIds).filter(
      (id) => !previousItemsRef.current.has(id)
    );

    if (newItemIds.length > 0) {
      const newItems = items.filter((item) =>
        newItemIds.includes(item.sessionId)
      );

      // Play notification sound
      playNotificationSound(soundType);

      // Show toast notification for each new item
      if (showToast && newItems.length > 0) {
        newItems.forEach((item) => {
          showSessionNotification(item, () => {
            onNavigateToSession?.(item.sessionId);
          });
        });
      }

      // Show browser notification if enabled
      if (showBrowser && newItems.length > 0) {
        const sessionName = newItems[0].sessionName || "Unnamed Session";
        const body =
          newItems.length === 1
            ? `${sessionName} is waiting for your input`
            : `${newItems.length} sessions need your attention`;

        showBrowserNotification(notificationTitle, {
          body,
          tag: "review-queue", // Prevents duplicate notifications
          requireInteraction: false,
          silent: true, // We already played our custom sound
        });
      }

      // Call optional callback
      if (onNewItems) {
        onNewItems(newItems);
      }
    }

    // Update previous items reference
    previousItemsRef.current = currentItemIds;
  }, [items, enabled, soundType, showBrowser, notificationTitle, onNewItems]);

  return {
    // Reset tracking (useful if you want to re-enable after disabling)
    reset: () => {
      previousItemsRef.current = new Set();
      isInitialLoadRef.current = true;
    },
  };
}
