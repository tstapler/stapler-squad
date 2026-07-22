import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionsSection } from "./SessionsSection";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("../SessionMonitor", () => ({ SessionMonitor: () => null }));

beforeEach(() => {
  localStorage.clear();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: `entity-${Math.random()}`,
    sessionId: `session-${Math.random()}`,
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

function makeItem(linkedSessions: LinkedSession[], overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_df0d5872",
    title: "Chronically stuck item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions,
    notes: "",
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

/** Builds the df0d5872-shaped fixture from research/features.md finding #5:
 * 6 triage + 3 review + 2 work sessions = 11 total linked sessions. */
function makeElevenSessions(): LinkedSession[] {
  const sessions: LinkedSession[] = [];
  for (let i = 0; i < 6; i++) {
    sessions.push(makeSession({ sessionId: `headless-triage-${i}`, role: "triage" }));
  }
  for (let i = 0; i < 3; i++) {
    sessions.push(makeSession({ sessionId: `headless-re-review-${i}`, role: "review" }));
  }
  for (let i = 0; i < 2; i++) {
    sessions.push(makeSession({ sessionId: `work-session-${i}`, role: "work" }));
  }
  return sessions;
}

describe("SessionsSection", () => {
  it("SessionsSection_should_RenderShowMoreButtonAndRevealRemainingSessions_When_ElevenSessionsLinkedLikeDf0d5872Case", () => {
    const item = makeItem(makeElevenSessions());
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    // 5 most recent shown by default (cap), plus "Show 6 more".
    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    const showMore = screen.getByTestId("sessions-show-more");
    expect(showMore).toHaveTextContent("Show 6 more");

    // Identity check (regression for the "shows oldest, not most recent" bug):
    // linkedSessions is ascending by createdAt, so the visible 5 must be the
    // TAIL of the list (the 3 most recent triage/review + both work sessions),
    // never the oldest triage sessions from the head of the list.
    expect(screen.getByText("work-session-1")).toBeInTheDocument();
    expect(screen.getByText("work-session-0")).toBeInTheDocument();
    expect(screen.getByText("headless-re-review-2")).toBeInTheDocument();
    expect(screen.queryByText("headless-triage-0")).not.toBeInTheDocument();
    expect(screen.queryByText("headless-triage-5")).not.toBeInTheDocument();

    fireEvent.click(showMore);

    // All 11 shown inline, in the same list, no route change.
    expect(screen.getAllByRole("listitem")).toHaveLength(11);
    expect(screen.queryByTestId("sessions-show-more")).not.toBeInTheDocument();

    // Once expanded, the previously-hidden oldest sessions must now be present.
    expect(screen.getByText("headless-triage-0")).toBeInTheDocument();
    expect(screen.getByText("headless-triage-5")).toBeInTheDocument();
  });

  it("renders no Show More button when session count is at or below the cap", () => {
    const item = makeItem([makeSession(), makeSession(), makeSession()]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.queryByTestId("sessions-show-more")).not.toBeInTheDocument();
  });

  it("renders nothing when there are no linked sessions", () => {
    const item = makeItem([]);
    const { container } = render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("SessionsSection_should_RenderAnchorLinkUnchanged_When_SessionKindIsWork", () => {
    const item = makeItem([makeSession({ sessionId: "a1b2c3d4", role: "work" })]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    expect(screen.getByRole("link", { name: /a1b2c3d4/ })).toHaveAttribute("href", "/?session=a1b2c3d4");
  });

  it("SessionsSection_should_RenderCollapsibleNotDeadAnchor_When_ClassifySessionKindReturnsBlockedGuardrail", () => {
    const item = makeItem([
      makeSession({
        entityId: "e-diff-error",
        sessionId: "diff-error-9c1d4a",
        role: "review",
        reviewVerdict: { overallOutcome: "FAIL", summary: "Review blocked: could not compute a diff." },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    expect(screen.queryByRole("link", { name: /diff-error-9c1d4a/ })).not.toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-session-e-diff-error")).toBeInTheDocument();
  });

  it("SessionsSection_should_RenderCollapsibleDiagnosticInsteadOfDeadAnchor_When_SessionKindIsManualReviewMarker", () => {
    // Previously a dead <a href="/?session=manual-review-..."> per the
    // Story 1.1.3 bug — now a Collapsible header expanding to BlockedNotice.
    const item = makeItem([
      makeSession({
        entityId: "e-manual-review",
        sessionId: "manual-review-a1b2c3d4-1721577600000000000",
        role: "review",
        reviewVerdict: { overallOutcome: "PASS", summary: "Manual review: verified fix locally" },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    expect(
      screen.queryByRole("link", { name: /manual-review-a1b2c3d4/ })
    ).not.toBeInTheDocument();
    const header = screen.getByTestId("collapsible-header-session-e-manual-review");
    expect(header.tagName).toBe("BUTTON");
    expect(header).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(header);

    expect(screen.getByTestId("blocked-notice")).toBeInTheDocument();
    expect(screen.getByText("Manual review: verified fix locally")).toBeInTheDocument();
  });

  it("SessionsSection_should_ExpandInlineDiagnosticPanelForAllFiveKinds_When_UserClicksEachSyntheticSessionRow", () => {
    const item = makeItem([
      makeSession({ sessionId: "work-uuid-1", role: "work", entityId: "e-work" }),
      makeSession({
        sessionId: "real-review-uuid-1",
        role: "review",
        entityId: "e-review",
        endedAt: new Date().toISOString(),
      }),
      makeSession({
        sessionId: "headless-triage-uuid-1",
        role: "triage",
        entityId: "e-headless",
        triageResult: { summary: "Looks good.", suggestions: [], clarifyingQuestions: [] },
      }),
      makeSession({
        sessionId: "review-blocked-uuid-1",
        role: "review",
        entityId: "e-blocked",
        reviewVerdict: { overallOutcome: "FAIL", summary: "Blocked by security check." },
      }),
      makeSession({
        sessionId: "manual-review-uuid-1",
        role: "review",
        entityId: "e-manual",
        reviewVerdict: { overallOutcome: "PASS", summary: "Verified manually." },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
      />
    );

    // Real sessions (work, review) — unchanged link behavior.
    expect(screen.getByRole("link", { name: /work-uuid-1/ })).toHaveAttribute("href", "/?session=work-uuid-1");
    expect(screen.getByRole("link", { name: /real-review-uuid-1/ })).toHaveAttribute(
      "href",
      "/?session=real-review-uuid-1"
    );

    // Synthetic sessions — Collapsible header, click to expand inline.
    const headlessHeader = screen.getByTestId("collapsible-header-session-e-headless");
    const blockedHeader = screen.getByTestId("collapsible-header-session-e-blocked");
    const manualHeader = screen.getByTestId("collapsible-header-session-e-manual");
    for (const header of [headlessHeader, blockedHeader, manualHeader]) {
      expect(header).toHaveAttribute("aria-expanded", "false");
    }

    fireEvent.click(headlessHeader);
    expect(screen.getByText("Looks good.")).toBeInTheDocument();

    fireEvent.click(blockedHeader);
    expect(screen.getByText("Blocked by security check.")).toBeInTheDocument();

    fireEvent.click(manualHeader);
    expect(screen.getByText("Verified manually.")).toBeInTheDocument();
  });
});
