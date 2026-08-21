import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ProgressHistorySection } from "./ProgressHistorySection";
import type { BacklogItem, ProgressNote } from "@/lib/hooks/useBacklogService";

beforeEach(() => {
  localStorage.clear();
});

function makeNote(overrides: Partial<ProgressNote> = {}): ProgressNote {
  return {
    id: `note-${Math.random()}`,
    criterionIndex: 0,
    note: "did the thing",
    status: "in_progress",
    ...overrides,
  };
}

function makeItem(progressNotes: ProgressNote[]): BacklogItem {
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
    progressNotes,
    activityNotes: [],
    totalEstimatedCostUsd: 0,
  };
}

describe("ProgressHistorySection", () => {
  it("ProgressHistorySection_should_ShowFourMoreButton_When_TwelveProgressNotesExist", () => {
    // Notes created in ascending createdAt order (index 0 oldest, index 11 most
    // recent), matching the real repository's ent.Asc(FieldCreatedAt) ordering.
    const notes = Array.from({ length: 12 }, (_, i) => makeNote({ id: `note-${i}`, note: `progress-note-${i}` }));
    render(<ProgressHistorySection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(8);
    const showMore = screen.getByTestId("progress-history-show-more");
    expect(showMore).toHaveTextContent("Show 4 more");

    // Identity check (regression for the "shows oldest, not most recent" bug):
    // the visible 8 must be the tail (notes 4-11), not the head (notes 0-7).
    expect(screen.getByText("progress-note-11")).toBeInTheDocument();
    expect(screen.getByText("progress-note-4")).toBeInTheDocument();
    expect(screen.queryByText("progress-note-0")).not.toBeInTheDocument();
    expect(screen.queryByText("progress-note-3")).not.toBeInTheDocument();

    fireEvent.click(showMore);

    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    expect(screen.queryByTestId("progress-history-show-more")).not.toBeInTheDocument();

    // Once expanded, the previously-hidden oldest notes must now be present.
    expect(screen.getByText("progress-note-0")).toBeInTheDocument();
    expect(screen.getByText("progress-note-3")).toBeInTheDocument();
  });

  it("renders no Show More button at or below the cap", () => {
    const notes = Array.from({ length: 5 }, (_, i) => makeNote({ id: `note-${i}` }));
    render(<ProgressHistorySection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    expect(screen.queryByTestId("progress-history-show-more")).not.toBeInTheDocument();
  });

  it("renders nothing when there are no progress notes", () => {
    const { container } = render(<ProgressHistorySection item={makeItem([])} defaultExpanded={true} />);
    expect(container).toBeEmptyDOMElement();
  });
});
