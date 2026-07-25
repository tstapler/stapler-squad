import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ActionsSection } from "./ActionsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

/**
 * Regression coverage for the "no reachable Approve Plan action for queued
 * items" bug (BUG-038 follow-up): reconcilePlanNotApprovedItems
 * (session/backlog_lifecycle.go) flags items stuck in `status: "queued"`,
 * but the only Approve Plan button lived inside the `status === "ready"`
 * block — unreachable for a queued item.
 */
function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Some queued item",
    status: "queued",
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

describe("ActionsSection — queued status Approve Plan action", () => {
  it("renders an Approve Plan button when the item is queued, unapproved, and has plan artifacts", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "/tmp/plan.md" })}
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
      />
    );
    expect(screen.getByTestId("backlog-action-approve-plan")).toBeInTheDocument();
  });

  it("calls onAction('approve_plan') when clicked", () => {
    const onAction = jest.fn();
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "/tmp/plan.md" })}
        actionLoading={null}
        latestWorkSession={undefined}
        showManualReview={false}
        manualReviewOutcome="PASS"
        manualReviewSummary=""
        onAction={onAction}
        onManualReviewOutcomeChange={noop}
        onManualReviewSummaryChange={noop}
        onManualReviewSubmit={noop}
        onManualReviewCancel={noop}
        terminalState={null}
      />
    );
    fireEvent.click(screen.getByTestId("backlog-action-approve-plan"));
    expect(onAction).toHaveBeenCalledWith("approve_plan");
  });

  it("does not render when the item has no plan artifacts yet", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "" })}
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
      />
    );
    expect(screen.queryByTestId("backlog-action-approve-plan")).not.toBeInTheDocument();
  });

  it("does not render once the plan is already approved", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "/tmp/plan.md", planApproved: true })}
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
      />
    );
    expect(screen.queryByTestId("backlog-action-approve-plan")).not.toBeInTheDocument();
  });
});
