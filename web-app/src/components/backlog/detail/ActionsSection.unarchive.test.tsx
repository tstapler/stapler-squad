import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ActionsSection } from "./ActionsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

/**
 * Coverage for the two independent Unarchive button render paths
 * (ActionsSection.tsx:104-115 and :369-379), added alongside the
 * UnarchiveBacklogItem RPC (PR #499 code review, MODERATE finding).
 *
 * The two paths are mutually exclusive:
 *  - terminalState === "archived": a live BacklogItemArchivedEvent arrived
 *    via the watch stream, independent of item.status.
 *  - actions.has("unarchive"): item.status === "archived" was already the
 *    case on initial load (see itemActions.ts's `case "archived":`).
 *
 * Both buttons share the same data-testid ("backlog-action-unarchive") by
 * design (tracked NIT, out of scope to dedupe here) — never rendered
 * simultaneously since one requires terminalState truthy, the other falsy.
 */
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
