import React from "react";
import { render, screen } from "@testing-library/react";
import { VcsWidget } from "./VcsWidget";
import type { VcsWidgetData } from "@/lib/vcs/types";

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

describe("VcsWidget", () => {
  it("VcsWidget_should_RenderSectionsInMergeabilityPillFirstOrder_When_FullModeWithPopulatedData", () => {
    const data = makeData({
      branch: "feat/order-test",
      github: {
        owner: "acme",
        repo: "widget",
        prUrl: "https://github.com/acme/widget/pull/99",
        prNumber: 99,
        prState: "open",
        isDraft: false,
        checkConclusion: "success",
        approvedCount: 0,
        changesReqCount: 0,
      },
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
      github: {
        owner: "acme",
        repo: "widget",
        prUrl: "https://github.com/acme/widget/pull/1",
        prNumber: 1,
        prState: "open",
        isDraft: false,
        checkConclusion: "success",
        approvedCount: 0,
        changesReqCount: 0,
      },
      aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 },
    });

    render(<VcsWidget data={data} mode="compact" />);

    expect(screen.queryByText("PR #1")).not.toBeInTheDocument();
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
});
