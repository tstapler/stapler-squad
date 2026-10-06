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

// Single source for the 4 base human-readable labels — consumed by both SeverityBadge.tsx
// (which layers its own "Severity not recorded" fallback on top for "") and
// ApprovalAnalyticsPanel.tsx (which falls back to the raw string for forward-compat with an
// unrecognized future level). Keep the fallback logic at each call site, not here.
export const RISK_LEVEL_LABELS: Record<RiskLevel, string> = {
  critical: "Critical",
  high: "High",
  medium: "Medium",
  low: "Low",
};
