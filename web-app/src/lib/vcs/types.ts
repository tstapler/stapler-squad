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
}

export type VcsWidgetData =
  | (VcsWidgetDataCommon & { kind: "live" })
  | (VcsWidgetDataCommon & {
      kind: "historical";
      snapshotAt: Date | null;
      snapshotCaptureFailed?: boolean;
    });
