/**
 * Tests for useSessionNotifications — toast/dedup/routing logic.
 *
 * Covers:
 *  1. HISTORY_ONLY_TYPES (TASK_COMPLETE, PROCESS_FINISHED, etc.) → addToHistoryOnly, no toast
 *  2. Non-history types (ERROR, WARNING) → addNotification (toast)
 *  3. 10-second dedup window suppresses duplicate toasts for same (sessionId, type)
 *  4. APPROVAL_NEEDED bypasses dedup — each notification fires independently
 *  5. INPUT_REQUIRED bypasses dedup — each notification fires independently
 *  6. Dedup window resets after 10 seconds
 *  7. Different sessionIds are never deduped against each other
 *  8. FR-6: showBrowserNotification called for non-history events (native notification alongside toast)
 *  9. FR-6: showBrowserNotification NOT called for history-only events
 * 10. FR-6: approval events use approval:<id> tag
 * 11. NFR-1: approval events never suppressed by dedup
 */

import { renderHook, act } from "@testing-library/react";

// ── Mocks ──────────────────────────────────────────────────────────────────

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(),
}));

jest.mock("@/gen/session/v1/session_pb", () => ({
  SessionService: {},
}));

jest.mock("@/gen/session/v1/events_pb", () => ({}));

jest.mock("@/lib/types/notification", () => ({}));
jest.mock("@/lib/utils/notificationMapping", () => ({
  mapNotificationType: jest.fn((t: number) => t),
  mapPriority: jest.fn((p: number) => p),
}));
jest.mock("@/lib/notification-policy", () => ({
  TOAST_DEDUP_WINDOW_MS: 10_000,
  NATIVE_ACTIONABLE_TTL_MS: 300_000,
  nativeAutoCloseMs: jest.fn().mockReturnValue(15_000),
}));

const mockShowBrowserNotification = jest.fn().mockResolvedValue(undefined);
const mockPlayPriorityNotificationSound = jest.fn();
jest.mock("@/lib/utils/notifications", () => ({
  showBrowserNotification: mockShowBrowserNotification,
  playPriorityNotificationSound: mockPlayPriorityNotificationSound,
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

// NotificationType enum values used by useSessionNotifications internals.
// These numeric values are sourced from proto/session/v1/types.proto and must
// match the generated types_pb.ts. If proto enum values change, update both
// the mock below and these constants. Hardcoded here because jest.mock()
// intercepts the generated module before its real values are available.
const NT = {
  APPROVAL_NEEDED: 1,
  INPUT_REQUIRED: 2,
  ERROR: 3,
  FAILURE: 4,
  WARNING: 5,
  TASK_COMPLETE: 6,
  PROCESS_FINISHED: 7,
  PROCESS_STARTED: 8,
  STATUS_CHANGE: 9,
  INFO: 10,
  DEBUG: 11,
  CONFIRMATION_NEEDED: 12,
  CUSTOM: 13,
};

const NP = {
  URGENT: 1,
  HIGH: 2,
  MEDIUM: 3,
  LOW: 4,
};

jest.mock("@/gen/session/v1/types_pb", () => ({
  NotificationType: NT,
  NotificationPriority: NP,
}));

const mockAddNotification = jest.fn();
const mockAddToHistoryOnly = jest.fn();

jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    addNotification: mockAddNotification,
    addToHistoryOnly: mockAddToHistoryOnly,
  }),
}));

// ── Import under test ──────────────────────────────────────────────────────
import { useSessionNotifications } from "../useSessionNotifications";

// ── Helpers ────────────────────────────────────────────────────────────────

interface TestNotificationEvent {
  sessionId: string;
  sessionName: string;
  notificationType: number;
  priority: number;
  title: string;
  message: string;
  metadata: Record<string, string>;
}

function makeEvent(notificationType: number, sessionId = "test-session"): any {
  const event: TestNotificationEvent = {
    sessionId,
    sessionName: `Session ${sessionId}`,
    notificationType,
    priority: NP.MEDIUM,
    title: "Test Notification",
    message: "Test message",
    metadata: {},
  };
  return event;
}

const DEDUP_WINDOW_MS = 10_000;

// ── Tests ──────────────────────────────────────────────────────────────────

describe("useSessionNotifications", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockAddNotification.mockClear();
    mockAddToHistoryOnly.mockClear();
    mockShowBrowserNotification.mockClear();
    mockPlayPriorityNotificationSound.mockClear();
    // Grant Notification permission so native notification path is exercised
    Object.defineProperty(window, "Notification", {
      value: { permission: "granted" },
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  // ── 1. HISTORY_ONLY routing ──────────────────────────────────────────────

  describe("HISTORY_ONLY_TYPES routing", () => {
    const historyOnlyTypes = [
      ["TASK_COMPLETE", NT.TASK_COMPLETE],
      ["PROCESS_FINISHED", NT.PROCESS_FINISHED],
      ["PROCESS_STARTED", NT.PROCESS_STARTED],
      ["STATUS_CHANGE", NT.STATUS_CHANGE],
      ["INFO", NT.INFO],
      ["DEBUG", NT.DEBUG],
    ] as const;

    it.each(historyOnlyTypes)(
      "%s goes to history only (no toast, no sound)",
      (_name, notificationType) => {
        const { result } = renderHook(() =>
          useSessionNotifications({ enableAudio: false })
        );

        act(() => {
          result.current(makeEvent(notificationType));
        });

        expect(mockAddToHistoryOnly).toHaveBeenCalledTimes(1);
        expect(mockAddNotification).not.toHaveBeenCalled();
      }
    );
  });

  // ── 2. Toast-worthy types ────────────────────────────────────────────────

  describe("non-history types", () => {
    it("ERROR fires addNotification (toast)", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
      expect(mockAddToHistoryOnly).not.toHaveBeenCalled();
    });

    it("WARNING fires addNotification", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.WARNING));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
    });

    it("APPROVAL_NEEDED fires addNotification", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.APPROVAL_NEEDED));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
    });

    it("INPUT_REQUIRED fires addNotification", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.INPUT_REQUIRED));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
    });
  });

  // ── 3. 10-second dedup window ────────────────────────────────────────────

  describe("dedup window (non-approval types)", () => {
    it("refreshes the toast but suppresses audio on a second identical event within DEDUP_WINDOW_MS", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: true })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
      });
      act(() => {
        result.current(makeEvent(NT.ERROR)); // same key, within 10s
      });

      // Toast is refreshed (addNotification called twice — second replaces via sessionId dedup)
      expect(mockAddNotification).toHaveBeenCalledTimes(2);
      // Audio fires only once — the duplicate refresh is silent
      expect(mockPlayPriorityNotificationSound).toHaveBeenCalledTimes(1);
    });

    it("fires again after DEDUP_WINDOW_MS elapses", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
      });

      act(() => {
        jest.advanceTimersByTime(DEDUP_WINDOW_MS + 1);
      });

      act(() => {
        result.current(makeEvent(NT.ERROR)); // dedup expired
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(2);
    });

    it("does not dedup events with different sessionIds", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR, "session-1"));
        result.current(makeEvent(NT.ERROR, "session-2"));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(2);
    });

    it("does not dedup events with different notificationTypes", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
        result.current(makeEvent(NT.WARNING));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(2);
    });
  });

  // ── 4. APPROVAL_NEEDED bypasses dedup ────────────────────────────────────

  describe("APPROVAL_NEEDED bypasses dedup window", () => {
    it("fires both calls even within DEDUP_WINDOW_MS", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.APPROVAL_NEEDED));
      });
      act(() => {
        result.current(makeEvent(NT.APPROVAL_NEEDED)); // dedup bypassed
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(2);
    });
  });

  // ── 5. INPUT_REQUIRED bypasses dedup ─────────────────────────────────────

  describe("INPUT_REQUIRED bypasses dedup window", () => {
    it("fires both calls even within DEDUP_WINDOW_MS", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.INPUT_REQUIRED));
      });
      act(() => {
        result.current(makeEvent(NT.INPUT_REQUIRED)); // dedup bypassed
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(2);
    });
  });

  // ── 6. CONFIRMATION_NEEDED is NOT in approval bypass set ─────────────────

  describe("CONFIRMATION_NEEDED is subject to dedup", () => {
    it("refreshes toast silently (no audio) within DEDUP_WINDOW_MS (not in bypass set)", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: true })
      );

      act(() => {
        result.current(makeEvent(NT.CONFIRMATION_NEEDED));
      });
      act(() => {
        result.current(makeEvent(NT.CONFIRMATION_NEEDED));
      });

      // Toast is refreshed on duplicate — addNotification called twice, audio once
      expect(mockAddNotification).toHaveBeenCalledTimes(2);
      expect(mockPlayPriorityNotificationSound).toHaveBeenCalledTimes(1);
    });
  });

  // ── 7. FR-6: Native notification alongside toast ──────────────────────────

  describe("FR-6: native notifications (showBrowserNotification)", () => {
    it("handleNotification_should_callShowBrowserNotification_When_nonHistoryEvent", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
      expect(mockShowBrowserNotification).toHaveBeenCalledTimes(1);
    });

    it("handleNotification_should_notCallShowBrowserNotification_When_historyOnlyEvent", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.TASK_COMPLETE));
      });

      expect(mockAddNotification).not.toHaveBeenCalled();
      expect(mockShowBrowserNotification).not.toHaveBeenCalled();
    });

    it("handleNotification_should_useApprovalTag_When_approvalIdPresent", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      const eventWithApproval = {
        ...makeEvent(NT.APPROVAL_NEEDED),
        metadata: { approval_id: "abc123" },
      };

      act(() => {
        result.current(eventWithApproval);
      });

      expect(mockShowBrowserNotification).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({ tag: "approval:abc123" })
      );
    });

    it("handleNotification_should_neverSuppressApproval_When_dedupWindowActive", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.APPROVAL_NEEDED, "test-session"));
      });
      act(() => {
        result.current(makeEvent(NT.APPROVAL_NEEDED, "test-session"));
      });

      // Both approval events should fire native notifications (never suppressed)
      expect(mockAddNotification).toHaveBeenCalledTimes(2);
      expect(mockShowBrowserNotification).toHaveBeenCalledTimes(2);

      // Verify the second call also uses requireInteraction:true and a session-based
      // tag (no approval_id in makeEvent's default metadata).
      expect(mockShowBrowserNotification).toHaveBeenNthCalledWith(
        2,
        "Test Notification",
        expect.objectContaining({
          requireInteraction: true,
          tag: `test-session:${NT.APPROVAL_NEEDED}`,
        })
      );
    });
  });
});
