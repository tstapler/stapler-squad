import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { WorkflowHistorySection } from "./WorkflowHistorySection";
import type { BacklogItem, StatusEvent } from "@/lib/hooks/useBacklogService";

beforeEach(() => {
  localStorage.clear();
});

function makeEvent(overrides: Partial<StatusEvent> = {}): StatusEvent {
  return {
    id: `ev-${Math.random()}`,
    fromStatus: "in_progress",
    toStatus: "review",
    triggeredBy: "system",
    ...overrides,
  };
}

function makeItem(statusEvents: StatusEvent[]): BacklogItem {
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
    linkedSessions: [],
    notes: "",
    statusEvents,
    progressNotes: [],
    totalEstimatedCostUsd: 0,
  };
}

describe("WorkflowHistorySection", () => {
  it("WorkflowHistorySection_should_ShowOneMoreButton_When_NineStatusEventsExist", () => {
    // Events created in ascending createdAt order (index 0 oldest, index 8 most
    // recent), matching the real repository's ent.Asc(FieldCreatedAt) ordering.
    const events = Array.from({ length: 9 }, (_, i) => makeEvent({ id: `ev-${i}`, note: `event-note-${i}` }));
    render(<WorkflowHistorySection item={makeItem(events)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(8);
    const showMore = screen.getByTestId("workflow-show-more");
    expect(showMore).toHaveTextContent("Show 1 more");

    // Identity check (regression for the "shows oldest, not most recent" bug):
    // the visible 8 must be the tail (events 1-8), not the head (events 0-7).
    expect(screen.getByText("event-note-8")).toBeInTheDocument();
    expect(screen.getByText("event-note-1")).toBeInTheDocument();
    expect(screen.queryByText("event-note-0")).not.toBeInTheDocument();

    fireEvent.click(showMore);

    expect(screen.getAllByRole("listitem")).toHaveLength(9);
    expect(screen.queryByTestId("workflow-show-more")).not.toBeInTheDocument();

    // Once expanded, the previously-hidden oldest event must now be present.
    expect(screen.getByText("event-note-0")).toBeInTheDocument();
  });

  it("renders no Show More button at or below the cap", () => {
    const events = Array.from({ length: 3 }, (_, i) => makeEvent({ id: `ev-${i}` }));
    render(<WorkflowHistorySection item={makeItem(events)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.queryByTestId("workflow-show-more")).not.toBeInTheDocument();
  });

  it("renders nothing when there are no status events", () => {
    const { container } = render(<WorkflowHistorySection item={makeItem([])} defaultExpanded={true} />);
    expect(container).toBeEmptyDOMElement();
  });
});
