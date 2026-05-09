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
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

// NotificationType enum values used by useSessionNotifications internals.
// These must stay consistent between the mock and test helpers below.
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

function makeEvent(notificationType: number, sessionId = "test-session"): any {
  return {
    sessionId,
    sessionName: `Session ${sessionId}`,
    notificationType,
    priority: NP.MEDIUM,
    title: "Test Notification",
    message: "Test message",
    metadata: {},
  };
}

const DEDUP_WINDOW_MS = 10_000;

// ── Tests ──────────────────────────────────────────────────────────────────

describe("useSessionNotifications", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockAddNotification.mockClear();
    mockAddToHistoryOnly.mockClear();
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
    it("suppresses a second identical event within DEDUP_WINDOW_MS", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.ERROR));
      });
      act(() => {
        result.current(makeEvent(NT.ERROR)); // same key, within 10s
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
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
    it("is deduplicated within DEDUP_WINDOW_MS (not in bypass set)", () => {
      const { result } = renderHook(() =>
        useSessionNotifications({ enableAudio: false })
      );

      act(() => {
        result.current(makeEvent(NT.CONFIRMATION_NEEDED));
      });
      act(() => {
        result.current(makeEvent(NT.CONFIRMATION_NEEDED));
      });

      expect(mockAddNotification).toHaveBeenCalledTimes(1);
    });
  });
});
