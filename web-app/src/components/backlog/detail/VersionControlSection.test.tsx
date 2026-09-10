import React from "react";
import { render, screen } from "@testing-library/react";
import { VersionControlSection } from "./VersionControlSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import type { VcsWidgetData, GithubSummary } from "@/lib/vcs/types";

beforeEach(() => {
  localStorage.clear();
});

function makeGithub(overrides: Partial<GithubSummary> = {}): GithubSummary {
  return {
    owner: "example",
    repo: "repo",
    prUrl: "https://github.com/example/repo/pull/42",
    prNumber: 42,
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

function makeWidgetData(overrides: Partial<VcsWidgetData> = {}): VcsWidgetData {
  return {
    kind: "live",
    branch: "feat/x",
    isClean: true,
    fileChanges: [],
    aheadOfMain: 0,
    behindMain: 0,
    branchExists: true,
    commits: [],
    github: makeGithub(),
    shipped: false,
    ...overrides,
  } as VcsWidgetData;
}

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("VersionControlSection", () => {
  it("renders nothing when widgetData is null", () => {
    const { container } = render(
      <VersionControlSection
        item={makeItem()}
        widgetData={null}
        activeSessionCount={0}
        worktreePath={undefined}
        defaultExpanded={true}
        onViewDiff={jest.fn()}
        onBrowseFiles={jest.fn()}
      />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("D4: shows its own PR link for a non-pr_pending status (PullRequestSection isn't rendering)", () => {
    render(
      <VersionControlSection
        item={makeItem({ status: "in_progress" })}
        widgetData={makeWidgetData()}
        activeSessionCount={0}
        worktreePath={undefined}
        defaultExpanded={true}
        onViewDiff={jest.fn()}
        onBrowseFiles={jest.fn()}
      />
    );
    expect(screen.getByRole("link", { name: /PR #42/ })).toBeInTheDocument();
  });

  it("D4: omits its own PR link for pr_pending status (PullRequestSection is the single source)", () => {
    render(
      <VersionControlSection
        item={makeItem({ status: "pr_pending", prUrl: "https://github.com/example/repo/pull/42", prNumber: 42 })}
        widgetData={makeWidgetData()}
        activeSessionCount={0}
        worktreePath={undefined}
        defaultExpanded={true}
        onViewDiff={jest.fn()}
        onBrowseFiles={jest.fn()}
      />
    );
    expect(screen.queryByRole("link", { name: /PR #42/ })).not.toBeInTheDocument();
  });
});
