import type { BacklogItem } from "@/lib/hooks/useBacklogService";

export type PlanReviewStatus =
  | "no_plan"
  | "pending_review"
  | "approved"
  | "changes_requested"
  | "skipped";

/**
 * Single source of truth for the 5-state plan-review status — never
 * persisted server-side, always derived. Both `PlanVerdictBox` (status card)
 * and `ActionsSection` (spawn-gate button visibility) call this rather than
 * each re-deriving state from raw fields, avoiding drift between the two
 * surfaces. See project_plans/plan-approval-ux/decisions/ADR-001.
 *
 * Precedence matters: `skipped` is checked first (a categorically different
 * meaning than "no plan yet"), then `changes_requested` — a non-empty
 * `planRejectionReason` wins even if `planApproved` is also (defensively,
 * should-never-happen) true, since the two are supposed to be mutually
 * exclusive post-symmetry-fix but the derivation must still resolve
 * deterministically if they ever coexist.
 */
export function derivePlanReviewStatus(
  item: Pick<BacklogItem, "skipPlanning" | "planApproved" | "planArtifactsPath" | "planRejectionReason">,
): PlanReviewStatus {
  if (item.skipPlanning) return "skipped";
  if (item.planRejectionReason) return "changes_requested";
  if (item.planApproved) return "approved";
  if (item.planArtifactsPath) return "pending_review";
  return "no_plan";
}
