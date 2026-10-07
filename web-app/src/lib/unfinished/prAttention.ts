import type { UserPR } from "@/gen/session/v1/types_pb";

export interface PRAttention {
  failingChecks: number;
  changesRequested: boolean;
  /** 0 when unknown; see `threadsUnknown`. */
  unresolvedThreads: number;
  mergeConflict: boolean;
  /** Thread count not loaded (renders "? threads"); contributes nothing. */
  threadsUnknown: boolean;
  /** Conflict state not loaded (renders "? conflict"); contributes nothing. */
  conflictUnknown: boolean;
  /** Counts toward the tab badge. Drafts never do. */
  needsAttention: boolean;
  /** Shows the Nudge button. Changes-requested-only is visible but not nudgeable. */
  nudgeable: boolean;
}

// Pending/in-progress is deliberately absent: it is not failing.
const FAILING_CONCLUSIONS = new Set(["failure", "timed_out", "action_required"]);

type PRAttentionInput = Pick<
  UserPR,
  | "isDraft"
  | "checkConclusion"
  | "failingChecks"
  | "changesReqCount"
  | "unresolvedThreadCount"
  | "hasMergeConflict"
>;

export function prAttention(pr: PRAttentionInput): PRAttention {
  const conclusionFailing = FAILING_CONCLUSIONS.has(pr.checkConclusion);
  const failingChecks = Math.max(pr.failingChecks.length, conclusionFailing ? 1 : 0);
  const changesRequested = pr.changesReqCount > 0;
  const threadsUnknown = pr.unresolvedThreadCount === undefined;
  const unresolvedThreads = pr.unresolvedThreadCount ?? 0;
  const conflictUnknown = pr.hasMergeConflict === undefined;
  const mergeConflict = pr.hasMergeConflict === true;

  const nudgeable =
    !pr.isDraft && (failingChecks > 0 || unresolvedThreads > 0 || mergeConflict);
  const needsAttention = !pr.isDraft && (nudgeable || changesRequested);

  return {
    failingChecks,
    changesRequested,
    unresolvedThreads,
    mergeConflict,
    threadsUnknown,
    conflictUnknown,
    needsAttention,
    nudgeable,
  };
}

export const DEGRADED_ATTENTION_TEXT =
  "Review-thread counts are not loaded yet, so the real number may be higher.";

export interface AttentionSummary {
  count: number;
  /** Count is a lower bound because some PRs have no detail data yet. */
  degraded: boolean;
}

export function summarizeAttention(prs: readonly (PRAttentionInput & Pick<UserPR, "detailsLoaded">)[]): AttentionSummary {
  return {
    count: prs.filter((pr) => prAttention(pr).needsAttention).length,
    degraded: prs.some((pr) => pr.detailsLoaded === false),
  };
}
