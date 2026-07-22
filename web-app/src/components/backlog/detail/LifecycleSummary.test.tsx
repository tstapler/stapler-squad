import React from "react";
import { render, screen } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { LifecycleSummary } from "./LifecycleSummary";

const useStuckBacklogItemsMock = jest.fn();
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: (...args: unknown[]) => useStuckBacklogItemsMock(...args),
}));

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
  it("LifecycleSummary_should_RenderBlockerChip_When_UseStuckBacklogItemsReturnsMatchingItemId", () => {
    useStuckBacklogItemsMock.mockReturnValue({
      items: [makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.STALE_WORK })],
      isLoading: false,
      error: null,
    });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.getByText("Stale work session")).toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderNoChip_When_UseStuckBacklogItemsReturnsNoMatchingItemId", () => {
    useStuckBacklogItemsMock.mockReturnValue({
      items: [makeStuckItem({ itemId: "itm_other_item" })],
      isLoading: false,
      error: null,
    });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderNoChip_When_UseStuckBacklogItemsIsStillLoading", () => {
    // While isLoading is true, items is still the hook's initial empty
    // array — the chip must render nothing, not a spinner or neutral
    // placeholder (design/ux.md Surface 1/2 loading-race note, AC 14).
    useStuckBacklogItemsMock.mockReturnValue({
      items: [],
      isLoading: true,
      error: null,
    });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });

  it("LifecycleSummary_should_RetainLastKnownBlockerChip_When_UseStuckBacklogItemsReturnsError", () => {
    // useStuckBacklogItems() retains its last-known `items` across a failed
    // refresh and populates `error` rather than blanking the list — the
    // chip must keep showing that last-known state, never a false
    // "all clear" (design/ux.md Surface 8).
    useStuckBacklogItemsMock.mockReturnValue({
      items: [makeStuckItem({ itemId: "itm_a1b2c3", reason: StuckReason.REWORK_CAP })],
      isLoading: false,
      error: new Error("Failed to load stuck backlog items"),
    });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.getByText("Rework cap hit")).toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderDataTestIdWithStageTrackerActive_When_ComposedFromAllThreeChildren", () => {
    useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3", status: "review" })} />);

    const summary = screen.getByTestId("lifecycle-summary");
    expect(summary).toBeInTheDocument();
    expect(screen.getByTestId("stage-node-review")).toHaveAttribute("aria-current", "step");
    expect(screen.getByTestId("liveness-line")).toBeInTheDocument();
  });

  it("LifecycleSummary_should_RenderPipelineBadge_When_PipelineModeResolvedAndNotDefault", () => {
    useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });

    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "resolved", name: "Fast Track", drifted: false }}
      />
    );

    expect(screen.getByTestId("lifecycle-pipeline-badge")).toHaveTextContent("Pipeline: Fast Track");
  });

  it("LifecycleSummary_should_OmitPipelineBadge_When_PipelineModeIsDefault", () => {
    useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });

    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "resolved", name: "default", drifted: false }}
      />
    );

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });

  it("omits the badge when no pipelineDisplay prop is passed at all", () => {
    useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });

    render(<LifecycleSummary item={makeItem({ id: "itm_a1b2c3" })} />);

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });

  it("omits the badge for an unrecognized pipeline mode", () => {
    useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });

    render(
      <LifecycleSummary
        item={makeItem({ id: "itm_a1b2c3" })}
        pipelineDisplay={{ kind: "unrecognized", slug: "legacy-fast" }}
      />
    );

    expect(screen.queryByTestId("lifecycle-pipeline-badge")).not.toBeInTheDocument();
  });
});
