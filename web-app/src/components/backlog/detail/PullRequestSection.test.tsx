import { render, screen } from "@testing-library/react";
import { PullRequestSection, prStatusLabel } from "./PullRequestSection";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
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
    autoApprovePlan: false,
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

  it("still shows Opened by Jules when a review session links after the jules_work session that produced the PR", () => {
    // Regression test: a review-gate session links *after* the jules_work
    // session it reviews (see julesDispatchGate.ts's skipReviewGate flow),
    // so the most-recently-linked session is "review", not "jules_work".
    // The badge must attribute the PR to the session that actually produced
    // it, not simply the last-linked session.
    const item = makeItem({
      linkedSessions: [
        makeSession({ sessionId: "jules-sessions/xyz", role: "jules_work" }),
        makeSession({ sessionId: "review-session-abc", role: "review" }),
      ],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    expect(screen.getByRole("link", { name: /View GitHub Pull Request #700/ })).toBeInTheDocument();
    const marker = screen.getByLabelText("Opened by Jules");
    expect(marker).toHaveTextContent("Jules");
  });
});

describe("prStatusLabel", () => {
  it("reports needs-fix when PR_NEEDS_FIX is open, even alongside other reasons", () => {
    expect(prStatusLabel([StuckReason.MULTIPLE_REASONS, StuckReason.PR_NEEDS_FIX])).toMatch(/needs fixes/);
  });

  it("reports ready to merge for PR_READY_UNMERGED", () => {
    expect(prStatusLabel([StuckReason.PR_READY_UNMERGED])).toBe("ready to merge");
  });

  it("falls back to waiting for merge with no relevant reason", () => {
    expect(prStatusLabel()).toBe("waiting for merge");
    expect(prStatusLabel([StuckReason.BOUNCING])).toBe("waiting for merge");
  });

  it("renders the derived label in the section", () => {
    render(
      <PullRequestSection
        item={makeItem()}
        actionLoading={null}
        onMarkDone={jest.fn()}
        stuckReasons={[StuckReason.PR_NEEDS_FIX]}
      />
    );
    expect(screen.getByText(/PR #700 — needs fixes/)).toBeInTheDocument();
  });
});
