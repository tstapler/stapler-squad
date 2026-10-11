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
  /**
   * GitHub's rollup says CI failed but no failing check was itemised (beyond
   * the polled page, or a cancelled/action-required run). Counts toward
   * attention but is not nudgeable: the server builds the prompt from the same
   * itemised list and would answer "nothing to fix".
   */
  checksFailingUnlisted: boolean;
}

// Rollup states come from normalizeCheckState (github/user_pr_cache.go), which
// folds FAILURE and ERROR into "failure"; pending is deliberately not failing.
// Per-check failure is whatever the server itemised (mapFailingChecks), so the
// button never promises more than the nudge prompt will contain.
const ROLLUP_FAILING = "failure";

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
  const failingChecks = pr.failingChecks.length;
  const checksFailingUnlisted = failingChecks === 0 && pr.checkConclusion === ROLLUP_FAILING;
  const changesRequested = pr.changesReqCount > 0;
  const threadsUnknown = pr.unresolvedThreadCount === undefined;
  const unresolvedThreads = pr.unresolvedThreadCount ?? 0;
  const conflictUnknown = pr.hasMergeConflict === undefined;
  const mergeConflict = pr.hasMergeConflict === true;

  const nudgeable =
    !pr.isDraft && (failingChecks > 0 || unresolvedThreads > 0 || mergeConflict);
  const needsAttention = !pr.isDraft && (nudgeable || changesRequested || checksFailingUnlisted);

  return {
    failingChecks,
    changesRequested,
    unresolvedThreads,
    mergeConflict,
    threadsUnknown,
    conflictUnknown,
    needsAttention,
    nudgeable,
    checksFailingUnlisted,
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
