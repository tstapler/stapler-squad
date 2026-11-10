package services

// Characterization (not specification) of TransitionBacklogItemStatus under
// the Default workflow engine, pinned before the engine-gated choke point
// (ADR-007) changes anything. Configured-engine gates are out of scope.
//
// Observed fact worth keeping: override_reason bypasses only the review/
// pr_pending -> ready verdict-clear guard and the ->done verdict/shipped-code
// guards (session/domain/backlog.go TransitionGuard). It does NOT bypass the
// AC, plan, plan-artifact or blocker guards, and never bypasses CanTransition.
