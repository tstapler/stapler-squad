package services

import (
	"context"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// question_answer_retry.go is the confirmed "Retry anyway" of design/ux.md rows
// 6 and 16: a digit was already written, so the row is answered and its
// pending-question entry consumed. The retry rebuilds the entry from the
// durable row and then takes the normal write path, whose DialogMatch under the
// lease fails closed as STALE when the dialog is gone or replaced.

// reopenForRetry prepares a confirmed retry. stop is true with the final result
// when the row cannot be retried; otherwise the question is registered again
// and the caller writes as usual. A row that was never answered is not a retry,
// so the flag changes nothing for it.
func (r *AnswerRouter) reopenForRetry(ctx context.Context, key QuestionKey) (res ActionResult, stop bool) {
	id, err := uuid.Parse(key.QuestionID)
	if r.rows == nil || r.reopen == nil || err != nil {
		return ActionResult{}, false
	}
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	if err != nil || row.Status() != session.GuidanceRequestStatusAnswered {
		return ActionResult{}, false
	}
	switch row.DeliveryState {
	case domain.DeliveryStateAcknowledged:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, row.Answer), true
	case domain.DeliveryStateSent:
		// The operator declares the unconfirmed send failed; losing the race to
		// an acknowledgement means nothing is left to retry.
		if moved, merr := r.rows.SetGuidanceDelivery(ctx, id, domain.DeliveryRouteLiveKeystroke, domain.DeliveryStateFailed, 0); merr != nil || !moved {
			return r.alreadyResolved(ctx, id), true
		}
	case domain.DeliveryStateFailed:
	default:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}, true
	}
	dialog, ok := dialogFromRow(row)
	if !ok || row.AskerSessionUUID != key.SessionUUID {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}, true
	}
	r.reopen(PendingQuestion{QuestionID: row.ID, SessionUUID: key.SessionUUID, Dialog: dialog, Token: QuestionToken(dialog.Question)})
	return ActionResult{}, false
}
