package services

import (
	"context"
	"strconv"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// notification_action_adapters.go binds the dispatcher's consumer-side
// interfaces to the existing handlers. Each adapter only maps a delegate's
// result onto an ActionOutcome; the delegate keeps its own checks.

// approvalServiceAdapter resolves approvals through ApprovalService, which keeps
// the CI-red guard, notification stamping and event broadcast.
type approvalServiceAdapter struct {
	svc   *ApprovalService
	store *ApprovalStore
}

// NewApprovalResolver adapts ApprovalService to ApprovalResolver.
func NewApprovalResolver(svc *ApprovalService, store *ApprovalStore) ApprovalResolver {
	return approvalServiceAdapter{svc: svc, store: store}
}

func (a approvalServiceAdapter) ResolveApproval(ctx context.Context, c ApprovalChoice) (ActionResult, error) {
	decision := "deny"
	if c.Allow {
		decision = "allow"
	}
	_, err := a.svc.resolveApproval(ctx, c.ApprovalID, decision, c.Reason, c.Override, resolutionSourceHuman)
	if err == nil {
		return applied(), nil
	}
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
	case connect.CodeFailedPrecondition:
		// Still pending: the CI-red guard blocked it ("Approve anyway" needs confirm).
		// Gone: a rule auto-resolved it while the operator looked.
		if _, pending := a.store.Get(c.ApprovalID); pending {
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
	case connect.CodeInvalidArgument, connect.CodeAborted:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	return ActionResult{}, err
}

// guidanceServiceAdapter answers through GuidanceRequestService.
type guidanceServiceAdapter struct{ svc *GuidanceRequestService }

// NewGuidanceAnswerer adapts GuidanceRequestService to GuidanceAnswerer.
func NewGuidanceAnswerer(svc *GuidanceRequestService) GuidanceAnswerer {
	return guidanceServiceAdapter{svc}
}

func (a guidanceServiceAdapter) AnswerGuidance(ctx context.Context, requestID, answer string) (ActionResult, error) {
	resp, err := a.svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: requestID, Answer: answer}))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
		case connect.CodeInvalidArgument:
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return ActionResult{}, err
	}
	if !resp.Msg.GetApplied() {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
	}
	return applied(), nil
}

// replyFunc is the existing hidden-session reply handler.
type replyFunc func(context.Context, *connect.Request[sessionv1.ReplyToPendingQuestionRequest]) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error)

// pendingQuestionPeeker reads one registered question.
type pendingQuestionPeeker interface {
	Peek(QuestionKey) (PendingQuestion, bool)
}

// sessionRouteFinder says whether a session is hidden, visible or unknown.
type sessionRouteFinder func(sessionID string) (hidden, known bool)

// hiddenReplyAdapter is the interim QuestionReplier. It serves hidden sessions
// through the existing reply handler and reports FLAG_OFF for visible ones.
//
// SEAM: when PaneReplyCore (Story 2.4) and the AnswerRouter (Task 3.4a) land,
// replace this type behind QuestionReplier; the dispatcher and UI do not change.
type hiddenReplyAdapter struct {
	find      sessionRouteFinder
	questions pendingQuestionPeeker
	reply     replyFunc
}

// NewHiddenReplyAdapter builds the interim QuestionReplier.
func NewHiddenReplyAdapter(find sessionRouteFinder, questions pendingQuestionPeeker, reply replyFunc) QuestionReplier {
	return hiddenReplyAdapter{find: find, questions: questions, reply: reply}
}

func (a hiddenReplyAdapter) RouteFor(sessionID string) (answerRoute, bool) {
	hidden, known := a.find(sessionID)
	if !known {
		return 0, false
	}
	if hidden {
		return routeLiveKeystrokeHidden, true
	}
	return routeLiveKeystrokeVisible, true
}

func (a hiddenReplyAdapter) ReplyToQuestion(ctx context.Context, in QuestionAnswer) (ActionResult, error) {
	hidden, known := a.find(in.SessionID)
	if !known {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "session_ended"}, nil
	}
	if !hidden {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, flagOffMessage), nil
	}
	q, ok := a.questions.Peek(QuestionKey{SessionUUID: in.SessionID, QuestionID: in.QuestionID})
	if !ok {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}, nil
	}
	digit := optionDigit(q.Dialog.Labels, in.OptionLabel)
	if digit == "" {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "that option is not available for this question"), nil
	}
	resp, err := a.reply(ctx, connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
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

// optionDigit is the 1-based option number of label as one ASCII digit.
func optionDigit(labels []string, label string) string {
	for i, l := range labels {
		if l == label && i < 9 {
			return strconv.Itoa(i + 1)
		}
	}
	return ""
}

func mapReplyOutcome(r *sessionv1.ReplyToPendingQuestionResponse) ActionResult {
	switch r.GetOutcome() {
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT:
		return applied()
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE, "")
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_replaced"}
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED, RetryAfter: r.GetRetryAfterSeconds()}
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_DISABLED:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, flagOffMessage)
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_UNSPECIFIED:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, "")
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, "")
}

// inProcessPeer lets the interim adapter call the reply handler in process: a
// request built by connect.NewRequest has no peer address, which the handler's
// local-caller verdict would refuse. The dispatcher has already run that
// verdict on the real peer. A network request always carries an address.
func inProcessPeer(req connect.AnyRequest) string {
	if addr := req.Peer().Addr; addr != "" {
		return addr
	}
	return "127.0.0.1:0"
}

// NewSessionRouteFinder reports hidden/visible/unknown for a session id using
// the service's instance lookup.
func (s *SessionService) NewSessionRouteFinder() sessionRouteFinder {
	return func(sessionID string) (hidden, known bool) {
		inst := s.findInstance(sessionID)
		if inst == nil {
			return false, false
		}
		return AccessFor(inst) == TerminalReadOnly, true
	}
}

// WireHiddenReplyAdapter installs the interim in-process peer and returns the
// QuestionReplier over this service's reply machinery.
func (s *SessionService) WireHiddenReplyAdapter() QuestionReplier {
	if s.replyPeerAddr == nil {
		s.replyPeerAddr = inProcessPeer
	}
	return NewHiddenReplyAdapter(s.NewSessionRouteFinder(), s.replyState().store, s.ReplyToPendingQuestion)
}
