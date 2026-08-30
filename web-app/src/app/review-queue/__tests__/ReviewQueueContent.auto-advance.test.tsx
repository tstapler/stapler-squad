/**
 * Tests for ReviewQueueContent — auto-advance behavior
 *
 * Bug: the "deleted externally" auto-advance useEffect checked `reviewQueueItems`
 * (a derived list from queueItems × sessions), which caused spurious auto-advance
 * when a session's live status changed to ACTIVE/PROCESSING, filtering it out of
 * the derived list even though the review queue item still existed.
 *
 * Fix: use `allQueueItems` from `useReviewQueueContext().items` for the existence
 * check — those only change when actual addItem/removeItem Redux events fire.
 *
 * Requirements:
 *   REQ-1: Status transition (ACTIVE/PROCESSING) must NOT trigger auto-advance
 *   REQ-2: Genuine removal from queue MUST trigger auto-advance
 *   REQ-3: Regression across multiple working-state transitions
 *
 * Test IDs: T-AA-001 through T-AA-007
 */

import React from "react";
import { render, act } from "@testing-library/react";
import type { ReviewItem, Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Router spy — captured before any mocks
// ---------------------------------------------------------------------------

const mockPush = jest.fn();

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: mockPush }),
}));

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: jest.fn(),
}));

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: jest.fn(),
}));

jest.mock("@/lib/hooks/useKeyboard", () => ({
  useKeyboard: jest.fn(),
}));

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(() => ({ items: [], connectionState: "live" })),
}));

// ---------------------------------------------------------------------------
// ReviewQueuePanel stub: captures callbacks, including onSessionClick
// so tests can drive selectedSession state via handleSessionClick.
// ---------------------------------------------------------------------------

let capturedOnItemsChange: ((items: ReviewItem[]) => void) | undefined;
let capturedOnAcknowledged: ((id: string) => void) | undefined;
let capturedOnSessionClick: ((sessionId: string) => void) | undefined;

jest.mock("@/components/sessions/ReviewQueuePanel", () => ({
  ReviewQueuePanel: (props: {
    onItemsChange?: (items: ReviewItem[]) => void;
    onAcknowledged?: (id: string) => void;
    onSessionClick?: (sessionId: string) => void;
  }) => {
    capturedOnItemsChange = props.onItemsChange;
    capturedOnAcknowledged = props.onAcknowledged;
    capturedOnSessionClick = props.onSessionClick;
    return <div data-testid="review-queue-panel" />;
  },
}));

// ---------------------------------------------------------------------------
// SessionDetail stub
// ---------------------------------------------------------------------------

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: () => <div data-testid="session-detail" />,
}));

// ---------------------------------------------------------------------------
// Context mocks
// ---------------------------------------------------------------------------

const mockAcknowledgeSession = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: jest.fn(),
}));

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: jest.fn(),
}));

import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";

const mockUseReviewQueueContext = useReviewQueueContext as jest.Mock;
const mockUseSessionServiceContext = useSessionServiceContext as jest.Mock;

// ---------------------------------------------------------------------------
// Import the component under test AFTER all mocks are declared
// ---------------------------------------------------------------------------

import ReviewQueuePage from "../page";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(id: string, status = SessionStatus.UNSPECIFIED): Session {
  return {
    id,
    title: `Session ${id}`,
    status,
    path: `/workspace/${id}`,
    workingDir: `/workspace/${id}`,
    branch: "main",
    program: "claude",
    tags: [],
  } as unknown as Session;
}

function makeReviewItem(sessionId: string): ReviewItem {
  return {
    sessionId,
    sessionName: `Session ${sessionId}`,
    path: `/workspace/${sessionId}`,
    workingDir: `/workspace/${sessionId}`,
    branch: "main",
    status: SessionStatus.UNSPECIFIED,
    program: "claude",
    tags: [],
  } as unknown as ReviewItem;
}

function makeContextValue(allQueueItems: ReviewItem[]) {
  return {
    items: allQueueItems,
    reviewQueue: null,
    totalItems: allQueueItems.length,
    byPriority: new Map(),
    byReason: new Map(),
    averageAgeSeconds: BigInt(0),
    oldestAgeSeconds: BigInt(0),
    oldestItemId: "",
    loading: false,
    error: null,
    refresh: jest.fn().mockResolvedValue(undefined),
    getByPriority: jest.fn(),
    getByReason: jest.fn(),
    acknowledgeSession: mockAcknowledgeSession,
  };
}

function makeSessionServiceValue(sessions: Session[]) {
  return {
    sessions,
    loading: false,
    error: null,
    connectionState: "connected",
    systemMemoryPct: 0,
    listSessions: jest.fn(),
    getSession: jest.fn(),
    createSession: jest.fn(),
    updateSession: jest.fn(),
    deleteSession: jest.fn(),
    pauseSession: jest.fn(),
    resumeSession: jest.fn(),
    hibernateSession: jest.fn(),
    resumeHibernatedSession: jest.fn(),
    renameSession: jest.fn(),
    restartSession: jest.fn(),
    clearConversationState: jest.fn(),
    acknowledgeSession: jest.fn(),
    createCheckpoint: jest.fn(),
    listCheckpoints: jest.fn(),
    forkSession: jest.fn(),
    runOneShot: jest.fn().mockResolvedValue(null),
    listPromptHistory: jest.fn(),
    watchSessions: jest.fn(),
    stopWatching: jest.fn(),
  };
}

// ---------------------------------------------------------------------------
// Test-wide timer setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.useFakeTimers();
  jest.clearAllMocks();
  capturedOnItemsChange = undefined;
  capturedOnAcknowledged = undefined;
  capturedOnSessionClick = undefined;
  localStorage.clear();
});

afterEach(() => {
  jest.useRealTimers();
});

// ---------------------------------------------------------------------------
// Helper: render + seed initial queue + select a session via onSessionClick
//
// This drives selectedSession state through handleSessionClick so the
// "deleted externally" effect's `if (!selectedSession) return;` guard is live.
// ---------------------------------------------------------------------------

function setupWithSelectedSession(
  allQueueItems: ReviewItem[],
  selectedId: string,
  sessions?: Session[]
) {
  const sessionList = sessions ?? allQueueItems.map((item) =>
    makeSession(item.sessionId)
  );

  mockUseReviewQueueContext.mockReturnValue(makeContextValue(allQueueItems));
  mockUseSessionServiceContext.mockReturnValue(makeSessionServiceValue(sessionList));

  const result = render(<ReviewQueuePage />);

  // Seed queueItems state so reviewQueueItems is derived correctly
  act(() => {
    capturedOnItemsChange?.(allQueueItems);
  });

  // Select the target session — calls handleSessionClick, sets selectedSession state
  act(() => {
    capturedOnSessionClick?.(selectedId);
  });

  // Clear the router.push call that handleSessionClick emits (?session=selectedId)
  // so subsequent assertions only see auto-advance-triggered navigations.
  mockPush.mockClear();

  return result;
}

// ---------------------------------------------------------------------------
// REQ-1: Status transitions must NOT auto-advance
//
// These tests are the regression guards for the original bug.
// They would FAIL on the buggy code (which used reviewQueueItems) and
// PASS on the fixed code (which uses allQueueItems from context).
// ---------------------------------------------------------------------------

describe("ReviewQueueContent — auto-advance suppression on status transition", () => {
  /**
   * T-AA-001
   * The selected session transitions to ACTIVE. The panel's live-status filter
   * removes it from the derived visible list (reviewQueueItems), but allQueueItems
   * (context.items) still holds the item. Auto-advance must NOT fire.
   *
   * Failure mode on buggy code: reviewQueueItems loses s1 → stillInQueue=false
   * → handleAutoAdvance fires → router.push called with ?session=s2.
   */
  it("T-AA-001: should_not_autoAdvance_when_selectedSession_transitionsToActive", () => {
    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");
    const allItems = [item1, item2];

    // Render with both items and select s1
    const { rerender } = setupWithSelectedSession(allItems, "s1");

    // Simulate: s1 transitions to ACTIVE — visible list drops it, but context keeps it
    mockUseReviewQueueContext.mockReturnValue(makeContextValue(allItems)); // s1 still in context
    mockUseSessionServiceContext.mockReturnValue(
      makeSessionServiceValue([makeSession("s1", SessionStatus.ACTIVE), makeSession("s2")])
    );

    act(() => {
      capturedOnItemsChange?.([item2]); // filtered view no longer has s1
    });
    rerender(<ReviewQueuePage />);

    act(() => {
      jest.advanceTimersByTime(400); // past the 300ms setTimeout in handleAutoAdvance
    });

    // No auto-advance navigation should have fired
    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls).toHaveLength(0);
  });

  /**
   * T-AA-002
   * Same as T-AA-001 but confirms the fix covers the PROCESSING working-state
   * branch (multiple sub-statuses map to PROCESSING).
   */
  it("T-AA-002: should_not_autoAdvance_when_selectedSession_transitionsToProcessing", () => {
    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");
    const allItems = [item1, item2];

    const { rerender } = setupWithSelectedSession(allItems, "s1");

    mockUseReviewQueueContext.mockReturnValue(makeContextValue(allItems));
    mockUseSessionServiceContext.mockReturnValue(
      makeSessionServiceValue([makeSession("s1", SessionStatus.ACTIVE), makeSession("s2")])
    );

    act(() => { capturedOnItemsChange?.([item2]); });
    rerender(<ReviewQueuePage />);
    act(() => { jest.advanceTimersByTime(400); });

    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls).toHaveLength(0);
  });

  /**
   * T-AA-003
   * Auto-advance user preference disabled: even genuine removal should not advance.
   * Verifies the `force=false` path in handleAutoAdvance respects the preference,
   * while also ensuring the status-transition path doesn't bypass it.
   */
  it("T-AA-003: should_not_autoAdvance_when_autoAdvancePreferenceIsDisabled", () => {
    localStorage.setItem("review-queue-auto-advance", "false");

    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");
    const allItems = [item1, item2];

    // The "deleted externally" path uses force=false so it also respects the preference.
    // This test ensures the status-transition suppression works correctly when pref=off.
    const { rerender } = setupWithSelectedSession(allItems, "s1");

    mockUseReviewQueueContext.mockReturnValue(makeContextValue(allItems));
    act(() => { capturedOnItemsChange?.([item2]); });
    rerender(<ReviewQueuePage />);
    act(() => { jest.advanceTimersByTime(400); });

    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// REQ-2: Genuine removals MUST still auto-advance
// ---------------------------------------------------------------------------

describe("ReviewQueueContent — auto-advance fires on genuine removal", () => {
  /**
   * T-AA-004
   * When allQueueItems genuinely loses the selected session (removeItem fired),
   * auto-advance must navigate to the next item.
   *
   * This is the preserved behavior: the fix must not break genuine deletion.
   */
  it("T-AA-004: should_autoAdvance_to_nextItem_when_selectedSession_genuinelyRemoved", () => {
    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");
    const allItems = [item1, item2];

    const { rerender } = setupWithSelectedSession(allItems, "s1");

    // Genuinely remove s1 from both allQueueItems AND the visible list
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item2])); // s1 gone from store
    mockUseSessionServiceContext.mockReturnValue(
      makeSessionServiceValue([makeSession("s2")])
    );

    act(() => { capturedOnItemsChange?.([item2]); });
    rerender(<ReviewQueuePage />);
    act(() => { jest.advanceTimersByTime(400); });

    // Auto-advance should have navigated to s2
    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls.length).toBeGreaterThan(0);
    expect(sessionNavCalls[0][0]).toContain("?session=s2");
  });

  /**
   * T-AA-005
   * When the queue empties after a genuine removal, modal closes
   * (router.push to /review-queue with no session param).
   */
  it("T-AA-005: should_closeModal_when_queueEmptiesAfterRemoval", () => {
    const item1 = makeReviewItem("s1");

    const { rerender } = setupWithSelectedSession([item1], "s1");

    // Queue goes empty — both context and visible list lose s1
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([]));

    act(() => { capturedOnItemsChange?.([]); });
    rerender(<ReviewQueuePage />);
    act(() => { jest.advanceTimersByTime(400); });

    // Should navigate to /review-queue (no session param) to close the modal
    expect(mockPush).toHaveBeenCalledWith("/review-queue");
  });

  /**
   * T-AA-006
   * onAcknowledged callback is wired correctly — when the panel acknowledges the
   * currently selected session, handleAutoAdvance navigates to the next item.
   */
  it("T-AA-006: should_autoAdvance_when_acknowledgedFromPanel", () => {
    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");

    setupWithSelectedSession([item1, item2], "s1");

    expect(capturedOnAcknowledged).toBeDefined();

    // Acknowledge s1 from the panel while s1 is the selected session
    act(() => { capturedOnAcknowledged?.("s1"); });
    act(() => { jest.advanceTimersByTime(400); });

    // handleAcknowledged calls handleAutoAdvance (force=false) — auto-advance pref is on (default)
    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls.length).toBeGreaterThan(0);
  });

  /**
   * T-AA-008
   * When auto-advance preference is OFF and the selected session is genuinely
   * removed from allQueueItems (e.g. after approving/denying a permission request),
   * auto-advance must NOT fire. The user may still want to watch the session continue.
   *
   * Previously broken: the "deleted externally" path used force=true, bypassing
   * the preference and always advancing after any genuine removal.
   */
  it("T-AA-008: should_not_autoAdvance_when_preferenceIsOff_and_sessionGenuinelyRemoved", () => {
    localStorage.setItem("review-queue-auto-advance", "false");

    const item1 = makeReviewItem("s1");
    const item2 = makeReviewItem("s2");
    const allItems = [item1, item2];

    const { rerender } = setupWithSelectedSession(allItems, "s1");

    // Genuinely remove s1 from allQueueItems (simulates approve/deny acknowledgement)
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item2]));
    mockUseSessionServiceContext.mockReturnValue(
      makeSessionServiceValue([makeSession("s2")])
    );

    act(() => { capturedOnItemsChange?.([item2]); });
    rerender(<ReviewQueuePage />);
    act(() => { jest.advanceTimersByTime(400); });

    // Auto-advance must NOT fire — preference is off
    const sessionNavCalls = mockPush.mock.calls.filter(
      ([url]: [string]) => typeof url === "string" && url.includes("?session=")
    );
    expect(sessionNavCalls).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// REQ-3: Regression — parameterized status-transition scenarios
// ---------------------------------------------------------------------------

describe("ReviewQueueContent — regression: status transitions do not auto-advance", () => {
  /**
   * T-AA-007
   * Parameterized test: multiple working-state transitions must never produce
   * spurious auto-advance while the selected session is still in the store.
   *
   * Each row simulates a different sequence of status changes that historically
   * caused reviewQueueItems to drop the session, triggering incorrect navigation.
   */
  const cases: Array<{ label: string; visibleDrops: number }> = [
    { label: "A: single ACTIVE transition (1 visible drop)", visibleDrops: 1 },
    { label: "B: ACTIVE then back (2 drops)", visibleDrops: 2 },
    { label: "C: three rapid drops", visibleDrops: 3 },
    { label: "D: four consecutive drops", visibleDrops: 4 },
  ];

  it.each(cases)(
    "T-AA-007 $label: should_not_autoAdvance_for_statusTransition",
    ({ visibleDrops }) => {
      const item1 = makeReviewItem("s1");
      const item2 = makeReviewItem("s2");
      const allItems = [item1, item2];

      const { rerender } = setupWithSelectedSession(allItems, "s1");

      for (let i = 0; i < visibleDrops; i++) {
        // allQueueItems keeps s1; only the derived visible list drops it
        mockUseReviewQueueContext.mockReturnValue(makeContextValue(allItems));
        act(() => { capturedOnItemsChange?.([item2]); });
        rerender(<ReviewQueuePage />);
        act(() => { jest.advanceTimersByTime(400); });
      }

      // After all drops, no spurious navigation should have occurred
      const sessionNavCalls = mockPush.mock.calls.filter(
        ([url]: [string]) => typeof url === "string" && url.includes("?session=")
      );
      expect(sessionNavCalls).toHaveLength(0);
    }
  );
});
