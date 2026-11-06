/**
 * AC3/AC4 (backlog-item-detail-ux): the board/list card and the detail panel
 * must agree on lifecycle status and blocker, and a PR link must have a single
 * authoritative location per status.
 */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { BacklogItemCard } from "../BacklogItemCard";
import { LifecycleSummary } from "./LifecycleSummary";
import { PullRequestSection } from "./PullRequestSection";
import { VersionControlSection } from "./VersionControlSection";

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({ useWatchBacklogItems: jest.fn() }));

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_x1",
    title: "Cross-surface item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    createdAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
    ...overrides,
  };
}

function makeStuckItem(reason: StuckReason): StuckBacklogItem {
  return {
    itemId: "itm_x1",
    title: "Cross-surface item",
    status: "in_progress",
    reason,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 4 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date()),
    prNumber: 0,
    prUrl: "",
    context: "",
  } as StuckBacklogItem;
}

describe("board/list card vs detail lifecycle model (AC3)", () => {
  it.each([
    ["idea", "Idea"],
    ["ready", "Ready"],
    ["in_progress", "In Progress"],
    ["review", "Review"],
    ["done", "Done"],
  ])("card and detail stage tracker both read %s as %s", (status, label) => {
    const item = makeItem({ status });
    const card = render(<BacklogItemCard item={item} onAction={jest.fn()} onClick={jest.fn()} />);
    expect(within(card.container).getByTestId("backlog-item-card-status")).toHaveTextContent(label);
    card.unmount();

    const detail = render(<LifecycleSummary item={item} />);
    expect(within(detail.container).getByTestId("stage-tracker")).toHaveAttribute(
      "aria-label",
      `Lifecycle stage: ${label}`
    );
  });

  it.each([StuckReason.STALE_WORK, StuckReason.PR_READY_UNMERGED])(
    "card and detail show the same blocker label for reason %s",
    (reason) => {
      const item = makeItem();
      const stuck = makeStuckItem(reason);
      const card = render(
        <BacklogItemCard item={item} onAction={jest.fn()} onClick={jest.fn()} stuckItem={stuck} />
      );
      const cardChip = within(card.container).getByTestId("blocker-chip");
      const cardLabel = cardChip.textContent;
      card.unmount();

      const detail = render(<LifecycleSummary item={item} stuckItem={stuck} />);
      const detailChip = within(detail.container).getByTestId("blocker-chip");
      expect(cardLabel).toBeTruthy();
      expect(detailChip.textContent).toContain(cardLabel as string);
    }
  );

  it("neither surface shows a blocker when the item is not stuck", () => {
    const item = makeItem();
    const card = render(<BacklogItemCard item={item} onAction={jest.fn()} onClick={jest.fn()} />);
    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
    card.unmount();
    render(<LifecycleSummary item={item} />);
    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });
});

describe("PR link surfaces per status (AC4)", () => {
  const prFields = { prUrl: "https://github.com/o/r/pull/7", prNumber: 7 };
  const prLinks = (c: HTMLElement) => c.querySelectorAll('a[href="https://github.com/o/r/pull/7"]');

  // BacklogItemDetail mounts PullRequestSection only while pr_pending; that
  // gating is asserted in BacklogItemDetail.test.tsx, so it is mirrored here.
  it("pr_pending: header badge and VCS link are suppressed, PullRequestSection links once", () => {
    const item = makeItem({ status: "pr_pending", ...prFields });
    const { container } = render(
      <>
        <LifecycleSummary item={item} />
        <PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />
        <VersionControlSection {...vcsProps} item={item} />
      </>
    );
    expect(prLinks(container)).toHaveLength(1);
    expect(screen.queryByTestId("lifecycle-pr-badge")).not.toBeInTheDocument();
    expect(screen.queryByTestId("vcs-section-pr-link")).not.toBeInTheDocument();
  });

  it("done: header badge (glance) plus one Version Control link, no PullRequestSection", () => {
    const item = makeItem({ status: "done", ...prFields });
    const { container } = render(
      <>
        <LifecycleSummary item={item} />
        <VersionControlSection {...vcsProps} item={item} />
      </>
    );
    expect(screen.getByTestId("lifecycle-pr-badge")).toBeInTheDocument();
    expect(screen.getByTestId("vcs-section-pr-link")).toBeInTheDocument();
    expect(prLinks(container)).toHaveLength(2);
  });
});

const vcsProps = {
  widgetData: null,
  activeSessionCount: 0,
  worktreePath: "",
  onViewDiff: jest.fn(),
  onBrowseFiles: jest.fn(),
  defaultExpanded: true,
};
