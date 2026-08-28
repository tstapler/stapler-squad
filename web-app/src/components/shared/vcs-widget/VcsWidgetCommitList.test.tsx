import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VcsWidgetCommitList } from "./VcsWidgetCommitList";
import type { CommitSummary } from "@/lib/vcs/types";

function makeCommit(overrides: Partial<CommitSummary> = {}): CommitSummary {
  return {
    sha: "abc1234",
    summary: "a commit",
    ...overrides,
  };
}

describe("VcsWidgetCommitList", () => {
  it("VcsWidgetCommitList_should_ExpandFullSummaryOnTap_When_RowClickedAtNarrowWidth", async () => {
    const longSummary =
      "feat: add a very long commit message that definitely exceeds one line of available width in the commit list row";
    render(<VcsWidgetCommitList commits={[makeCommit({ summary: longSummary })]} mode="compact" />);

    expect(screen.queryByTestId("commit-row-expanded")).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: longSummary }));

    const expanded = screen.getByTestId("commit-row-expanded");
    expect(expanded).toHaveTextContent(longSummary);
  });

  it("VcsWidgetCommitList_should_CapAt20WithShowAllButton_When_FullModeHas30Commits", () => {
    const commits = Array.from({ length: 30 }, (_, i) => makeCommit({ sha: `sha-${i}`, summary: `commit ${i}` }));
    render(<VcsWidgetCommitList commits={commits} mode="full" />);

    expect(screen.getByText("commit 19")).toBeInTheDocument();
    expect(screen.queryByText("commit 20")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show all 30 commits" })).toBeInTheDocument();
  });

  it("caps compact mode at 5 with no expand-all button", () => {
    const commits = Array.from({ length: 30 }, (_, i) => makeCommit({ sha: `sha-${i}`, summary: `commit ${i}` }));
    render(<VcsWidgetCommitList commits={commits} mode="compact" />);

    expect(screen.getByText("commit 4")).toBeInTheDocument();
    expect(screen.queryByText("commit 5")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Show all/ })).not.toBeInTheDocument();
  });

  it("reveals all 30 commits after clicking Show all in full mode", async () => {
    const commits = Array.from({ length: 30 }, (_, i) => makeCommit({ sha: `sha-${i}`, summary: `commit ${i}` }));
    render(<VcsWidgetCommitList commits={commits} mode="full" />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Show all 30 commits" }));

    expect(screen.getByText("commit 29")).toBeInTheDocument();
  });

  it("renders nothing when there are no commits", () => {
    const { container } = render(<VcsWidgetCommitList commits={[]} mode="full" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetCommitList_should_CapAt20WithShowAllButton_When_FullModeHas25Commits", async () => {
    const commits = Array.from({ length: 25 }, (_, i) => makeCommit({ sha: `sha-${i}`, summary: `commit ${i}` }));
    render(<VcsWidgetCommitList commits={commits} mode="full" />);

    expect(screen.getByText("commit 19")).toBeInTheDocument();
    expect(screen.queryByText("commit 20")).not.toBeInTheDocument();
    const showAllButton = screen.getByRole("button", { name: "Show all 25 commits" });
    expect(showAllButton).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(showAllButton);

    for (let i = 0; i < 25; i++) {
      expect(screen.getByText(`commit ${i}`)).toBeInTheDocument();
    }
  });

  it("VcsWidgetCommitList_should_ExpandFullSummaryOnTap_When_FullModeRowHasVeryLongSummary", async () => {
    const longSummary = "feat: ".concat("a".repeat(194)); // ~200 chars total
    render(<VcsWidgetCommitList commits={[makeCommit({ summary: longSummary })]} mode="full" />);

    expect(screen.queryByTestId("commit-row-expanded")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: longSummary })).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: longSummary }));

    const expanded = screen.getByTestId("commit-row-expanded");
    expect(expanded).toHaveTextContent(longSummary);
  });

  it("VcsWidgetCommitList_should_DecorateFirstRowAsHead_When_ModeIsFull", () => {
    const commits = [
      makeCommit({ sha: "sha-0", summary: "commit 0" }),
      makeCommit({ sha: "sha-1", summary: "commit 1" }),
      makeCommit({ sha: "sha-2", summary: "commit 2" }),
    ];
    render(<VcsWidgetCommitList commits={commits} mode="full" />);

    const buttons = screen.getAllByRole("button", { name: /commit \d/ });
    expect(buttons[0]).toHaveAttribute("data-head", "true");
    expect(buttons[1]).not.toHaveAttribute("data-head");
    expect(buttons[2]).not.toHaveAttribute("data-head");
  });

  it("VcsWidgetCommitList_should_RenderNotice_When_UnavailableAndNoCommits", () => {
    const { container } = render(<VcsWidgetCommitList commits={[]} mode="full" unavailable />);

    expect(container).not.toBeEmptyDOMElement();
    expect(screen.getByText("Couldn't load commit history — try refreshing.")).toBeInTheDocument();
  });

  it("VcsWidgetCommitList_should_RenderNothing_When_NoCommitsAndNotUnavailable", () => {
    const { container } = render(<VcsWidgetCommitList commits={[]} mode="full" unavailable={false} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetCommitList_should_ShowTruncationNote_When_TruncatedAndFullListShown", () => {
    const commits = Array.from({ length: 20 }, (_, i) => makeCommit({ sha: `sha-${i}`, summary: `commit ${i}` }));
    render(<VcsWidgetCommitList commits={commits} mode="full" truncated />);

    expect(screen.queryByRole("button", { name: /Show all/ })).not.toBeInTheDocument();
    expect(
      screen.getByText("Showing the 20 most recent commits — there may be more."),
    ).toBeInTheDocument();
  });
});
