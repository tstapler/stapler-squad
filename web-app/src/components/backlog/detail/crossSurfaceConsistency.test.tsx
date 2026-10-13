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
import { getStatusLabel } from "@/lib/backlog/status";
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
  it.each(["idea", "ready", "in_progress", "review", "done"])(
    "card status label for %s matches the label the detail model uses",
    (status) => {
      const item = makeItem({ status });
      render(<BacklogItemCard item={item} onAction={jest.fn()} onClick={jest.fn()} />);
      expect(screen.getByTestId("backlog-item-card-status")).toHaveTextContent(getStatusLabel(status));
    }
  );

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

describe("PR link has one authoritative location per status (AC4)", () => {
  const prFields = { prUrl: "https://github.com/o/r/pull/7", prNumber: 7 };

  function renderPrSurfaces(item: BacklogItem) {
    return render(
      <>
        <LifecycleSummary item={item} />
        {item.status === "pr_pending" && (
          <PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />
        )}
        <VersionControlSection item={item} widgetData={null}
          activeSessionCount={0}
          worktreePath=""
          onViewDiff={jest.fn()}
          onBrowseFiles={jest.fn()}
          defaultExpanded
        />
      </>
    );
  }

  it("pr_pending: only PullRequestSection links the PR", () => {
    const { container } = renderPrSurfaces(makeItem({ status: "pr_pending", ...prFields }));
    expect(container.querySelectorAll('a[href="https://github.com/o/r/pull/7"]')).toHaveLength(1);
    expect(screen.queryByTestId("lifecycle-pr-badge")).not.toBeInTheDocument();
    expect(screen.queryByTestId("vcs-section-pr-link")).not.toBeInTheDocument();
  });
});
