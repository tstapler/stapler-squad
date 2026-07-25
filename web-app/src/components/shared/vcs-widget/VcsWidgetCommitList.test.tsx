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
});
