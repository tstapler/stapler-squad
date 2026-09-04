// Mirrors the EscalationCategory taxonomy in pkg/classifier/escalation.go. Keep in sync by
// hand — there is no codegen bridge for this Go string-constant set today (see
// project_plans/escalation-reasoning/implementation/plan.md's Pattern Decisions table for why
// a proto enum was rejected as overkill for a backend-only string key).
export type EscalationCategory =
  | "no-match"
  | "explicit-rule"
  | "domain-age"
  | "secret-scan"
  | "unclassifiable"
  | "unexpected";
