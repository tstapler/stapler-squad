import { PollError, type SessionPullRequest } from "@/gen/session/v1/types_pb";
import { prRowState, prTarget, type PrRowState } from "./pullRequests";

/** Everything GitHubBadge needs to draw one PR chip: text always visible, shape from the state. */
export interface PrChipView {
  state: PrRowState;
  label: string;
  ariaLabel: string;
  title: string;
  stale: boolean;
}

const LABEL: Record<PrRowState, string> = {
  open: "Open",
  draft: "Draft",
  merged: "Merged",
  closed: "Closed",
  unknown: "Checking",
  unverified: "Unverified",
};

const STATUS_WORDS: Record<PrRowState, string> = {
  open: "open",
  draft: "draft",
  merged: "merged",
  closed: "closed",
  unknown: "checking",
  unverified: "closed or merged (unverified)",
};

function detail(pr: SessionPullRequest, state: PrRowState): string[] {
  const parts = [`GitHub PR ${prTarget(pr)}`, state === "unverified" ? "Closed or merged (unverified)" : `State: ${STATUS_WORDS[state]}`];
  if (pr.approvedCount > 0) parts.push(`${pr.approvedCount} approval${pr.approvedCount > 1 ? "s" : ""}`);
  if (pr.changesRequestedCount > 0) parts.push(`${pr.changesRequestedCount} change request${pr.changesRequestedCount > 1 ? "s" : ""}`);
  if (pr.checkConclusion) parts.push(`CI: ${pr.checkConclusion}`);
  return parts;
}

export function chipView(pr: SessionPullRequest): PrChipView {
  const state = prRowState(pr);
  const stale = pr.pollError !== PollError.NONE;
  const status = stale ? `${STATUS_WORDS[state]}, may be out of date` : STATUS_WORDS[state];
  return {
    state,
    label: LABEL[state],
    stale,
    ariaLabel: `View GitHub Pull Request #${pr.number}, ${pr.owner}/${pr.repo}, status: ${status}, opens in new tab`,
    title: [...detail(pr, state), ...(stale ? ["Status may be out of date"] : [])].join(" · "),
  };
}
