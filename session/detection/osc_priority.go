package detection

// IsOSCExecutingPromotable reports whether a text-pattern-derived DetectedStatus
// is eligible to be promoted to Executing by an OSC-derived spinner signal.
// Single source of truth shared by applyOSCStatusOverride (session package,
// DetectedStatus side) and IdleDetector.DetectStateFromContentWithOSC (IdleState
// side) — see osc-status-signals architecture-review.md BLOCKER 2. Neither
// direction ever promotes over a higher-urgency status (Error, NeedsApproval,
// InputRequired, TestsFailing, Success, WaitingForAgent, Executing) — a false
// OSC-idle or stale-spinner title must never mask a state needing user
// attention. Cases are enumerated exhaustively (rather than a default
// fallthrough) so a future new DetectedStatus constant fails golangci-lint's
// exhaustive check instead of silently defaulting to "never promotable".
func IsOSCExecutingPromotable(status DetectedStatus) bool {
	switch status {
	case StatusReady, StatusUnknown, StatusIdle, StatusProcessing:
		return true
	case StatusNeedsApproval, StatusInputRequired, StatusError, StatusTestsFailing,
		StatusExecuting, StatusSuccess, StatusWaitingForAgent, StatusCompacting:
		return false
	}
	return false
}

// IsOSCIdlePromotable reports whether a text-pattern-derived DetectedStatus is
// eligible to be promoted to Idle by an OSC-derived ✳ signal. See
// IsOSCExecutingPromotable's doc comment for why cases are enumerated
// exhaustively.
func IsOSCIdlePromotable(status DetectedStatus) bool {
	switch status {
	case StatusReady, StatusUnknown:
		return true
	case StatusProcessing, StatusNeedsApproval, StatusInputRequired, StatusError,
		StatusTestsFailing, StatusIdle, StatusExecuting, StatusSuccess,
		StatusWaitingForAgent, StatusCompacting:
		return false
	}
	return false
}
