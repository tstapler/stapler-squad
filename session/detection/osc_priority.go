package detection

// IsOSCExecutingPromotable reports whether a text-pattern-derived DetectedStatus
// is eligible to be promoted to Executing by an OSC-derived spinner signal.
// Single source of truth shared by applyOSCStatusOverride and
// IdleDetector.DetectStateFromContentWithOSC — see architecture-review.md
// BLOCKER 2. Cases are enumerated exhaustively so a future DetectedStatus
// constant fails the exhaustive lint check rather than silently defaulting.
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
