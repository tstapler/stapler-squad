import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ActionsSection } from "./ActionsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

// Covers the two mutually-exclusive Unarchive render paths (ActionsSection.tsx:104-115,
// :369-379) — live terminalState vs. item.status on load — which share one testid by design.
function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Some item",
    status: "idea",
    priority: 2,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

const noop = () => {};

function renderActions(overrides: Partial<React.ComponentProps<typeof ActionsSection>> = {}) {
  return render(
    <ActionsSection
      item={makeItem()}
      actionLoading={null}
      latestWorkSession={undefined}
      showManualReview={false}
      manualReviewOutcome="PASS"
      manualReviewSummary=""
      onAction={noop}
      onManualReviewOutcomeChange={noop}
      onManualReviewSummaryChange={noop}
      onManualReviewSubmit={noop}
      onManualReviewCancel={noop}
      terminalState={null}
      {...overrides}
    />
  );
}

describe("ActionsSection — Unarchive button", () => {
  it("renders the Unarchive button in the terminal-state notice when terminalState is 'archived'", () => {
    renderActions({ terminalState: "archived" });
    expect(screen.getByTestId("backlog-action-unarchive")).toBeInTheDocument();
  });

  it("does not render the Unarchive button when terminalState is 'removed'", () => {
    renderActions({ terminalState: "removed" });
    expect(screen.queryByTestId("backlog-action-unarchive")).not.toBeInTheDocument();
  });

  it("calls onAction('unarchive') when the terminal-state Unarchive button is clicked", () => {
    const onAction = jest.fn();
    renderActions({ terminalState: "archived", onAction });
    fireEvent.click(screen.getByTestId("backlog-action-unarchive"));
    expect(onAction).toHaveBeenCalledWith("unarchive");
  });

  it("renders the Unarchive button when the item's own status is 'archived' (no live terminal event)", () => {
    renderActions({ item: makeItem({ status: "archived" }), terminalState: null });
    expect(screen.getByTestId("backlog-action-unarchive")).toBeInTheDocument();
  });

  it("does not render the Unarchive button for a non-archived item with no terminal event", () => {
    renderActions({ item: makeItem({ status: "idea" }), terminalState: null });
    expect(screen.queryByTestId("backlog-action-unarchive")).not.toBeInTheDocument();
  });

  it("calls onAction('unarchive') when the status-derived Unarchive button is clicked", () => {
    const onAction = jest.fn();
    renderActions({ item: makeItem({ status: "archived" }), terminalState: null, onAction });
    fireEvent.click(screen.getByTestId("backlog-action-unarchive"));
    expect(onAction).toHaveBeenCalledWith("unarchive");
  });
});
