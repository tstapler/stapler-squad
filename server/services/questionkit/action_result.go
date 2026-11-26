package questionkit

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ActionResult is what a delegate reports. Expected conditions are outcomes;
// only an infrastructure fault is a returned error.
type ActionResult struct {
	Outcome     sessionv1.ActionOutcome
	StaleReason string
	RetryAfter  int32
	Detail      string
}

// Applied is the result of an action that took effect.
func Applied() ActionResult {
	return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}
}

// Outcome is a result carrying only an outcome and its detail.
func Outcome(o sessionv1.ActionOutcome, detail string) ActionResult {
	return ActionResult{Outcome: o, Detail: detail}
}

// NotWired is the result of an action whose delegate is not installed.
func NotWired() ActionResult {
	return Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "this action is not available on this server")
}

// ReplyOutcomeResult is what a reply attempt returns and what the idempotency
// table caches.
type ReplyOutcomeResult struct {
	Outcome    sessionv1.ReplyOutcome
	RetryAfter int32
	// Err is a Connect error the leader returned; followers return it too.
	Err error
}

// StaleReasonAnswerPartial is ActionResult.StaleReason when part of a
// multi-question answer was typed and the dialog then changed. The card keeps
// Submit disabled and offers Open session or a confirmed Dismiss, never Retry.
const StaleReasonAnswerPartial = "answer_aborted_partial"
