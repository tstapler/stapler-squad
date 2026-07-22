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

    fireEvent.click(showMore);

    // All 11 shown inline, in the same list, no route change.
    expect(screen.getAllByRole("listitem")).toHaveLength(11);
    expect(screen.queryByTestId("sessions-show-more")).not.toBeInTheDocument();
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

  it("renders real work/review sessions as links and synthetic sessions as non-clickable spans", () => {
    const item = makeItem([
      makeSession({ sessionId: "a1b2c3d4", role: "work" }),
      makeSession({ sessionId: "headless-triage-xyz", role: "triage" }),
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

    expect(screen.getByRole("link", { name: /a1b2c3d4/ })).toHaveAttribute("href", "/?session=a1b2c3d4");
    expect(screen.queryByRole("link", { name: /headless-triage-xyz/ })).not.toBeInTheDocument();
  });
});
