package session

import "github.com/tstapler/stapler-squad/session/detection"

// status_mapping.go documents the relationship between the three status types:
//
//   - Status:         lifecycle state of an Instance (Creating, Active, Paused, Stopped, Hibernated)
//   - DetectedStatus: what the status-detector observed in terminal output (StatusReady, StatusError, ...)
//   - AttentionReason: why a session appears in the review queue (ReasonErrorState, ReasonApprovalPending, ...)
//
// The current code has these mappings implicit and scattered across review_queue_poller.go,
// instance_status.go, and claude_controller.go.  These functions make them explicit and testable.

// AttentionReasonFromDetected maps a DetectedStatus to the AttentionReason that should be
// used when adding the session to the review queue.  Returns the zero AttentionReason
// (empty string) when no attention is needed for that status.
func AttentionReasonFromDetected(detected detection.DetectedStatus) AttentionReason {
	switch detected {
	case detection.StatusNeedsApproval:
		return ReasonApprovalPending
	case detection.StatusInputRequired:
		return ReasonInputRequired
	case detection.StatusError:
		return ReasonErrorState
	case detection.StatusTestsFailing:
		return ReasonTestsFailing
	case detection.StatusSuccess:
		return ReasonTaskComplete
	case detection.StatusIdle:
		return ReasonIdle
	// Active/processing states do not require attention.
	case detection.StatusActive, detection.StatusProcessing, detection.StatusReady, detection.StatusUnknown:
		return ""
	default:
		return ""
	}
}

// StatusFromDetected maps a DetectedStatus to the corresponding lifecycle Status.
// All detected states map to Active because the instance process is still executing.
// NeedsApproval, InputRequired, Error, and TestsFailing are sub-status signals
// surfaced via GetEffectiveStatus() — they do not change the lifecycle state.
func StatusFromDetected(detected detection.DetectedStatus) Status {
	// All detected states indicate an active process.
	return Active
}
