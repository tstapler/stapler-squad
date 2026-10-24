package session

import "github.com/tstapler/stapler-squad/session/detection"

// statusDecision is the queue entry a detected status maps to; in statusRules, Context is the default.
type statusDecision struct {
	Reason   AttentionReason
	Priority Priority
	Context  string
}

// statusRules is the single source of truth for statuses that put a session in the queue.
var statusRules = map[detection.DetectedStatus]statusDecision{
	detection.StatusNeedsApproval: {ReasonApprovalPending, PriorityHigh, "Waiting for approval to proceed"},
	detection.StatusInputRequired: {ReasonInputRequired, PriorityMedium, "Waiting for explicit user input"},
	detection.StatusError:         {ReasonErrorState, PriorityUrgent, "Error state detected"},
	detection.StatusTestsFailing:  {ReasonTestsFailing, PriorityHigh, "Tests are failing"},
	detection.StatusSuccess:       {ReasonTaskComplete, PriorityLow, "Task completed successfully"},
}

// reasonForStatus maps a detected status to its queue decision, preferring the provided
// context over the rule's default. ok is false for statuses that don't enqueue directly.
func reasonForStatus(status detection.DetectedStatus, providedContext string) (decision statusDecision, ok bool) {
	rule, ok := statusRules[status]
	if !ok {
		return statusDecision{}, false
	}
	rule.Context = effectiveCtx(providedContext, rule.Context)
	return rule, true
}

// isPromptReason marks reasons the raw terminal scanner can false-positive during a resize
// redraw, so a controller-less Remove must not drop them. It is deliberately distinct from
// needsHumanReasons (suppression.go): that set decides ack-bypass, and includes ErrorState
// but not WaitingForUser.
func isPromptReason(r AttentionReason) bool {
	return r == ReasonApprovalPending || r == ReasonInputRequired || r == ReasonWaitingForUser
}
