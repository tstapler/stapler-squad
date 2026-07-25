package session

import "github.com/tstapler/stapler-squad/session/detection"

// status_mapping.go documents the relationship between the three status types:
//
//   - Status:         lifecycle state of an Instance (Creating, Active, Paused, Stopped, Hibernated)
//   - DetectedStatus: what the status-detector observed in terminal output (StatusReady, StatusError, ...)
//   - AttentionReason: why a session appears in the review queue (ReasonErrorState, ReasonApprovalPending, ...)
//
// This mapping used to be implicit and scattered across review_queue_poller.go,
// instance_status.go, and claude_controller.go.  These functions make it explicit, testable,
// and — via AttentionReasonFromDetected — the single place review_queue_determiner.go's
// Determine() delegates to when deriving a reason/priority/context from a DetectedStatus.

// AttentionReasonFromDetected maps a DetectedStatus to the AttentionReason, Priority, and
// default context message that should be used when adding the session to the review queue.
// Returns the zero AttentionReason (empty string), zero Priority, and empty context when no
// attention is needed for that status.
//
// This is the single source of truth for DetectedStatus -> (reason, priority, context)
// mapping; callers combine the returned context with a caller-specific override (e.g. a
// live StatusContext string) via effectiveCtx when one is available.
func AttentionReasonFromDetected(detected detection.DetectedStatus) (reason AttentionReason, priority Priority, context string) {
	switch detected {
	case detection.StatusNeedsApproval:
		return ReasonApprovalPending, PriorityHigh, "Waiting for approval to proceed"
	case detection.StatusInputRequired:
		return ReasonInputRequired, PriorityMedium, "Waiting for explicit user input"
	case detection.StatusError:
		return ReasonErrorState, PriorityUrgent, "Error state detected"
	case detection.StatusTestsFailing:
		return ReasonTestsFailing, PriorityHigh, "Tests are failing"
	case detection.StatusSuccess:
		return ReasonTaskComplete, PriorityLow, "Task completed successfully"
	case detection.StatusIdle:
		return ReasonIdle, PriorityLow, "Session idle - ready for next task"
	// Active/processing states do not require attention.
	case detection.StatusExecuting, detection.StatusProcessing, detection.StatusWaitingForAgent, detection.StatusReady, detection.StatusUnknown:
		return "", 0, ""
	default:
		return "", 0, ""
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
