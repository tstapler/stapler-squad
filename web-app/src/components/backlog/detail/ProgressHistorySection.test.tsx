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
    totalEstimatedCostUsd: 0,
  };
}

describe("ProgressHistorySection", () => {
  it("ProgressHistorySection_should_ShowFourMoreButton_When_TwelveProgressNotesExist", () => {
    const notes = Array.from({ length: 12 }, (_, i) => makeNote({ id: `note-${i}` }));
    render(<ProgressHistorySection item={makeItem(notes)} defaultExpanded={true} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(8);
    const showMore = screen.getByTestId("progress-history-show-more");
    expect(showMore).toHaveTextContent("Show 4 more");

    fireEvent.click(showMore);

    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    expect(screen.queryByTestId("progress-history-show-more")).not.toBeInTheDocument();
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
