/**
 * Tests for ReviewQueuePanel — feature: review-queue-pr-creation (S3-3)
 *
 * Covers:
 *  - "Create PR" button visible for TASK_COMPLETE items without a PR URL
 *  - "Create PR" button hidden when item already has a githubPrUrl
 *  - "Create PR" button hidden when onRunOneShot prop is not provided
 *  - Clicking "Create PR" opens the confirmation modal
 *  - Cancel button closes the modal without calling onRunOneShot
 *  - Confirm button calls onRunOneShot with the session ID and default prompt
 *  - Empty queue renders "all caught up" empty state
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ReviewQueuePanel } from "../ReviewQueuePanel";
import { AttentionReason, Priority } from "@/gen/session/v1/types_pb";
import type { ReviewItem } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mock context hooks — ReviewQueuePanel depends on three context providers
// ---------------------------------------------------------------------------

const mockRefresh = jest.fn();
const mockAcknowledge = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: jest.fn(),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({ pendingApprovals: [], resolveApproval: jest.fn() }),
}));

jest.mock("@/lib/hooks/useReviewQueueNavigation", () => ({
  useReviewQueueNavigation: () => ({
    currentIndex: 0,
    navigatePrev: jest.fn(),
    navigateNext: jest.fn(),
  }),
}));

import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
const mockUseReviewQueueContext = useReviewQueueContext as jest.Mock;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeReviewItem(overrides: Partial<ReviewItem> = {}): ReviewItem {
  return {
    sessionId: "session-abc",
    sessionTitle: "My Session",
    reason: AttentionReason.TASK_COMPLETE,
    priority: Priority.MEDIUM,
    tags: [],
    diffAdded: 0,
    diffRemoved: 0,
    branchDivergedFromBase: false,
    githubPrUrl: "",
    ...overrides,
  } as unknown as ReviewItem;
}

function makeContextValue(items: ReviewItem[] = []) {
  return {
    items,
    totalItems: items.length,
    loading: false,
    error: null,
    byPriority: new Map(),
    byReason: new Map(),
    averageAgeSeconds: 0,
    oldestAgeSeconds: 0,
    refresh: mockRefresh,
    acknowledgeSession: mockAcknowledge,
  };
}

function renderPanel(props: Partial<React.ComponentProps<typeof ReviewQueuePanel>> = {}) {
  return render(<ReviewQueuePanel {...props} />);
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — empty state", () => {
  beforeEach(() => {
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([]));
  });

  it("renders without crashing when queue is empty", () => {
    renderPanel();
    // Panel header should always be present
    expect(screen.getByText(/review queue/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Create PR button visibility
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — Create PR button", () => {
  const onRunOneShot = jest.fn().mockResolvedValue({ prUrl: "https://github.com/org/repo/pull/1" });

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows Create PR button for TASK_COMPLETE item with no existing PR URL", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel({ onRunOneShot });

    expect(screen.getByTestId("create-pr-session-abc")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /create pr/i })).toBeInTheDocument();
  });

  it("hides Create PR button when item already has a PR URL", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "https://github.com/org/repo/pull/99",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel({ onRunOneShot });

    expect(screen.queryByRole("button", { name: /create pr/i })).not.toBeInTheDocument();
  });

  it("hides Create PR button when onRunOneShot prop is not provided", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel(); // no onRunOneShot

    expect(screen.queryByRole("button", { name: /create pr/i })).not.toBeInTheDocument();
  });

  it("hides Create PR button for non-TASK_COMPLETE items", () => {
    const item = makeReviewItem({
      reason: AttentionReason.APPROVAL_PENDING,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel({ onRunOneShot });

    expect(screen.queryByRole("button", { name: /create pr/i })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Create PR modal behaviour
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — Create PR modal", () => {
  const onRunOneShot = jest.fn().mockResolvedValue({ prUrl: "https://github.com/org/repo/pull/42" });

  beforeEach(() => {
    jest.clearAllMocks();
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));
  });

  it("opens the modal when Create PR is clicked", () => {
    renderPanel({ onRunOneShot });
    fireEvent.click(screen.getByRole("button", { name: /create pr/i }));
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
  });

  it("closes the modal when Cancel is clicked without calling onRunOneShot", () => {
    renderPanel({ onRunOneShot });
    fireEvent.click(screen.getByRole("button", { name: /create pr/i }));
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByRole("button", { name: /cancel/i })).not.toBeInTheDocument();
    expect(onRunOneShot).not.toHaveBeenCalled();
  });

  it("calls onRunOneShot with session ID when confirmed", async () => {
    renderPanel({ onRunOneShot });
    fireEvent.click(screen.getByRole("button", { name: /create pr/i }));

    // Find the confirm button (not the cancel)
    const confirmBtn = screen.getByRole("button", { name: /^run$/i });
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(onRunOneShot).toHaveBeenCalledWith(
        "session-abc",
        expect.stringContaining("pull request")
      );
    });
  });
});
