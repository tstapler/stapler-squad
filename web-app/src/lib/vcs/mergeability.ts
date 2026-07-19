import type { VcsWidgetData } from "./types";

export type MergeabilityState =
  | "shipped"
  | "snapshot_unavailable"
  | "no_pr"
  | "draft"
  | "conflicted"
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
  if (data.github.changesReqCount > 0) return "changes_requested";
  if (data.github.checkConclusion === "failure") return "ci_failing";
  if (data.github.prState === "closed") return "closed_unshipped";
  if (data.github.checkConclusion === "pending" || data.github.checkConclusion === "") {
    return "ci_pending";
  }
  return "ready_to_merge";
}
