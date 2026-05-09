/**
 * Tests for useReviewQueueNotifications dwell-time fix.
 *
 * Covers the three critical paths:
 * 1. Auto-approved items (enter+leave before DWELL_TIME_MS) → NO notification
 * 2. Genuine escalated approvals (stay >= DWELL_TIME_MS) → notification fires
 * 3. Initial-load items → NO notification (seeded as already-notified)
 *
 * Uses jest fake timers to control setTimeout without real delays.
 */

import { renderHook, act } from "@testing-library/react";

// ── Mocks ──────────────────────────────────────────────────────────────────

jest.mock("@/gen/session/v1/types_pb", () => ({
  AttentionReason: {
    APPROVAL_PENDING: 1,
    INPUT_REQUIRED: 2,
    WAITING_FOR_USER: 3,
    ERROR_STATE: 4,
    TESTS_FAILING: 5,
    STALE: 6,
    TASK_COMPLETE: 7,
    IDLE: 8,
    UNCOMMITTED_CHANGES: 9,
    IDLE_TIMEOUT: 10,
    UNSPECIFIED: 0,
  },
}));

const mockPlayNotificationSound = jest.fn();
const mockShowBrowserNotification = jest.fn();

jest.mock("@/lib/utils/notifications", () => ({
  playNotificationSound: (...args: unknown[]) =>
    mockPlayNotificationSound(...args),
  showBrowserNotification: (...args: unknown[]) =>
    mockShowBrowserNotification(...args),
  NotificationSound: { DING: "ding" },
}));

const mockShowSessionNotification = jest.fn();
const mockAddToHistoryOnly = jest.fn();
const mockMarkAsReadBySessionId = jest.fn();

jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    showSessionNotification: mockShowSessionNotification,
    addToHistoryOnly: mockAddToHistoryOnly,
    markAsReadBySessionId: mockMarkAsReadBySessionId,
  }),
}));

// shouldNotify returns true by default (no localStorage record) — tests can override per-case
const mockShouldNotify = jest.fn().mockReturnValue(true);
const mockMarkNotified = jest.fn();
const mockMarkAcknowledged = jest.fn();
const mockMarkNotifiedBatch = jest.fn();
const mockCleanupExpired = jest.fn();

jest.mock("@/lib/utils/notificationStorage", () => ({
  shouldNotify: (id: string) => mockShouldNotify(id),
  markNotified: (...args: unknown[]) => mockMarkNotified(...args),
  markAcknowledged: (...args: unknown[]) => mockMarkAcknowledged(...args),
  markNotifiedBatch: (...args: unknown[]) => mockMarkNotifiedBatch(...args),
  cleanupExpired: () => mockCleanupExpired(),
}));

// ── Import under test ──────────────────────────────────────────────────────
import { useReviewQueueNotifications } from "../useReviewQueueNotifications";

// ── Helpers ────────────────────────────────────────────────────────────────

const DWELL_TIME_MS = 3_000;

function makeItem(sessionId: string, reason = 1 /* APPROVAL_PENDING */): any {
  return {
    sessionId,
    sessionName: `Session ${sessionId}`,
    reason,
    context: `Context for ${sessionId}`,
    priority: 2,
  };
}

// ── Tests ──────────────────────────────────────────────────────────────────

describe("useReviewQueueNotifications — dwell-time filter", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockPlayNotificationSound.mockClear();
    mockShowSessionNotification.mockClear();
    mockAddToHistoryOnly.mockClear();
    mockMarkAsReadBySessionId.mockClear();
    mockShouldNotify.mockReturnValue(true);
    mockMarkNotifiedBatch.mockClear();
    mockCleanupExpired.mockClear();
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  // ── 1. Initial load ──────────────────────────────────────────────────────

  describe("initial load", () => {
    it("does not notify for items already present when hook mounts", () => {
      const items = [makeItem("session-a"), makeItem("session-b")];
      renderHook(() =>
        useReviewQueueNotifications(items, { enabled: true })
      );

      // Advance past dwell time — no notification should fire because
      // initial items are seeded into notifiedItemsRef
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS + 500);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
      expect(mockPlayNotificationSound).not.toHaveBeenCalled();
    });
  });

  // ── 2. Auto-approve suppression ──────────────────────────────────────────

  describe("auto-approved items (enter and leave before dwell)", () => {
    it("does not notify when item leaves queue before DWELL_TIME_MS", () => {
      const item = makeItem("session-auto");
      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      // Item enters queue
      act(() => {
        rerender({ items: [item] });
      });

      // Item leaves before dwell expires (simulates auto-approve completing in ~100ms)
      act(() => {
        jest.advanceTimersByTime(100);
        rerender({ items: [] });
      });

      // Advance past dwell time — timer should have been cancelled
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
      expect(mockPlayNotificationSound).not.toHaveBeenCalled();
    });

    it("cancels the dwell timer when item leaves queue early", () => {
      const clearTimeoutSpy = jest.spyOn(global, "clearTimeout");
      const item = makeItem("session-cancel");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(500);
        rerender({ items: [] }); // item leaves
      });

      expect(clearTimeoutSpy).toHaveBeenCalled();
      clearTimeoutSpy.mockRestore();
    });
  });

  // ── 3. Genuine escalated approval ────────────────────────────────────────

  describe("genuine approvals (stay >= DWELL_TIME_MS)", () => {
    it("notifies after item dwells for DWELL_TIME_MS without a REST poll", () => {
      const item = makeItem("session-escalated");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      // Item enters queue
      act(() => {
        rerender({ items: [item] });
      });

      // Immediately after entering: no notification (dwell not met)
      expect(mockShowSessionNotification).not.toHaveBeenCalled();

      // Advance exactly DWELL_TIME_MS — the setTimeout fires, setDwellTick increments,
      // React re-renders the hook, and the filter now passes
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);
      expect(mockPlayNotificationSound).toHaveBeenCalledTimes(1);
    });

    it("passes the correct ReviewItem to showSessionNotification", () => {
      const item = makeItem("session-check-payload");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledWith(
        item,
        expect.any(Function),
        expect.any(Function)
      );
    });

    it("does NOT notify a second time if item remains in queue after dwell fires", () => {
      const item = makeItem("session-no-double");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      // First dwell fires → notify
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);

      // Simulate another render (e.g., 30s poll) — item still in queue
      act(() => {
        jest.advanceTimersByTime(30_000);
        rerender({ items: [item] });
      });

      // No second notification
      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);
    });
  });

  // ── 4. Tier routing ───────────────────────────────────────────────────────

  describe("tier routing", () => {
    it("adds Tier 3 items to history only (no toast, no sound)", () => {
      // TASK_COMPLETE = 7 → Tier 3
      const item = makeItem("session-t3", 7);

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
      expect(mockPlayNotificationSound).not.toHaveBeenCalled();
      expect(mockAddToHistoryOnly).toHaveBeenCalledTimes(1);
      expect(mockAddToHistoryOnly).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: "session-t3" })
      );
    });

    it("shows toast + sound for Tier 2 items (ERROR_STATE = 4)", () => {
      const item = makeItem("session-t2", 4 /* ERROR_STATE */);

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);
      // Tier 2 does NOT play sound
      expect(mockPlayNotificationSound).not.toHaveBeenCalled();
    });
  });

  // ── 5. shouldNotify localStorage gate ────────────────────────────────────

  describe("localStorage grace period (shouldNotify)", () => {
    it("does not notify if shouldNotify returns false (already acked)", () => {
      mockShouldNotify.mockReturnValue(false);
      const item = makeItem("session-acked");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
    });
  });

  // ── 6. Re-entry after removal ─────────────────────────────────────────────

  describe("re-entry after removal", () => {
    it("notifies again when same session re-enters queue after leaving", () => {
      const item = makeItem("session-reentry");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      // First entry: dwell fires → notify
      act(() => {
        rerender({ items: [item] });
      });
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });
      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);

      // Item leaves queue
      act(() => {
        rerender({ items: [] });
      });

      // Re-set shouldNotify (simulate 1h later / grace period expired)
      mockShouldNotify.mockReturnValue(true);

      // Item re-enters (new approval request)
      act(() => {
        rerender({ items: [item] });
      });

      // New dwell timer fires
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledTimes(2);
    });

    it("calls markAsReadBySessionId when item leaves queue", () => {
      const item = makeItem("session-read");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        rerender({ items: [] });
      });

      expect(mockMarkAsReadBySessionId).toHaveBeenCalledWith(["session-read"]);
    });
  });

  // ── 7. reset() ────────────────────────────────────────────────────────────

  describe("reset()", () => {
    it("cancels all pending timers and allows initial-load items to be re-evaluated", () => {
      const clearTimeoutSpy = jest.spyOn(global, "clearTimeout");
      const item = makeItem("session-reset");

      const { rerender, result } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      // Pending timer exists — reset should clear it
      act(() => {
        result.current.reset();
      });

      expect(clearTimeoutSpy).toHaveBeenCalled();

      // After reset, advancing time should NOT fire because timers were cleared
      // and isInitialLoad is true again — next render seeds notified state
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
      clearTimeoutSpy.mockRestore();
    });
  });

  // ── 8. enabled=false ──────────────────────────────────────────────────────

  describe("enabled flag", () => {
    it("does not notify when enabled=false even after dwell", () => {
      const item = makeItem("session-disabled");

      const { rerender } = renderHook(
        ({ items }) =>
          useReviewQueueNotifications(items, { enabled: false }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS + 500);
      });

      expect(mockShowSessionNotification).not.toHaveBeenCalled();
      expect(mockPlayNotificationSound).not.toHaveBeenCalled();
    });
  });

  // ── 9. Multiple items ────────────────────────────────────────────────────

  describe("multiple items entering simultaneously", () => {
    it("notifies for all items that dwell (Tier 1 batched into one sound)", () => {
      const items = [makeItem("s1"), makeItem("s2"), makeItem("s3")];

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      // One notification per item
      expect(mockShowSessionNotification).toHaveBeenCalledTimes(3);
      // Sound plays once for the batch
      expect(mockPlayNotificationSound).toHaveBeenCalledTimes(1);
    });

    it("only notifies items that remain after dwell (auto-approved items filtered out)", () => {
      const itemA = makeItem("session-stays");
      const itemB = makeItem("session-auto-out");

      const { rerender } = renderHook(
        ({ items }) => useReviewQueueNotifications(items, { enabled: true }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      // Both enter
      act(() => {
        rerender({ items: [itemA, itemB] });
      });

      // itemB auto-approved and leaves before dwell
      act(() => {
        jest.advanceTimersByTime(200);
        rerender({ items: [itemA] });
      });

      // Dwell fires for itemA (itemB's timer was cancelled)
      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(mockShowSessionNotification).toHaveBeenCalledTimes(1);
      expect(mockShowSessionNotification).toHaveBeenCalledWith(
        itemA,
        expect.any(Function),
        expect.any(Function)
      );
    });
  });

  // ── 10. onNewItems callback ───────────────────────────────────────────────

  describe("onNewItems callback", () => {
    it("invokes onNewItems with items that passed dwell", () => {
      const onNewItems = jest.fn();
      const item = makeItem("session-callback");

      const { rerender } = renderHook(
        ({ items }) =>
          useReviewQueueNotifications(items, { enabled: true, onNewItems }),
        { initialProps: { items: [] as ReturnType<typeof makeItem>[] } }
      );

      act(() => {
        rerender({ items: [item] });
      });

      act(() => {
        jest.advanceTimersByTime(DWELL_TIME_MS);
      });

      expect(onNewItems).toHaveBeenCalledTimes(1);
      expect(onNewItems).toHaveBeenCalledWith([item]);
    });
  });
});
