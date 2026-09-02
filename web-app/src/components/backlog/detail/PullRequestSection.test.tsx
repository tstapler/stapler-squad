import { render, screen } from "@testing-library/react";
import { PullRequestSection } from "./PullRequestSection";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: `entity-${Math.random()}`,
    sessionId: `session-${Math.random()}`,
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_pr_section",
    title: "Item with a PR",
    status: "pr_pending",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    prUrl: "https://github.com/tstapler/stapler-squad/pull/700",
    prNumber: 700,
    ...overrides,
  };
}

describe("PullRequestSection", () => {
  it("renders the unmodified GitHubBadge plus an adjacent Jules marker when the PR-producing session role is jules_work", () => {
    const item = makeItem({
      linkedSessions: [makeSession({ sessionId: "jules-sessions/xyz", role: "jules_work" })],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    const prLink = screen.getByRole("link", { name: /View GitHub Pull Request #700/ });
    expect(prLink).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/pull/700");

    const marker = screen.getByLabelText("Opened by Jules");
    expect(marker).toHaveTextContent("Jules");
  });

  it("renders no Jules marker when the PR-producing session's role is not jules_work", () => {
    const item = makeItem({
      linkedSessions: [makeSession({ sessionId: "work-1", role: "work" })],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    expect(screen.getByRole("link", { name: /View GitHub Pull Request #700/ })).toBeInTheDocument();
    expect(screen.queryByLabelText("Opened by Jules")).not.toBeInTheDocument();
  });
});
