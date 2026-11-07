import type { AcCriterionStatus } from "@/lib/hooks/useBacklogService";

/** A criterion counts toward the done total when the agent finished it or the owner waived it. */
export const isAcSatisfied = (c: { status: AcCriterionStatus }): boolean => c.status === "done" || c.status === "waived";
