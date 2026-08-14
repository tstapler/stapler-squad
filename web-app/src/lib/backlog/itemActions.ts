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
  | "unarchive"
  | "reopen"
  | "send_back_idea"
  | "send_back_ready"
  | "delete";

export interface ItemActionabilityInput
  extends Pick<
    BacklogItem,
    | "status"
    | "skipPlanning"
    | "planApproved"
    | "planArtifactsPath"
    | "prUrl"
    | "linkedSessions"
    | "triageStatus"
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

// KNOWN_STATUS_MEMBERSHIP, not a hand-typed Set<KnownBacklogStatus>, is what makes
// asKnownStatus's runtime routing gate stay in sync with the KnownBacklogStatus union at
// COMPILE time. A plain `new Set<KnownBacklogStatus>([...])` looks type-safe but isn't: TS
// only checks that each listed literal belongs to the union, never that every union member
// is listed — add a 10th status to KnownBacklogStatus (useBacklogService.ts) without adding
// it here, and the switch below still forces you to add a `case` for it (its own `never`
// check catches that), but nothing forces this object to grow too. With that desync,
// asKnownStatus silently treats real items in the new status as "unknown" (status: undefined
// below), routing them to the delete-only branch instead of the case you just wrote —
// `tsc --strict` exits 0 the whole time. The exhaustiveness guarantee only ever covered the
// switch, not the gate that lets values reach it. `satisfies Record<KnownBacklogStatus, true>`
// closes that gap: TS errors here on both a missing key (a new status not yet added) and an
// extra key (a status removed from the union), so the object's key set can never silently
// drift from KnownBacklogStatus.
// Exported so itemActions.test.ts can assert its key count against a literal
// reference list — a cheap runtime tripwire on top of the `satisfies` check
// above (which is the actual compile-time guarantee; this just makes the
// invariant visible in test output too).
export const KNOWN_STATUS_MEMBERSHIP = {
  idea: true,
  refining: true,
  ready: true,
  queued: true,
  in_progress: true,
  review: true,
  pr_pending: true,
  done: true,
  archived: true,
} satisfies Record<KnownBacklogStatus, true>;

function asKnownStatus(status: string): KnownBacklogStatus | undefined {
  return Object.prototype.hasOwnProperty.call(KNOWN_STATUS_MEMBERSHIP, status)
    ? (status as KnownBacklogStatus)
    : undefined;
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
        if (hasPlan) {
          actions.add("approve_plan");
        } else if (item.triageStatus === "failed") {
          // A ready item can also be reached via "Mark Ready" straight from idea,
          // with no plan and no triage session ever having run (triageStatus
          // undefined) — showing "Retry Triage" there would duplicate the
          // "Trigger Triage" button above for a case with nothing to retry. Only
          // surface it when there's actual evidence a triage attempt happened
          // and produced nothing usable, matching the triage-failed banner's own
          // condition in BacklogItemDetail.tsx.
          actions.add("retry_triage");
        }
      }
      break;
    case "queued":
      if (isGatedOnPlanApproval) {
        if (hasPlan) {
          actions.add("approve_plan");
        } else if (item.triageStatus === "failed") {
          actions.add("retry_triage");
        }
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
      // ActionsSection replaces everything with an informational notice once
      // terminalState fires from a *live* archive event, but an item can also
      // simply BE archived on first load (e.g. opened directly from a "Show
      // Archived" list) without that watch ever having fired. Exposing
      // "unarchive" here — checked in ActionsSection's non-terminal branch —
      // covers that case; the terminal branch has its own Unarchive button
      // for the live-event path.
      actions.add("unarchive");
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
