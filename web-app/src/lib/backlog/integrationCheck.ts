import type { BacklogItem } from "@/lib/hooks/useBacklogService";

export type IntegrationCheckState = "awaiting" | "passed" | "failed" | "archived";

type IntegrationChild = Pick<BacklogItem, "status" | "acCriteria"> & Partial<Pick<BacklogItem, "statusEvents">>;

/** Maps an integration child to the epic-facing state, mirroring the backend integrationGate: passed is done, or archived out of done. statusEvents must be oldest-first. */
export function integrationCheckState(child: IntegrationChild | undefined): IntegrationCheckState {
  if (!child) return "awaiting";
  if (child.status === "done") return "passed";
  if (child.status === "archived") {
    const archival = [...(child.statusEvents ?? [])]
      .reverse()
      .find((e) => e.toStatus === "archived" && e.fromStatus !== "archived");
    return archival?.fromStatus === "done" ? "passed" : "archived";
  }
  if ((child.acCriteria ?? []).some((ac) => (ac.status as string) === "fail")) return "failed";
  return "awaiting";
}

export const INTEGRATION_CHECK_LABEL: Record<IntegrationCheckState, string> = {
  awaiting: "Awaiting Integration Check",
  passed: "Integration Passed",
  failed: "Integration Failed",
  archived: "Integration Check Archived",
};
