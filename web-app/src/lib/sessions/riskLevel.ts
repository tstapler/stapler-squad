// Mirrors classifier.RiskLevel (pkg/classifier/classifier.go). Keep in sync by hand — there is
// no codegen bridge for this Go string-constant set today, matching escalationCategory.ts's
// existing convention.
export type RiskLevel = "low" | "medium" | "high" | "critical";

// riskLevelRank orders RiskLevel strings by severity, highest first. Unrecorded ("") ranks
// alongside "high" — fail-safe, since an unclassified request must never sort as if it were
// safe. Mirrors session/review_queue_poller.go's riskLevelRank (Go side).
const RISK_LEVEL_RANK: Record<string, number> = {
  critical: 4,
  high: 3,
  "": 3,
  medium: 2,
  low: 1,
};

export function riskLevelRank(riskLevel: string): number {
  return RISK_LEVEL_RANK[riskLevel] ?? RISK_LEVEL_RANK[""];
}
