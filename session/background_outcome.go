package session

// BackgroundOutcome is the server-computed result of a hidden (background)
// session, surfaced as Session.background_outcome. See ADR-001 decision 6.
type BackgroundOutcome string

const (
	BackgroundOutcomeCompleted BackgroundOutcome = "completed"
	BackgroundOutcomeFailed    BackgroundOutcome = "failed"
	BackgroundOutcomeUnknown   BackgroundOutcome = "unknown"
)

// failureEndReasons are the ItemSession.end_reason values that mean the
// session failed. end_reason is free text from several producers, so this is
// an allowlist: any value not listed is a non-failure.
var failureEndReasons = map[string]struct{}{
	"pool_saturated":         {},
	"idle":                   {},
	"timeout":                {},
	"claude_not_found":       {},
	"subprocess_start_error": {},
	"fanout_ceiling":         {},
	"cost_ceiling":           {},
	"other":                  {},
	"parse_error":            {},
	"dispatch_failed":        {},
	"dispatch_incomplete":    {},
	"jules_timed_out":        {},
	"jules_session_missing":  {},
	"jules_failed":           {},
}

// nonFailureEndReasons are the known end_reason values that are not failures.
// It exists so a new producer must be classified deliberately
// (TestEndReasonClassification_ShouldCoverEveryKnownProducer).
var nonFailureEndReasons = map[string]struct{}{
	"":                          {},
	"shutdown":                  {}, // our own restart
	"jules_completed":           {},
	"jules_completed_no_pr":     {},
	"cost_budget_exceeded":      {}, // CapacityMonitor guardrail via stopGuardrailSession
	"idle_wait_loop_escalation": {}, // CapacityMonitor guardrail via stopGuardrailSession
}

// IsFailureEndReason reports whether endReason is on the failure allowlist.
func IsFailureEndReason(endReason string) bool {
	_, ok := failureEndReasons[endReason]
	return ok
}

// ComputeBackgroundOutcome derives a hidden session's outcome. is is nil when
// the session has no ItemSession. A recorded verdict wins over a failure
// reason; the diagnose role records no verdict, so it is always unknown.
func ComputeBackgroundOutcome(status Status, is *ItemSessionSummary) BackgroundOutcome {
	if is != nil && is.Role == SessionRoleDiagnose {
		return BackgroundOutcomeUnknown
	}
	if is != nil && hasRecordedResult(is) {
		return BackgroundOutcomeCompleted
	}
	if status == Crashed || (is != nil && IsFailureEndReason(is.EndReason)) {
		return BackgroundOutcomeFailed
	}
	return BackgroundOutcomeUnknown
}

func hasRecordedResult(is *ItemSessionSummary) bool {
	switch is.Role {
	case SessionRoleReview:
		return is.ReviewVerdict != nil
	case SessionRoleTriage:
		return is.TriageResult != ""
	}
	return false
}
