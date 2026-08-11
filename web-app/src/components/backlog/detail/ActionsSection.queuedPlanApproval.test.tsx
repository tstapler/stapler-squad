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
 *
 * Also covers the 2026-08-03 follow-on fix (item be676dab,
 * docs/tasks/backlog-feature-improvement.md): Approve Plan used to be gated
 * on `item.planArtifactsPath` being truthy — incidental data that's usually,
 * but not always, populated when the item is gated on plan approval. When a
 * triage session ran and left nothing usable behind, planArtifactsPath is
 * empty even though the item is still very much gated, and the old condition
 * rendered nothing at all — a silent dead end with no retry affordance. The
 * fix (web-app/src/lib/backlog/itemActions.ts) derives visibility from the
 * actual gate condition (`!skipPlanning && !planApproved`) and shows "Retry
 * Triage" instead of silently omitting everything when no plan exists yet.
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

  it("renders Retry Triage instead of Approve Plan when the item has no plan artifacts and its latest triage session failed", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "", triageStatus: "failed" })}
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
    expect(screen.getByTestId("backlog-action-retry-triage")).toBeInTheDocument();
  });

  it("calls onAction('retry_triage') when the Retry Triage button is clicked", () => {
    const onAction = jest.fn();
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "", triageStatus: "failed" })}
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
    fireEvent.click(screen.getByTestId("backlog-action-retry-triage"));
    expect(onAction).toHaveBeenCalledWith("retry_triage");
  });

  // MAJOR finding from PR #322 review: gated + no plan alone is not evidence a
  // triage attempt actually failed — an item can reach queued (or ready) via
  // paths that never ran triage at all. Neither Approve Plan nor Retry Triage
  // should appear without triageStatus === "failed" as direct evidence.
  it("renders neither Approve Plan nor Retry Triage when the item has no plan artifacts and no evidence of a failed triage attempt", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "", triageStatus: undefined })}
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
    expect(screen.queryByTestId("backlog-action-retry-triage")).not.toBeInTheDocument();
  });

  it("renders neither Approve Plan nor Retry Triage once the plan is already approved, even with a failed triage attempt on record", () => {
    render(
      <ActionsSection
        item={makeItem({ planArtifactsPath: "", planApproved: true, triageStatus: "failed" })}
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
    expect(screen.queryByTestId("backlog-action-retry-triage")).not.toBeInTheDocument();
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
