import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ActivityLogSection } from "./ActivityLogSection";
import type { ActivityNote, BacklogItem } from "@/lib/hooks/useBacklogService";

beforeEach(() => {
  localStorage.clear();
});

function makeNote(overrides: Partial<ActivityNote> = {}): ActivityNote {
  return {
    id: `note-${Math.random()}`,
    message: "checked in on this",
    authorSessionUuid: "",
    authorSessionTitle: "",
    ...overrides,
  };
}

function makeItem(activityNotes: ActivityNote[]): BacklogItem {
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
    statusEvents: [],
    progressNotes: [],
    activityNotes,
    totalEstimatedCostUsd: 0,
  };
}

describe("ActivityLogSection", () => {
  it("renders nothing when there are no activity notes", () => {
    const { container } = render(<ActivityLogSection item={makeItem([])} defaultExpanded={true} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders all notes visible when at or below the 8-item cap", () => {
    const notes = Array.from({ length: 5 }, (_, i) => makeNote({ id: `note-${i}`, message: `note-${i}` }));
    render(<ActivityLogSection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    expect(screen.queryByTestId("activity-log-show-more")).not.toBeInTheDocument();
  });

  it("caps rendering at 8 and reveals the rest via Show N more", () => {
    // Notes in ascending createdAt order (index 0 oldest, index 11 most
    // recent), matching ListActivityNotesForItem's Asc(created_at) ordering.
    const notes = Array.from({ length: 12 }, (_, i) => makeNote({ id: `note-${i}`, message: `activity-note-${i}` }));
    render(<ActivityLogSection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(8);
    const showMore = screen.getByTestId("activity-log-show-more");
    expect(showMore).toHaveTextContent("Show 4 more");

    // The visible 8 are the tail (notes 4-11), not the head.
    expect(screen.getByText("activity-note-11")).toBeInTheDocument();
    expect(screen.getByText("activity-note-4")).toBeInTheDocument();
    expect(screen.queryByText("activity-note-0")).not.toBeInTheDocument();
    expect(screen.queryByText("activity-note-3")).not.toBeInTheDocument();

    fireEvent.click(showMore);

    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    expect(screen.queryByTestId("activity-log-show-more")).not.toBeInTheDocument();
    expect(screen.getByText("activity-note-0")).toBeInTheDocument();
    expect(screen.getByText("activity-note-3")).toBeInTheDocument();
  });

  it("falls back through the author title -> UUID -> manual chain", () => {
    const notes = [
      makeNote({ id: "note-a", message: "has a title", authorSessionTitle: "worker-session" }),
      makeNote({ id: "note-b", message: "uuid only", authorSessionUuid: "12345678-abcd-ef01-2345-6789abcdef01" }),
      makeNote({ id: "note-c", message: "fully manual" }),
    ];
    render(<ActivityLogSection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getByText("worker-session")).toBeInTheDocument();
    expect(screen.getByText("12345678")).toBeInTheDocument();
    expect(screen.getByText("manual")).toBeInTheDocument();
  });
});
