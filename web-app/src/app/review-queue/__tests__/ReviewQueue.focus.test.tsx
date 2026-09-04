/**
 * Focus-restoration regression test for the review-queue session-detail
 * modal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike
 * ReviewQueueContent.auto-advance.test.tsx) so the real trap-and-restore
 * behavior runs end to end. page.tsx's handleSessionClick is the shared
 * funnel for every click-based opener (queue list, working-sessions row)
 * and captures the trigger via `document.activeElement` synchronously
 * inside that handler — this harness mirrors that exact wiring via a real
 * button that invokes the ReviewQueuePanel stub's captured onSessionClick
 * callback, proving focus returns to the actual opening element on close.
 */

import React from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import type { ReviewItem, Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

const mockPush = jest.fn();
let mockSessionParam: string | null = null;

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: (key: string) => (key === "session" ? mockSessionParam : null) }),
  useRouter: () => ({ push: mockPush }),
}));

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: jest.fn(),
}));

jest.mock("@/lib/hooks/useKeyboard", () => ({
  useKeyboard: jest.fn(),
}));

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(() => ({ items: [], connectionState: "live" })),
}));

let capturedOnSessionClick: ((sessionId: string) => void) | undefined;

jest.mock("@/components/sessions/ReviewQueuePanel", () => ({
  ReviewQueuePanel: (props: { onSessionClick?: (sessionId: string) => void }) => {
    capturedOnSessionClick = props.onSessionClick;
    return <div data-testid="review-queue-panel" />;
  },
}));

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: (props: { onClose?: () => void }) => (
    <div data-testid="session-detail">
      <button onClick={props.onClose}>Close</button>
    </div>
  ),
}));

function makeSession(id: string): Session {
  return {
    id,
    title: `Session ${id}`,
    status: SessionStatus.UNSPECIFIED,
    path: `/workspace/${id}`,
    workingDir: `/workspace/${id}`,
    branch: "main",
    program: "claude",
    tags: [],
  } as unknown as Session;
}

const sessionOne = makeSession("s1");
const sessionTwo = makeSession("s2");

// Stable object/array references — page.tsx's effects depend on
// `items`/`sessions` by reference, so returning a fresh object/array literal
// on every render (as a `jest.fn(() => ({...}))` factory would) retriggers
// those effects every render and causes an infinite update loop ("Maximum
// update depth exceeded"). Define the mock values once, outside the mock
// factories, and include the real sessions so handleSessionClick's lookup
// (`sessions.find((s) => s.id === sessionId)`) actually resolves and opens
// the modal.
const stableSessions: Session[] = [sessionOne, sessionTwo];
const stableQueueItems: ReviewItem[] = [];
const stableByPriority = new Map();
const stableByReason = new Map();
const mockAcknowledgeSession = jest.fn().mockResolvedValue(undefined);
const stableReviewQueueContextValue = {
  items: stableQueueItems,
  reviewQueue: null,
  totalItems: 0,
  byPriority: stableByPriority,
  byReason: stableByReason,
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

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: jest.fn(() => stableReviewQueueContextValue),
}));

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: jest.fn(() => ({
    sessions: stableSessions,
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
  })),
}));

import ReviewQueuePage from "../page";

// Real openers that mirror production: each invokes the shared funnel
// (page.tsx's handleSessionClick, reached here via the ReviewQueuePanel
// stub's captured onSessionClick) after being focused, exactly as a real
// queue-row or working-session button click would leave it focused.
function Openers() {
  return (
    <>
      <button
        data-testid="opener-1"
        onClick={() => capturedOnSessionClick?.(sessionOne.id)}
      >
        Open Session One
      </button>
      <button
        data-testid="opener-2"
        onClick={() => capturedOnSessionClick?.(sessionTwo.id)}
      >
        Open Session Two
      </button>
    </>
  );
}

function Harness() {
  return (
    <>
      <Openers />
      <ReviewQueuePage />
    </>
  );
}

describe("Review queue session-detail modal focus restoration", () => {
  beforeEach(() => {
    mockPush.mockClear();
    capturedOnSessionClick = undefined;
  });

  it("SessionDetailModal_should_restoreFocusToFirstOpener_When_openedFromOpener1", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("opener-1");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByText("Close"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("SessionDetailModal_should_restoreFocusToSecondOpener_When_openedFromOpener2", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("opener-2");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByText("Close"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("opener-1"));
  });

  it("SessionDetailModal_should_restoreFocus_When_closedByClickingOverlay", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("opener-1");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    // The overlay's onClick handler is handleCloseSessionDetail; the dialog
    // content itself stops propagation, so click the overlay directly.
    fireEvent.click(screen.getByRole("dialog").parentElement as HTMLElement);

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("SessionDetailModal_should_openWithoutCrashing_When_openedViaDeepLinkWithNoPriorClick", async () => {
    // Deep-linking (?session=s1) opens the modal with no click at all, so
    // sessionTriggerRef.current stays null -- page.tsx documents this as
    // deliberate. useFocusTrap must no-op restoring focus rather than throw.
    mockSessionParam = sessionOne.id;
    try {
      render(<Harness />);
      await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

      fireEvent.click(screen.getByText("Close"));

      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    } finally {
      mockSessionParam = null;
    }
  });
});
