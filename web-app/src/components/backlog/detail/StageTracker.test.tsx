import React from "react";
import { render, screen } from "@testing-library/react";
import { StageTracker, deriveStageDisplay } from "./StageTracker";

// The jest styleMock for `.css.ts` files wraps every export in a callable
// proxy function, which triggers a benign "Invalid value for prop
// className" React warning — same pre-existing limitation silenced in
// BlockerChip.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

describe("deriveStageDisplay", () => {
  it("StageTracker_should_ActivateIdea_When_StatusIsIdea", () => {
    expect(deriveStageDisplay("idea")).toEqual({ activeStage: "idea", archived: false });
  });

  it("StageTracker_should_FoldRefiningIntoIdea_When_StatusIsRefining", () => {
    expect(deriveStageDisplay("refining")).toEqual({ activeStage: "idea", archived: false });
  });

  it("StageTracker_should_ActivateReady_When_StatusIsReady", () => {
    expect(deriveStageDisplay("ready")).toEqual({ activeStage: "ready", archived: false });
  });

  it("StageTracker_should_HighlightInProgressWithQueuedModifier_When_StatusIsQueued", () => {
    expect(deriveStageDisplay("queued")).toEqual({
      activeStage: "in_progress",
      modifier: "Queued",
      archived: false,
    });
  });

  it("StageTracker_should_ActivateInProgress_When_StatusIsInProgress", () => {
    expect(deriveStageDisplay("in_progress")).toEqual({ activeStage: "in_progress", archived: false });
  });

  it("StageTracker_should_ActivateReview_When_StatusIsReview", () => {
    expect(deriveStageDisplay("review")).toEqual({ activeStage: "review", archived: false });
  });

  it("StageTracker_should_HighlightReviewWithPrPendingModifier_When_StatusIsPrPending", () => {
    expect(deriveStageDisplay("pr_pending")).toEqual({
      activeStage: "review",
      modifier: "PR pending",
      archived: false,
    });
  });

  it("StageTracker_should_ActivateDone_When_StatusIsDone", () => {
    expect(deriveStageDisplay("done")).toEqual({ activeStage: "done", archived: false });
  });

  it("StageTracker_should_MarkArchived_When_StatusIsArchived", () => {
    const display = deriveStageDisplay("archived");
    expect(display.archived).toBe(true);
  });
});

describe("StageTracker", () => {
  it("StageTracker_should_HighlightInProgressWithQueuedModifier_When_StatusIsQueued", () => {
    render(<StageTracker status="queued" />);

    expect(screen.getByTestId("stage-node-in_progress")).toHaveAttribute("aria-current", "step");
    expect(screen.getByTestId("stage-modifier-badge")).toHaveTextContent("Queued");
    expect(screen.getAllByTestId(/^stage-node-/)).toHaveLength(5);
    expect(screen.queryByTestId("stage-archived-ribbon")).not.toBeInTheDocument();
  });

  it("StageTracker_should_HighlightReviewWithPrPendingModifier_When_StatusIsPrPending", () => {
    render(<StageTracker status="pr_pending" />);

    expect(screen.getByTestId("stage-node-review")).toHaveAttribute("aria-current", "step");
    expect(screen.getByTestId("stage-modifier-badge")).toHaveTextContent("PR pending");
  });

  it("StageTracker_should_ActivateIdeaNode_When_StatusIsRefining", () => {
    render(<StageTracker status="refining" />);

    expect(screen.getByTestId("stage-node-idea")).toHaveAttribute("aria-current", "step");
    expect(screen.queryByTestId("stage-modifier-badge")).not.toBeInTheDocument();
    expect(screen.getAllByTestId(/^stage-node-/)).toHaveLength(5);
  });

  it("StageTracker_should_RenderArchivedRibbonOverNeutralTracker_When_StatusIsArchived", () => {
    render(<StageTracker status="archived" />);

    expect(screen.getByTestId("stage-archived-ribbon")).toHaveTextContent("Archived");
    // No node is marked as the active stage — the pre-archive stage is
    // never guessed.
    for (const node of screen.getAllByTestId(/^stage-node-/)) {
      expect(node).not.toHaveAttribute("aria-current");
    }
    // Still exactly 5 nodes underneath the ribbon — never a 6th node.
    expect(screen.getAllByTestId(/^stage-node-/)).toHaveLength(5);
  });

  it("StageTracker_should_RenderExactlyFiveNodes_When_StatusIsAnyKnownValue", () => {
    render(<StageTracker status="done" />);
    expect(screen.getAllByTestId(/^stage-node-/)).toHaveLength(5);
  });
});
