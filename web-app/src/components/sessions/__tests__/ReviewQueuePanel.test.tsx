/**
 * Tests for ReviewQueuePanel — feature: review-queue-pr-creation
 *                            + feature: rules:create-from-review-queue (Epic 4)
 *
 * Covers:
 *  - "Create PR" trigger visible (enabled) for TASK_COMPLETE items with commits ahead
 *  - "Create PR" trigger disabled when there are no commits ahead (State B)
 *  - "View PR" link shown instead of the trigger when item already has a githubPrUrl (State C)
 *  - "Create PR" trigger hidden for non-TASK_COMPLETE items
 *  - Clicking the trigger opens the shared CreatePullRequestModal (Epic 2.4)
 *  - Empty queue renders "all caught up" empty state
 *  - "Create Rule" button visible for APPROVAL_PENDING items with a command
 *  - Clicking "Create Rule" opens the rule modal with loading state
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReviewQueuePanel, isCreateRuleEligibleCategory } from "../ReviewQueuePanel";
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

// Epic 2.4: ReviewQueuePanel now resolves draftPullRequest/createPullRequest straight from
// SessionServiceContext (mirrors SessionActionsOverflow.tsx's Epic 2.3 wiring).
const mockDraftPullRequest = jest.fn();
const mockCreatePullRequest = jest.fn();

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    draftPullRequest: mockDraftPullRequest,
    createPullRequest: mockCreatePullRequest,
  }),
}));

// CreatePullRequestModal has its own dedicated test suite (CreatePullRequestModal.test.tsx) —
// stub it here so these tests verify wiring (trigger -> open/close) without duplicating that
// coverage or dealing with the modal's own async draft-fetch lifecycle.
jest.mock("../CreatePullRequestModal", () => ({
  CreatePullRequestModal: ({
    session,
    isOpen,
    onClose,
  }: {
    session: { id: string };
    isOpen: boolean;
    onClose: () => void;
  }) =>
    isOpen ? (
      <div data-testid="create-pr-modal" data-session-id={session.id}>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
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
    diffStats: undefined,
    // false is the "no commits ahead" (State B) signal the Create PR trigger disables on.
    hasCommitsAhead: false,
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
// Create PR trigger visibility (three states — ux.md Surface 1 & 2)
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — Create PR trigger", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows an enabled Create PR trigger for a TASK_COMPLETE item with commits ahead (State A)", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
      hasCommitsAhead: true,
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    const trigger = screen.getByTestId("create-pr-trigger-session-abc");
    expect(trigger).toBeInTheDocument();
    expect(trigger).not.toBeDisabled();
  });

  it("shows a disabled Create PR trigger when there are no commits ahead (State B)", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
      diffStats: undefined,
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    const trigger = screen.getByTestId("create-pr-trigger-session-abc");
    expect(trigger).toBeDisabled();
    expect(trigger).toHaveAttribute("title", "No commits ahead of main yet");
  });

  it("shows a View PR link instead of the trigger when item already has a PR URL (State C)", () => {
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "https://github.com/org/repo/pull/99",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.queryByTestId("create-pr-trigger-session-abc")).not.toBeInTheDocument();
    const link = screen.getByTestId("github-pr-link");
    expect(link).toHaveAttribute("href", "https://github.com/org/repo/pull/99");
    expect(link).toHaveTextContent("#99");
  });

  it("hides the Create PR trigger area for non-TASK_COMPLETE items", () => {
    const item = makeReviewItem({
      reason: AttentionReason.APPROVAL_PENDING,
      githubPrUrl: "",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.queryByTestId("create-pr-trigger-session-abc")).not.toBeInTheDocument();
    expect(screen.queryByTestId("github-pr-link")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Create PR modal wiring — the modal itself is unit-tested in
// CreatePullRequestModal.test.tsx; these tests only cover trigger -> open/close.
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — Create PR modal wiring", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    const item = makeReviewItem({
      reason: AttentionReason.TASK_COMPLETE,
      githubPrUrl: "",
      hasCommitsAhead: true,
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));
  });

  it("opens the shared CreatePullRequestModal for the clicked session when the trigger is clicked", () => {
    renderPanel();
    fireEvent.click(screen.getByTestId("create-pr-trigger-session-abc"));

    const modal = screen.getByTestId("create-pr-modal");
    expect(modal).toBeInTheDocument();
    expect(modal).toHaveAttribute("data-session-id", "session-abc");
  });

  it("closes the modal when the modal's onClose fires", () => {
    renderPanel();
    fireEvent.click(screen.getByTestId("create-pr-trigger-session-abc"));
    fireEvent.click(screen.getByRole("button", { name: /close/i }));

    expect(screen.queryByTestId("create-pr-modal")).not.toBeInTheDocument();
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
      escalation_reason_category: "no-match",
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

describe("ReviewQueuePanel — exclude filters", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  function openFilters() {
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
  }

  it("cycles a priority pill through neutral -> include -> exclude -> neutral", () => {
    const urgent = makeReviewItem({ sessionId: "s-urgent", sessionName: "Urgent Item", priority: Priority.URGENT });
    const low = makeReviewItem({ sessionId: "s-low", sessionName: "Low Item", priority: Priority.LOW });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([urgent, low]));

    renderPanel();
    openFilters();

    const pill = () => screen.getByRole("button", { name: /Urgent \(1\)/ });

    // neutral -> include
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-urgent")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-low")).not.toBeInTheDocument();

    // include -> exclude
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).toHaveTextContent("🚫");
    expect(screen.queryByTestId("review-item-s-urgent")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-low")).toBeInTheDocument();

    // exclude -> neutral
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).not.toHaveTextContent("🚫");
    expect(screen.getByTestId("review-item-s-urgent")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-low")).toBeInTheDocument();
  });

  it("excludes items by program when a program pill is clicked twice", () => {
    const claude = makeReviewItem({ sessionId: "s1", sessionName: "First Item", program: "claude" });
    const aider = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", program: "aider" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([claude, aider]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /aider \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s2")).not.toBeInTheDocument();
  });

  it("excludes items by category when a category pill is clicked twice", () => {
    const bugfix = makeReviewItem({ sessionId: "s1", sessionName: "First Item", category: "bugfix" });
    const feature = makeReviewItem({ sessionId: "s2", sessionName: "Second Item", category: "feature" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([bugfix, feature]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /feature \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s2")).not.toBeInTheDocument();
  });

  it("counts excluded values toward the active filter count and clear-all resets them", () => {
    const urgent = makeReviewItem({ sessionId: "s1", sessionName: "S1", priority: Priority.URGENT });
    const low = makeReviewItem({ sessionId: "s2", sessionName: "S2", priority: Priority.LOW });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([urgent, low]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /Urgent \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(screen.queryByTestId("review-item-s1")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /clear active filter/i }));

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s2")).toBeInTheDocument();
    expect(pill).not.toHaveTextContent("🚫");
  });

  it("persists an excluded priority to the URL and hydrates it back on mount", () => {
    const urgent = makeReviewItem({ sessionId: "s1", sessionName: "S1", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([urgent]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /Urgent \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining(`priorityExclude=${Priority.URGENT}`),
      expect.objectContaining({ scroll: false })
    );

    mockSearchParams = new URLSearchParams({ priorityExclude: String(Priority.URGENT) });
    renderPanel();
    fireEvent.click(screen.getAllByRole("button", { name: /^Filter/ })[1]);

    const rehydrated = screen.getAllByRole("button", { name: /Urgent \(1\)/ })[1];
    expect(rehydrated).toHaveAttribute("aria-pressed", "false");
    expect(rehydrated).toHaveTextContent("🚫");
  });

  it("cycles a reason pill through neutral -> include -> exclude -> neutral", () => {
    const errorItem = makeReviewItem({ sessionId: "s-error", sessionName: "Error Item", reason: AttentionReason.ERROR_STATE });
    const idleItem = makeReviewItem({ sessionId: "s-idle", sessionName: "Idle Item", reason: AttentionReason.IDLE });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([errorItem, idleItem]));

    renderPanel();
    openFilters();

    const pill = () => screen.getByRole("button", { name: /Error \(1\)/ });

    // neutral -> include
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-error")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-idle")).not.toBeInTheDocument();

    // include -> exclude
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).toHaveTextContent("🚫");
    expect(screen.queryByTestId("review-item-s-error")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-idle")).toBeInTheDocument();

    // exclude -> neutral
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).not.toHaveTextContent("🚫");
    expect(screen.getByTestId("review-item-s-error")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-idle")).toBeInTheDocument();
  });

  it("persists an excluded reason to the URL and hydrates it back on mount", () => {
    const errorItem = makeReviewItem({ sessionId: "s1", sessionName: "S1", reason: AttentionReason.ERROR_STATE });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([errorItem]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /Error \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining(`reasonExclude=${AttentionReason.ERROR_STATE}`),
      expect.objectContaining({ scroll: false })
    );

    mockSearchParams = new URLSearchParams({ reasonExclude: String(AttentionReason.ERROR_STATE) });
    renderPanel();
    fireEvent.click(screen.getAllByRole("button", { name: /^Filter/ })[1]);

    const rehydrated = screen.getAllByRole("button", { name: /Error \(1\)/ })[1];
    expect(rehydrated).toHaveAttribute("aria-pressed", "false");
    expect(rehydrated).toHaveTextContent("🚫");
  });

  it("cycles a severity pill through neutral -> include -> exclude -> neutral", () => {
    const critical = makeReviewItem({
      sessionId: "s-critical",
      sessionName: "Critical Item",
      reason: AttentionReason.APPROVAL_PENDING,
      metadata: { pending_approval_id: "appr-1", risk_level: "critical" },
    } as Partial<ReviewItem>);
    const low = makeReviewItem({
      sessionId: "s-low",
      sessionName: "Low Item",
      reason: AttentionReason.APPROVAL_PENDING,
      metadata: { pending_approval_id: "appr-2", risk_level: "low" },
    } as Partial<ReviewItem>);
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([critical, low]));

    renderPanel();
    openFilters();

    const pill = () => screen.getByRole("button", { name: /Critical \(1\)/ });

    // neutral -> include
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-critical")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-low")).not.toBeInTheDocument();

    // include -> exclude
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).toHaveTextContent("🚫");
    expect(screen.queryByTestId("review-item-s-critical")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-low")).toBeInTheDocument();

    // exclude -> neutral
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).not.toHaveTextContent("🚫");
    expect(screen.getByTestId("review-item-s-critical")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-low")).toBeInTheDocument();
  });

  it("persists an excluded severity to the URL and hydrates it back on mount", () => {
    const critical = makeReviewItem({
      sessionId: "s1",
      sessionName: "S1",
      reason: AttentionReason.APPROVAL_PENDING,
      metadata: { pending_approval_id: "appr-1", risk_level: "critical" },
    } as Partial<ReviewItem>);
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([critical]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /Critical \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("severityExclude=critical"),
      expect.objectContaining({ scroll: false })
    );

    mockSearchParams = new URLSearchParams({ severityExclude: "critical" });
    renderPanel();
    fireEvent.click(screen.getAllByRole("button", { name: /^Filter/ })[1]);

    const rehydrated = screen.getAllByRole("button", { name: /Critical \(1\)/ })[1];
    expect(rehydrated).toHaveAttribute("aria-pressed", "false");
    expect(rehydrated).toHaveTextContent("🚫");
  });

  it("cycles a tag pill through neutral -> include -> exclude -> neutral", () => {
    const backend = makeReviewItem({ sessionId: "s-backend", sessionName: "Backend Item", tags: ["backend"] });
    const frontend = makeReviewItem({ sessionId: "s-frontend", sessionName: "Frontend Item", tags: ["frontend"] });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([backend, frontend]));

    renderPanel();
    openFilters();

    const pill = () => screen.getByRole("button", { name: /backend \(1\)/ });

    // neutral -> include
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-backend")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-frontend")).not.toBeInTheDocument();

    // include -> exclude
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).toHaveTextContent("🚫");
    expect(screen.queryByTestId("review-item-s-backend")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-frontend")).toBeInTheDocument();

    // exclude -> neutral
    fireEvent.click(pill());
    expect(pill()).toHaveAttribute("aria-pressed", "false");
    expect(pill()).not.toHaveTextContent("🚫");
    expect(screen.getByTestId("review-item-s-backend")).toBeInTheDocument();
    expect(screen.getByTestId("review-item-s-frontend")).toBeInTheDocument();
  });

  it("persists an excluded tag to the URL and hydrates it back on mount", () => {
    const backend = makeReviewItem({ sessionId: "s1", sessionName: "S1", tags: ["backend"] });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([backend]));

    renderPanel();
    openFilters();

    const pill = screen.getByRole("button", { name: /backend \(1\)/ });
    fireEvent.click(pill); // include
    fireEvent.click(pill); // exclude

    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("tagExclude=backend"),
      expect.objectContaining({ scroll: false })
    );

    mockSearchParams = new URLSearchParams({ tagExclude: "backend" });
    renderPanel();
    fireEvent.click(screen.getAllByRole("button", { name: /^Filter/ })[1]);

    const rehydrated = screen.getAllByRole("button", { name: /backend \(1\)/ })[1];
    expect(rehydrated).toHaveAttribute("aria-pressed", "false");
    expect(rehydrated).toHaveTextContent("🚫");
  });
});

// ---------------------------------------------------------------------------
// Severity (review-queue-severity Epic 6)
// ---------------------------------------------------------------------------

describe("ReviewQueuePanel — severity", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  function openFilters() {
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
  }

  function makeApprovalItem(overrides: Partial<ReviewItem> & { riskLevel?: string } = {}): ReviewItem {
    const { riskLevel, ...rest } = overrides;
    const metadata: Record<string, string> = { pending_approval_id: "appr-1" };
    if (riskLevel !== undefined) metadata["risk_level"] = riskLevel;
    return makeReviewItem({
      reason: AttentionReason.APPROVAL_PENDING,
      metadata,
      ...rest,
    } as Partial<ReviewItem>);
  }

  it("ReviewQueuePanel_should_SortItemsBySeverityDescending_When_DefaultSortFieldIsSeverity", () => {
    const low = makeApprovalItem({ sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" });
    const critical = makeApprovalItem({ sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" });
    const medium = makeApprovalItem({ sessionId: "s-medium", sessionName: "Medium Item", riskLevel: "medium" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([low, critical, medium]));

    renderPanel();

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-critical", "s-medium", "s-low"]);
  });

  it("ReviewQueuePanel_should_RankUnrecordedBetweenCriticalAndMedium_When_DefaultSortApplied", () => {
    const critical = makeApprovalItem({ sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" });
    const unrecorded = makeApprovalItem({ sessionId: "s-unrecorded", sessionName: "Unrecorded Item" });
    const medium = makeApprovalItem({ sessionId: "s-medium", sessionName: "Medium Item", riskLevel: "medium" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([medium, unrecorded, critical]));

    renderPanel();

    const ids = Array.from(document.querySelectorAll("[data-session-id]")).map((el) =>
      el.getAttribute("data-session-id")
    );
    expect(ids).toEqual(["s-critical", "s-unrecorded", "s-medium"]);
  });

  it("ReviewQueuePanel_should_ShowOnlyCriticalItems_When_CriticalSeverityChipClicked", () => {
    const critical = makeApprovalItem({ sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" });
    const low = makeApprovalItem({ sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([critical, low]));

    renderPanel();
    openFilters();

    const criticalChip = screen.getByRole("button", { name: /Critical \(1\)/ });
    fireEvent.click(criticalChip);

    expect(criticalChip).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-critical")).toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-low")).not.toBeInTheDocument();
  });

  it("ReviewQueuePanel_should_ShowSharedEmptyState_When_SeverityFilterMatchesZeroItems", () => {
    // Low-severity approval-pending item, plus an unrelated Idle-reason item with no
    // risk_level metadata at all. Low (severity dim) AND Idle (reason dim) each individually
    // match one item, but their combination (AND across dimensions) matches zero.
    const low = makeApprovalItem({ sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" });
    const idle = makeReviewItem({ sessionId: "s-idle", sessionName: "Idle Item", reason: AttentionReason.IDLE });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([low, idle]));

    renderPanel();
    openFilters();

    fireEvent.click(screen.getByRole("button", { name: /Low \(1\)/ }));
    fireEvent.click(screen.getByRole("button", { name: /Idle \(1\)/ }));

    expect(screen.queryByTestId("review-item-s-low")).not.toBeInTheDocument();
    expect(screen.queryByTestId("review-item-s-idle")).not.toBeInTheDocument();
    expect(screen.getByText(/No items match the current filter/i)).toBeInTheDocument();
  });

  it("ReviewQueuePanel_should_RenderCompactSeverityBadgeNextToEscalationReason_When_ApprovalPendingItemRenders", () => {
    const item = makeApprovalItem({ sessionId: "s-1", sessionName: "S1", riskLevel: "critical" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("severity-badge-critical")).toHaveAttribute("aria-label", "Critical risk");
  });

  it("ReviewQueuePanel_should_RenderNotRecordedBadgeState_When_RiskLevelMetadataKeyIsAbsent", () => {
    const item = makeApprovalItem({ sessionId: "s-1", sessionName: "S1" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("severity-badge-unrecorded")).toHaveAttribute("aria-label", "Severity not recorded");
  });

  it("ReviewQueuePanel_should_BucketUnrecognizedRiskLevel_When_FilteringByNotRecorded", () => {
    // PR #411 review finding: an unrecognized future risk_level value must still be
    // reachable via the "Not recorded" chip, not become its own unfilterable key.
    const unrecognized = makeApprovalItem({ sessionId: "s-future", sessionName: "Future Item", riskLevel: "extreme" });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([unrecognized]));

    renderPanel();
    openFilters();

    const notRecordedChip = screen.getByRole("button", { name: /Not recorded \(1\)/ });
    fireEvent.click(notRecordedChip);

    expect(notRecordedChip).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-item-s-future")).toBeInTheDocument();
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

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
    fireEvent.change(screen.getByLabelText(/group by/i), { target: { value: "program" } });

    expect(screen.getByTestId("review-group-claude")).toBeInTheDocument();
    expect(screen.getByTestId("review-group-aider")).toBeInTheDocument();

    // Action buttons (Create PR) render correctly for both items despite grouping.
    expect(screen.getByTestId("create-pr-trigger-s1")).toBeInTheDocument();
    expect(screen.getByTestId("create-pr-trigger-s2")).toBeInTheDocument();

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
    mockSearchParams = new URLSearchParams({
      priority: String(Priority.URGENT),
      q: "login",
      category: "bugfix",
      tag: "backend",
      sort: "name",
      group: "program",
    });
    const item = makeReviewItem({
      sessionId: "s1",
      sessionName: "First Item",
      priority: Priority.URGENT,
      category: "bugfix",
      tags: ["backend"],
      program: "claude",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));

    expect(screen.getByRole("button", { name: "Urgent (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("review-queue-search")).toHaveValue("login");
    expect(screen.getByRole("button", { name: "bugfix (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "backend (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText(/sort by/i)).toHaveValue("name");
    expect(screen.getByLabelText(/group by/i)).toHaveValue("program");
  });

  it("writes filter changes through to the URL via useFilterState", () => {
    const item = makeReviewItem({
      sessionId: "s1",
      sessionName: "First Item",
      priority: Priority.URGENT,
      category: "bugfix",
      tags: ["backend"],
      program: "claude",
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));

    fireEvent.click(screen.getByRole("button", { name: "Urgent (1)" }));
    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining(`priority=${Priority.URGENT}`),
      expect.objectContaining({ scroll: false })
    );

    fireEvent.click(screen.getByRole("button", { name: "bugfix (1)" }));
    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("category=bugfix"),
      expect.objectContaining({ scroll: false })
    );

    fireEvent.click(screen.getByRole("button", { name: "backend (1)" }));
    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("tag=backend"),
      expect.objectContaining({ scroll: false })
    );

    fireEvent.change(screen.getByLabelText(/sort by/i), { target: { value: "name" } });
    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("sort=name"),
      expect.objectContaining({ scroll: false })
    );

    fireEvent.change(screen.getByLabelText(/group by/i), { target: { value: "program" } });
    expect(mockReplace).toHaveBeenCalledWith(
      expect.stringContaining("group=program"),
      expect.objectContaining({ scroll: false })
    );
  });

  it("ignores non-numeric priority values in the URL instead of filtering out every item", () => {
    // Regression test: parseNumSet previously kept NaN when hydrating from a non-numeric
    // URL value (e.g. ?priority=abc), producing a non-empty Set(NaN). Since no item's
    // priority ever equals NaN, priorityFilter.size > 0 caused every item to be filtered out.
    mockSearchParams = new URLSearchParams({ priority: "abc" });
    const item = makeReviewItem({ sessionId: "s1", sessionName: "First Item", priority: Priority.URGENT });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("review-item-s1")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^Filter/ }));
    expect(screen.getByRole("button", { name: "Urgent (1)" })).toHaveAttribute("aria-pressed", "false");
  });
});

// ---------------------------------------------------------------------------
// Epic 3.1 (escalation-reasoning): reason line rendering
// ---------------------------------------------------------------------------

import { escalationReasonText } from "../ReviewQueuePanel.css";
import { button } from "@/components/ui/Button.css";

describe("escalation reason", () => {
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

  it("renders the reason text with the category-driven emoji prefix (no-match)", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "rm -rf /tmp/foo",
        escalation_reason: "No matching rule; escalated for manual review.",
        escalation_reason_category: "no-match",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.getByText("❓ No matching rule; escalated for manual review.")
    ).toBeInTheDocument();
  });

  it("renders backend text verbatim with a different emoji for a different category (explicit-rule)", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "git branch -D main",
        escalation_reason: "Branch deletion modifies repository structure and should be reviewed.",
        escalation_reason_category: "explicit-rule",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.getByText("🛑 Branch deletion modifies repository structure and should be reviewed.")
    ).toBeInTheDocument();
  });

  it("shows the orphaned-approval fallback copy when escalation_reason is absent", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "git push origin main",
        // no escalation_reason / escalation_reason_category — pre-feature approval
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(
      screen.getByText("Reason not recorded — this request predates escalation-reason tracking.")
    ).toBeInTheDocument();
  });

  it("applies the bounded escalationReasonText class (not bare itemContext) for a long reason", () => {
    const longReason = "x".repeat(600);
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "rm -rf /tmp/foo",
        escalation_reason: longReason,
        escalation_reason_category: "no-match",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    const reasonEl = screen.getByText(`❓ ${longReason}`);
    expect(reasonEl).toHaveClass(String(escalationReasonText));
  });

  it("renders create-rule button with intent=secondary when category is no-match", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "rm -rf /tmp/foo",
        tool_name: "Bash",
        escalation_reason: "No matching rule; escalated for manual review.",
        escalation_reason_category: "no-match",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    const createRuleButton = screen.getByTestId("create-rule-session-approval");
    expect(createRuleButton).toBeInTheDocument();
    expect(createRuleButton).toHaveClass(
      String(button({ intent: "secondary", size: "md" }))
    );
  });

  it("omits create-rule button when category is domain-age", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "curl https://newly-registered-domain.example/install.sh",
        tool_name: "Bash",
        escalation_reason: "Domain registered 3 days ago.",
        escalation_reason_category: "domain-age",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.queryByTestId("create-rule-session-approval")).not.toBeInTheDocument();
  });

  it("ReviewQueue_should_showCreateRuleButton_On_OrphanedApprovalMissingCategory", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "git push origin main",
        tool_name: "Bash",
        // no escalation_reason / escalation_reason_category — orphaned pre-deploy approval
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("create-rule-session-approval")).toBeInTheDocument();
  });

  it.each(["explicit-rule", "secret-scan", "unclassifiable", "unexpected"])(
    "omits create-rule button when category is %s",
    (category) => {
      const item = makeApprovalItem({
        metadata: {
          pending_approval_id: "approval-123",
          tool_input_command: "rm -rf /tmp/foo",
          tool_name: "Bash",
          escalation_reason: "test reason",
          escalation_reason_category: category,
        },
      });
      mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

      renderPanel();

      expect(screen.queryByTestId("create-rule-session-approval")).not.toBeInTheDocument();
    }
  );

  it("shows create-rule button for an unrecognized/future category (fail-open by design)", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "git push origin main",
        tool_name: "Bash",
        escalation_reason_category: "some-future-category",
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("create-rule-session-approval")).toBeInTheDocument();
  });

  describe("isCreateRuleEligibleCategory", () => {
    it("returns true for an orphaned approval (category undefined)", () => {
      expect(isCreateRuleEligibleCategory(undefined)).toBe(true);
    });

    it("returns true for no-match", () => {
      expect(isCreateRuleEligibleCategory("no-match")).toBe(true);
    });

    it.each(["explicit-rule", "domain-age", "secret-scan", "unclassifiable", "unexpected"])(
      "returns false for %s (ties this guard to the 5 known non-no-match EscalationCategory constants)",
      (category) => {
        expect(isCreateRuleEligibleCategory(category)).toBe(false);
      }
    );

    it("returns true for an unrecognized/future category (fail-open by design)", () => {
      expect(isCreateRuleEligibleCategory("some-future-category")).toBe(true);
    });
  });
});
