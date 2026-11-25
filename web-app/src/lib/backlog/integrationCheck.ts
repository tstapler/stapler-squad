import type { BacklogItem } from "@/lib/hooks/useBacklogService";

export type IntegrationCheckState = "awaiting" | "passed" | "failed";

/** Maps an integration child's status to the epic-facing state; only done passes, an archived child never ran the check. */
export function integrationCheckState(child: Pick<BacklogItem, "status" | "acCriteria"> | undefined): IntegrationCheckState {
  if (!child) return "awaiting";
  if (child.status === "done") return "passed";
  if (child.status === "archived") return "failed";
  if ((child.acCriteria ?? []).some((ac) => (ac.status as string) === "fail")) return "failed";
  return "awaiting";
}

export const INTEGRATION_CHECK_LABEL: Record<IntegrationCheckState, string> = {
  awaiting: "Awaiting Integration Check",
  passed: "Integration Passed",
  failed: "Integration Failed",
};
