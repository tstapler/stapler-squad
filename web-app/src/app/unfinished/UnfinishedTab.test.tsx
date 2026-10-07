import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { makeAttentionPRs, makePR } from "./testFixtures";

const mockUseSearchParams = jest.fn();
const mockReplace = jest.fn();
const mockHistoryReplace = jest.spyOn(window.history, "replaceState").mockImplementation(() => {});

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockUseSearchParams(),
  useRouter: () => ({ replace: mockReplace }),
}));

const mockDismissWorktree = jest.fn();
const mockSnoozeWorktree = jest.fn();
const mockWatchUserPRs = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    dismissWorktree: mockDismissWorktree,
    snoozeWorktree: mockSnoozeWorktree,
    watchUserPRs: mockWatchUserPRs,
    listGitHubCLIHosts: jest.fn().mockResolvedValue({ ghAvailable: false, hosts: [] }),
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/api/transport", () => ({ getWatchTransport: () => ({}) }));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost",
  createAuthInterceptor: () => jest.fn(),
}));

let mockWorktrees: Array<{ repoName: string; repoPath: string; branch: string; hasUncommitted: boolean; commitsAhead: number; commitsBehind: number }> = [];
jest.mock("@/lib/hooks/useUnfinishedWork", () => ({
  useUnfinishedWork: () => ({
    worktrees: mockWorktrees,
    lastScanTime: null,
    isScanning: false,
    triggerScan: jest.fn(),
  }),
}));

jest.mock("@/components/unfinished/UnfinishedRepoGroup", () => ({
  UnfinishedRepoGroup: (props: {
    repoName: string;
    worktrees: Array<{ repoPath: string; branch: string }>;
    onDismiss: (repoPath: string, branch: string) => void;
    onSnooze: (repoPath: string, branch: string) => void;
  }) => (
    <div data-testid="unfinished-repo-group">
      {props.repoName}
      <button onClick={() => props.onDismiss(props.worktrees[0].repoPath, props.worktrees[0].branch)}>dismiss</button>
      <button onClick={() => props.onSnooze(props.worktrees[0].repoPath, props.worktrees[0].branch)}>snooze</button>
    </div>
  ),
}));

jest.mock("@/components/unfinished/BacklogQueueSection", () => ({
  BacklogQueueSection: () => <div data-testid="backlog-queue-section" />,
}));

const mockStuckItemsSection = jest.fn();
jest.mock("@/components/backlog-stuck/StuckItemsSection", () => ({
  StuckItemsSection: (props: { focusItemId?: string }) => {
    mockStuckItemsSection(props);
    return <div data-testid="stuck-items-section" />;
  },
}));

let mockStuckItems: Array<{ itemId: string }> = [];
let mockStuckLastFetched: Date | null = new Date(0);
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: mockStuckItems, lastFetched: mockStuckLastFetched }),
}));

import { UnfinishedTab } from "./UnfinishedTab";
import { TAB_STATS_STORAGE_KEY } from "@/lib/unfinished/tabStats";
import type { UserPR } from "@/gen/session/v1/types_pb";

function streamPRs(prs: UserPR[]) {
  mockWatchUserPRs.mockImplementation(async function* () {
    yield { eventType: "snapshot", authState: { available: true, accounts: [] }, prs, accountStatuses: [] };
  });
}

const tab = (name: string | RegExp) => screen.getByRole("tab", { name });

describe("UnfinishedTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    mockUseSearchParams.mockReturnValue(new URLSearchParams());
    mockWorktrees = [];
    mockStuckItems = [];
    mockStuckLastFetched = new Date(0);
    // Default: stream never delivers, so sync tests see no late state updates.
    mockWatchUserPRs.mockImplementation(async function* () {
      await new Promise(() => {});
    });
  });

  it("unfinishedTab_should_SelectPRsAndOmitStuckContent_When_NoParamsAndEmptyStorage", () => {
    render(<UnfinishedTab />);
    expect(tab(/^PRs/)).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByTestId("stuck-items-section")).not.toBeInTheDocument();
    expect(mockStuckItemsSection).not.toHaveBeenCalled();
  });

  it("unfinishedTab_should_MountStuckPanelWithFocusItemIdOnFirstMount_When_ItemDeepLink", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "item-7" }));
    render(<UnfinishedTab />);
    expect(mockStuckItemsSection.mock.calls[0][0]).toEqual({ focusItemId: "item-7" });
    expect(screen.queryByTestId("prs-skeleton")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("GitHub Pull Requests")).not.toBeInTheDocument();
  });

  it("unfinishedTab_should_SelectStuckAndPassFocusItemId_When_ItemDeepLink", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "item-7" }));
    render(<UnfinishedTab />);
    expect(tab(/^Stuck/)).toHaveAttribute("aria-selected", "true");
    expect(mockStuckItemsSection).toHaveBeenCalledWith(expect.objectContaining({ focusItemId: "item-7" }));
  });

  it("unfinishedTab_should_ReplaceUrlWithTabWorktreesAndNotReselectStuck_When_LeavingDeepLink", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "item-7" }));
    const { rerender } = render(<UnfinishedTab />);
    fireEvent.mouseDown(tab(/^Worktrees/));
    fireEvent.click(tab(/^Worktrees/));
    expect(mockHistoryReplace).toHaveBeenCalledWith(window.history.state, "", "?tab=worktrees");

    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "worktrees" }));
    rerender(<UnfinishedTab />);
    expect(tab(/^Worktrees/)).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByTestId("stuck-items-section")).not.toBeInTheDocument();
  });

  it("unfinishedTab_should_ShowPRsBadge3_When_FailingCIAndConflictAndQueueActive", async () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "queue" }));
    streamPRs(makeAttentionPRs());
    render(<UnfinishedTab />);
    expect(await screen.findByRole("tab", { name: "PRs, 3 need attention" })).toBeInTheDocument();
    expect(screen.getByTestId("backlog-queue-section")).toBeInTheDocument();
  });

  it("unfinishedTab_should_AppendPlusToBadge_When_DetailsNotLoaded", async () => {
    streamPRs(makeAttentionPRs({ detailsLoaded: false, unresolvedThreadCount: undefined }));
    render(<UnfinishedTab />);
    const prsTab = await screen.findByRole("tab", { name: "PRs, 3 or more need attention" });
    expect(prsTab).toHaveTextContent("3+");
  });

  it("unfinishedTab_should_DropPlusSuffix_When_DetailsLoadedTrueAfterRefresh", async () => {
    streamPRs(makeAttentionPRs({ detailsLoaded: false, unresolvedThreadCount: undefined }));
    const first = render(<UnfinishedTab />);
    await screen.findByRole("tab", { name: "PRs, 3 or more need attention" });
    first.unmount();

    streamPRs(makeAttentionPRs());
    render(<UnfinishedTab />);
    expect(await screen.findByRole("tab", { name: "PRs, 3 need attention" })).not.toHaveTextContent("3+");
  });

  it("unfinishedTab_should_SubscribeOnceAndNotFlashZero_When_TabsSwitch", async () => {
    streamPRs(makeAttentionPRs());
    const { rerender } = render(<UnfinishedTab />);
    await screen.findByRole("tab", { name: "PRs, 3 need attention" });

    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "queue" }));
    rerender(<UnfinishedTab />);
    expect(tab("PRs, 3 need attention")).toBeInTheDocument();
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "prs" }));
    rerender(<UnfinishedTab />);
    expect(tab("PRs, 3 need attention")).toBeInTheDocument();
    expect(mockWatchUserPRs).toHaveBeenCalledTimes(1);
  });

  it("unfinishedTab_should_ShowNotFoundNoticeAndClearItemOnShowAll_When_ItemMissing", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "gone-1" }));
    mockStuckItems = [{ itemId: "other" }];
    const { rerender } = render(<UnfinishedTab />);
    const notice = screen.getByRole("status");
    expect(notice).toHaveTextContent("Item gone-1 was not found. It may have been completed or removed.");
    expect(screen.getByTestId("stuck-items-section")).toBeInTheDocument();

    fireEvent.click(within(notice).getByRole("button", { name: "Show all stuck items" }));
    expect(mockReplace).toHaveBeenCalledWith("?tab=stuck", { scroll: false });

    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "stuck" }));
    rerender(<UnfinishedTab />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

    // Empty list: notice still shows alongside the (mocked) list/empty state.
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "gone-1" }));
    mockStuckItems = [];
    rerender(<UnfinishedTab />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.getByTestId("stuck-items-section")).toBeInTheDocument();
  });

  it("unfinishedTab_should_MoveFocusToStuckListHeading_When_ShowAllStuckItemsClicked", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "gone-1" }));
    render(<UnfinishedTab />);
    fireEvent.click(screen.getByRole("button", { name: "Show all stuck items" }));
    expect(document.activeElement).toBe(
      within(screen.getByTestId("up-next-panel-stuck")).getByRole("heading", { level: 2 })
    );
  });

  it("unfinishedTab_should_NotShowNotFoundNotice_When_StuckListNotLoadedYet", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "gone-1" }));
    mockStuckLastFetched = null;
    render(<UnfinishedTab />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("unfinishedTab_should_KeepWorktreeFilterDismissSnooze_When_InsideWorktreesTab", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "worktrees" }));
    mockWorktrees = [
      { repoName: "api", repoPath: "/r/api", branch: "a", hasUncommitted: true, commitsAhead: 0, commitsBehind: 0 },
      { repoName: "web", repoPath: "/r/web", branch: "b", hasUncommitted: false, commitsAhead: 2, commitsBehind: 0 },
    ];
    render(<UnfinishedTab />);
    expect(screen.getAllByTestId("unfinished-repo-group")).toHaveLength(2);

    fireEvent.click(screen.getByRole("button", { name: "Ahead" }));
    const groups = screen.getAllByTestId("unfinished-repo-group");
    expect(groups).toHaveLength(1);
    expect(groups[0]).toHaveTextContent("web");

    fireEvent.click(within(groups[0]).getByRole("button", { name: "dismiss" }));
    fireEvent.click(within(groups[0]).getByRole("button", { name: "snooze" }));
    expect(mockDismissWorktree).toHaveBeenCalledWith(expect.objectContaining({ repoPath: "/r/web", branch: "b" }));
    expect(mockSnoozeWorktree).toHaveBeenCalledWith(expect.objectContaining({ repoPath: "/r/web", branch: "b" }));
  });

  it("unfinishedTab_should_PreservePRSearchText_When_SwitchingToQueueAndBack", async () => {
    streamPRs([makePR({ title: "api fix" })]);
    const { rerender } = render(<UnfinishedTab />);
    const box = await screen.findByRole("searchbox");
    fireEvent.change(box, { target: { value: "api" } });

    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "queue" }));
    rerender(<UnfinishedTab />);
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "prs" }));
    rerender(<UnfinishedTab />);
    expect(await screen.findByRole("searchbox")).toHaveValue("api");
  });

  it("unfinishedTab_should_PreservePRSearchAndFilter_When_SwitchingTabsAndBack", async () => {
    streamPRs([makePR({ title: "api fix" })]);
    const { rerender } = render(<UnfinishedTab />);
    fireEvent.change(await screen.findByRole("searchbox"), { target: { value: "api" } });
    fireEvent.click(screen.getByRole("button", { name: "Draft" }));

    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "queue" }));
    rerender(<UnfinishedTab />);
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ tab: "prs" }));
    rerender(<UnfinishedTab />);
    expect(await screen.findByRole("searchbox")).toHaveValue("api");
    expect(screen.getByRole("button", { name: "Draft" })).toHaveAttribute("aria-pressed", "true");
  });

  it("unfinishedTab_should_NotPutFilterSortSearchInUrl_When_FilterChanged", async () => {
    streamPRs([makePR()]);
    render(<UnfinishedTab />);
    fireEvent.change(await screen.findByRole("searchbox"), { target: { value: "api" } });
    fireEvent.click(screen.getByRole("button", { name: "Draft" }));
    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockHistoryReplace).not.toHaveBeenCalled();
  });

  it("unfinishedTab_should_ExplainTabVsNavBadgeAndRenderPlusInHeaderCount_When_Degraded", async () => {
    streamPRs(makeAttentionPRs({ detailsLoaded: false, unresolvedThreadCount: undefined }));
    render(<UnfinishedTab />);
    const prsTab = await screen.findByRole("tab", { name: "PRs, 3 or more need attention" });
    const description = prsTab.getAttribute("title") ?? "";
    expect(description).toContain(
      "At least 3 PRs need attention. Review-thread counts are not loaded yet, so the real number may be higher."
    );
    expect(description).toContain(
      "PRs with failing CI, conflicts, unresolved threads or requested changes. The sidebar badge uses its own count."
    );
    const header = screen.getByTestId("github-prs-attention");
    expect(header).toHaveTextContent("3+ need attention");
    expect(header).toHaveTextContent("Review-thread counts are not loaded yet");
  });

  it("unfinishedTab_should_LeaveUnfinishedNavBadgeUntouched_When_TabBadgeDegraded", async () => {
    mockWorktrees = [
      { repoName: "r", repoPath: "/r", branch: "a", hasUncommitted: true, commitsAhead: 0, commitsBehind: 0 },
      { repoName: "r", repoPath: "/r", branch: "b", hasUncommitted: true, commitsAhead: 0, commitsBehind: 0 },
    ];
    streamPRs(makeAttentionPRs({ detailsLoaded: false, unresolvedThreadCount: undefined }));
    const { UnfinishedNavBadge } = jest.requireActual("@/components/unfinished/UnfinishedNavBadge");
    render(
      <>
        <UnfinishedNavBadge />
        <UnfinishedTab />
      </>
    );
    await screen.findByRole("tab", { name: "PRs, 3 or more need attention" });
    // The sidebar badge keeps counting worktrees: no PR count, no "+" suffix leaks into it.
    const navBadge = screen.getByTestId("unfinished-nav-badge");
    expect(navBadge).toHaveTextContent(/^2$/);
    expect(navBadge).toHaveAttribute("aria-label", "2 unfinished items");
  });

  it("unfinishedTab_should_ShowSkeletonAndAllowTabSwitch_When_PRsLoading", () => {
    // authState undefined means the WatchUserPRs stream has not delivered yet.
    mockWatchUserPRs.mockImplementation(() => (async function* () { await new Promise(() => {}); })());
    const { rerender } = render(<UnfinishedTab />);
    // The shell is interactive while the PR stream is pending: all four tabs and the PRs panel render.
    expect(screen.getAllByRole("tab")).toHaveLength(4);
    expect(within(screen.getByTestId("up-next-panel-prs")).getByText("Connecting to GitHub…")).toBeInTheDocument();
    fireEvent.mouseDown(tab(/^Queue/));
    fireEvent.click(tab(/^Queue/));
    expect(mockHistoryReplace).toHaveBeenCalledWith(window.history.state, "", "?tab=queue");

    // Once the URL follows, the Queue panel mounts and the pending PRs placeholder is gone.
    mockUseSearchParams.mockReturnValue(new URLSearchParams("tab=queue"));
    rerender(<UnfinishedTab />);
    expect(screen.getByTestId("backlog-queue-section")).toBeInTheDocument();
    expect(screen.queryByText("Connecting to GitHub…")).not.toBeInTheDocument();
  });

  it("unfinishedTab_should_EmitFirstPrCardOnce_When_PRsArrive", async () => {
    streamPRs([makePR()]);
    render(<UnfinishedTab />);
    await waitFor(() => {
      const stats = JSON.parse(window.localStorage.getItem(TAB_STATS_STORAGE_KEY) ?? "{}");
      expect(typeof stats.firstPrCardMs).toBe("number");
    });
  });
});
