import type { VcsWidgetData } from "./types";

export type MergeabilityState =
  | "shipped"
  | "snapshot_unavailable"
  | "no_pr"
  | "draft"
  | "conflicted"
  | "diverged"
  | "changes_requested"
  | "ci_failing"
  | "closed_unshipped"
  | "ci_pending"
  | "ready_to_merge";

// Precedence order below is deliberate: shipped status always wins over a
// stale "closed"/"merged" GitHub state (ADR-003 bug fix), and a durable
// snapshot capture failure is checked before every GitHub-signal branch so it
// can never be misread as "no PR ever existed" or "CI still running"
// (architecture-review BLOCKER fix).
export function deriveMergeabilityState(data: VcsWidgetData): MergeabilityState {
  if (data.shipped) return "shipped";
  if (data.kind === "historical" && data.snapshotCaptureFailed === true) {
    return "snapshot_unavailable";
  }
  if (!data.github) return "no_pr";
  if (data.github.isDraft) return "draft";
  if (data.fileChanges.some((f) => f.section === "conflict")) return "conflicted";
  // Mirrors deriveBlockingReasons's github_diverged check below: GitHub's own
  // mergeable computation catches base-branch divergence the local worktree
  // conflict check above can't see if it hasn't fetched/rebased. Without this
  // branch, a diverged-but-locally-clean PR fell through to "ci_pending" or
  // "ready_to_merge" — actively misleading, since the pill would claim the PR
  // is ready to merge when GitHub reports it can't be.
  if (data.github.mergeable === "conflicting") return "diverged";
  if (data.github.changesReqCount > 0) return "changes_requested";
  if (data.github.checkConclusion === "failure") return "ci_failing";
  if (data.github.prState === "closed") return "closed_unshipped";
  if (data.github.checkConclusion === "pending" || data.github.checkConclusion === "") {
    return "ci_pending";
  }
  return "ready_to_merge";
}

export type BlockingReasonKey =
  | "draft"
  | "conflicted"
  | "github_diverged"
  | "changes_requested"
  | "ci_failing"
  | "ci_pending"
  | "closed_unshipped";

export interface BlockingReason {
  key: BlockingReasonKey;
  label: string;
}

// Unlike deriveMergeabilityState (first-match precedence, for the compact-mode
// pill), this evaluates every predicate independently — a PR that is
// simultaneously draft + changes-requested + CI-failing surfaces all 3, not just
// the first (requirements.md scope item 4).
export function deriveBlockingReasons(data: VcsWidgetData): BlockingReason[] {
  if (data.shipped || !data.github) return [];
  const reasons: BlockingReason[] = [];
  if (data.github.isDraft) reasons.push({ key: "draft", label: "Draft" });
  if (data.fileChanges.some((f) => f.section === "conflict")) {
    reasons.push({ key: "conflicted", label: "Local merge conflicts" });
  }
  // Distinct from the local `conflicted` check above: GitHub's own mergeable
  // computation catches base-branch divergence the local worktree may not know
  // about if it hasn't fetched/rebased.
  if (data.github.mergeable === "conflicting") {
    reasons.push({ key: "github_diverged", label: "Diverged from base branch" });
  }
  if (data.github.changesReqCount > 0) {
    reasons.push({ key: "changes_requested", label: `Changes requested (${data.github.changesReqCount})` });
  }
  if (data.github.checkConclusion === "failure") reasons.push({ key: "ci_failing", label: "Checks failing" });
  if (data.github.prState === "closed") reasons.push({ key: "closed_unshipped", label: "Closed — not merged" });
  if (data.github.checkConclusion === "pending" || data.github.checkConclusion === "") {
    reasons.push({ key: "ci_pending", label: "Checks running" });
  }
  return reasons;
}
