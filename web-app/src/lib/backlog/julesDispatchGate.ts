import type { LinkedSession } from "@/lib/hooks/useBacklogService";

/**
 * Story 3.2.2's Jules gating result. `reason` is non-null exactly when
 * `disabled` is true and mirrors the precedence order below. `hidden` means
 * the feature itself is off (`GetJulesConfig.enabled === false`) — the
 * button renders nothing at all rather than a disabled affordance.
 */
export interface JulesDispatchGate {
  hidden: boolean;
  disabled: boolean;
  reason: string | null;
  /** The branch JulesDispatchDialog should open pre-filled with. */
  branch: string;
}

/** The subset of GetJulesConfig's response resolveJulesDispatchGate needs. */
export interface JulesConfigForGate {
  enabled: boolean;
  hasApiKey: boolean;
}

/**
 * Single source of truth for Story 3.2.2's "Dispatch to Jules" gating,
 * resolved entirely over GetJulesConfig + the item's already-loaded
 * sessions — no new RPC field. Called once from BacklogItemDetail.tsx and
 * passed down to the presentational ActionsSection, which does no gating
 * computation of its own (mirrors derivePlanReviewStatus's role for the
 * plan-review gate).
 *
 * Precedence order from ux.md §3.1 — only the first matching condition
 * applies, so e.g. "no key" and "no branch" both being true always reads as
 * "no key" alone, never both at once: feature off (hidden) -> no key ->
 * Jules session already open -> no known branch -> enabled. `branch` is the
 * newest non-empty worktreeBranch across the item's sessions, the same
 * value SessionsSection's per-row branch badge reads (§4.1) — sessions is
 * in creation order, so `.at(-1)` on the filtered list is the newest.
 */
export function resolveJulesDispatchGate(
  julesConfig: JulesConfigForGate | null,
  sessions: LinkedSession[],
): JulesDispatchGate | undefined {
  if (!julesConfig || !julesConfig.enabled) return { hidden: true, disabled: true, reason: null, branch: "" };

  const branch = sessions.filter((s) => !!s.worktreeBranch).at(-1)?.worktreeBranch ?? "";

  if (!julesConfig.hasApiKey) {
    return {
      hidden: false,
      disabled: true,
      reason: "Add a Jules API key in Settings to enable cloud sessions.",
      branch,
    };
  }

  const hasOpenJulesSession = sessions.some((s) => s.role === "jules_work" && !s.endedAt);
  if (hasOpenJulesSession) {
    return {
      hidden: false,
      disabled: true,
      reason: "A Jules session is already running for this item.",
      branch,
    };
  }

  if (!branch) {
    return {
      hidden: false,
      disabled: true,
      reason: "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules.",
      branch,
    };
  }

  return { hidden: false, disabled: false, reason: null, branch };
}
