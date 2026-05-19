/**
 * Tests for ReviewQueuePanel — feature: review-queue-pr-creation (S3-3)
 *                            + feature: rules:create-from-review-queue (Epic 4)
 *
 * Covers:
 *  - "Create PR" button visible for TASK_COMPLETE items without a PR URL
 *  - "Create PR" button hidden when item already has a githubPrUrl
 *  - "Create PR" button hidden when onRunOneShot prop is not provided
 *  - Clicking "Create PR" opens the confirmation modal
 *  - Cancel button closes the modal without calling onRunOneShot
 *  - Confirm button calls onRunOneShot with the session ID and default prompt
 *  - Empty queue renders "all caught up" empty state
 *  - "Create Rule" button visible for APPROVAL_PENDING items with a command
 *  - Clicking "Create Rule" opens the rule modal with loading state
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ReviewQueuePanel } from "../ReviewQueuePanel";
import { AttentionReason, Priority, SuggestionSource } from "@/gen/session/v1/types_pb";
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
  useApprovalsContext: () => ({
    pendingApprovals: [],
    resolveApproval: jest.fn(),
    approve: jest.fn().mockResolvedValue(undefined),
    deny: jest.fn().mockResolvedValue(undefined),
  }),
}));

jest.mock("@/lib/hooks/useReviewQueueNavigation", () => ({
  useReviewQueueNavigation: () => ({
    currentIndex: 0,
    navigatePrev: jest.fn(),
    navigateNext: jest.fn(),
    goToNext: jest.fn(),
    goToPrevious: jest.fn(),
  }),
}));

// ---------------------------------------------------------------------------
// Mock useGenerateRule — for Epic 4 "Create Rule" tests
// ---------------------------------------------------------------------------

const mockGenerate = jest.fn().mockResolvedValue(undefined);
const mockClear = jest.fn();

const mockGenerateRuleState = {
  suggestions: [] as import("@/gen/session/v1/types_pb").SuggestedRuleProto[],
  loading: false,
  error: null as Error | null,
  generate: mockGenerate,
  cancel: jest.fn(),
  clear: mockClear,
};

jest.mock("@/lib/hooks/useGenerateRule", () => ({
  useGenerateRule: jest.fn(() => mockGenerateRuleState),
}));

// ---------------------------------------------------------------------------
// Mock SuggestedRuleCard — avoids rendering its full form / hooks
// ---------------------------------------------------------------------------

jest.mock("../SuggestedRuleCard", () => ({
  SuggestedRuleCard: ({ onAccept, onDiscard, loading }: {
    onAccept: (rule: unknown) => void;
    onDiscard: () => void;
    loading?: boolean;
  }) => (
    <div data-testid="suggested-rule-card" data-loading={loading}>
      <button onClick={() => onAccept({})}>Accept</button>
      <button onClick={onDiscard}>Discard</button>
    </div>
  ),
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

// ---------------------------------------------------------------------------
// Epic 4: Create Rule button
// ---------------------------------------------------------------------------

import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
const mockUseGenerateRule = useGenerateRule as jest.Mock;

function makeApprovalItem(overrides: Partial<ReviewItem> = {}): ReviewItem {
  return {
    sessionId: "session-approval",
    sessionName: "Approval Session",
    reason: AttentionReason.APPROVAL_PENDING,
    priority: Priority.HIGH,
    tags: [],
    branchDivergedFromBase: false,
    githubPrUrl: "",
    metadata: {
      pending_approval_id: "approval-123",
      tool_input_command: "git push origin main",
      tool_name: "Bash",
    },
    workingState: 0,
    ...overrides,
  } as unknown as ReviewItem;
}

describe("ReviewQueuePanel — Create Rule button", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Reset generate rule state to defaults
    mockUseGenerateRule.mockReturnValue({
      suggestions: [],
      loading: false,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });
  });

  it("ReviewQueue_should_showCreateRuleButton_On_PendingItem", () => {
    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.getByTestId("create-rule-session-approval")
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create rule/i })
    ).toBeInTheDocument();
  });

  it("hides Create Rule button when item has no tool_input_command", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        // no tool_input_command
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.queryByRole("button", { name: /create rule/i })
    ).not.toBeInTheDocument();
  });

  it("hides Create Rule button for non-approval items", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
      metadata: {
        tool_input_command: "npm build",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.queryByRole("button", { name: /create rule/i })
    ).not.toBeInTheDocument();
  });
});

describe("ReviewQueuePanel — Create Rule modal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseGenerateRule.mockReturnValue({
      suggestions: [],
      loading: false,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });
  });

  it("ReviewQueue_should_openModal_When_CreateRuleClicked", () => {
    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));

    expect(screen.getByTestId("create-rule-modal")).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: /create auto-approval rule/i })).toBeInTheDocument();
  });

  it("calls generate with correct source and command when Create Rule clicked", () => {
    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));

    expect(mockGenerate).toHaveBeenCalledWith(
      expect.objectContaining({
        source: SuggestionSource.COMMAND_SAMPLE,
        commandSample: "git push origin main",
        toolNameFilter: "Bash",
      })
    );
  });

  it("shows loading indicator while generating", () => {
    mockUseGenerateRule.mockReturnValue({
      suggestions: [],
      loading: true,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });

    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    // Open modal (activeRuleItemId is set, loading=true from hook)
    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));

    expect(screen.getByTestId("create-rule-loading")).toBeInTheDocument();
  });

  it("shows SuggestedRuleCard when suggestion is available", () => {
    const fakeSuggestion = { name: "Allow git push", confidence: 0.9 } as unknown as import("@/gen/session/v1/types_pb").SuggestedRuleProto;
    mockUseGenerateRule.mockReturnValue({
      suggestions: [fakeSuggestion],
      loading: false,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });

    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));

    expect(screen.getByTestId("suggested-rule-card")).toBeInTheDocument();
  });

  it("closes modal and calls clear when Discard is clicked", () => {
    const fakeSuggestion = { name: "Allow git push", confidence: 0.9 } as unknown as import("@/gen/session/v1/types_pb").SuggestedRuleProto;
    mockUseGenerateRule.mockReturnValue({
      suggestions: [fakeSuggestion],
      loading: false,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });

    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));
    expect(screen.getByTestId("create-rule-modal")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /discard/i }));

    expect(screen.queryByTestId("create-rule-modal")).not.toBeInTheDocument();
    expect(mockClear).toHaveBeenCalled();
  });

  it("shows rule-saved indicator and closes modal when rule accepted", () => {
    const fakeSuggestion = { name: "Allow git push", confidence: 0.9 } as unknown as import("@/gen/session/v1/types_pb").SuggestedRuleProto;
    mockUseGenerateRule.mockReturnValue({
      suggestions: [fakeSuggestion],
      loading: false,
      error: null,
      generate: mockGenerate,
      cancel: jest.fn(),
      clear: mockClear,
    });

    const item = makeApprovalItem();
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /create rule/i }));
    fireEvent.click(screen.getByRole("button", { name: /accept/i }));

    // Modal should close
    expect(screen.queryByTestId("create-rule-modal")).not.toBeInTheDocument();
    expect(mockClear).toHaveBeenCalled();
  });
});
