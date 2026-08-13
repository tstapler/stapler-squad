import { FileStatus } from "@/gen/session/v1/types_pb";
import type { VCSStatus, FileChange, Session, UnfinishedWorktree } from "@/gen/session/v1/types_pb";
import type { BacklogItemShipStatus, ShippedCommit, ShippedFileStat } from "@/gen/session/v1/backlog_pb";
import type {
  VcsWidgetData,
  FileChangeSummary,
  FileChangeStatus,
  CommitSummary,
  GithubSummary,
  PrState,
  CheckConclusion,
} from "./types";

// toPrState/toCheckConclusion are the only places a raw GitHub string (an
// unconstrained proto `string` field) may be narrowed to PrState/CheckConclusion.
// Every adapter below must call through them — never assign a raw field directly.
export function toPrState(raw: string): PrState {
  return raw === "open" || raw === "closed" || raw === "merged" ? raw : "open";
}

export function toCheckConclusion(raw: string): CheckConclusion {
  return raw === "success" || raw === "failure" || raw === "pending" ? raw : "";
}

function mapFileStatus(status: FileStatus): FileChangeStatus {
  switch (status) {
    case FileStatus.MODIFIED:
      return "modified";
    case FileStatus.ADDED:
      return "added";
    case FileStatus.DELETED:
      return "deleted";
    case FileStatus.RENAMED:
      return "renamed";
    case FileStatus.COPIED:
      return "copied";
    case FileStatus.UNTRACKED:
      return "untracked";
    case FileStatus.IGNORED:
      return "ignored";
    case FileStatus.CONFLICT:
      return "conflict";
    default:
      return "unknown";
  }
}

function toFileChangeSummary(file: FileChange, section: FileChangeSummary["section"]): FileChangeSummary {
  return {
    path: file.path,
    oldPath: file.oldPath || undefined,
    status: mapFileStatus(file.status),
    additions: file.additions,
    deletions: file.deletions,
    section,
  };
}

function flattenFileChanges(status: VCSStatus): FileChangeSummary[] {
  return [
    ...status.conflictFiles.map((f) => toFileChangeSummary(f, "conflict")),
    ...status.stagedFiles.map((f) => toFileChangeSummary(f, "staged")),
    ...status.unstagedFiles.map((f) => toFileChangeSummary(f, "unstaged")),
    ...status.untrackedFiles.map((f) => toFileChangeSummary(f, "untracked")),
  ];
}

function fromSessionGithub(session?: Session): GithubSummary | null {
  if (!session || !session.githubOwner) return null;
  return {
    owner: session.githubOwner,
    repo: session.githubRepo,
    prUrl: session.githubPrUrl,
    prNumber: session.githubPrNumber,
    prState: toPrState(session.githubPrState),
    isDraft: session.githubPrIsDraft,
    checkConclusion: toCheckConclusion(session.githubCheckConclusion),
    approvedCount: session.githubApprovedCount,
    changesReqCount: session.githubChangesReqCount,
  };
}

export function fromSessionVcs(status: VCSStatus, session?: Session): VcsWidgetData {
  return {
    kind: "live",
    branch: status.branch,
    isClean: status.isClean,
    fileChanges: flattenFileChanges(status),
    aheadOfMain: status.aheadBy,
    behindMain: status.behindBy,
    branchExists: true,
    commits: [],
    github: fromSessionGithub(session),
    shipped: false,
  };
}

function toDate(ts?: { seconds: bigint }): Date | null {
  return ts ? new Date(Number(ts.seconds) * 1000) : null;
}

function toCommitSummary(commit: ShippedCommit): CommitSummary {
  return {
    sha: commit.sha,
    summary: commit.summary,
    authorName: commit.authorName || undefined,
    authoredAt: toDate(commit.authoredAt) ?? undefined,
  };
}

// GitHub PR URLs look like https://github.com/{owner}/{repo}/pull/{n}. Some
// callers already have prNumber from a dedicated proto field (UnfinishedWorktree)
// and only need owner/repo parsed out; BacklogItemShipStatus has no dedicated
// prNumber field, so this also extracts it from the URL for that caller.
function parseGithubUrl(url: string): { owner: string; repo: string; prNumber: number } {
  const match = url.match(/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
  return match
    ? { owner: match[1], repo: match[2], prNumber: Number(match[3]) }
    : { owner: "", repo: "", prNumber: 0 };
}

function toShipFileChangeSummary(file: ShippedFileStat): FileChangeSummary {
  return {
    path: file.path,
    status: mapFileStatus(file.status),
    additions: file.additions,
    deletions: file.deletions,
    // ShippedFileStat has no staged/unstaged/conflict distinction — historical
    // snapshots only record "changed", so default every entry to "unstaged"
    // (documented known simplification, Task 4.1.1b).
    section: "unstaged",
  };
}

function fromShipStatusGithub(status: BacklogItemShipStatus): GithubSummary | null {
  if (!status.prUrl) return null;
  const { owner, repo, prNumber } = parseGithubUrl(status.prUrl);
  return {
    owner,
    repo,
    prUrl: status.prUrl,
    prNumber,
    // Historical snapshots only carry CI/review data, not a raw PR state
    // string — a captured snapshot belongs to a shipped item, so "merged" is
    // the only state consistent with that lifecycle.
    prState: "merged",
    isDraft: false,
    checkConclusion: toCheckConclusion(status.shippedCheckConclusion),
    approvedCount: status.shippedApprovedCount,
    changesReqCount: status.shippedChangesReqCount,
  };
}

// Older/simpler ship-status data only ever recorded the last commit, not a
// full commit list — fall back to synthesizing a single CommitSummary from
// those fields when commits is empty (legacy ShipStatusDisplay parity).
function commitsOrLastCommitFallback(status: BacklogItemShipStatus): CommitSummary[] {
  if (status.commits.length > 0) return status.commits.map(toCommitSummary);
  if (!status.lastCommitSha) return [];
  return [{ sha: status.lastCommitSha, summary: status.lastCommitMessage }];
}

export function fromShipStatus(status: BacklogItemShipStatus): VcsWidgetData {
  const hasSnapshot = status.snapshotAt != null;
  return {
    kind: "historical",
    branch: status.branchName,
    isClean: true,
    fileChanges: hasSnapshot ? status.fileStats.map(toShipFileChangeSummary) : [],
    aheadOfMain: status.aheadOfMain,
    behindMain: status.behindMain,
    branchExists: status.branchExists,
    commits: commitsOrLastCommitFallback(status),
    github: hasSnapshot ? fromShipStatusGithub(status) : null,
    shipped: status.shipped,
    loadError: status.error || undefined,
    snapshotAt: hasSnapshot ? toDate(status.snapshotAt) : toDate(status.lastCommitAt),
    // Mapped independently of hasSnapshot: when both capture groups fail,
    // status.snapshotAt can stay nil while snapshotCaptureFailed is still
    // true (Story 3.3.1) — VcsWidgetGithubRow's failure-copy branch depends
    // on this field being true in exactly that case.
    snapshotCaptureFailed: status.snapshotCaptureFailed,
  };
}

function fromUnfinishedWorktreeGithub(wt: UnfinishedWorktree): GithubSummary | null {
  if (!wt.githubPrNumber) return null;
  const { owner, repo } = parseGithubUrl(wt.githubPrUrl);
  return {
    owner,
    repo,
    prUrl: wt.githubPrUrl,
    prNumber: wt.githubPrNumber,
    prState: toPrState(wt.githubPrState),
    isDraft: false,
    checkConclusion: "",
    approvedCount: 0,
    changesReqCount: 0,
  };
}

export function fromUnfinishedWorktree(wt: UnfinishedWorktree): VcsWidgetData {
  return {
    kind: "live",
    branch: wt.branch,
    isClean: !wt.hasUncommitted,
    fileChanges: [],
    aheadOfMain: wt.commitsAhead,
    behindMain: wt.commitsBehind,
    branchExists: true,
    commits: wt.aheadCommitMessages.map((summary) => ({ sha: "", summary })),
    github: fromUnfinishedWorktreeGithub(wt),
    shipped: false,
    aggregateStats: {
      filesChanged: wt.changedFiles,
      additions: wt.linesAdded,
      deletions: wt.linesRemoved,
    },
  };
}
