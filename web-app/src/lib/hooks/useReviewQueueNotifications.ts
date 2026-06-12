"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { ReviewItem, AttentionReason } from "@/gen/session/v1/types_pb";
import {
  playNotificationSound,
  showBrowserNotification,
  closeNativeNotification,
  notificationTag,
  NotificationSound,
} from "@/lib/utils/notifications";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import {
  shouldNotify,
  markNotified,
  markAcknowledged,
  markNotifiedBatch,
  cleanupExpired,
} from "@/lib/utils/notificationStorage";

/**
 * Tier 1 — Interrupt: blocks Claude execution, needs immediate user response.
 * Shows persistent toast + OS notification with sound.
 */
const TIER1_REASONS = new Set([
  AttentionReason.APPROVAL_PENDING,
  AttentionReason.INPUT_REQUIRED,
  AttentionReason.WAITING_FOR_USER,
]);

/**
 * Tier 2 — Surface: actionable failure, user should know soon.
 * Shows brief toast (auto-minimizes) + OS notification if tab hidden.
 */
const TIER2_REASONS = new Set([
  AttentionReason.ERROR_STATE,
  AttentionReason.TESTS_FAILING,
  AttentionReason.STALE,
]);

/**
 * Tier 3 — History only: informational, no interrupt.
 * TASK_COMPLETE, IDLE, UNCOMMITTED_CHANGES, IDLE_TIMEOUT → history panel only.
 */
function getTier(reason: AttentionReason): 1 | 2 | 3 {
  if (TIER1_REASONS.has(reason)) return 1;
  if (TIER2_REASONS.has(reason)) return 2;
  return 3;
}

/**
 * Minimum milliseconds an item must remain in the queue before a notification fires.
 * This filters out items that briefly enter the queue during auto-approve processing.
 */
const DWELL_TIME_MS = 3_000;

interface UseReviewQueueNotificationsOptions {
  /**
   * Enable/disable notifications
   * @default true
   */
  enabled?: boolean;

  /**
   * Sound type to play for Tier 1 notifications
   * @default NotificationSound.DING
   */
  soundType?: NotificationSound;

  /**
   * Show browser notification in addition to sound
   * @default true
   */
  showBrowserNotification?: boolean;

  /**
   * Show in-app toast notification for Tier 1/2
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

  /**
   * Callback when a session is acknowledged from the notification toast.
   * This should call the backend acknowledge API.
   */
  onAcknowledge?: (sessionId: string) => void;
}

/**
 * Hook that monitors review queue items and notifies the user based on urgency tier.
 *
 * Tier 1 (APPROVAL_PENDING, INPUT_REQUIRED, WAITING_FOR_USER):
 *   - Persistent toast (no auto-close) + browser notification with OS sound + history
 *
 * Tier 2 (ERROR_STATE, TESTS_FAILING, STALE):
 *   - Brief toast (auto-minimizes) + history only (no browser notification unless tab hidden)
 *
 * Tier 3 (TASK_COMPLETE, IDLE, UNCOMMITTED_CHANGES, etc.):
 *   - History panel only — no toast, no sound, no interruption
 *
 * Dwell-time filter: items must remain in the queue for at least 3 seconds before
 * a notification fires. This prevents spurious notifications from auto-approved items
 * that briefly appear in the queue while the classifier is running.
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
    onAcknowledge,
  } = options;

  const { showSessionNotification, addToHistoryOnly, markAsReadBySessionId, removeToastBySessionId } = useNotifications();

  // Track previous items to detect removals (used only for markAsReadBySessionId)
  const previousItemsRef = useRef<Set<string>>(new Set());
  const isInitialLoadRef = useRef(true);

  // Dwell-time tracking: maps sessionId -> timestamp when item first appeared
  const itemFirstSeenRef = useRef<Map<string, number>>(new Map());

  // Which items have already fired a notification this queue-lifetime (in-memory dedup).
  // Separate from previousItemsRef so dwell-expired items can be re-evaluated across renders.
  const notifiedItemsRef = useRef<Set<string>>(new Set());

  // Pending dwell timers: maps sessionId -> setTimeout handle.
  // Each new item schedules a timer so the effect re-runs after DWELL_TIME_MS
  // without waiting for the next 30-second REST poll.
  const pendingDwellRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // Incremented by dwell timers to trigger an effect re-run at the right moment.
  const [dwellTick, setDwellTick] = useState(0);

  // Acknowledge handler that updates localStorage and calls backend
  const handleAcknowledge = useCallback(
    (sessionId: string) => {
      markAcknowledged(sessionId);
      onAcknowledge?.(sessionId);
    },
    [onAcknowledge]
  );

  useEffect(() => {
    if (!enabled) return;

    // Periodic cleanup of expired records
    cleanupExpired();

    // Build current item set with reason lookup
    const currentItemMap = new Map(items.map((item) => [item.sessionId, item]));
    const currentItemIds = new Set(currentItemMap.keys());
    const now = Date.now();

    // Remove dwell-time entries for items no longer in the queue
    for (const id of itemFirstSeenRef.current.keys()) {
      if (!currentItemIds.has(id)) {
        itemFirstSeenRef.current.delete(id);
      }
    }

    // On initial load, mark all current items as already notified to prevent duplicate alerts
    if (isInitialLoadRef.current) {
      isInitialLoadRef.current = false;
      markNotifiedBatch(Array.from(currentItemIds));
      previousItemsRef.current = currentItemIds;
      notifiedItemsRef.current = new Set(currentItemIds);
      // Seed dwell-time map so existing items don't fire on first render
      for (const id of currentItemIds) {
        itemFirstSeenRef.current.set(id, 0); // 0 = already present before we started watching
      }
      return;
    }

    // Record first-seen timestamp and schedule a dwell timer for genuinely new items.
    // The timer fires setDwellTick after DWELL_TIME_MS, triggering a re-run of this effect
    // so we don't have to wait for the next 30-second REST poll to evaluate dwell expiry.
    for (const id of currentItemIds) {
      if (!itemFirstSeenRef.current.has(id)) {
        itemFirstSeenRef.current.set(id, now);
        // Only schedule a dwell timer if not already notified (e.g. after a reset)
        if (!notifiedItemsRef.current.has(id)) {
          const timer = setTimeout(() => {
            pendingDwellRef.current.delete(id);
            setDwellTick((n) => n + 1);
          }, DWELL_TIME_MS);
          pendingDwellRef.current.set(id, timer);
        }
      }
    }

    // Find items ready to notify:
    // 1. Not yet notified this queue-lifetime (notifiedItemsRef, not previousItemsRef)
    // 2. Should be notified (not in localStorage grace period)
    // 3. Have dwelled in the queue long enough (dwell-time filter for auto-approve)
    const newItemIds = Array.from(currentItemIds).filter((id) => {
      if (notifiedItemsRef.current.has(id)) return false; // Already notified — skip
      if (!shouldNotify(id)) return false;
      const firstSeen = itemFirstSeenRef.current.get(id) ?? now;
      if (firstSeen === 0) return false; // Was already present at initial load
      return now - firstSeen >= DWELL_TIME_MS;
    });

    if (newItemIds.length > 0) {
      const newItems = newItemIds
        .map((id) => currentItemMap.get(id))
        .filter((item): item is ReviewItem => item !== undefined);

      markNotifiedBatch(newItemIds);
      newItemIds.forEach((id) => notifiedItemsRef.current.add(id));

      // Split by tier
      const tier1Items = newItems.filter((item) => getTier(item.reason) === 1);
      const tier2Items = newItems.filter((item) => getTier(item.reason) === 2);
      const tier3Items = newItems.filter((item) => getTier(item.reason) === 3);

      // Tier 1: interrupt toast + sound + browser notification
      if (tier1Items.length > 0) {
        playNotificationSound(soundType);

        if (showToast) {
          tier1Items.forEach((item) => {
            showSessionNotification(
              item,
              () => { onNavigateToSession?.(item.sessionId); },
              () => { handleAcknowledge(item.sessionId); }
            );
          });
        }

        if (showBrowser && tier1Items.length > 0) {
          const sessionName = tier1Items[0].sessionName || "Unnamed Session";
          const body =
            tier1Items.length === 1
              ? `${sessionName} is waiting for your input`
              : `${tier1Items.length} sessions need your approval`;

          showBrowserNotification(notificationTitle, {
            body,
            tag: notificationTag.tier1Review(tier1Items[0].sessionId),
            requireInteraction: true, // Tier 1: persist in OS notification center
          });
        }
      }

      // Tier 2: brief toast (auto-minimizes via NotificationToast) + silent browser notification if hidden
      if (tier2Items.length > 0) {
        if (showToast) {
          tier2Items.forEach((item) => {
            showSessionNotification(
              item,
              () => { onNavigateToSession?.(item.sessionId); },
              () => { handleAcknowledge(item.sessionId); }
            );
          });
        }

        if (showBrowser && typeof document !== "undefined" && document.hidden) {
          const sessionName = tier2Items[0].sessionName || "Unnamed Session";
          const body =
            tier2Items.length === 1
              ? `${sessionName} has an issue`
              : `${tier2Items.length} sessions have issues`;

          showBrowserNotification(notificationTitle, {
            body,
            tag: `review-queue-tier2`,
            requireInteraction: false,
            silent: true, // Tier 2: no OS sound, we're informing not interrupting
          });
        }
      }

      // Tier 3: history only — no toast, no sound
      if (tier3Items.length > 0) {
        tier3Items.forEach((item) => {
          addToHistoryOnly({
            sessionId: item.sessionId,
            sessionName: item.sessionName || "Unnamed Session",
            message: item.context || "Task completed",
            priority: "low",
            notificationType: "task_complete",
            onView: () => { onNavigateToSession?.(item.sessionId); },
          });
        });
      }

      if (onNewItems) {
        onNewItems(newItems);
      }
    }

    // Items that left the queue — mark their notifications as read and reset local state
    // so that if they re-enter the queue (e.g. another approval) they can notify again.
    const removedIds = Array.from(previousItemsRef.current).filter(
      (id) => !currentItemIds.has(id)
    );
    if (removedIds.length > 0) {
      markAsReadBySessionId(removedIds);
      removeToastBySessionId(removedIds); // FR-4: dismiss active toasts for removed sessions
      removedIds.forEach((id) => {
        notifiedItemsRef.current.delete(id);
        // Cancel any pending dwell timer for items that left before dwell expired
        const timer = pendingDwellRef.current.get(id);
        if (timer !== undefined) {
          clearTimeout(timer);
          pendingDwellRef.current.delete(id);
        }
        // FR-5: close native OS notification for this session
        closeNativeNotification(notificationTag.tier1Review(id));
      });
    }

    previousItemsRef.current = currentItemIds;
  }, [
    items,
    enabled,
    dwellTick, // Re-run when a dwell timer fires so we can evaluate items without waiting for next poll
    soundType,
    showBrowser,
    showToast,
    notificationTitle,
    onNewItems,
    onNavigateToSession,
    handleAcknowledge,
    markAsReadBySessionId,
    removeToastBySessionId,
    showSessionNotification,
    addToHistoryOnly,
  ]);

  return {
    reset: () => {
      // Cancel all pending dwell timers before resetting state
      for (const timer of pendingDwellRef.current.values()) {
        clearTimeout(timer);
      }
      pendingDwellRef.current.clear();
      previousItemsRef.current = new Set();
      itemFirstSeenRef.current = new Map();
      notifiedItemsRef.current = new Set();
      isInitialLoadRef.current = true;
    },
    acknowledge: handleAcknowledge,
    markNotified,
  };
}
