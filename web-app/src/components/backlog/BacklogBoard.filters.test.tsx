/**
 * Board view didn't share filter/sort state with list view (see
 * .backlog-context.md AC 0/1/5/7). BacklogBoard now accepts an optional
 * `filters` prop and runs it through the same filterBacklogItems() used by
 * the list view, so a status/priority filter set on /backlog also narrows
 * what's shown on /backlog/board (AC 0, AC 1). A column emptied entirely by
 * the active filter (but which has items before filtering) shows a distinct
 * "No items match filter" message instead of the generic "No items" one
 * (AC 5). showArchived off drops archived items from the board the same way
 * it does on the list (AC 7) — see stageOf()'s comment in BacklogBoard.tsx
 * for why archived items never render in any column regardless of the
 * toggle.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogFilterState } from "@/lib/hooks/useBacklogFilters";
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
    status: "idea",
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

const NO_OP_FILTERS: BacklogFilterState = {
  search: "",
  statusFilter: [],
  priorityFilter: [],
  showArchived: false,
};

function renderBoard(items: BacklogItem[], filters: BacklogFilterState) {
  mockUseWatchBacklogItems.mockReturnValue({ items, connectionState: "live" });
  return render(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} filters={filters} />);
}

describe("BacklogBoard shared filter state (board view filter/sort parity)", () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it("BacklogBoard_should_HideNonMatchingStatusItems_When_StatusFilterIsSet", () => {
    renderBoard(
      [makeItem({ id: "1", status: "idea", title: "Idea item" }), makeItem({ id: "2", status: "ready", title: "Ready item" })],
      { ...NO_OP_FILTERS, statusFilter: ["ready"] }
    );
    expect(screen.queryByText("Idea item")).not.toBeInTheDocument();
    expect(screen.getByText("Ready item")).toBeInTheDocument();
  });

  it("BacklogBoard_should_HideNonMatchingPriorityItems_When_PriorityFilterIsSet", () => {
    renderBoard(
      [makeItem({ id: "1", priority: 1, title: "P1 item" }), makeItem({ id: "2", priority: 4, title: "P4 item" })],
      { ...NO_OP_FILTERS, priorityFilter: [1] }
    );
    expect(screen.getByText("P1 item")).toBeInTheDocument();
    expect(screen.queryByText("P4 item")).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_ShowNoItemsMatchFilterMessage_When_FilterEmptiesAColumnWithItems", () => {
    renderBoard([makeItem({ id: "1", status: "idea", priority: 4, title: "Filtered-out item" })], {
      ...NO_OP_FILTERS,
      priorityFilter: [1],
    });
    const column = screen.getByTestId("backlog-column-idea");
    expect(column).toHaveTextContent("No items match filter");
    expect(column.querySelector('[data-testid="backlog-column-empty-filtered"]')).toBeInTheDocument();
  });

  it("BacklogBoard_should_ShowGenericEmptyMessage_When_ColumnHasNoItemsRegardlessOfFilter", () => {
    renderBoard([makeItem({ id: "1", status: "ready", title: "Only in ready" })], NO_OP_FILTERS);
    const column = screen.getByTestId("backlog-column-idea");
    expect(column).toHaveTextContent("No items");
    expect(column.querySelector('[data-testid="backlog-column-empty-filtered"]')).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_HideArchivedItems_When_ShowArchivedIsFalse", () => {
    renderBoard([makeItem({ id: "1", status: "archived", title: "Archived item" })], {
      ...NO_OP_FILTERS,
      showArchived: false,
    });
    expect(screen.queryByText("Archived item")).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_StillHideArchivedItems_When_ShowArchivedIsTrue", () => {
    // Archived items resolve to no Stage (stageOf() returns null), so there is
    // no archived column to reveal them in even when showArchived is on —
    // this is the "defined, non-broken" behavior AC 7 requires, not a bug.
    renderBoard([makeItem({ id: "1", status: "archived", title: "Archived item" })], {
      ...NO_OP_FILTERS,
      showArchived: true,
    });
    expect(screen.queryByText("Archived item")).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_FoldRawStatusIntoMappedColumnWithCorrectBadge_When_StatusHasNoLiteralColumn", () => {
    // "queued" and "pr_pending" have no column of their own (BUG-037) —
    // stageOf() folds them into In Progress / Review respectively (same
    // mapping StageTracker uses on item detail), and the card still renders
    // its own raw-status badge via getStatusLabel(), independent of which
    // column it landed in.
    renderBoard(
      [
        makeItem({ id: "1", status: "queued", title: "Queued item" }),
        makeItem({ id: "2", status: "pr_pending", title: "PR pending item" }),
      ],
      NO_OP_FILTERS
    );

    const inProgressColumn = screen.getByTestId("backlog-column-in_progress");
    expect(inProgressColumn).toContainElement(screen.getByText("Queued item"));
    const reviewColumn = screen.getByTestId("backlog-column-review");
    expect(reviewColumn).toContainElement(screen.getByText("PR pending item"));

    const queuedCard = screen.getByText("Queued item").closest('[role="listitem"]');
    expect(queuedCard?.querySelector('[data-testid="backlog-item-card-status"]')).toHaveTextContent("Queued");
    const prPendingCard = screen.getByText("PR pending item").closest('[role="listitem"]');
    expect(prPendingCard?.querySelector('[data-testid="backlog-item-card-status"]')).toHaveTextContent("pr pending");
  });
});
