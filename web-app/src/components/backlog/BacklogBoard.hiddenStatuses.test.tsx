/**
 * BUG-037: the board's 5 columns filtered items by an exact match against
 * item.status ("idea" | "ready" | "in_progress" | "review" | "done"). An
 * item whose status was "queued", "pr_pending", or "refining" matched none
 * of the 5 columns and rendered nowhere on the board -- no card, no count,
 * no error -- even though the item-detail page's StageTracker already had a
 * canonical mapping for exactly this ("queued" -> In Progress + badge,
 * "pr_pending" -> Review + badge, "refining" -> Idea). The board now reuses
 * that same mapping (see stageOf() in BacklogBoard.tsx) so these items fold
 * into their mapped column instead of disappearing.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(),
}));
const mockUseWatchBacklogItems = useWatchBacklogItems as jest.Mock;

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Test item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    liveVersion: 1,
    ...overrides,
  } as BacklogItem;
}

function renderBoard(items: BacklogItem[]) {
  mockUseWatchBacklogItems.mockReturnValue({ items, connectionState: "live" });
  return render(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);
}

describe("BacklogBoard hidden statuses (BUG-037)", () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it("BacklogBoard_should_RenderInProgressColumn_When_StatusIsQueued", () => {
    renderBoard([makeItem({ status: "queued", title: "Queued item" })]);
    const column = screen.getByTestId("backlog-column-in_progress");
    expect(column).toHaveTextContent("Queued item");
  });

  it("BacklogBoard_should_RenderReviewColumn_When_StatusIsPrPending", () => {
    renderBoard([makeItem({ status: "pr_pending", title: "PR pending item" })]);
    const column = screen.getByTestId("backlog-column-review");
    expect(column).toHaveTextContent("PR pending item");
  });

  it("BacklogBoard_should_RenderIdeaColumn_When_StatusIsRefining", () => {
    renderBoard([makeItem({ status: "refining", title: "Refining item" })]);
    const column = screen.getByTestId("backlog-column-idea");
    expect(column).toHaveTextContent("Refining item");
  });

  it("BacklogBoard_should_RenderNoColumn_When_StatusIsArchived", () => {
    renderBoard([makeItem({ status: "archived", title: "Archived item" })]);
    expect(screen.queryByText("Archived item")).not.toBeInTheDocument();
  });
});
