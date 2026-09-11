import React from "react";
import { render, screen } from "@testing-library/react";
import { ActionsSection } from "./ActionsSection";
import type { JulesDispatchGate } from "./ActionsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

/**
 * Story 3.2.2 (google-jules-integration): the gated "Dispatch to Jules"
 * button — hidden/disabled states per ux.md §3.1's precedence order
 * (feature off -> no key -> Jules session already open -> no known branch
 * -> enabled). Gating itself is resolved in BacklogItemDetail.tsx
 * (resolveJulesDispatchGate, not exercised here); this suite only proves
 * ActionsSection renders each already-resolved `JulesDispatchGate` value
 * correctly, matching the props-down/callbacks-up split the component's
 * own doc comment describes.
 */
function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Some ready item",
    status: "ready",
    priority: 2,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: true,
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

function renderActionsSection(julesDispatchGate: JulesDispatchGate | undefined, item: BacklogItem = makeItem()) {
  render(
    <ActionsSection
      item={item}
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
      julesDispatchGate={julesDispatchGate}
      onDispatchToJulesClick={noop}
    />
  );
}

describe("ActionsSection — Dispatch to Jules gating", () => {
  it("renders no dispatch-to-jules element when GetJulesConfig reports enabled:false", () => {
    renderActionsSection({ hidden: true, disabled: true, reason: null, branch: "" });
    expect(screen.queryByTestId("dispatch-to-jules")).not.toBeInTheDocument();
  });

  it("disables the button with the add-a-key description when enabled but has_api_key is false", () => {
    renderActionsSection({
      hidden: false,
      disabled: true,
      reason: "Add a Jules API key in Settings to enable cloud sessions.",
      branch: "",
    });

    const button = screen.getByTestId("dispatch-to-jules");
    expect(button).toBeDisabled();
    const reason = screen.getByTestId("dispatch-to-jules-reason");
    expect(reason).toHaveTextContent("Add a Jules API key in Settings to enable cloud sessions.");
    expect(button).toHaveAttribute("aria-describedby", reason.id);
  });

  it("disables the button with the already-running description when an open jules_work session exists", () => {
    renderActionsSection({
      hidden: false,
      disabled: true,
      reason: "A Jules session is already running for this item.",
      branch: "backlog/fix-flaky-poller-test",
    });

    const button = screen.getByTestId("dispatch-to-jules");
    expect(button).toBeDisabled();
    expect(screen.getByTestId("dispatch-to-jules-reason")).toHaveTextContent(
      "A Jules session is already running for this item."
    );
  });

  it("disables the button with the no-branch description when enabled, keyed, no open session, but zero ItemSession rows carry a worktree_branch", () => {
    renderActionsSection({
      hidden: false,
      disabled: true,
      reason: "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules.",
      branch: "",
    });

    const button = screen.getByTestId("dispatch-to-jules");
    expect(button).toBeDisabled();
    expect(screen.getByTestId("dispatch-to-jules-reason")).toHaveTextContent(
      "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules."
    );
  });

  it("enables the button and omits the reason for a ready item with everything configured", () => {
    renderActionsSection({
      hidden: false,
      disabled: false,
      reason: null,
      branch: "backlog/fix-flaky-poller-test",
    });

    const button = screen.getByTestId("dispatch-to-jules");
    expect(button).toBeEnabled();
    expect(screen.queryByTestId("dispatch-to-jules-reason")).not.toBeInTheDocument();
    expect(button).not.toHaveAttribute("aria-describedby");
  });

  it("shows only the add-a-key description when both no-key and no-branch conditions apply (key check precedes branch check)", () => {
    renderActionsSection({
      hidden: false,
      disabled: true,
      reason: "Add a Jules API key in Settings to enable cloud sessions.",
      branch: "",
    });

    expect(screen.getByTestId("dispatch-to-jules-reason")).toHaveTextContent(
      "Add a Jules API key in Settings to enable cloud sessions."
    );
    expect(screen.queryAllByTestId("dispatch-to-jules-reason")).toHaveLength(1);
  });
});
