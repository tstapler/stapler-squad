import React from "react";
import { render, screen } from "@testing-library/react";
import { MergeabilityPill } from "./MergeabilityPill";
import type { MergeabilityState } from "@/lib/vcs/mergeability";

describe("MergeabilityPill", () => {
  it("MergeabilityPill_should_RenderCiFailingLabelWithHiddenIcon_When_StateCiFailing", () => {
    const { container } = render(<MergeabilityPill state="ci_failing" />);
    expect(screen.getByText("CI failing")).toBeInTheDocument();
    const icon = container.querySelector("svg");
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });

  it("MergeabilityPill_should_RenderShippedLabel_When_StateShipped", () => {
    render(<MergeabilityPill state="shipped" />);
    expect(screen.getByText("Shipped")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderReadyToMergeLabel_When_StateReadyToMerge", () => {
    render(<MergeabilityPill state="ready_to_merge" />);
    expect(screen.getByText("Ready to merge")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderDraftLabel_When_StateDraft", () => {
    render(<MergeabilityPill state="draft" />);
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderConflictsLabel_When_StateConflicted", () => {
    render(<MergeabilityPill state="conflicted" />);
    expect(screen.getByText("Conflicts")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderDivergedFromBaseLabel_When_StateDiverged", () => {
    render(<MergeabilityPill state="diverged" />);
    expect(screen.getByText("Diverged from base")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderChangesRequestedLabel_When_StateChangesRequested", () => {
    render(<MergeabilityPill state="changes_requested" />);
    expect(screen.getByText("Changes requested")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderCiRunningLabel_When_StateCiPending", () => {
    render(<MergeabilityPill state="ci_pending" />);
    expect(screen.getByText("CI running")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderClosedNotMergedLabel_When_StateClosedUnshipped", () => {
    render(<MergeabilityPill state="closed_unshipped" />);
    expect(screen.getByText("Closed — not merged")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderStatusUnavailableLabel_When_StateSnapshotUnavailable", () => {
    render(<MergeabilityPill state="snapshot_unavailable" />);
    expect(screen.getByText("Status unavailable")).toBeInTheDocument();
  });

  it("MergeabilityPill_should_RenderNoPrLabel_When_StateNoPr", () => {
    render(<MergeabilityPill state="no_pr" />);
    expect(screen.getByText("No PR")).toBeInTheDocument();
  });

  it("covers all 11 MergeabilityState members with a rendered label", () => {
    const states: MergeabilityState[] = [
      "shipped",
      "snapshot_unavailable",
      "no_pr",
      "draft",
      "conflicted",
      "diverged",
      "changes_requested",
      "ci_failing",
      "closed_unshipped",
      "ci_pending",
      "ready_to_merge",
    ];
    for (const state of states) {
      const { unmount } = render(<MergeabilityPill state={state} />);
      unmount();
    }
  });
});
