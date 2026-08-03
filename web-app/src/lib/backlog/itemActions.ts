// Single source of truth for "what actions does this backlog item's CURRENT
// state expose" — status + gate flags in, an action-id set out.
//
// Why this exists (docs/tasks/backlog-feature-improvement.md's 2026-08-03
// entry, item be676dab): action visibility in ActionsSection.tsx and
// TriageReviewPanel/PlanningSection used to be keyed off INCIDENTAL data that
// is supposed to correlate with the item's real state but isn't guaranteed
// to — `item.triageResult` truthy, `item.planArtifactsPath` truthy — instead
// of the actual state-machine gate condition
// (`!item.skipPlanning && !item.planApproved`, mirroring
// session/domain's ErrPlanRequired / DequeueNextQueuedItems' planning gate).
// When a triage session ran and produced nothing usable, `triageResult` and
// `planArtifactsPath` are BOTH empty even though the item is still very much
// gated on plan approval — the old conditions rendered nothing at all in
// that case, a silent dead end with no retry affordance. This module fixes
// that by deriving action availability from the gate condition directly, and
// makes the derivation exhaustive over every `KnownBacklogStatus` so a future
// new status is a compile error here (see the `never` check in the
// `default` arm) rather than a silently-missing branch.
import type { BacklogItem, KnownBacklogStatus } from "@/lib/hooks/useBacklogService";

export type BacklogActionId =
  | "mark_ready"
  | "trigger_triage"
  | "spawn_session"
  | "spawn_session_autonomous"
  | "approve_plan"
  | "retry_triage"
  | "view_session"
  | "restart_session"
  | "ship_pr"
  | "override_done"
  | "re_review"
  | "manual_review"
  | "archive"
  | "reopen"
  | "send_back_idea"
  | "send_back_ready"
  | "delete";

export interface ItemActionabilityInput
  extends Pick<
    BacklogItem,
    "status" | "skipPlanning" | "planApproved" | "planArtifactsPath" | "prUrl" | "linkedSessions"
  > {}

export interface AvailableActions {
  /**
   * Actions the current status + gate flags make available at all —
   * independent of transient per-click UI state (actionLoading in flight,
   * empty acCriteria, missing repoPath, etc.), which stays the caller's
   * concern. This set answers only "is this action meaningful right now."
   */
  actions: ReadonlySet<BacklogActionId>;
  /**
   * True when the item requires a plan to be approved before it can advance
   * — the authoritative check ("no SkipPlanning, no PlanApproved") both
   * `approve_plan` and `retry_triage` are derived from below, replacing the
   * old `planArtifactsPath`-presence proxy.
   */
  isGatedOnPlanApproval: boolean;
  /** True when a real plan exists to approve (`planArtifactsPath` is set). */
  hasPlan: boolean;
}

const KNOWN_STATUSES = new Set<KnownBacklogStatus>([
  "idea",
  "refining",
  "ready",
  "queued",
  "in_progress",
  "review",
  "pr_pending",
  "done",
  "archived",
]);

function asKnownStatus(status: string): KnownBacklogStatus | undefined {
  return KNOWN_STATUSES.has(status as KnownBacklogStatus) ? (status as KnownBacklogStatus) : undefined;
}

/** Statuses with an earlier stage to return to — mirrors the original
 * ActionsSection.tsx array, now sourced from one place instead of two
 * separately-maintained string arrays. */
const CAN_SEND_BACK_IDEA: ReadonlySet<KnownBacklogStatus> = new Set([
  "refining",
  "ready",
  "in_progress",
  "review",
  "pr_pending",
  "done",
]);
const CAN_SEND_BACK_READY: ReadonlySet<KnownBacklogStatus> = new Set([
  "in_progress",
  "review",
  "pr_pending",
  "done",
]);

/**
 * getAvailableActions is the single source of truth for which actions a
 * backlog item's current state exposes. Exhaustive over `KnownBacklogStatus`
 * via a switch with a `never` check in `default` — adding a new
 * `BacklogStatus` value without a case here is a compile error, not a silent
 * gap.
 */
export function getAvailableActions(item: ItemActionabilityInput): AvailableActions {
  const isGatedOnPlanApproval = !item.skipPlanning && !item.planApproved;
  const hasPlan = !!item.planArtifactsPath;
  const actions = new Set<BacklogActionId>();

  const status = asKnownStatus(item.status);

  switch (status) {
    case "idea":
      actions.add("mark_ready");
      actions.add("trigger_triage");
      break;
    case "refining":
      // No status-specific primary action today — only backward transitions
      // (added below) apply while refining.
      break;
    case "ready":
      actions.add("trigger_triage");
      actions.add("spawn_session");
      actions.add("spawn_session_autonomous");
      if (isGatedOnPlanApproval) {
        actions.add(hasPlan ? "approve_plan" : "retry_triage");
      }
      break;
    case "queued":
      if (isGatedOnPlanApproval) {
        actions.add(hasPlan ? "approve_plan" : "retry_triage");
      }
      break;
    case "in_progress":
      if (item.linkedSessions.length > 0) {
        actions.add("view_session");
        actions.add("restart_session");
      }
      break;
    case "review":
      if (!item.prUrl) actions.add("ship_pr");
      actions.add("override_done");
      actions.add("re_review");
      actions.add("manual_review");
      actions.add("restart_session");
      break;
    case "pr_pending":
      // No status-specific primary action today — only backward transitions.
      break;
    case "done":
      actions.add("archive");
      actions.add("reopen");
      break;
    case "archived":
      // Terminal — ActionsSection replaces everything with an informational
      // notice once terminalState is set, but an item can also simply BE
      // archived without that watch having fired; no actions apply either way.
      break;
    case undefined:
      // Unknown/forward-compatible status string — no actions assumed.
      break;
    default: {
      const _exhaustive: never = status;
      return _exhaustive;
    }
  }

  if (status && CAN_SEND_BACK_IDEA.has(status)) {
    actions.add("send_back_idea");
  }
  if (status && CAN_SEND_BACK_READY.has(status)) {
    actions.add("send_back_ready");
  }

  actions.add("delete");

  return { actions, isGatedOnPlanApproval, hasPlan };
}
