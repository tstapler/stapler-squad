import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { UnfinishedWorktreeSchema } from "@/gen/session/v1/types_pb";
import { UnfinishedItemDetail } from "./UnfinishedItemDetail";

const pushMock = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: jest.fn(() => ({ push: pushMock })),
}));

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getWorktreeAISummary: jest.fn().mockResolvedValue({ summary: "" }),
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("./CommitPushModal", () => ({
  CommitPushModal: () => <div data-testid="commit-push-modal" />,
}));

jest.mock("./WorktreeDiffModal", () => ({
  WorktreeDiffModal: () => <div data-testid="worktree-diff-modal" />,
}));

beforeEach(() => {
  pushMock.mockClear();
});

describe("UnfinishedItemDetail", () => {
  it("UnfinishedItemDetail_should_RenderCompactVcsWidgetWithStatsAndCommits_When_WorktreeHasChanges", () => {
    const worktree = create(UnfinishedWorktreeSchema, {
      changedFiles: 5,
      linesAdded: 42,
      linesRemoved: 8,
      aheadCommitMessages: ["fix: typo"],
    });

    render(<UnfinishedItemDetail worktree={worktree} />);

    expect(screen.getByTestId("vcs-widget-loaded")).toBeInTheDocument();
    expect(screen.getByText("5 files changed")).toBeInTheDocument();
    expect(screen.getByText("+42")).toBeInTheDocument();
    expect(screen.getByText("-8")).toBeInTheDocument();
    expect(screen.getByText("fix: typo")).toBeInTheDocument();
  });

  it("UnfinishedItemDetail_should_PreserveActionButtonRowUnaffected_When_StatsBlockReplaced", () => {
    const worktree = create(UnfinishedWorktreeSchema, {
      changedFiles: 5,
      linesAdded: 42,
      linesRemoved: 8,
      aheadCommitMessages: ["fix: typo"],
      repoPath: "/repo",
      branch: "feature/x",
      worktreePath: "/repo/worktrees/x",
      sessionIds: [],
    });

    render(<UnfinishedItemDetail worktree={worktree} />);

    const openSessionBtn = screen.getByRole("button", { name: "Open Session" });
    const viewDiffBtn = screen.getByRole("button", { name: "View Diff" });
    const commitPushBtn = screen.getByRole("button", { name: "Commit & Push" });
    const summarizeBtn = screen.getByRole("button", { name: "Generate AI summary of changes" });

    expect(openSessionBtn).toBeInTheDocument();
    expect(viewDiffBtn).toBeInTheDocument();
    expect(commitPushBtn).toBeInTheDocument();
    expect(summarizeBtn).toBeInTheDocument();

    fireEvent.click(openSessionBtn);
    expect(pushMock).toHaveBeenCalledTimes(1);
    const pushedUrl = pushMock.mock.calls[0][0] as string;
    expect(decodeURIComponent(pushedUrl)).toContain("/repo/worktrees/x");

    fireEvent.click(viewDiffBtn);
    expect(screen.getByTestId("worktree-diff-modal")).toBeInTheDocument();

    fireEvent.click(commitPushBtn);
    expect(screen.getByTestId("commit-push-modal")).toBeInTheDocument();
  });
});
