import { create } from "@bufbuild/protobuf";
import {
  VCSStatusSchema,
  FileChangeSchema,
  SessionSchema,
  UnfinishedWorktreeSchema,
  FileStatus,
} from "@/gen/session/v1/types_pb";
import { BacklogItemShipStatusSchema, ShippedCommitSchema, ShippedFileStatSchema } from "@/gen/session/v1/backlog_pb";
import { fromSessionVcs, fromShipStatus, fromUnfinishedWorktree, toPrState, toCheckConclusion } from "./adapters";
import { deriveMergeabilityState } from "./mergeability";

describe("toPrState", () => {
  it("toPrState_should_PassThroughRecognizedValues_When_RawIsOpenClosedOrMerged", () => {
    expect(toPrState("open")).toBe("open");
    expect(toPrState("closed")).toBe("closed");
    expect(toPrState("merged")).toBe("merged");
  });

  it("toPrState_should_DefaultToOpen_When_RawUnrecognized", () => {
    expect(toPrState("")).toBe("open");
    expect(toPrState("abandoned")).toBe("open");
  });
});

describe("toCheckConclusion", () => {
  it("toCheckConclusion_should_PassThroughRecognizedValues_When_RawIsSuccessFailureOrPending", () => {
    expect(toCheckConclusion("success")).toBe("success");
    expect(toCheckConclusion("failure")).toBe("failure");
    expect(toCheckConclusion("pending")).toBe("pending");
  });

  it("toCheckConclusion_should_DefaultToEmpty_When_RawUnrecognized", () => {
    expect(toCheckConclusion("action_required")).toBe("");
    expect(toCheckConclusion("")).toBe("");
  });
});

describe("fromSessionVcs", () => {
  it("fromSessionVcs_should_MapConflictFileToConflictSection_When_StatusHasConflictFiles", () => {
    const status = create(VCSStatusSchema, {
      branch: "feat/vcs-widget",
      isClean: false,
      conflictFiles: [
        create(FileChangeSchema, { path: "src/foo.ts", status: FileStatus.CONFLICT, additions: 3, deletions: 1 }),
      ],
    });
    const session = create(SessionSchema, {
      githubOwner: "tstapler",
      githubRepo: "stapler-squad",
      githubPrNumber: 42,
      githubCheckConclusion: "success",
      githubApprovedCount: 1,
      githubChangesReqCount: 0,
    });

    const result = fromSessionVcs(status, session);

    expect(result.kind).toBe("live");
    expect("snapshotAt" in result).toBe(false);
    expect(result.fileChanges).toEqual([
      { path: "src/foo.ts", status: "conflict", additions: 3, deletions: 1, section: "conflict" },
    ]);
    expect(result.github).toEqual({
      owner: "tstapler",
      repo: "stapler-squad",
      prUrl: "",
      prNumber: 42,
      prState: "open",
      isDraft: false,
      checkConclusion: "success",
      approvedCount: 1,
      changesReqCount: 0,
    });
  });

  it("fromSessionVcs_should_ReturnNullGithub_When_SessionOmittedOrGithubOwnerEmpty", () => {
    const status = create(VCSStatusSchema, { branch: "feat/foo" });

    expect(fromSessionVcs(status).github).toBeNull();

    const sessionNoOwner = create(SessionSchema, { githubOwner: "" });
    expect(fromSessionVcs(status, sessionNoOwner).github).toBeNull();
  });
});

describe("fromShipStatus", () => {
  it("fromShipStatus_should_MapCommitsAndSnapshotAt_When_StatusPopulated", () => {
    const authoredAt = { seconds: BigInt(Math.floor(new Date("2026-07-15").getTime() / 1000)), nanos: 0 };
    const status = create(BacklogItemShipStatusSchema, {
      shipped: true,
      shippedVia: "pr",
      branchExists: false,
      commits: [
        create(ShippedCommitSchema, {
          sha: "a1b2c3d",
          summary: "fix: widget bug",
          authorName: "Tyler Stapler",
          authoredAt,
        }),
      ],
      lastCommitAt: authoredAt,
    });

    const result = fromShipStatus(status);

    expect(result.kind).toBe("historical");
    expect(result.branchExists).toBe(false);
    expect(result.commits).toEqual([
      {
        sha: "a1b2c3d",
        summary: "fix: widget bug",
        authorName: "Tyler Stapler",
        authoredAt: new Date("2026-07-15"),
      },
    ]);
    expect(result.kind === "historical" && result.snapshotAt).toEqual(new Date("2026-07-15"));
  });

  it("fromShipStatus_should_SetLoadErrorNotThrow_When_StatusErrorNonEmpty", () => {
    const status = create(BacklogItemShipStatusSchema, {
      error: "no work session ever committed code for this item",
    });

    const result = fromShipStatus(status);

    expect(result.loadError).toBe("no work session ever committed code for this item");
    expect(result.fileChanges).toEqual([]);
    expect(result.commits).toEqual([]);
  });

  it("fromShipStatus_should_MapGithubAndFileStatsFromSnapshot_When_SnapshotAtPopulated", () => {
    const snapshotAt = { seconds: BigInt(Math.floor(new Date("2026-07-17T10:00:00Z").getTime() / 1000)), nanos: 0 };
    const status = create(BacklogItemShipStatusSchema, {
      prUrl: "https://github.com/tstapler/stapler-squad/pull/42",
      shippedCheckConclusion: "success",
      shippedApprovedCount: 2,
      shippedChangesReqCount: 0,
      fileStats: [
        create(ShippedFileStatSchema, { path: "src/foo.ts", status: FileStatus.MODIFIED, additions: 5, deletions: 2 }),
      ],
      snapshotAt,
      snapshotCaptureFailed: false,
    });

    const result = fromShipStatus(status);

    expect(result.github).toEqual({
      owner: "tstapler",
      repo: "stapler-squad",
      prUrl: "https://github.com/tstapler/stapler-squad/pull/42",
      prNumber: 42,
      prState: "merged",
      isDraft: false,
      checkConclusion: "success",
      approvedCount: 2,
      changesReqCount: 0,
    });
    expect(result.fileChanges).toEqual([
      { path: "src/foo.ts", status: "modified", additions: 5, deletions: 2, section: "unstaged" },
    ]);
    expect(result.kind === "historical" && result.snapshotAt).toEqual(new Date("2026-07-17T10:00:00Z"));
    expect(result.kind === "historical" && result.snapshotCaptureFailed).toBe(false);
  });

  it("fromShipStatus_should_PreservePartiallySuccessfulGithubGroup_When_SnapshotCaptureFailedTrueAndFileStatsEmpty", () => {
    const snapshotAt = { seconds: BigInt(Math.floor(new Date("2026-07-17T10:00:00Z").getTime() / 1000)), nanos: 0 };
    const status = create(BacklogItemShipStatusSchema, {
      prUrl: "https://github.com/tstapler/stapler-squad/pull/42",
      shippedCheckConclusion: "success",
      shippedApprovedCount: 2,
      shippedChangesReqCount: 0,
      fileStats: [],
      snapshotAt,
      snapshotCaptureFailed: true,
    });

    const result = fromShipStatus(status);

    expect(result.kind === "historical" && result.snapshotCaptureFailed).toBe(true);
    expect(result.github).toEqual({
      owner: "tstapler",
      repo: "stapler-squad",
      prUrl: "https://github.com/tstapler/stapler-squad/pull/42",
      prNumber: 42,
      prState: "merged",
      isDraft: false,
      checkConclusion: "success",
      approvedCount: 2,
      changesReqCount: 0,
    });
    expect(result.fileChanges).toEqual([]);
    expect(deriveMergeabilityState(result)).toBe("snapshot_unavailable");
  });

  it("fromShipStatus_should_PreserveZeroValueBehavior_When_SnapshotAtUnset", () => {
    const status = create(BacklogItemShipStatusSchema, {});

    const result = fromShipStatus(status);

    expect(result.github).toBeNull();
    expect(result.kind === "historical" && result.snapshotAt).toBeNull();
  });

  it("fromShipStatus_should_SynthesizeSingleCommitFromLastCommitFields_When_CommitsArrayEmpty", () => {
    const status = create(BacklogItemShipStatusSchema, {
      commits: [],
      lastCommitSha: "d4e5f6a",
      lastCommitMessage: "fix: legacy single-commit fallback",
    });

    const result = fromShipStatus(status);

    expect(result.commits).toEqual([
      { sha: "d4e5f6a", summary: "fix: legacy single-commit fallback" },
    ]);
  });

  it("fromShipStatus_should_MapEmptyCommits_When_CommitsAndLastCommitShaBothEmpty", () => {
    const status = create(BacklogItemShipStatusSchema, { commits: [], lastCommitSha: "" });

    const result = fromShipStatus(status);

    expect(result.commits).toEqual([]);
  });
});

describe("fromUnfinishedWorktree", () => {
  it("fromUnfinishedWorktree_should_PopulateGroupedAggregateStatsAndCommits_When_WorktreeHasChanges", () => {
    const wt = create(UnfinishedWorktreeSchema, {
      changedFiles: 5,
      linesAdded: 42,
      linesRemoved: 8,
      aheadCommitMessages: ["fix: typo", "feat: add widget"],
      githubPrNumber: 7,
      githubPrUrl: "https://github.com/tstapler/stapler-squad/pull/7",
      githubPrState: "open",
    });

    const result = fromUnfinishedWorktree(wt);

    expect(result.kind).toBe("live");
    expect(result.aggregateStats).toEqual({ filesChanged: 5, additions: 42, deletions: 8 });
    expect(result.fileChanges).toEqual([]);
    expect(result.commits).toEqual([
      { sha: "", summary: "fix: typo" },
      { sha: "", summary: "feat: add widget" },
    ]);
    expect(result.github).toEqual({
      owner: "tstapler",
      repo: "stapler-squad",
      prUrl: "https://github.com/tstapler/stapler-squad/pull/7",
      prNumber: 7,
      prState: "open",
      isDraft: false,
      checkConclusion: "",
      approvedCount: 0,
      changesReqCount: 0,
    });
  });

  it("fromUnfinishedWorktree_should_ReturnNullGithub_When_PrUrlUnparseable", () => {
    const malformedUrlWithPr = create(UnfinishedWorktreeSchema, {
      githubPrNumber: 3,
      githubPrUrl: "not-a-valid-url",
      githubPrState: "open",
    });
    const malformed = fromUnfinishedWorktree(malformedUrlWithPr);
    expect(malformed.github).toEqual({
      owner: "",
      repo: "",
      prUrl: "not-a-valid-url",
      prNumber: 3,
      prState: "open",
      isDraft: false,
      checkConclusion: "",
      approvedCount: 0,
      changesReqCount: 0,
    });

    const noPr = create(UnfinishedWorktreeSchema, { githubPrUrl: "", githubPrState: "" });
    expect(fromUnfinishedWorktree(noPr).github).toBeNull();
  });
});
