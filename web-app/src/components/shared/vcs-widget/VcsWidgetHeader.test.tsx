import React from "react";
import { render, screen } from "@testing-library/react";
import { VcsWidgetHeader } from "./VcsWidgetHeader";
import type { VcsWidgetData } from "@/lib/vcs/types";

function makeData(overrides: Partial<VcsWidgetData> = {}): VcsWidgetData {
  return {
    kind: "live",
    branch: "feat/vcs-widget",
    isClean: false,
    fileChanges: [],
    aheadOfMain: 3,
    behindMain: 1,
    branchExists: true,
    commits: [],
    github: null,
    shipped: false,
    ...overrides,
  } as VcsWidgetData;
}

describe("VcsWidgetHeader", () => {
  it("VcsWidgetHeader_should_RenderBranchAheadBehindAndAriaLabeledCopyButton_When_FullModeWithWorktreePath", () => {
    render(
      <VcsWidgetHeader
        data={makeData()}
        mode="full"
        worktreePath="/home/tstapler/.stapler-squad/worktrees/feat-vcs-widget"
      />
    );

    expect(screen.getByText("feat/vcs-widget")).toBeInTheDocument();
    expect(screen.getByText("Uncommitted changes")).toBeInTheDocument();
    expect(screen.getByText("↑3 ahead")).toBeInTheDocument();
    expect(screen.getByText("↓1 behind")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy worktree path" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Browse files in this worktree" })).toBeInTheDocument();
  });

  it("VcsWidgetHeader_should_HideActiveSessionsIndicator_When_ActiveSessionCountIsOneOrOmitted", () => {
    const { rerender } = render(
      <VcsWidgetHeader data={makeData()} mode="full" activeSessionCount={1} />
    );
    expect(screen.queryByText(/active sessions/)).not.toBeInTheDocument();

    rerender(<VcsWidgetHeader data={makeData()} mode="full" />);
    expect(screen.queryByText(/active sessions/)).not.toBeInTheDocument();

    rerender(<VcsWidgetHeader data={makeData()} mode="full" activeSessionCount={3} />);
    expect(screen.getByText("3 active sessions")).toBeInTheDocument();
  });

  it("hides ahead/behind row when both counts are zero", () => {
    render(<VcsWidgetHeader data={makeData({ aheadOfMain: 0, behindMain: 0 })} mode="full" />);
    expect(screen.queryByText(/ahead/)).not.toBeInTheDocument();
    expect(screen.queryByText(/behind/)).not.toBeInTheDocument();
  });

  it("hides the worktree path row in compact mode even when worktreePath is set", () => {
    render(
      <VcsWidgetHeader data={makeData()} mode="compact" worktreePath="/some/path" />
    );
    expect(screen.queryByRole("button", { name: "Copy worktree path" })).not.toBeInTheDocument();
  });

  it("renders Clean text when isClean is true", () => {
    render(<VcsWidgetHeader data={makeData({ isClean: true })} mode="full" />);
    expect(screen.getByText("Clean")).toBeInTheDocument();
  });

  it("VcsWidgetHeader_should_RenderDeletedBranchCopy_When_BranchExistsFalse", () => {
    render(<VcsWidgetHeader data={makeData({ branchExists: false })} mode="full" />);
    expect(screen.getByText("(deleted — already merged)")).toBeInTheDocument();
  });

  it("VcsWidgetHeader_should_OmitDeletedBranchCopy_When_BranchExistsTrue", () => {
    render(<VcsWidgetHeader data={makeData({ branchExists: true })} mode="full" />);
    expect(screen.queryByText("(deleted — already merged)")).not.toBeInTheDocument();
  });
});
