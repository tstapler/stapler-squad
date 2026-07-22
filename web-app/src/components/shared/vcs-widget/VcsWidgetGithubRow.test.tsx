import React from "react";
import { render, screen } from "@testing-library/react";
import { VcsWidgetGithubRow } from "./VcsWidgetGithubRow";
import type { GithubSummary, VcsWidgetData } from "@/lib/vcs/types";

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
    ...overrides,
  };
}

function makeData(overrides: Partial<VcsWidgetData> = {}): VcsWidgetData {
  return {
    kind: "live",
    branch: "feat/x",
    isClean: true,
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

describe("VcsWidgetGithubRow", () => {
  it("VcsWidgetGithubRow_should_RenderApprovedAndChangesRequestedWithAriaLabel_When_GithubPopulated", () => {
    render(
      <VcsWidgetGithubRow
        data={makeData({ github: makeGithub({ approvedCount: 2, changesReqCount: 1 }) })}
      />
    );

    expect(screen.getByLabelText("2 approved")).toBeInTheDocument();
    expect(screen.getByLabelText("1 changes requested")).toBeInTheDocument();
  });

  it("VcsWidgetGithubRow_should_ReturnNull_When_KindLiveAndGithubNull", () => {
    const { container } = render(<VcsWidgetGithubRow data={makeData({ kind: "live", github: null })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the draft icon/badge when the PR is a draft", () => {
    render(<VcsWidgetGithubRow data={makeData({ github: makeGithub({ isDraft: true }) })} />);
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("links to the PR URL and shows the PR number", () => {
    render(<VcsWidgetGithubRow data={makeData({ github: makeGithub({ prNumber: 42 }) })} />);
    const link = screen.getByRole("link", { name: /PR #42/ });
    expect(link).toHaveAttribute("href", "https://github.com/example/repo/pull/42");
  });

  it("VcsWidgetGithubRow_should_RenderFullCaptureFailureCopy_When_SnapshotCaptureFailedTrueAndGithubNull", () => {
    render(
      <VcsWidgetGithubRow
        data={makeData({ kind: "historical", snapshotAt: null, snapshotCaptureFailed: true, github: null })}
      />
    );

    expect(screen.getByText("Couldn't capture PR status at ship time")).toBeInTheDocument();
    expect(screen.queryByText(/^CI:/)).not.toBeInTheDocument();
  });

  it("VcsWidgetGithubRow_should_RenderPartialCaptureFailureCopyAlongsideRealData_When_GithubPartiallyPopulated", () => {
    render(
      <VcsWidgetGithubRow
        data={makeData({
          kind: "historical",
          snapshotAt: new Date("2026-07-17T10:00:00Z"),
          snapshotCaptureFailed: true,
          github: makeGithub({ prNumber: 42, checkConclusion: "success" }),
        })}
      />
    );

    const link = screen.getByRole("link", { name: /PR #42/ });
    expect(link).toHaveAttribute("href", "https://github.com/example/repo/pull/42");
    expect(screen.getByText("CI: success")).toBeInTheDocument();
    expect(screen.getByText("Couldn't fully capture PR status at ship time")).toBeInTheDocument();
  });

  it("VcsWidgetGithubRow_should_StillRenderPrLinkText_When_ShowPrLinkPropOmitted", () => {
    // No `showPrLink` prop passed (existing call-site shape, e.g. VcsPanel.tsx,
    // UnfinishedItemDetail.tsx) → defaults true, PR link line still renders —
    // proves the D4 opt-out is additive, not a default-off breaking change
    // (Blocker A guard, Story 3.1.2 Task 3.1.2d).
    render(<VcsWidgetGithubRow data={makeData({ github: makeGithub({ prNumber: 42 }) })} />);
    expect(screen.getByRole("link", { name: /PR #42/ })).toBeInTheDocument();
  });

  it("omits only the PR link line when showPrLink={false}, keeping review/CI status", () => {
    render(
      <VcsWidgetGithubRow
        data={makeData({
          github: makeGithub({ prNumber: 42, approvedCount: 1, checkConclusion: "success" }),
        })}
        showPrLink={false}
      />
    );
    expect(screen.queryByRole("link", { name: /PR #42/ })).not.toBeInTheDocument();
    expect(screen.getByLabelText("1 approved")).toBeInTheDocument();
    expect(screen.getByText("CI: success")).toBeInTheDocument();
  });
});
