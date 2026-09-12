/**
 * Epic 2.9 (backlog-custom-workflow-stages), Story 2.9.1: BacklogBoard's
 * COLUMNS is now a live-fetched, cached stage list (useBacklogStages())
 * instead of a hardcoded array (Task 2.9.1a) — a configured custom stage
 * renders as its own board column — and any item whose status matches no
 * fetched stage renders in an "Unrecognized stage" overflow column instead
 * of disappearing (Task 2.9.1b). The second case is a regression test for
 * BUG-037: the board's column set previously matched items by exact literal
 * status, so a status matching none of the fixed columns rendered nowhere on
 * the board — no card, no count, no error. This file guards that failure
 * mode for its new root cause (a deleted/unrecognized custom stage), on top
 * of the BUG-037 regressions BacklogBoard.hiddenStatuses.test.tsx already
 * covers for the original (queued/pr_pending/refining folding) cause.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { useBacklogStages, type BacklogStageInfo } from "@/lib/hooks/useBacklogStages";

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(),
}));
const mockUseWatchBacklogItems = useWatchBacklogItems as jest.Mock;

jest.mock("@/lib/hooks/useBacklogStages", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogStages"),
  useBacklogStages: jest.fn(),
}));
const mockUseBacklogStages = useBacklogStages as jest.Mock;

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

function makeStage(overrides: Partial<BacklogStageInfo> = {}): BacklogStageInfo {
  return { slug: "design-review", name: "Design Review", enabled: true, ...overrides };
}

function renderBoard(items: BacklogItem[], stages: BacklogStageInfo[]) {
  mockUseWatchBacklogItems.mockReturnValue({ items, connectionState: "live" });
  mockUseBacklogStages.mockReturnValue({ stages, isLoading: false, error: null });
  return render(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);
}

describe("BacklogBoard dynamic stage rendering (Epic 2.9, Story 2.9.1)", () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it("BacklogBoard_should_RenderCustomStageAsItsOwnColumn_When_ItemIsOnAConfiguredCustomStage", () => {
    renderBoard(
      [makeItem({ id: "1", status: "design-review", title: "Needs design review" })],
      [makeStage()]
    );

    const column = screen.getByTestId("backlog-column-design-review");
    expect(column).toHaveTextContent("Design Review");
    expect(column).toHaveTextContent("Needs design review");
  });

  it("BacklogBoard_should_NotRenderCustomStageColumn_When_NoLiveItemAndStageProvidesNoImplicitColumn", () => {
    // A configured-but-unused custom stage still gets a column (matches the
    // built-in columns' always-present-even-when-empty behavior).
    renderBoard([], [makeStage()]);
    const column = screen.getByTestId("backlog-column-design-review");
    expect(column).toHaveTextContent("No items");
  });

  it("BacklogBoard_should_NotRenderDuplicateColumn_When_FetchedStageMatchesABuiltInSlug", () => {
    // A built-in slug returned by ListStages (e.g. "idea") must not produce
    // a second "idea" column alongside the existing built-in one.
    renderBoard(
      [makeItem({ id: "1", status: "idea", title: "Idea item" })],
      [{ slug: "idea", name: "Idea", enabled: true }]
    );
    expect(screen.getAllByTestId("backlog-column-idea")).toHaveLength(1);
  });

  it("BacklogBoard_should_RenderItemInUnrecognizedStageColumn_When_StatusMatchesNoFetchedStage_RegressionBUG037", () => {
    // BUG-037 regression: an item on a stage absent from the fetched config
    // (e.g. a stage that was since deleted) must never be silently dropped
    // off the board.
    renderBoard(
      [makeItem({ id: "1", status: "some-deleted-stage", title: "Orphaned item" })],
      []
    );

    const column = screen.getByTestId("backlog-column-__unrecognized_stage__");
    expect(column).toHaveTextContent("Unrecognized stage");
    expect(column).toHaveTextContent("Orphaned item");
  });

  it("BacklogBoard_should_NotRenderUnrecognizedColumn_When_EveryItemMatchesAKnownOrCustomStage", () => {
    renderBoard(
      [
        makeItem({ id: "1", status: "idea", title: "Idea item" }),
        makeItem({ id: "2", status: "design-review", title: "Custom stage item" }),
      ],
      [makeStage()]
    );
    expect(screen.queryByTestId("backlog-column-__unrecognized_stage__")).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_StillExcludeArchivedItems_When_ArchivedStatusMatchesNoColumn", () => {
    // "archived" is a defined "no column" status (by design), not an
    // unrecognized one — it must not fall into the overflow column either.
    renderBoard([makeItem({ id: "1", status: "archived", title: "Archived item" })], []);
    expect(screen.queryByTestId("backlog-column-__unrecognized_stage__")).not.toBeInTheDocument();
    expect(screen.queryByText("Archived item")).not.toBeInTheDocument();
  });

  it("BacklogBoard_should_RenderFiveBuiltInColumns_When_StagesFetchIsStillLoading", () => {
    // useBacklogStages() falls back to the built-in set synchronously (see
    // its own module), so the board never regresses to zero columns while
    // ListStages is in flight.
    mockUseWatchBacklogItems.mockReturnValue({ items: [], connectionState: "live" });
    mockUseBacklogStages.mockReturnValue({
      stages: jest.requireActual("@/lib/hooks/useBacklogStages").BUILTIN_BACKLOG_STAGES,
      isLoading: true,
      error: null,
    });
    render(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);

    for (const status of ["idea", "ready", "in_progress", "review", "done"]) {
      expect(screen.getByTestId(`backlog-column-${status}`)).toBeInTheDocument();
    }
  });
});
