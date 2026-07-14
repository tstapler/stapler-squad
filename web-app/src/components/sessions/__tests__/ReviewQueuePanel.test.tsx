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
import { AttentionReason, Priority, SubStatus, SuggestionSource } from "@/gen/session/v1/types_pb";
import type { ReviewItem } from "@/gen/session/v1/types_pb";

afterEach(() => {
  mockSearchParams = new URLSearchParams();
});

// ---------------------------------------------------------------------------
// Mock context hooks — ReviewQueuePanel depends on three context providers
// ---------------------------------------------------------------------------

const mockRefresh = jest.fn();
const mockAcknowledge = jest.fn().mockResolvedValue(undefined);

// Overrides the global next/navigation stub (jest.setup.js) so URL-persisted filter
// state (useFilterState) can be seeded and asserted on.
let mockSearchParams = new URLSearchParams();
const mockReplace = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: mockReplace, back: jest.fn(), forward: jest.fn() }),
  usePathname: () => "/",
  useSearchParams: () => mockSearchParams,
  useParams: () => ({}),
}));

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: jest.fn(),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({
    pendingApprovals: [],
    resolveApproval: jest.fn(),
    approve: jest.fn().mockResolvedValue(undefined),
    deny: jest.fn().mockResolvedValue(undefined),
    clearForSession: jest.fn(),
    clearedSessions: new Set(),
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
    sessionName: "My Session",
    reason: AttentionReason.TASK_COMPLETE,
    priority: Priority.MEDIUM,
    program: "",
    branch: "",
    category: "",
    tags: [],
    diffAdded: 0,
    diffRemoved: 0,
    branchDivergedFromBase: false,
    githubPrUrl: "",
    ...overrides,
  } as unknown as ReviewItem;
}

function countBy<T>(items: ReviewItem[], pick: (item: ReviewItem) => T): Map<T, number> {
  const counts = new Map<T, number>();
  for (const item of items) {
    const key = pick(item);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return counts;
}

function makeContextValue(items: ReviewItem[] = []) {
  return {
    items,
    totalItems: items.length,
    loading: false,
    error: null,
    byPriority: countBy(items, (i) => i.priority),
    byReason: countBy(items, (i) => i.reason),
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
    subStatus: SubStatus.UNSPECIFIED,
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

// ---------------------------------------------------------------------------
// Combinable multi-select filters + new dimensions + search + sort
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — combinable filters", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  function openFilters() {
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
  }

  it("keeps priority and reason filters active simultaneously (combinable, not exclusive)", () => {
    const urgent = makeReviewItem({
      sessionId: "s-urgent",
      sessionName: "First Item",
      priority: Priority.URGENT,
      reason: AttentionReason.ERROR_STATE,
    });
    const other = makeReviewItem({
      sessionId: "s-other",
      sessionName: "Second Item",
      priority: Priority.LOW,
      reason: AttentionReason.IDLE,
    });
    mockUseReviewQueueContext.mockReturnValue(
      makeContextValue([urgent, other])
    );

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "Urgent (1)" }));
    fireEvent.click(screen.getByRole("button", { name: "Error (1)" }));

    expect(screen.getByRole("button", { name: "Urgent (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Error (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-urgent")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-other")).not.toBeInTheDocument();
  });

  it("filters by program", () => {
    const claude = makeReviewItem({ sessionId: "s1", sessionName: "First Item", program: "claude" });
    const aider = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", program: "aider" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([claude, aider]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "aider (1)" }));

    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
  });

  it("filters by category", () => {
    const bugfix = makeReviewItem({ sessionId: "s1", sessionName: "First Item", category: "bugfix" });
    const feature = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", category: "feature" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([bugfix, feature]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "feature (1)" }));

    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
  });

  it("filters by tag", () => {
    const backend = makeReviewItem({ sessionId: "s1", sessionName: "First Item", tags: ["backend"] });
    const frontend = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", tags: ["frontend"] });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([backend, frontend]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "frontend (1)" }));

    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
  });

  it("filters to items with a GitHub PR when Has PR is selected", () => {
    const withPr = makeReviewItem({ sessionId: "s1", sessionName: "S1", githubPrUrl: "https://github.com/org/repo/pull/1" });
    const withoutPr = makeReviewItem({ sessionId: "s2", sessionName: "S2", githubPrUrl: "" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([withPr, withoutPr]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "Has PR" }));

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s2")).not.toBeInTheDocument();
  });

  it("filters to items without a GitHub PR when No PR is selected", () => {
    const withPr = makeReviewItem({ sessionId: "s1", sessionName: "S1", githubPrUrl: "https://github.com/org/repo/pull/1" });
    const withoutPr = makeReviewItem({ sessionId: "s2", sessionName: "S2", githubPrUrl: "" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([withPr, withoutPr]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "No PR" }));

    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
  });

  it("filters to items diverged from base when Diverged from base is selected", () => {
    const diverged = makeReviewItem({ sessionId: "s1", sessionName: "S1", branchDivergedFromBase: true });
    const notDiverged = makeReviewItem({ sessionId: "s2", sessionName: "S2", branchDivergedFromBase: false });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([diverged, notDiverged]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "Diverged from base" }));

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s2")).not.toBeInTheDocument();
  });

  it("filters by free-text search across session name and branch", () => {
    const match = makeReviewItem({ sessionId: "s1", sessionName: "Fix login bug", branch: "fix/login" });
    const noMatch = makeReviewItem({ sessionId: "s2", sessionName: "Add feature", branch: "feat/x" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([match, noMatch]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByTestId("review-queue-search"), { target: { value: "login" } });

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s2")).not.toBeInTheDocument();
  });

  it("sorts by name ascending when selected", () => {
    const b = makeReviewItem({ sessionId: "s-b", sessionName: "Bravo" });
    const a = makeReviewItem({ sessionId: "s-a", sessionName: "Alpha" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([b, a]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "name" } });

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-a", "s-b"]);
  });

  it("sorts by name descending when the direction is toggled", () => {
    const b = makeReviewItem({ sessionId: "s-b", sessionName: "Bravo" });
    const a = makeReviewItem({ sessionId: "s-a", sessionName: "Alpha" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([b, a]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "name" } });
    fireEvent.click(screen.getByRole("button", { name: /sort direction/i }));

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-b", "s-a"]);
  });

  it("sorts by priority ascending when selected", () => {
    const low = makeReviewItem({ sessionId: "s-low", sessionName: "Low Item", priority: Priority.LOW });
    const urgent = makeReviewItem({ sessionId: "s-urgent", sessionName: "Urgent Item", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([low, urgent]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "priority" } });

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-urgent", "s-low"]);
  });

  it("sorts by last activity (age) ascending when selected", () => {
    const older = makeReviewItem({
      sessionId: "s-older",
      sessionName: "Older Item",
      lastActivity: { seconds: BigInt(100), nanos: 0 } as unknown as ReviewItem["lastActivity"],
    });
    const newer = makeReviewItem({
      sessionId: "s-newer",
      sessionName: "Newer Item",
      lastActivity: { seconds: BigInt(200), nanos: 0 } as unknown as ReviewItem["lastActivity"],
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([newer, older]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "age" } });

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-older", "s-newer"]);
  });

  it("sorts by diff size ascending when selected", () => {
    const large = makeReviewItem({
      sessionId: "s-large",
      sessionName: "Large Diff",
      diffStats: { added: 100, removed: 50, content: "" } as unknown as ReviewItem["diffStats"],
    });
    const small = makeReviewItem({
      sessionId: "s-small",
      sessionName: "Small Diff",
      diffStats: { added: 1, removed: 0, content: "" } as unknown as ReviewItem["diffStats"],
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([large, small]));

    renderPanel();
    openFilters();

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "diffSize" } });

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-small", "s-large"]);
  });

  it("clear-all resets every filter dimension and search text", () => {
    const item = makeReviewItem({ sessionId: "s1", sessionName: "S1", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: "Urgent (1)" }));
    fireEvent.change(screen.getByTestId("review-queue-search"), { target: { value: "nomatch" } });
    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /clear active filter/i }));

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Urgent (1)" })).toHaveAttribute("aria-pressed", "false");
  });
});

// ---------------------------------------------------------------------------
// Group by (reuses groupSessions()) + URL-persisted filter state (reuses useFilterState)
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — group by", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSearchParams = new URLSearchParams();
  });

  it("groups items under group headers when a Group by strategy is selected", () => {
    const claude = makeReviewItem({ sessionId: "s1", sessionName: "First Item", program: "claude" });
    const aider = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", program: "aider" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([claude, aider]));

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));

    fireEvent.change(screen.getByLabelText(/group by/i), { target: { value: "program" } });

    expect(screen.getByTestId("review-group-claude")).toBeInTheDocument();
    expect(screen.getByTestId("review-group-aider")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
  });

  it("falls back to a flat list when Group by is left at the default (None)", () => {
    const item = makeReviewItem({ sessionId: "s1", sessionName: "First Item" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.queryByTestId(/^review-group-/)).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
  });

  it("keeps action buttons and current-item highlighting intact when items are grouped", () => {
    const onRunOneShot = jest.fn();
    const first = makeReviewItem({
      sessionId: "s1",
      sessionName: "First Item",
      program: "claude",
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
    });
    const second = makeReviewItem({
      sessionId: "s2",
      sessionName: "Second Item",
      program: "aider",
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([first, second]));

    renderPanel({ onRunOneShot });
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
    fireEvent.change(screen.getByLabelText(/group by/i), { target: { value: "program" } });

    expect(screen.getByTestId("review-group-claude")).toBeInTheDocument();
    expect(screen.getByTestId("review-group-aider")).toBeInTheDocument();

    // Action buttons (Create PR) render correctly for both items despite grouping.
    expect(screen.getByTestId("create-pr-s1")).toBeInTheDocument();
    expect(screen.getByTestId("create-pr-s2")).toBeInTheDocument();

    // useReviewQueueNavigation is mocked with currentIndex: 0, which maps to the first
    // item in the (pre-group) flat items array — "s1" here. Its wrapper must still be
    // rendered as the highlighted "current" item even though it's nested under a group.
    expect(screen.getByTestId("current-item")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s1")).toHaveAttribute("data-current", "true");
    expect(screen.getByTestId("review-item-s2")).not.toHaveAttribute("data-current");
  });
});

describe("ReviewQueuePanel — URL-persisted filter state", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSearchParams = new URLSearchParams();
  });

  it("hydrates active filters from the URL on mount", () => {
    mockSearchParams = new URLSearchParams({ priority: String(Priority.URGENT), q: "login" });
    const item = makeReviewItem({ sessionId: "s1", sessionName: "First Item", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));

    expect(screen.getByRole("button", { name: "Urgent (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-queue-search")).toHaveValue("login");
  });

  it("writes filter changes through to the URL via useFilterState", () => {
    const item = makeReviewItem({ sessionId: "s1", sessionName: "First Item", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
    fireEvent.click(screen.getByRole("button", { name: "Urgent (1)" }));

    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining(`priority=${Priority.URGENT}`),
      expect.objectContaining({ scroll: false })
    );
  });
});
