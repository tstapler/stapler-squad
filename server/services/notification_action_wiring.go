package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/server/services/notifyactions"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// NotificationActionWiring is what WireNotificationActions needs beyond the
// SessionService itself. Nil members leave their action kinds unavailable.
type NotificationActionWiring struct {
	Storage  *session.Storage
	Guidance *guidance.GuidanceRequestService
	// Backlog is the TransitionBacklogItemStatus delegate; leave it a nil interface (not a boxed nil
	// pointer) to disable backlog actions.
	Backlog notifyactions.BacklogTransitioner
	// Cancels backs CANCEL_WAKE and CANCEL_PEER_HELP (optional).
	Cancels notifyactions.CancelDelegate
	// Mirror re-registers an expired hook question from its durable row (optional).
	Mirror *questionkit.GuidanceHookMirror
}

// WireNotificationActions installs the descriptor builder on notification
// history and the dispatcher behind InvokeNotificationAction. Call it once the
// notification store is wired.
func (s *SessionService) WireNotificationActions(w NotificationActionWiring) {
	deps := notifyactions.Deps{
		Approvals:     s.approvalSvc,
		ApprovalStore: s.approvalStore,
		Guidance:      w.Guidance,
		Transitions:   w.Backlog,
		Merger:        s,
		Cancels:       w.Cancels,
		Audit:         s.auditSink,
	}
	if w.Storage != nil {
		deps.Storage = w.Storage
	}
	src := deps.DescriptorSources()
	src.Questions = s.replyState().store
	src.Canary = s.wireDialogCanary()
	acks := s.wireGuidanceAcks(w.Storage)
	s.wireHookReply(w.Storage, acks)
	router := s.WireAnswerRouter(answerRouterDeps(w, acks))
	src.Routes = router
	deps.Questions = router
	deps.History = s.GetNotificationStore()
	if w.Guidance != nil {
		w.Guidance.SetDeliveryProbes(s.deliveryProbes(acks))
	}
	deps.Registry = s.InjectedFlagRegistry()
	s.notificationSvc.SetDescriptorBuilder(notify.NewDescriptorBuilder(src, deps.Registry))
	s.actionDispatcher = notifyactions.NewDispatcher(deps)
}

// InvokeNotificationAction applies one inline notification action.
// +api: notification:invoke-action
// +api: InvokeNotificationAction
func (s *SessionService) InvokeNotificationAction(
	ctx context.Context,
	req *connect.Request[sessionv1.InvokeNotificationActionRequest],
) (*connect.Response[sessionv1.InvokeNotificationActionResponse], error) {
	if s.actionDispatcher == nil {
		return connect.NewResponse(&sessionv1.InvokeNotificationActionResponse{
			Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, Detail: flagsvc.FlagOffMessage,
		}), nil
	}
	resp, err := s.actionDispatcher.Invoke(ctx, notifyactions.ActionCall{Msg: req.Msg, Peer: req.Peer().Addr, Header: req.Header()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// SetNotificationActionDispatcher wires the dispatcher behind the RPC.
func (s *SessionService) SetNotificationActionDispatcher(d *notifyactions.NotificationActionDispatcher) {
	s.actionDispatcher = d
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
		visible: s.paneReplyCore, guard: access.NewHumanActiveGuard(access.DefaultHumanActivity(), tmux.ListClientActivity),
		flags: notifyactions.NewConfigFlags(flagsvc.CompiledFlagEvidence, s.flagReader()), rows: d.Rows, acks: d.Acks,
	}
	r.ensure = s.ensureRegistered(d.Mirror)
	store := s.replyState().store
	r.reopen = func(q questionkit.PendingQuestion) {
		store.Forget(questionkit.QuestionKey{SessionUUID: q.SessionUUID, QuestionID: q.QuestionID})
		store.Register(q)
	}
	return r
}
