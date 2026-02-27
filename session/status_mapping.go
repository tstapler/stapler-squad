package session

// status_mapping.go documents the relationship between the three status types:
//
//   - Status:         lifecycle state of an Instance (Running, Ready, Paused, NeedsApproval, Loading)
//   - DetectedStatus: what the status-detector observed in terminal output (StatusReady, StatusError, ...)
//   - AttentionReason: why a session appears in the review queue (ReasonErrorState, ReasonApprovalPending, ...)
//
// The current code has these mappings implicit and scattered across review_queue_poller.go,
// instance_status.go, and claude_controller.go.  These functions make them explicit and testable.

// AttentionReasonFromDetected maps a DetectedStatus to the AttentionReason that should be
// used when adding the session to the review queue.  Returns the zero AttentionReason
// (empty string) when no attention is needed for that status.
func AttentionReasonFromDetected(detected DetectedStatus) AttentionReason {
	switch detected {
	case StatusNeedsApproval:
		return ReasonApprovalPending
	case StatusInputRequired:
		return ReasonInputRequired
	case StatusError:
		return ReasonErrorState
	case StatusTestsFailing:
		return ReasonTestsFailing
	case StatusSuccess:
		return ReasonTaskComplete
	case StatusIdle:
		return ReasonIdle
	// Active/processing states do not require attention.
	case StatusActive, StatusProcessing, StatusReady, StatusUnknown:
		return ""
	default:
		return ""
	}
}

// StatusFromDetected maps a DetectedStatus to the corresponding lifecycle Status.
// This documents the intended transition table even though the review queue poller
// currently does not update Instance.Status directly on every detection cycle.
//
// Key design decisions captured here:
//   - Error and TestsFailing keep the lifecycle as Running because the instance process
//     is still executing; only the output signals a problem.
//   - NeedsApproval and InputRequired both map to NeedsApproval because both mean
//     "the instance is blocked, waiting for the user".
func StatusFromDetected(detected DetectedStatus) Status {
	switch detected {
	case StatusReady, StatusIdle, StatusSuccess:
		return Ready
	case StatusProcessing, StatusActive:
		return Running
	case StatusNeedsApproval, StatusInputRequired:
		return NeedsApproval
	case StatusError, StatusTestsFailing:
		// Error/test-failure at the lifecycle level is still Running —
		// the instance process has not exited.
		return Running
	default:
		return Running
	}
}
