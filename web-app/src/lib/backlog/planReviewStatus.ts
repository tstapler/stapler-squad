import type { BacklogItem } from "@/lib/hooks/useBacklogService";

export type PlanReviewStatus =
  | "no_plan"
  | "pending_review"
  | "approved"
  | "changes_requested"
  | "skipped";

/**
 * Single source of truth for the 5-state plan-review status — never
 * persisted server-side, always derived. See ADR-001
 * (project_plans/plan-approval-ux/decisions/).
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
