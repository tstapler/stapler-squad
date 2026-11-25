import type { BacklogItem } from "@/lib/hooks/useBacklogService";

export type IntegrationCheckState = "awaiting" | "passed" | "failed";

/** Maps an integration child's status to the epic-facing state; archived counts as resolved (the operator escape hatch). */
export function integrationCheckState(child: Pick<BacklogItem, "status" | "acCriteria"> | undefined): IntegrationCheckState {
  if (!child) return "awaiting";
  if (child.status === "done" || child.status === "archived") return "passed";
  if ((child.acCriteria ?? []).some((ac) => (ac.status as string) === "fail")) return "failed";
  return "awaiting";
}

export const INTEGRATION_CHECK_LABEL: Record<IntegrationCheckState, string> = {
  awaiting: "Awaiting Integration Check",
  passed: "Integration Passed",
  failed: "Integration Failed",
};
