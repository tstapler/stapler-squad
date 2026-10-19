package services

import (
	"context"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// question_delivery_track.go keeps the durable delivery state of a keystroke
// answer: pending before the write, then sent, delivery_failed or stale. Every
// move is a guarded UPDATE (EntRepository.SetDelivery), so a racing writer
// loses instead of overwriting. A question with no durable row is not tracked.

// SentDelivery is a keystroke that may have reached a pane.
type SentDelivery struct {
	RowID       string
	SessionUUID string
	Dialog      session.QuestionDialog
	Route       domain.DeliveryRoute
	// Indeterminate marks a write that may or may not have landed
	// (SEND_INDETERMINATE); it is unconfirmed from the first moment.
	Indeterminate bool
}

type deliveryTrack struct {
	rows answerRows
	acks deliverySink
	id   uuid.UUID
	on   bool
}

// beginDelivery tracks questionID when it names a still-open durable row.
func (r *AnswerRouter) beginDelivery(ctx context.Context, questionID string) deliveryTrack {
	t := deliveryTrack{rows: r.rows, acks: r.acks}
	if r.rows == nil {
		return t
	}
	id, err := uuid.Parse(questionID)
	if err != nil {
		return t
	}
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	if err != nil || row.Status() != session.GuidanceRequestStatusPending {
		return t
	}
	t.id, t.on = id, true
	return t
}

func (t deliveryTrack) move(ctx context.Context, state domain.DeliveryState, attempts int) {
	if !t.on {
		return
	}
	applied, err := t.rows.SetGuidanceDelivery(ctx, t.id, domain.DeliveryRouteLiveKeystroke, state, attempts)
	if err != nil || !applied {
		log.Warn("[AnswerRouter] delivery state not recorded", "row", t.id, "state", state, "applied", applied, "err", err)
	}
}

// pending marks the attempt; one attempt is counted per write.
func (t deliveryTrack) pending(ctx context.Context) { t.move(ctx, domain.DeliveryStatePending, 1) }

// finish records where the attempt ended. A write that may have landed also
// answers the row and hands the delivery to the acknowledgement detector.
func (t deliveryTrack) finish(ctx context.Context, res ActionResult, err error, sent SentDelivery, answer string) {
	if !t.on {
		return
	}
	if err != nil {
		t.move(ctx, domain.DeliveryStateFailed, 0)
		return
	}
	switch res.Outcome {
	case sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE:
		if _, aerr := t.rows.AnswerGuidanceRequest(ctx, t.id, answer); aerr != nil {
			log.Warn("[AnswerRouter] keystroke sent but answer not stored", "row", t.id, "err", aerr)
		}
		t.move(ctx, domain.DeliveryStateSent, 0)
		if t.acks != nil {
			t.acks.Sent(sent)
		}
	case sessionv1.ActionOutcome_ACTION_OUTCOME_STALE:
		if res.StaleReason == StaleReasonAnswerPartial {
			t.move(ctx, domain.DeliveryStateAbortedPartial, 0)
			return
		}
		t.move(ctx, domain.DeliveryStateStale, 0)
	case sessionv1.ActionOutcome_ACTION_OUTCOME_UNSPECIFIED, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN,
		sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF:
		t.move(ctx, domain.DeliveryStateFailed, 0)
	}
}
