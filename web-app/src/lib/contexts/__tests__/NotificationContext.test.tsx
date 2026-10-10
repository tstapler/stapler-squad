import React from "react";
import { renderHook, act } from "@testing-library/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import { TOAST_STALE_MS, ACTIONABLE_TOAST_STALE_MS } from "@/lib/notification-policy";
import type { NotificationData } from "@/lib/types/notification";
import type { ReviewItem } from "@/gen/session/v1/types_pb";

// AttentionReason numeric values — matches proto/session/v1/types.proto
const AttentionReason = {
  UNSPECIFIED: 0,
  APPROVAL_PENDING: 1,
  INPUT_REQUIRED: 2,
  ERROR_STATE: 3,
  IDLE_TIMEOUT: 4,
  TASK_COMPLETE: 5,
  UNCOMMITTED_CHANGES: 6,
  IDLE: 7,
  STALE: 8,
  WAITING_FOR_USER: 9,
  TESTS_FAILING: 10,
} as const;

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockMarkAsRead = jest.fn().mockResolvedValue(undefined);
const mockClearHistory = jest.fn().mockResolvedValue(undefined);

// Capture the subscribe handler so tests can simulate cross-tab messages.
let capturedSubscribeHandler: ((msg: any) => void) | null = null;
let mockBroadcast = jest.fn();

jest.mock("@/lib/utils/broadcastChannel", () => ({
  createNotificationSyncChannel: () => ({
    broadcast: mockBroadcast,
    subscribe: (handler: (msg: any) => void) => {
      capturedSubscribeHandler = handler;
      return () => { capturedSubscribeHandler = null; };
    },
  }),
}));

const mockFlags: Record<string, boolean> = {};
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => mockFlags[name] ?? false,
}));

jest.mock("@/lib/utils/notificationStorage", () => ({
  markAcknowledged: jest.fn(),
}));

let mockServerUnread = 0;
jest.mock("@/lib/hooks/useNotificationHistory", () => ({
  useNotificationHistory: () => ({
    notifications: [],
    unreadCount: mockServerUnread,
    loading: false,
    error: null,
    hasMore: false,
    markAsRead: mockMarkAsRead,
    clearHistory: mockClearHistory,
    loadMore: jest.fn().mockResolvedValue(undefined),
    refresh: jest.fn().mockResolvedValue(undefined),
  }),
}));

jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({
    logNotificationDismissed: jest.fn(),
    logNotificationMarkedRead: jest.fn(),
    logNotificationPanelOpened: jest.fn(),
    logNotificationPanelClosed: jest.fn(),
    logNotificationMarkedAllRead: jest.fn(),
    logNotificationRemoved: jest.fn(),
    logNotificationHistoryCleared: jest.fn(),
    logNotificationSessionViewed: jest.fn(),
    logNotificationViewed: jest.fn(),
  }),
}));

jest.mock("@/lib/utils/notificationGrouping", () => ({
  groupNotifications: (items: any[]) =>
    items.map((item) => ({ notification: item, count: 1, allIds: [item.id] })),
}));

// Prevent toast component timers from interfering with context tests
jest.mock("@/components/ui/NotificationToast", () => ({
  NotificationToast: () => null,
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function wrapper({ children }: { children: React.ReactNode }) {
  return <NotificationProvider>{children}</NotificationProvider>;
}

function makeNotification(
  overrides: Partial<Omit<NotificationData, "id" | "timestamp">> = {}
): Omit<NotificationData, "id" | "timestamp"> {
  return {
    sessionId: "session-1",
    sessionName: "Test Session",
    message: "Something needs your attention",
    ...overrides,
  };
}

function makeReviewItem(reason: number, sessionId = "session-1"): ReviewItem {
  return {
    sessionId,
    sessionName: "Test Session",
    reason,
    context: "Some context",
    priority: 2,
  } as unknown as ReviewItem;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("NotificationContext", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedSubscribeHandler = null;
    mockBroadcast = jest.fn();
  });

  describe("addNotification", () => {
    it("adds to active toasts", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].sessionId).toBe("session-1");
    });

    it("adds to notification history", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });

      expect(result.current.notificationHistory).toHaveLength(1);
      expect(result.current.notificationHistory[0].isRead).toBe(false);
    });

    it("replaces existing toast from the same session (no stacking)", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ message: "first" }));
      });
      act(() => {
        result.current.addNotification(makeNotification({ message: "second" }));
      });

      // Only one toast visible
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].message).toBe("second");

      // Both in history
      expect(result.current.notificationHistory).toHaveLength(2);
    });

    it("allows simultaneous toasts from different sessions", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
        result.current.addNotification(makeNotification({ sessionId: "s2" }));
      });

      expect(result.current.notifications).toHaveLength(2);
    });

    it("assigns id and timestamp automatically", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });

      const n = result.current.notifications[0];
      expect(n.id).toBeTruthy();
      expect(n.timestamp).toBeGreaterThan(0);
    });
  });

  describe("addToHistoryOnly", () => {
    it("does not add an active toast", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addToHistoryOnly(makeNotification());
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("does add to notification history", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addToHistoryOnly(makeNotification());
      });

      expect(result.current.notificationHistory).toHaveLength(1);
    });
  });

  describe("removeNotification", () => {
    it("removes the toast from active list", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.removeNotification(id);
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("leaves the notification in history (not an acknowledge)", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.removeNotification(id);
      });

      expect(result.current.notificationHistory).toHaveLength(1);
      expect(result.current.notificationHistory[0].isRead).toBe(false);
    });

    it("does not call backend markAsRead", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.removeNotification(id);
      });

      expect(mockMarkAsRead).not.toHaveBeenCalled();
    });
  });

  describe("acknowledgeNotification", () => {
    it("removes the toast from active list", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(id);
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("marks the notification as read in history", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(id);
      });

      expect(result.current.notificationHistory[0].isRead).toBe(true);
    });

    it("persists the read state to the backend", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(id);
      });

      expect(mockMarkAsRead).toHaveBeenCalledWith([id]);
    });

    it("leaves notification in history (does not delete the record)", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(id);
      });

      expect(result.current.notificationHistory).toHaveLength(1);
    });

    it("accepts an array of ids and acknowledges all", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
        result.current.addNotification(makeNotification({ sessionId: "s2" }));
      });
      const ids = result.current.notifications.map((n) => n.id);

      act(() => {
        result.current.acknowledgeNotification(ids);
      });

      expect(result.current.notifications).toHaveLength(0);
      expect(result.current.notificationHistory.every((n) => n.isRead)).toBe(true);
      expect(mockMarkAsRead).toHaveBeenCalledWith(ids);
    });

    it("is idempotent — acknowledging an already-closed id is a no-op", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification());
      });
      const id = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(id);
        result.current.acknowledgeNotification(id);
      });

      expect(result.current.notifications).toHaveLength(0);
      expect(result.current.notificationHistory).toHaveLength(1);
    });
  });

  describe("stale sweep", () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    it("removes non-actionable toast after TOAST_STALE_MS", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "info" }));
      });
      expect(result.current.notifications).toHaveLength(1);

      act(() => {
        jest.advanceTimersByTime(TOAST_STALE_MS);
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("keeps non-actionable toast before TOAST_STALE_MS", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "info" }));
      });

      act(() => {
        jest.advanceTimersByTime(TOAST_STALE_MS - 60_001); // just before the sweep would remove it
      });

      expect(result.current.notifications).toHaveLength(1);
    });

    it("keeps approval_needed toast past TOAST_STALE_MS", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "approval_needed" }));
      });

      act(() => {
        jest.advanceTimersByTime(TOAST_STALE_MS); // non-actionable would be gone by now
      });

      expect(result.current.notifications).toHaveLength(1);
    });

    it("removes approval_needed toast after ACTIONABLE_TOAST_STALE_MS", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "approval_needed" }));
      });

      act(() => {
        jest.advanceTimersByTime(ACTIONABLE_TOAST_STALE_MS);
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("removes question toast after ACTIONABLE_TOAST_STALE_MS", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "question" }));
      });

      act(() => {
        jest.advanceTimersByTime(ACTIONABLE_TOAST_STALE_MS);
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("stale sweep leaves history untouched", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "info" }));
      });

      act(() => {
        jest.advanceTimersByTime(TOAST_STALE_MS);
      });

      expect(result.current.notifications).toHaveLength(0);
      expect(result.current.notificationHistory).toHaveLength(1);
    });

    it("preserves actionable toasts while sweeping non-actionable ones", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1", notificationType: "info" }));
        result.current.addNotification(makeNotification({ sessionId: "s2", notificationType: "approval_needed" }));
      });

      act(() => {
        jest.advanceTimersByTime(TOAST_STALE_MS);
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].notificationType).toBe("approval_needed");
    });
  });

  describe("showSessionNotification — reviewItemToNotificationType mapping", () => {
    it.each([
      [AttentionReason.APPROVAL_PENDING, "approval_needed"],
      [AttentionReason.WAITING_FOR_USER, "approval_needed"],
      [AttentionReason.INPUT_REQUIRED, "question"],
      [AttentionReason.ERROR_STATE, "error"],
      [AttentionReason.TESTS_FAILING, "error"],
      [AttentionReason.STALE, "warning"],
      [AttentionReason.TASK_COMPLETE, "task_complete"],
      [AttentionReason.IDLE, "info"],
      [AttentionReason.UNSPECIFIED, "info"],
    ])("reason %i → notificationType %s", (reason, expectedType) => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.showSessionNotification(makeReviewItem(reason));
      });

      expect(result.current.notifications[0].notificationType).toBe(expectedType);
    });

    it("populates sessionId, sessionName, and message from the ReviewItem", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.showSessionNotification(makeReviewItem(AttentionReason.APPROVAL_PENDING));
      });

      const n = result.current.notifications[0];
      expect(n.sessionId).toBe("session-1");
      expect(n.sessionName).toBe("Test Session");
      expect(n.message).toBeTruthy();
    });

    it("passes onView and onAcknowledge callbacks through to the notification", () => {
      const onView = jest.fn();
      const onAcknowledge = jest.fn();
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.showSessionNotification(
          makeReviewItem(AttentionReason.APPROVAL_PENDING),
          onView,
          onAcknowledge
        );
      });

      const n = result.current.notifications[0];
      n.onView?.();
      n.onAcknowledge?.();
      expect(onView).toHaveBeenCalledTimes(1);
      expect(onAcknowledge).toHaveBeenCalledTimes(1);
    });
  });

  describe("approval toast guard (addNotification displacement prevention)", () => {
    it("does not displace an approval toast with a non-approval notification from the same session", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });
      const onApprove = jest.fn();
      const onDeny = jest.fn();

      act(() => {
        result.current.addNotification(
          makeNotification({ notificationType: "approval_needed", onApprove, onDeny })
        );
      });
      const approvalId = result.current.notifications[0].id;

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "info" }));
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].id).toBe(approvalId);
      expect(result.current.notifications[0].notificationType).toBe("approval_needed");
    });

    it("allows a new approval-capable notification to replace an existing approval toast", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(
          makeNotification({ message: "first approval", notificationType: "approval_needed", onApprove: jest.fn(), onDeny: jest.fn() })
        );
      });

      act(() => {
        result.current.addNotification(
          makeNotification({ message: "second approval", notificationType: "approval_needed", onApprove: jest.fn(), onDeny: jest.fn() })
        );
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].message).toBe("second approval");
    });

    it("allows notifications from a different session to appear alongside an approval toast", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(
          makeNotification({ sessionId: "s1", notificationType: "approval_needed", onApprove: jest.fn(), onDeny: jest.fn() })
        );
        result.current.addNotification(
          makeNotification({ sessionId: "s2", notificationType: "info" })
        );
      });

      expect(result.current.notifications).toHaveLength(2);
    });

    it("allows normal replacement when no approval toast is active", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ message: "first" }));
      });
      act(() => {
        result.current.addNotification(makeNotification({ message: "second" }));
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].message).toBe("second");
    });

    it("suppressed notification is still recorded in history", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(
          makeNotification({ notificationType: "approval_needed", onApprove: jest.fn(), onDeny: jest.fn() })
        );
      });
      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "info" }));
      });

      // Toast suppressed — only 1 active
      expect(result.current.notifications).toHaveLength(1);
      // Both in history
      expect(result.current.notificationHistory).toHaveLength(2);
    });
  });

  // T-FE-03, T-FE-04
  describe("removeToastBySessionId", () => {
    it("T-FE-03: removes all toasts for the given session, leaves others untouched", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1", message: "s1 toast" }));
        result.current.addNotification(makeNotification({ sessionId: "s2", message: "s2 toast" }));
      });
      expect(result.current.notifications).toHaveLength(2);

      act(() => {
        result.current.removeToastBySessionId("s1");
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].sessionId).toBe("s2");
    });

    it("T-FE-03: does NOT mark history as read (toast-only removal)", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });

      act(() => {
        result.current.removeToastBySessionId("s1");
      });

      expect(result.current.notifications).toHaveLength(0);
      expect(result.current.notificationHistory).toHaveLength(1);
      expect(result.current.notificationHistory[0].isRead).toBe(false);
      expect(mockMarkAsRead).not.toHaveBeenCalled();
    });

    it("T-FE-04: accepts an array of session IDs and removes all matching toasts", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
        result.current.addNotification(makeNotification({ sessionId: "s2" }));
        result.current.addNotification(makeNotification({ sessionId: "s3" }));
      });
      expect(result.current.notifications).toHaveLength(3);

      act(() => {
        result.current.removeToastBySessionId(["s1", "s3"]);
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].sessionId).toBe("s2");
    });

    it("T-FE-04: is a no-op when no toast exists for the given session", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });

      act(() => {
        result.current.removeToastBySessionId("nonexistent");
      });

      expect(result.current.notifications).toHaveLength(1);
    });
  });

  // T-FE-01, T-FE-02
  describe("removeToastByApprovalId", () => {
    it("removes toast whose metadata.approval_id matches, leaves others untouched", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(
          makeNotification({
            sessionId: "s1",
            notificationType: "approval_needed",
            metadata: { approval_id: "appr-123" },
          })
        );
        result.current.addNotification(
          makeNotification({
            sessionId: "s2",
            notificationType: "approval_needed",
            metadata: { approval_id: "appr-456" },
          })
        );
      });

      expect(result.current.notifications).toHaveLength(2);

      act(() => {
        result.current.removeToastByApprovalId("appr-123");
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].metadata?.approval_id).toBe("appr-456");
    });

    it("is a no-op when no toast has the given approval_id", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });

      expect(result.current.notifications).toHaveLength(1);

      act(() => {
        result.current.removeToastByApprovalId("does-not-exist");
      });

      // Toast without metadata.approval_id is untouched
      expect(result.current.notifications).toHaveLength(1);
    });
  });

  // Cross-tab sync via BroadcastChannel (F9)
  describe("cross-tab BroadcastChannel sync", () => {
    it("removes toast when NOTIFICATION_DISMISSED arrives from another tab", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });
      expect(result.current.notifications).toHaveLength(1);
      const notificationId = result.current.notifications[0].id;

      // Simulate another tab broadcasting a NOTIFICATION_DISMISSED message
      act(() => {
        capturedSubscribeHandler?.({ type: "NOTIFICATION_DISMISSED", notificationId });
      });

      expect(result.current.notifications).toHaveLength(0);
    });

    it("marks notification as read in history when NOTIFICATION_DISMISSED arrives", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });
      const notificationId = result.current.notifications[0].id;

      act(() => {
        capturedSubscribeHandler?.({ type: "NOTIFICATION_DISMISSED", notificationId });
      });

      expect(result.current.notificationHistory).toHaveLength(1);
      expect(result.current.notificationHistory[0].isRead).toBe(true);
    });

    it("does not affect other notifications when NOTIFICATION_DISMISSED targets a specific id", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
        result.current.addNotification(makeNotification({ sessionId: "s2" }));
      });
      expect(result.current.notifications).toHaveLength(2);
      const firstId = result.current.notifications[0].id;

      act(() => {
        capturedSubscribeHandler?.({ type: "NOTIFICATION_DISMISSED", notificationId: firstId });
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].sessionId).toBe("s2");
    });

    it("acknowledgeNotification broadcasts NOTIFICATION_DISMISSED via the channel", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });
      const notificationId = result.current.notifications[0].id;

      act(() => {
        result.current.acknowledgeNotification(notificationId);
      });

      expect(mockBroadcast).toHaveBeenCalledWith({
        type: "NOTIFICATION_DISMISSED",
        notificationId,
      });
    });

    it("NOTIFICATION_ACKNOWLEDGED messages are intentionally not handled (server stream owns cross-tab ack)", () => {
      // NOTIFICATION_ACKNOWLEDGED is in the NotificationSyncMessage union but has no
      // handler in NotificationContext — cross-tab session acknowledgement is driven
      // by the sessionAcknowledged event from the server stream, not BroadcastChannel.
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ sessionId: "s1" }));
      });
      expect(result.current.notifications).toHaveLength(1);

      // Sending NOTIFICATION_ACKNOWLEDGED should be a no-op for active toasts
      act(() => {
        capturedSubscribeHandler?.({ type: "NOTIFICATION_ACKNOWLEDGED", sessionId: "s1" });
      });

      // Toast still present — NOTIFICATION_ACKNOWLEDGED has no handler
      expect(result.current.notifications).toHaveLength(1);
    });
  });
  describe("toast timer registry (Story 3.1)", () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    it("showActionToast expires on its own: success after 5s, error after 10s", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.showActionToast("Saved", "success", "ok");
        result.current.showActionToast("Failed", "error", "bad");
      });
      expect(result.current.notifications).toHaveLength(2);

      act(() => {
        jest.advanceTimersByTime(5_000);
      });
      expect(result.current.notifications.map((n) => n.message)).toEqual(["Failed"]);

      act(() => {
        jest.advanceTimersByTime(5_000);
      });
      expect(result.current.notifications).toHaveLength(0);
    });

    it("showUndoToast expires after the given duration", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.showUndoToast("Deleted", jest.fn(), 2_000);
      });
      act(() => {
        jest.advanceTimersByTime(1_999);
      });
      expect(result.current.notifications).toHaveLength(1);
      act(() => {
        jest.advanceTimersByTime(1);
      });
      expect(result.current.notifications).toHaveLength(0);
    });

    it("context_should_cancel_pending_timers_when_clearAll_or_unmount", () => {
      const { result, unmount } = renderHook(() => useNotifications(), { wrapper });
      const baseline = jest.getTimerCount(); // the stale-sweep interval

      act(() => {
        result.current.showActionToast("Saved", "success", "k1");
      });
      act(() => {
        jest.advanceTimersByTime(1_500); // let the Announcer's one-second hold finish
      });
      expect(jest.getTimerCount()).toBe(baseline + 1);

      act(() => {
        result.current.clearAll();
      });
      expect(jest.getTimerCount()).toBe(baseline);
      expect(result.current.notifications).toHaveLength(0);

      act(() => {
        result.current.showActionToast("Saved again", "success", "k2");
      });
      unmount();
      expect(jest.getTimerCount()).toBe(0);
    });

    it("notification_context_should_pass_all_existing_tests_unmodified_and_shrink_below_532_lines", () => {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const fs = require("fs") as typeof import("fs");
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const path = require("path") as typeof import("path");
      const source = fs.readFileSync(path.join(process.cwd(), "src/lib/contexts/NotificationContext.tsx"), "utf8");
      expect(source.split("\n").length).toBeLessThan(532);
    });
  });
  describe("isPendingDecision (Story 3.2)", () => {
    it("clears the pending flag on a history row once it is marked read", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });

      act(() => {
        result.current.addNotification(makeNotification({ notificationType: "warning", isPendingDecision: true }));
      });
      const id = result.current.notificationHistory[0].id;
      expect(result.current.notificationHistory[0].isPendingDecision).toBe(true);

      act(() => {
        result.current.markAsRead(id);
      });
      expect(result.current.notificationHistory[0].isRead).toBe(true);
      expect(result.current.notificationHistory[0].isPendingDecision).toBe(false);
    });
  });
  describe("cross-tab bulk dismissal (Story 3.9)", () => {
    const seed = (result: { current: ReturnType<typeof useNotifications> }) => {
      act(() => {
        result.current.addNotification({ ...makeNotification({ sessionId: "sa" }), id: "a" });
        result.current.addNotification({ ...makeNotification({ sessionId: "sb" }), id: "b" });
        result.current.addNotification({ ...makeNotification({ sessionId: "sc" }), id: "c" });
      });
    };

    it("bulk_sync_should_apply_id_set_idempotently_and_ignore_late_arrivals_when_message_replayed", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });
      seed(result);
      const message = { type: "NOTIFICATIONS_BULK_DISMISSED", kind: "dismissed", ids: ["a", "b"] };

      act(() => capturedSubscribeHandler?.(message));
      expect(result.current.notifications.map((n) => n.id)).toEqual(["c"]);

      // A toast that arrived after the click is unaffected, and a replay changes nothing.
      act(() => {
        result.current.addNotification({ ...makeNotification({ sessionId: "sd" }), id: "d" });
      });
      act(() => capturedSubscribeHandler?.(message));
      expect(result.current.notifications.map((n) => n.id)).toEqual(["c", "d"]);
    });

    it("a moved message drops the toast but leaves its history row unread", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });
      seed(result);

      act(() => capturedSubscribeHandler?.({ type: "NOTIFICATIONS_BULK_DISMISSED", kind: "moved", ids: ["a"] }));

      expect(result.current.notifications.map((n) => n.id)).toEqual(["b", "c"]);
      const row = result.current.notificationHistory.find((n) => n.id === "a");
      expect(row).toBeDefined();
      expect(row?.isRead).toBe(false);
    });

    it("a dismissed message also drops the cleared history rows", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });
      seed(result);

      act(() => capturedSubscribeHandler?.({ type: "NOTIFICATIONS_BULK_DISMISSED", kind: "dismissed", ids: ["a"] }));
      expect(result.current.notificationHistory.map((n) => n.id).sort()).toEqual(["b", "c"]);
    });

    it("moveAllToTray broadcasts the moved ids once", () => {
      const { result } = renderHook(() => useNotifications(), { wrapper });
      seed(result);

      act(() => {
        result.current.moveAllToTray();
      });

      expect(mockBroadcast).toHaveBeenCalledWith({
        type: "NOTIFICATIONS_BULK_DISMISSED",
        kind: "moved",
        ids: ["a", "b", "c"],
      });
    });
  });
});

describe("Quiet mode (Story 4.5)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    mockFlags.notification_tray_v2 = true;
  });
  afterEach(() => {
    mockFlags.notification_tray_v2 = false;
  });

  it("quiet_mode_should_demote_a_custom_toast_to_the_tray_and_still_toast_a_pending_decision", () => {
    const { result } = renderHook(() => useNotifications(), { wrapper });
    act(() => result.current.setQuietMode(true));
    expect(result.current.quietMode).toBe(true);

    act(() => {
      result.current.addNotification(makeNotification({ sessionId: "s-custom", notificationType: "custom" }));
    });
    expect(result.current.notifications).toHaveLength(0);
    expect(result.current.notificationHistory).toHaveLength(1);
    expect(result.current.getUnreadCount()).toBe(1);

    act(() => {
      result.current.addNotification(
        makeNotification({ sessionId: "s-err", notificationType: "error", isPendingDecision: true }),
      );
    });
    expect(result.current.notifications).toHaveLength(1);
    expect(result.current.notifications[0].sessionId).toBe("s-err");
  });

  it("quiet_mode_should_persist_and_survive_a_reload", () => {
    const first = renderHook(() => useNotifications(), { wrapper });
    act(() => first.result.current.setQuietMode(true));
    first.unmount();

    const second = renderHook(() => useNotifications(), { wrapper });
    expect(second.result.current.quietMode).toBe(true);
    act(() => {
      second.result.current.addNotification(makeNotification({ notificationType: "custom" }));
    });
    expect(second.result.current.notifications).toHaveLength(0);
  });

  it("quiet_mode_should_render_off_and_not_crash_when_localstorage_throws", () => {
    const get = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const set = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useNotifications(), { wrapper });
    expect(result.current.quietMode).toBe(false);
    expect(() => act(() => result.current.setQuietMode(true))).not.toThrow();
    // The in-memory setting still applies for this page even though it cannot persist.
    expect(result.current.quietMode).toBe(true);
    get.mockRestore();
    set.mockRestore();
  });

  it("quiet_mode_should_not_demote_anything_when_the_flag_is_off", () => {
    mockFlags.notification_tray_v2 = false;
    const { result } = renderHook(() => useNotifications(), { wrapper });
    act(() => result.current.setQuietMode(true));
    act(() => {
      result.current.addNotification(makeNotification({ notificationType: "custom" }));
    });
    expect(result.current.notifications).toHaveLength(1);
  });
});

describe("unread count floor (TH-1)", () => {
  afterEach(() => {
    mockServerUnread = 0;
  });

  it("badge_should_use_the_server_unread_total_when_only_a_page_of_history_is_loaded", () => {
    mockServerUnread = 120;
    const { result } = renderHook(() => useNotifications(), { wrapper });
    expect(result.current.getUnreadCount()).toBe(120);
  });

  it("badge_should_count_live_rows_above_a_stale_server_total", () => {
    mockServerUnread = 1;
    const { result } = renderHook(() => useNotifications(), { wrapper });
    act(() => {
      result.current.addNotification(makeNotification({ sessionId: "a" }));
      result.current.addNotification(makeNotification({ sessionId: "b" }));
      result.current.addNotification(makeNotification({ sessionId: "c" }));
    });
    expect(result.current.getUnreadCount()).toBe(3);
  });
});
