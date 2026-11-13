package session

import (
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/queue"
)

// statusDecision is the queue entry a detected status maps to; in statusRules, Context is the default.
type statusDecision struct {
	Reason   queue.AttentionReason
	Priority queue.Priority
	Context  string
}

// statusRules is the single source of truth for statuses that put a session in the queue.
var statusRules = map[detection.DetectedStatus]statusDecision{
	detection.StatusNeedsApproval: {queue.ReasonApprovalPending, queue.PriorityHigh, "Waiting for approval to proceed"},
	detection.StatusInputRequired: {queue.ReasonInputRequired, queue.PriorityMedium, "Waiting for explicit user input"},
	detection.StatusError:         {queue.ReasonErrorState, queue.PriorityUrgent, "Error state detected"},
	detection.StatusTestsFailing:  {queue.ReasonTestsFailing, queue.PriorityHigh, "Tests are failing"},
	detection.StatusSuccess:       {queue.ReasonTaskComplete, queue.PriorityLow, "Task completed successfully"},
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
func isPromptReason(r queue.AttentionReason) bool {
	return r == queue.ReasonApprovalPending || r == queue.ReasonInputRequired || r == queue.ReasonWaitingForUser
}
