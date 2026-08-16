import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { routes } from "@/lib/routes";
import { LifecycleSummary } from "./LifecycleSummary";

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

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_a1b2c3",
    title: "Fix flaky WIP-cap test",
    status: "review",
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
    createdAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
    ...overrides,
  };
}

function makeStuckItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "itm_a1b2c3",
    title: "Fix flaky WIP-cap test",
    status: "in_progress",
    reason: StuckReason.STALE_WORK,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 4 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date()),
    prNumber: 0,
    prUrl: "",
    context: "",
    ...overrides,
  } as StuckBacklogItem;
}

describe("LifecycleSummary", () => {
  it("LifecycleSummary_should_RenderBlockerChip_When_StuckItemPropIsProvided", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        stuckItem={makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.STALE_WORK })}
      />
    );

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.getByText("Stale work session")).toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderNoChip_When_StuckItemPropIsUndefined", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} stuckItem={undefined} />);

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderNoChip_When_StuckItemPropIsOmittedEntirely", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderDataTestIdWithStageTrackerActive_When_ComposedFromAllThreeChildren", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3", status: "review" })} />);

    const summary = screen.getByTestId("lifecycle-summary");
    expect(summary).toBeInTheDocument();
    expect(screen.getByTestId("stage-node-review")).toHaveAttribute("aria-current", "step");
    expect(screen.getByTestId("liveness-line")).toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderPipelineBadge_When_PipelineModeResolvedAndNotDefault", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "resolved", name: "Fast Track", drifted: false }}
      />
    );

    expect(screen.getByTestId("lifecycle-pipeline-badge")).toHaveTextContent("Pipeline: Fast Track");
  });

  it("LifecycleSummary_should_OmitPipelineBadge_When_PipelineModeIsDefault", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "resolved", name: "default", drifted: false }}
      />
    );

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });

  it("omits the badge when no pipelineDisplay prop is passed at all", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });

  it("omits the badge for an unrecognized pipeline mode", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "unrecognized", slug: "legacy-fast" }}
      />
    );

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_InvokeOnTriggerRemediationNow_When_RetryButtonClicked", async () => {
    const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        stuckItem={makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.STALE_WORK })}
        onTriggerRemediationNow={onTriggerRemediationNow}
      />
    );

    await user.click(screen.getByTestId("blocker-chip-retry"));

    expect(onTriggerRemediationNow).toHaveBeenCalledTimes(1);
    expect(onTriggerRemediationNow).toHaveBeenCalledWith("itm_a1b2c3", StuckReason.STALE_WORK);
  });

  it("LifecycleSummary_should_RenderReadOnlyChip_When_OnTriggerRemediationNowIsOmitted", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        stuckItem={makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.STALE_WORK })}
      />
    );

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.queryByTestId("blocker-chip-retry")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderUnfinishedLink_When_StuckItemIsPresent", () => {
    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        stuckItem={makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.STALE_WORK })}
      />
    );

    const link = screen.getByTestId("lifecycle-unfinished-link");
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", routes.unfinishedItem("itm_a1b2c3"));
  });

  it("LifecycleSummary_should_OmitUnfinishedLink_When_StuckItemIsUndefined", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} stuckItem={undefined} />);

    expect(screen.queryByTestId("lifecycle-unfinished-link")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderReworkCapBadge_When_OverrideIsSet", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3", reworkCapOverride: 5 })} />);

    expect(screen.getByTestId("lifecycle-rework-cap-badge")).toHaveTextContent("Rework cap: 5");
  });

  it("LifecycleSummary_should_RenderUnlimitedReworkCapBadge_When_OverrideIsExplicitlyZero", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3", reworkCapOverride: 0 })} />);

    expect(screen.getByTestId("lifecycle-rework-cap-badge")).toHaveTextContent("Rework cap: unlimited");
  });

  it("LifecycleSummary_should_OmitReworkCapBadge_When_OverrideIsUndefined", () => {
    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3", reworkCapOverride: undefined })} />);

    expect(screen.queryByTestId("lifecycle-rework-cap-badge")).not.toBeInTheDocument();
  });
});
