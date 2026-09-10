import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { VcsWidget } from "./VcsWidget";
import type { VcsWidgetData, GithubSummary } from "@/lib/vcs/types";

// VcsWidgetComments (rendered only when data.github + sessionId are both
// present) calls GetPRComments via createClient — mocked here so tests that
// do exercise that combination don't hit a real transport. A single shared
// mock (not a fresh jest.fn() per createClient() call) so tests can assert
// on call history across rerenders — see the queue-navigation remount test
// below.
const mockGetPRComments = jest.fn().mockResolvedValue({ comments: [] });
jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({ getPRComments: (...args: unknown[]) => mockGetPRComments(...args) })),
}));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

function makeData(overrides: Partial<VcsWidgetData> = {}): VcsWidgetData {
  return {
    kind: "live",
    branch: "feat/order-test",
    isClean: false,
    fileChanges: [],
    aheadOfMain: 0,
    behindMain: 0,
    branchExists: true,
    commits: [],
    github: null,
    shipped: false,
    ...overrides,
  } as VcsWidgetData;
}

// Shared factory for the repeated 13-field GithubSummary literal below —
// mirrors the `makeCheck`/`githubSummary` convention already used in
// MergeabilityPill.test.tsx and VersionControlSection.test.tsx.
function makeGithub(overrides: Partial<GithubSummary> = {}): GithubSummary {
  return {
    owner: "acme",
    repo: "widget",
    prUrl: "https://github.com/acme/widget/pull/1",
    prNumber: 1,
    prState: "open",
    isDraft: false,
    checkConclusion: "success",
    approvedCount: 0,
    changesReqCount: 0,
    mergeable: "unknown",
    checks: [],
    reviewFeedback: [],
    ...overrides,
  };
}

describe("VcsWidget", () => {
  it("VcsWidget_should_RenderSectionsInMergeabilityPillFirstOrder_When_FullModeWithPopulatedData", () => {
    const data = makeData({
      branch: "feat/order-test",
      github: makeGithub({ prUrl: "https://github.com/acme/widget/pull/99", prNumber: 99 }),
      fileChanges: [
        {
          path: "src/order.go",
          status: "modified",
          additions: 3,
          deletions: 1,
          section: "unstaged",
        },
      ],
      commits: [{ sha: "abc123", summary: "test commit order" }],
    });

    const { container } = render(<VcsWidget data={data} mode="full" />);

    const html = container.innerHTML;
    const pillIdx = html.indexOf("Ready to merge");
    const headerIdx = html.indexOf("feat/order-test");
    const githubIdx = html.indexOf("PR #99");
    const fileIdx = html.indexOf("src/order.go");
    const commitIdx = html.indexOf("test commit order");

    expect(pillIdx).toBeGreaterThan(-1);
    expect(headerIdx).toBeGreaterThan(pillIdx);
    expect(githubIdx).toBeGreaterThan(headerIdx);
    expect(fileIdx).toBeGreaterThan(githubIdx);
    expect(commitIdx).toBeGreaterThan(fileIdx);
  });

  it("VcsWidget_should_OmitRefreshButton_When_DataKindHistorical", () => {
    const onRefresh = jest.fn();
    render(
      <VcsWidget
        data={makeData({ kind: "historical", snapshotAt: new Date() } as VcsWidgetData)}
        mode="full"
        onRefresh={onRefresh}
      />
    );

    expect(screen.queryByRole("button", { name: "Refresh VCS status" })).not.toBeInTheDocument();
  });

  it("renders the refresh button when kind is live and onRefresh is provided", () => {
    const onRefresh = jest.fn();
    render(<VcsWidget data={makeData({ kind: "live" })} mode="full" onRefresh={onRefresh} />);

    expect(screen.getByRole("button", { name: "Refresh VCS status" })).toBeInTheDocument();
  });

  it("omits the refresh button when kind is live but no onRefresh is provided", () => {
    render(<VcsWidget data={makeData({ kind: "live" })} mode="full" />);

    expect(screen.queryByRole("button", { name: "Refresh VCS status" })).not.toBeInTheDocument();
  });

  it("compact mode never renders per-file rows even when fileChanges is populated", () => {
    const onNavigateToFile = jest.fn();
    const data = makeData({
      fileChanges: [
        {
          path: "src/compact-omitted.go",
          status: "modified",
          additions: 1,
          deletions: 1,
          section: "unstaged",
        },
      ],
      aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 },
    });

    render(<VcsWidget data={data} mode="compact" onNavigateToFile={onNavigateToFile} />);

    expect(screen.queryByText("src/compact-omitted.go")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /compact-omitted/ })
    ).not.toBeInTheDocument();
  });

  it("compact mode omits VcsWidgetGithubRow but shows the aggregate stat line", () => {
    const data = makeData({
      github: makeGithub(),
      aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 },
    });

    render(<VcsWidget data={data} mode="compact" />);

    expect(screen.queryByText("PR #1")).not.toBeInTheDocument();
    expect(screen.getByText("5 files changed")).toBeInTheDocument();
    expect(screen.getByText("+42")).toBeInTheDocument();
    expect(screen.getByText("-8")).toBeInTheDocument();
  });

  it("VcsWidget_should_RenderAggregateStatLine_When_FullModeWithAggregateStatsPresent", () => {
    const data = makeData({
      aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 },
    });

    render(<VcsWidget data={data} mode="full" />);

    expect(screen.getByText("5 files changed")).toBeInTheDocument();
    expect(screen.getByText("+42")).toBeInTheDocument();
    expect(screen.getByText("-8")).toBeInTheDocument();
  });

  it("caps compact-mode commit list at 5 entries", () => {
    const commits = Array.from({ length: 8 }, (_, i) => ({
      sha: `sha${i}`,
      summary: `commit ${i}`,
    }));
    render(<VcsWidget data={makeData({ commits })} mode="compact" />);

    for (let i = 0; i < 5; i++) {
      expect(screen.getByText(`commit ${i}`)).toBeInTheDocument();
    }
    for (let i = 5; i < 8; i++) {
      expect(screen.queryByText(`commit ${i}`)).not.toBeInTheDocument();
    }
  });

  it("has an aria-live=polite region present in full mode", () => {
    const { container } = render(<VcsWidget data={makeData()} mode="full" />);
    const liveRegions = container.querySelectorAll('[aria-live="polite"]');
    expect(liveRegions.length).toBeGreaterThanOrEqual(1);
    liveRegions.forEach((el) => expect(el).toHaveAttribute("role", "status"));
  });

  it("renders the 'As of' snapshot timestamp when historical with a snapshotAt", () => {
    const snapshotAt = new Date(Date.now() - 60_000);
    render(
      <VcsWidget
        data={makeData({ kind: "historical", snapshotAt } as VcsWidgetData)}
        mode="full"
      />
    );

    expect(screen.getByTestId("vcs-widget-snapshot-timestamp")).toHaveTextContent("As of");
  });

  it("renders loadError in neutral styling when historical with no snapshotAt", () => {
    render(
      <VcsWidget
        data={
          makeData({
            kind: "historical",
            snapshotAt: null,
            loadError: "No history captured",
          }) as VcsWidgetData
        }
        mode="full"
      />
    );

    expect(screen.getByText("No history captured")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("VcsWidget_should_RenderDefaultNoHistoryCopy_When_HistoricalWithNoSnapshotAtAndNoLoadError", () => {
    render(
      <VcsWidget
        data={
          makeData({
            kind: "historical",
            snapshotAt: null,
            loadError: undefined,
          }) as VcsWidgetData
        }
        mode="full"
      />
    );

    expect(
      screen.getByText(
        "No history captured for this item — it shipped before detailed tracking was added."
      )
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renders data-testid vcs-widget-loaded on the root element", () => {
    render(<VcsWidget data={makeData()} mode="full" />);
    expect(screen.getByTestId("vcs-widget-loaded")).toBeInTheDocument();
  });

  it("renders a View Diff affordance in full mode when onViewDiff is provided", () => {
    const onViewDiff = jest.fn();
    render(<VcsWidget data={makeData({ shipped: true })} mode="full" onViewDiff={onViewDiff} />);
    expect(screen.getByTestId("vcs-widget-view-diff")).toBeInTheDocument();
  });

  it("omits the View Diff affordance in compact mode even when onViewDiff is provided", () => {
    const onViewDiff = jest.fn();
    render(
      <VcsWidget data={makeData({ shipped: true })} mode="compact" onViewDiff={onViewDiff} />
    );
    expect(screen.queryByTestId("vcs-widget-view-diff")).not.toBeInTheDocument();
  });

  it("VcsWidget_should_ForwardOnBrowseFilesToHeader_When_FullModeWithWorktreePath", () => {
    const onBrowseFiles = jest.fn();
    render(
      <VcsWidget
        data={makeData()}
        mode="full"
        worktreePath="/tmp/some-worktree"
        onBrowseFiles={onBrowseFiles}
      />
    );

    screen.getByRole("button", { name: "Browse files in this worktree" }).click();
    expect(onBrowseFiles).toHaveBeenCalledTimes(1);
  });

  it("VcsWidget_should_OmitBrowseFilesButton_When_CompactModeEvenWithOnBrowseFiles", () => {
    const onBrowseFiles = jest.fn();
    render(
      <VcsWidget
        data={makeData()}
        mode="compact"
        worktreePath="/tmp/some-worktree"
        onBrowseFiles={onBrowseFiles}
      />
    );

    expect(screen.queryByRole("button", { name: "Browse files in this worktree" })).not.toBeInTheDocument();
  });

  it("VcsWidget_should_RenderBothStalenessLabels_When_LiveWithBothTimestampsPresent", () => {
    const statusAsOf = new Date(Date.now() - 5_000);
    const lastCheckedAt = new Date(Date.now() - 15_000);
    render(
      <VcsWidget
        data={makeData({
          kind: "live",
          statusAsOf,
          github: makeGithub({ lastCheckedAt }),
        })}
        mode="full"
      />
    );

    expect(screen.getByText(/^Local:/)).toBeInTheDocument();
    expect(screen.getByText(/^PR status confirmed/)).toBeInTheDocument();
  });

  it("VcsWidget_should_RenderOnlyLocalLabel_When_LiveWithGithubLastCheckedAtMissing", () => {
    const statusAsOf = new Date(Date.now() - 5_000);
    render(
      <VcsWidget
        data={makeData({ kind: "live", statusAsOf, github: null })}
        mode="full"
      />
    );

    expect(screen.getByText(/^Local:/)).toBeInTheDocument();
    expect(screen.queryByText(/^PR status confirmed/)).not.toBeInTheDocument();
  });

  it("VcsWidget_should_RenderOnlyPrConfirmedLabel_When_LiveWithStatusAsOfMissing", () => {
    const lastCheckedAt = new Date(Date.now() - 15_000);
    render(
      <VcsWidget
        data={makeData({
          kind: "live",
          statusAsOf: undefined,
          github: makeGithub({ lastCheckedAt }),
        })}
        mode="full"
      />
    );

    expect(screen.queryByText(/^Local:/)).not.toBeInTheDocument();
    expect(screen.getByText(/^PR status confirmed/)).toBeInTheDocument();
  });

  it("VcsWidget_should_RenderCommentsSection_When_FullModeWithGithubAndSessionIdBothPresent", () => {
    render(
      <VcsWidget
        data={makeData({ github: makeGithub() })}
        mode="full"
        sessionId="session-1"
      />
    );

    expect(screen.getByTestId("collapsible-header-pr-comments")).toBeInTheDocument();
  });

  it("VcsWidget_should_OmitCommentsSection_When_SessionIdMissingEvenWithGithubData", () => {
    render(
      <VcsWidget
        data={makeData({ github: makeGithub() })}
        mode="full"
      />
    );

    expect(screen.queryByTestId("collapsible-header-pr-comments")).not.toBeInTheDocument();
  });

  it("VcsWidget_should_RenderAllDisclosureSectionsCollapsed_When_FullModeWithChecksReviewsAndCommentsPresent", () => {
    render(
      <VcsWidget
        data={makeData({
          github: makeGithub({
            changesReqCount: 1,
            checks: [
              { name: "build", context: "ci/build", state: "completed", status: "completed", conclusion: "success" },
            ],
            reviewFeedback: [
              { author: "reviewer1", state: "CHANGES_REQUESTED", body: "Please fix this" },
            ],
          }),
        })}
        mode="full"
        sessionId="session-1"
      />
    );

    // CollapsibleSection's own defaultExpanded={false} is a no-op once composed inside the
    // real CollapsibleGroup (see Collapsible.tsx) — the group's own initial-state handling is
    // what actually drives this. This component doesn't branch on viewport, so asserting the
    // group starts fully collapsed here covers both desktop and narrow-viewport rendering
    // (Open Question 5 resolution: uniform closed-by-default, no viewport-specific behavior).
    const headers = screen.getAllByTestId(/^collapsible-header-/);
    expect(headers.length).toBe(3);
    headers.forEach((header) => expect(header).toHaveAttribute("aria-expanded", "false"));
  });

  it("VcsWidget_should_RefetchComments_When_SessionIdChangesWithoutUnmounting", async () => {
    // Regression test: review-queue navigation (SessionDetail's onNext/onPrevious)
    // changes the `sessionId` prop on an already-mounted VcsWidget while staying on
    // the VCS tab, instead of remounting the whole subtree. Before this fix,
    // VcsWidgetComments's fetch-guard never reset, so switching from session-1's PR
    // to session-2's PR left session-1's cached comments on screen with no refetch.
    mockGetPRComments.mockClear();
    mockGetPRComments
      .mockResolvedValueOnce({ comments: [{ id: 1n, author: "octocat", body: "PR one", isReview: false }] })
      .mockResolvedValueOnce({ comments: [{ id: 2n, author: "hubot", body: "PR two", isReview: false }] });

    const githubFor = (prNumber: number) => ({
      owner: "acme",
      repo: "widget",
      prUrl: `https://github.com/acme/widget/pull/${prNumber}`,
      prNumber,
      prState: "open" as const,
      isDraft: false,
      checkConclusion: "success" as const,
      approvedCount: 0,
      changesReqCount: 0,
      mergeable: "unknown" as const,
      checks: [],
      reviewFeedback: [],
    });

    const { rerender } = render(
      <VcsWidget
        data={makeData({ github: githubFor(1) })}
        mode="full"
        sessionId="session-1"
      />
    );

    fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));
    await waitFor(() => expect(screen.getByText("octocat")).toBeInTheDocument());
    expect(mockGetPRComments).toHaveBeenCalledTimes(1);
    expect(mockGetPRComments).toHaveBeenNthCalledWith(1, { id: "session-1" });

    // Simulate queue navigation: same VcsWidget instance, new session's data.
    rerender(
      <VcsWidget
        data={makeData({ github: githubFor(2) })}
        mode="full"
        sessionId="session-2"
      />
    );

    await waitFor(() => expect(screen.getByText("hubot")).toBeInTheDocument());
    expect(screen.queryByText("octocat")).not.toBeInTheDocument();
    expect(mockGetPRComments).toHaveBeenCalledTimes(2);
    expect(mockGetPRComments).toHaveBeenNthCalledWith(2, { id: "session-2" });
  });
});
