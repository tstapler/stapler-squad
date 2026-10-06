import React from "react";
import { render, screen } from "@testing-library/react";
import { LivenessLine, deriveLastActivity, type LivenessSourceItem } from "./LivenessLine";

function iso(msAgo: number): string {
  return new Date(Date.now() - msAgo).toISOString();
}

function makeItem(overrides: Partial<LivenessSourceItem> = {}): LivenessSourceItem {
  return {
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    createdAt: iso(7 * 24 * 60 * 60 * 1000),
    ...overrides,
  };
}

describe("deriveLastActivity", () => {
  it("LivenessLine_should_ShowMostRecentStatusEventTimestamp_When_ItIsTheLatestOfAllThreeSources", () => {
    const item = makeItem({
      linkedSessions: [
        {
          entityId: "e1",
          sessionId: "s1",
          role: "work",
          startedAt: iso(2 * 60 * 60 * 1000),
          endedAt: iso(60 * 60 * 1000), // 1h ago — older than the status event below
          estimatedCostUsd: 0,
        },
      ],
      statusEvents: [
        { id: "ev1", fromStatus: "in_progress", toStatus: "review", triggeredBy: "system", createdAt: iso(12 * 60 * 1000) },
      ],
      progressNotes: [
        { id: "pn1", criterionIndex: 0, note: "note", status: "done", createdAt: iso(3 * 60 * 60 * 1000) },
      ],
    });

    const last = deriveLastActivity(item);
    expect(last).toBeInstanceOf(Date);
    // Within a few seconds of "12 minutes ago".
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(13 * 60 * 1000);
    expect(Date.now() - (last as Date).getTime()).toBeGreaterThan(11 * 60 * 1000);
  });

  it("LivenessLine_should_FallBackToItemCreatedAt_When_NoSessionsStatusEventsOrProgressNotesExist", () => {
    const createdAt = iso(9 * 24 * 60 * 60 * 1000);
    const item = makeItem({ linkedSessions: [], statusEvents: [], progressNotes: [], createdAt });

    const last = deriveLastActivity(item);
    expect(last?.toISOString()).toBe(createdAt);
  });

  it("returns the session's activity when it is the most recent source", () => {
    const item = makeItem({
      linkedSessions: [
        {
          entityId: "e1",
          sessionId: "s1",
          role: "work",
          startedAt: iso(10 * 60 * 1000),
          endedAt: iso(5 * 60 * 1000),
          estimatedCostUsd: 0,
        },
      ],
      statusEvents: [
        { id: "ev1", fromStatus: "idea", toStatus: "ready", triggeredBy: "system", createdAt: iso(60 * 60 * 1000) },
      ],
      progressNotes: [],
    });

    const last = deriveLastActivity(item);
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(6 * 60 * 1000);
  });

  it("returns the progress note's activity when it is the most recent source", () => {
    const item = makeItem({
      linkedSessions: [],
      statusEvents: [],
      progressNotes: [
        { id: "pn1", criterionIndex: 0, note: "note", status: "in_progress", createdAt: iso(90 * 1000) },
      ],
    });

    const last = deriveLastActivity(item);
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(2 * 60 * 1000);
  });

  it("deriveLastActivity_should_PreferLastCommitAt_When_MoreRecentThanEndedAt", () => {
    const item = makeItem({
      linkedSessions: [
        {
          entityId: "e1",
          sessionId: "s1",
          role: "work",
          startedAt: iso(60 * 60 * 1000),
          endedAt: iso(30 * 60 * 1000),
          lastCommitAt: iso(5 * 60 * 1000),
          estimatedCostUsd: 0,
        },
      ],
      statusEvents: [],
      progressNotes: [],
    });

    const last = deriveLastActivity(item);
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(6 * 60 * 1000);
  });

  it("deriveLastActivity_should_PreferLastFileTouchAt_When_LastCommitAtAbsent", () => {
    const item = makeItem({
      linkedSessions: [
        {
          entityId: "e1",
          sessionId: "s1",
          role: "work",
          startedAt: iso(60 * 60 * 1000),
          // no endedAt — session still running
          lastFileTouchAt: iso(2 * 60 * 1000),
          estimatedCostUsd: 0,
        },
      ],
      statusEvents: [],
      progressNotes: [],
    });

    const last = deriveLastActivity(item);
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(3 * 60 * 1000);
  });

  it("deriveLastActivity_should_PreferEndedAt_When_MoreRecentThanLastCommitAt", () => {
    const item = makeItem({
      linkedSessions: [
        {
          entityId: "e1",
          sessionId: "s1",
          role: "work",
          startedAt: iso(3 * 60 * 60 * 1000),
          // lastCommitAt is stale (3 hours old) — the session kept working
          // and ended much more recently. The most-recent timestamp should
          // win, not whichever field is checked first.
          lastCommitAt: iso(3 * 60 * 60 * 1000),
          endedAt: iso(10 * 60 * 1000),
          estimatedCostUsd: 0,
        },
      ],
      statusEvents: [],
      progressNotes: [],
    });

    const last = deriveLastActivity(item);
    expect(Date.now() - (last as Date).getTime()).toBeLessThan(11 * 60 * 1000);
  });
});

describe("LivenessLine", () => {
  it("renders 'Last activity Nm ago' from the most recent source", () => {
    const item = makeItem({
      linkedSessions: [],
      statusEvents: [
        { id: "ev1", fromStatus: "in_progress", toStatus: "review", triggeredBy: "system", createdAt: iso(12 * 60 * 1000) },
      ],
      progressNotes: [],
    });

    render(<LivenessLine item={item} />);
    expect(screen.getByTestId("liveness-line")).toHaveTextContent("Last activity 12m ago");
  });

  it("LivenessLine_should_FallBackToItemCreatedAt_When_NoSessionsStatusEventsOrProgressNotesExist", () => {
    const item = makeItem({
      linkedSessions: [],
      statusEvents: [],
      progressNotes: [],
      createdAt: iso(3 * 24 * 60 * 60 * 1000),
    });

    render(<LivenessLine item={item} />);
    expect(screen.getByTestId("liveness-line")).toHaveTextContent("Last activity 3d ago");
    expect(screen.queryByText("unknown")).not.toBeInTheDocument();
  });

  it("does not wrap itself in an aria-live/role=status region", () => {
    const item = makeItem();
    render(<LivenessLine item={item} />);
    const el = screen.getByTestId("liveness-line");
    expect(el).not.toHaveAttribute("aria-live");
    expect(el).not.toHaveAttribute("role");
  });
});
