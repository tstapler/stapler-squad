package services

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// question_answer_router.go is the AnswerRouter of ADR-005: it picks a delivery
// route for an ANSWER_QUESTION by session state and runs it. It is the
// QuestionReplier behind the dispatcher. Nothing here revives a session: the
// resume and read_only routes only store the answer, and the operator resumes
// through Open session (see .claude/rules/noarchivedrevival.md).

// detailHumanActive is the NOT_SENT detail when a person is typing in the pane.
const detailHumanActive = "human_active"

// answerPhase is how alive the session of a question is.
type answerPhase uint8

const (
	phaseUnknown  answerPhase = iota // not found: deleted or never existed
	phaseLive                        // running; its dialog can take a keystroke
	phaseStopped                     // suspended, stopped or crashed; not archived
	phaseArchived                    // deliberately retired
)

// routedSession is a session lookup result.
type routedSession struct {
	inst  *session.Instance
	phase answerPhase
}

type answerSessionFinder func(sessionID string) routedSession

// answerRows is the slice of guidance storage the router reads and writes.
type answerRows interface {
	GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*session.GuidanceRequestData, error)
	AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (bool, error)
	SetGuidanceDelivery(ctx context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, addAttempts int) (bool, error)
}

// deliverySink is told about every keystroke that may have reached a pane, so
// acknowledgement can be detected (Task 3.4e).
type deliverySink interface {
	Sent(d SentDelivery)
}

// AnswerRouterDeps are the collaborators of the router. Nil members disable
// what needs them: no Rows means no durable routes and no delivery state.
type AnswerRouterDeps struct {
	Rows   answerRows
	Mirror *GuidanceHookMirror
	Acks   deliverySink
}

// AnswerRouter implements QuestionReplier over the ADR-005 ladder.
type AnswerRouter struct {
	sessions  answerSessionFinder
	questions pendingQuestionPeeker
	reopen    func(PendingQuestion)
	ensure    func(ctx context.Context, key QuestionKey) bool
	hidden    replyFunc
	visible   func() *PaneReplyCore
	guard     *HumanActiveGuard
	flags     actionFlags
	rows      answerRows
	acks      deliverySink
}

// RouteFor says where an answer to a question of sessionID would go.
func (r *AnswerRouter) RouteFor(sessionID string) (answerRoute, bool) {
	s := r.sessions(sessionID)
	switch s.phase {
	case phaseLive:
		if AccessFor(s.inst) == TerminalReadOnly {
			return routeLiveKeystrokeHidden, true
		}
		return routeLiveKeystrokeVisible, true
	case phaseStopped, phaseArchived:
		return routeDurableRecord, true
	case phaseUnknown:
	}
	return 0, false
}

// deliveryRouteFor is the domain route of a session in phase: the one place
// phase maps to a route.
func deliveryRouteFor(p answerPhase) domain.DeliveryRoute {
	switch p {
	case phaseLive:
		return domain.DeliveryRouteLiveKeystroke
	case phaseStopped:
		return domain.DeliveryRouteResume
	case phaseArchived:
		return domain.DeliveryRouteReadOnly
	case phaseUnknown:
	}
	return ""
}

// ReplyToQuestion answers one question by the route its session allows.
func (r *AnswerRouter) ReplyToQuestion(ctx context.Context, in QuestionAnswer) (ActionResult, error) {
	s := r.sessions(in.SessionID)
	route := deliveryRouteFor(s.phase)
	if route == "" {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "session_ended"}, nil
	}
	gate, _ := r.RouteFor(in.SessionID)
	if in.ExpectedRoute != nil && *in.ExpectedRoute != gate {
		// The tier and flag gate ran on another route; a live keystroke must not
		// slip past the Tier B remote gate because the session changed since.
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "route_changed"}, nil
	}
	if !r.flags.Answer(gate) {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, flagOffMessage), nil
	}
	switch route {
	case domain.DeliveryRouteLiveKeystroke:
		return r.liveKeystroke(ctx, s, in)
	case domain.DeliveryRouteResume, domain.DeliveryRouteReadOnly:
		return r.storeAnswer(ctx, route, in)
	case domain.DeliveryRouteHookReply:
		// A hook_reply answer is consumed by the hook that asks for it (hook_reply.go);
		// an operator ANSWER_QUESTION never selects it.
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "this question cannot be answered here"), nil
}

// liveKeystroke writes one digit into the on-screen dialog.
func (r *AnswerRouter) liveKeystroke(ctx context.Context, s routedSession, in QuestionAnswer) (ActionResult, error) {
	if qs, ok := r.sequenceQuestions(ctx, in.QuestionID); ok {
		return r.liveSequence(ctx, s, in, qs)
	}
	key := QuestionKey{SessionUUID: in.SessionID, QuestionID: in.QuestionID}
	if in.Confirm {
		if res, stop := r.reopenForRetry(ctx, key); stop {
			return res, nil
		}
	}
	q, ok := r.questions.Peek(key)
	if !ok && r.ensure != nil && r.ensure(ctx, key) {
		q, ok = r.questions.Peek(key)
	}
	if !ok {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}, nil
	}
	digit := optionDigit(q.Dialog.Labels, in.OptionLabel)
	if digit == "" {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "that option is not available for this question"), nil
	}
	visible := AccessFor(s.inst) == TerminalReadWrite
	if visible && r.guard != nil && r.guard.Active(ctx, s.inst) {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, detailHumanActive), nil
	}
	track := r.beginDelivery(ctx, in.QuestionID)
	track.pending(ctx)
	var res ActionResult
	var err error
	if visible {
		res, err = r.writeVisible(ctx, s.inst, in, digit[0])
	} else {
		res, err = r.writeHidden(ctx, in, digit)
	}
	track.finish(ctx, res, err, SentDelivery{
		RowID: in.QuestionID, SessionUUID: s.inst.GetStableID(), Dialog: q.Dialog, Route: domain.DeliveryRouteLiveKeystroke,
		Indeterminate: res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE,
	}, in.OptionLabel)
	return res, err
}

// writeHidden goes through the existing hidden-session reply handler, which keeps
// its own flag, idempotency and audit.
func (r *AnswerRouter) writeHidden(ctx context.Context, in QuestionAnswer, digit string) (ActionResult, error) {
	resp, err := r.hidden(ctx, connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
		SessionId: in.SessionID, QuestionId: in.QuestionID, ReplyText: digit, ReplyId: in.ReplyID,
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeInvalidArgument || connect.CodeOf(err) == connect.CodePermissionDenied {
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return ActionResult{}, err
	}
	return mapReplyOutcome(resp.Msg), nil
}

// writeVisible runs the shared PaneReplyCore against a visible session. The
// dispatcher already ran the local-caller verdict on the real peer; the core's
// audit line records the in-process peer as the hidden path does.
func (r *AnswerRouter) writeVisible(ctx context.Context, inst *session.Instance, in QuestionAnswer, digit byte) (ActionResult, error) {
	refusal, err := EvaluateLocalWrite(ctx, inProcessPeerAddr, LocalWriteLocalCaller)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	res, _ := r.visible().Run(ctx, AnswerTargetVisible, replyInput{
		sessionID: in.SessionID, sessionUUID: inst.GetStableID(), questionID: in.QuestionID, replyID: in.ReplyID, digit: digit,
	}, inst, steerRequestFacts{refusal: refusal})
	if res.Err != nil {
		return ActionResult{}, res.Err
	}
	return mapReplyOutcomeValue(res.Outcome, res.RetryAfter), nil
}

// storeAnswer records the answer on the durable row and pushes nothing.
func (r *AnswerRouter) storeAnswer(ctx context.Context, route domain.DeliveryRoute, in QuestionAnswer) (ActionResult, error) {
	if r.rows == nil {
		return notWired(), nil
	}
	id, err := uuid.Parse(in.QuestionID)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "question_id is not a stored question"), nil
	}
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	switch {
	case errors.Is(err, session.ErrNotFound):
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "session_ended"}, nil
	case err != nil:
		return ActionResult{}, fmt.Errorf("read question %s: %w", in.QuestionID, err)
	}
	if res, done := settledAnswer(row); done {
		return res, nil
	}
	if !answerFitsQuestion(row, in.OptionLabel) {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "that option is not available for this question"), nil
	}
	stored, err := r.rows.AnswerGuidanceRequest(ctx, id, in.OptionLabel)
	if err != nil {
		return ActionResult{}, fmt.Errorf("store answer %s: %w", in.QuestionID, err)
	}
	if !stored {
		return r.alreadyResolved(ctx, id), nil
	}
	if _, err := r.rows.SetGuidanceDelivery(ctx, id, route, domain.DeliveryStateSavedForLater, 0); err != nil {
		log.Warn("[AnswerRouter] answer stored but delivery state not recorded", "question", in.QuestionID, "err", err)
	}
	return applied(), nil
}

// answerFitsQuestion reports whether answer is one the question offers, as the
// live path and hook_reply.go already require. A row with no recorded options
// takes free text.
func answerFitsQuestion(row *session.GuidanceRequestData, answer string) bool {
	if row.QuestionsJSON != "" {
		payload, err := domain.ParseQuestionPayload([]byte(row.QuestionsJSON))
		if err != nil {
			return false
		}
		qs := payload.Questions()
		if len(qs) == 1 && !qs[0].MultiSelect {
			return optionOffered(qs[0], answer)
		}
		picks, err := DecodeSequencePicks(answer)
		if err != nil || len(picks) != len(qs) {
			return false
		}
		for i, labels := range picks {
			for _, l := range labels {
				if !optionOffered(qs[i], l) {
					return false
				}
			}
		}
		return true
	}
	if opts := decodeGuidanceOptions(row.Options); len(opts) > 0 {
		return slices.Contains(opts, answer)
	}
	return true
}

func optionOffered(q domain.Question, label string) bool {
	return slices.ContainsFunc(q.Options, func(o domain.Option) bool { return o.Label == label })
}

// settledAnswer reports a row that can no longer take an answer: cancelled (a
// session that ended takes a hook or peer row with it) or already answered.
func settledAnswer(row *session.GuidanceRequestData) (ActionResult, bool) {
	switch row.Status() {
	case session.GuidanceRequestStatusCancelled:
		if row.CancelReason == domain.CancelReasonSessionEnded {
			return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "session_ended"}, true
		}
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED}, true
	case session.GuidanceRequestStatusAnswered:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, row.Answer), true
	case session.GuidanceRequestStatusPending:
	}
	return ActionResult{}, false
}

// alreadyResolved reports the value that won a lost race.
func (r *AnswerRouter) alreadyResolved(ctx context.Context, id uuid.UUID) ActionResult {
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, "")
	}
	res, _ := settledAnswer(row)
	if res.Outcome == 0 {
		res = outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, "")
	}
	return res
}

// inProcessPeerAddr stands in for the dispatcher's caller, which it has already verified.
const inProcessPeerAddr = "127.0.0.1:0"
