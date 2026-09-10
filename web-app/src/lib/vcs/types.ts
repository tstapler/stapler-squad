export type FileChangeSection = "conflict" | "staged" | "unstaged" | "untracked";

export type FileChangeStatus =
  | "modified"
  | "added"
  | "deleted"
  | "renamed"
  | "copied"
  | "untracked"
  | "ignored"
  | "conflict"
  | "unknown";

export interface FileChangeSummary {
  path: string;
  oldPath?: string;
  status: FileChangeStatus;
  additions: number;
  deletions: number;
  section: FileChangeSection;
}

export interface CommitSummary {
  sha: string;
  summary: string;
  authorName?: string;
  authoredAt?: Date;
}

export interface CheckItemSummary {
  name: string;
  context: string;
  state: string;
  status: string;
  conclusion: string;
}

export interface ReviewFeedbackSummary {
  author: string;
  state: string;
  body: string;
}

export type PrState = "open" | "closed" | "merged";
export type CheckConclusion = "success" | "failure" | "pending" | "";

export interface GithubSummary {
  owner: string;
  repo: string;
  prUrl: string;
  prNumber: number;
  prState: PrState;
  isDraft: boolean;
  checkConclusion: CheckConclusion;
  approvedCount: number;
  changesReqCount: number;
  mergeable: string;
  checks: CheckItemSummary[];
  reviewFeedback: ReviewFeedbackSummary[];
  lastCheckedAt?: Date;
}

export type VcsWidgetMode = "full" | "compact";

interface VcsWidgetDataCommon {
  branch: string;
  isClean: boolean;
  fileChanges: FileChangeSummary[];
  aheadOfMain: number;
  behindMain: number;
  branchExists: boolean;
  commits: CommitSummary[];
  github: GithubSummary | null;
  shipped: boolean;
  /**
   * Set when the underlying source failed to load (e.g. "no work session ever
   * committed code for this item") — distinguishes a benign empty state from a
   * hard error so VcsWidget can render dedicated error copy instead of a blank
   * widget.
   */
  loadError?: string;
  /**
   * Populated only by fromUnfinishedWorktree (and any future source with
   * aggregate-only stats, no per-file breakdown) — grouped per the
   * "no field explosion" design goal, replacing 3 separate top-level
   * optionals.
   */
  aggregateStats?: {
    filesChanged: number;
    additions: number;
    deletions: number;
  };
  /** Local-git-status freshness — independent of github.lastCheckedAt (PR staleness). */
  statusAsOf?: Date;
  /** True when `commits` was capped server-side before reaching the branch's full count. */
  commitsTruncated?: boolean;
  /**
   * True when the backend attempted to fetch the commit list and it failed or timed
   * out — distinct from `commits` simply being empty because the branch has no
   * unshipped commits yet. Lets the UI render "couldn't load commits" instead of a
   * silent empty state.
   */
  commitsUnavailable?: boolean;
}

export type VcsWidgetData =
  | (VcsWidgetDataCommon & { kind: "live" })
  | (VcsWidgetDataCommon & {
      kind: "historical";
      snapshotAt: Date | null;
      snapshotCaptureFailed?: boolean;
    });
