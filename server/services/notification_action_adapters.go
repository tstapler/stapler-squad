package services

import (
	"context"
	"strconv"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tmux"
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
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, a.decision(c.ApprovalID)), nil
	case connect.CodeFailedPrecondition:
		// Still pending: the CI-red guard blocked it ("Approve anyway" needs confirm).
		// Gone: a rule auto-resolved it while the operator looked.
		if _, pending := a.store.Get(c.ApprovalID); pending {
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, a.decision(c.ApprovalID)), nil
	case connect.CodeInvalidArgument, connect.CodeAborted:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	return ActionResult{}, err
}

// decision is the stamped outcome of an already-resolved approval ("allow",
// "deny", ...), the value an "Answered elsewhere" card shows; "" when unknown.
func (a approvalServiceAdapter) decision(approvalID string) string {
	if a.svc == nil || a.svc.notificationStore == nil {
		return ""
	}
	if rec, ok := a.svc.notificationStore.GetByID(approvalID); ok {
		return rec.Metadata["approval_decision"]
	}
	return ""
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
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, resp.Msg.GetRequest().GetAnswer()), nil
	}
	return applied(), nil
}

// pendingQuestionPeeker reads one registered question.
type pendingQuestionPeeker interface {
	Peek(QuestionKey) (PendingQuestion, bool)
}

// replyFunc is the existing hidden-session reply handler.
type replyFunc func(context.Context, *connect.Request[sessionv1.ReplyToPendingQuestionRequest]) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error)

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
	return mapReplyOutcomeValue(r.GetOutcome(), r.GetRetryAfterSeconds())
}

func mapReplyOutcomeValue(o sessionv1.ReplyOutcome, retryAfter int32) ActionResult {
	switch o {
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT:
		return applied()
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE, "")
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_gone"}
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "dialog_replaced"}
	case sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED, RetryAfter: retryAfter}
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

// answerSessions classifies a session for the router from the live poller.
func (s *SessionService) answerSessions() answerSessionFinder {
	return func(sessionID string) routedSession {
		inst := s.findInstance(sessionID)
		switch {
		case inst == nil:
			return routedSession{}
		case inst.IsArchived():
			return routedSession{inst: inst, phase: phaseArchived}
		case inst.Snapshot().Status.IsSuspended():
			return routedSession{inst: inst, phase: phaseStopped}
		}
		return routedSession{inst: inst, phase: phaseLive}
	}
}

// WireAnswerRouter installs the in-process peer the router needs to call the
// hidden reply handler and returns the QuestionReplier over this service.
func (s *SessionService) WireAnswerRouter(d AnswerRouterDeps) *AnswerRouter {
	if s.replyPeerAddr == nil {
		s.replyPeerAddr = inProcessPeer
	}
	r := &AnswerRouter{
		sessions: s.answerSessions(), questions: s.replyState().store, hidden: s.ReplyToPendingQuestion,
		visible: s.paneReplyCore, guard: NewHumanActiveGuard(defaultHumanActivity, tmux.ListClientActivity),
		flags: configActionFlags{ev: compiledFlagEvidence}, rows: d.Rows, acks: d.Acks,
	}
	if d.Mirror != nil {
		r.ensure = d.Mirror.EnsureRegistered
	}
	return r
}
