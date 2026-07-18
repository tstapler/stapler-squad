import type { VcsWidgetData, GithubSummary } from "./types";
import { deriveMergeabilityState } from "./mergeability";

function githubSummary(overrides: Partial<GithubSummary> = {}): GithubSummary {
  return {
    owner: "tstapler",
    repo: "stapler-squad",
    prUrl: "https://github.com/tstapler/stapler-squad/pull/1",
    prNumber: 1,
    prState: "open",
    isDraft: false,
    checkConclusion: "",
    approvedCount: 0,
    changesReqCount: 0,
    ...overrides,
  };
}

function liveData(overrides: Partial<VcsWidgetData> = {}): VcsWidgetData {
  return {
    kind: "live",
    branch: "feat/foo",
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

describe("deriveMergeabilityState", () => {
  it("deriveMergeabilityState_should_ReturnShipped_When_ShippedTrueAndGithubPrStateClosed", () => {
    const data = liveData({ shipped: true, github: githubSummary({ prState: "closed" }) });
    expect(deriveMergeabilityState(data)).toBe("shipped");
  });

  it("deriveMergeabilityState_should_ReturnNoPr_When_GithubIsNull", () => {
    const data = liveData({ github: null });
    expect(deriveMergeabilityState(data)).toBe("no_pr");
  });

  it("deriveMergeabilityState_should_ReturnDraft_When_GithubIsDraftTrue", () => {
    const data = liveData({ github: githubSummary({ isDraft: true }) });
    expect(deriveMergeabilityState(data)).toBe("draft");
  });

  it("deriveMergeabilityState_should_ReturnConflicted_When_FileChangesHasConflictSection", () => {
    const data = liveData({
      github: githubSummary(),
      fileChanges: [{ path: "src/foo.ts", status: "conflict", additions: 1, deletions: 1, section: "conflict" }],
    });
    expect(deriveMergeabilityState(data)).toBe("conflicted");
  });

  it("deriveMergeabilityState_should_ReturnChangesRequested_When_GithubChangesReqCountPositive", () => {
    const data = liveData({ github: githubSummary({ changesReqCount: 1 }) });
    expect(deriveMergeabilityState(data)).toBe("changes_requested");
  });

  it("deriveMergeabilityState_should_ReturnCiFailing_When_GithubCheckConclusionFailure", () => {
    const data = liveData({ github: githubSummary({ checkConclusion: "failure" }) });
    expect(deriveMergeabilityState(data)).toBe("ci_failing");
  });

  it("deriveMergeabilityState_should_ReturnClosedUnshipped_When_GithubPrStateClosedAndNotShipped", () => {
    const data = liveData({
      shipped: false,
      github: githubSummary({ prState: "closed", checkConclusion: "success" }),
    });
    expect(deriveMergeabilityState(data)).toBe("closed_unshipped");
  });

  it("deriveMergeabilityState_should_ReturnCiPending_When_CheckConclusionPendingOrEmpty", () => {
    const pending = liveData({ github: githubSummary({ checkConclusion: "pending" }) });
    const empty = liveData({ github: githubSummary({ checkConclusion: "" }) });
    expect(deriveMergeabilityState(pending)).toBe("ci_pending");
    expect(deriveMergeabilityState(empty)).toBe("ci_pending");
  });

  it("deriveMergeabilityState_should_ReturnReadyToMerge_When_GithubOpenAndChecksSucceeded", () => {
    const data = liveData({ github: githubSummary({ prState: "open", checkConclusion: "success" }) });
    expect(deriveMergeabilityState(data)).toBe("ready_to_merge");
  });

  it("deriveMergeabilityState_should_ReturnSnapshotUnavailable_When_HistoricalCaptureFailedBeforeNoPrOrCiPending", () => {
    const withNullGithub: VcsWidgetData = {
      kind: "historical",
      branch: "feat/foo",
      isClean: true,
      fileChanges: [],
      aheadOfMain: 0,
      behindMain: 0,
      branchExists: false,
      commits: [],
      github: null,
      shipped: false,
      snapshotAt: null,
      snapshotCaptureFailed: true,
    };
    const withPartialGithub: VcsWidgetData = {
      ...withNullGithub,
      github: githubSummary({ checkConclusion: "" }),
    };

    expect(deriveMergeabilityState(withNullGithub)).toBe("snapshot_unavailable");
    expect(deriveMergeabilityState(withPartialGithub)).toBe("snapshot_unavailable");
  });
});
