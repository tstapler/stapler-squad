package services

import (
	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
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
	Backlog BacklogTransitioner
	// Cancels backs CANCEL_WAKE and CANCEL_PEER_HELP (optional).
	Cancels CancelDelegate
	// Mirror re-registers an expired hook question from its durable row (optional).
	Mirror *questionkit.GuidanceHookMirror
}

// WireNotificationActions installs the descriptor builder on notification
// history and the dispatcher behind InvokeNotificationAction. Call it once the
// notification store is wired.
func (s *SessionService) WireNotificationActions(w NotificationActionWiring) {
	src := notify.DescriptorSources{
		Approvals: NewApprovalLookup(s.approvalStore),
		Questions: s.replyState().store,
		Canary:    s.wireDialogCanary(),
	}
	acks := s.wireGuidanceAcks(w.Storage)
	s.wireHookReply(w.Storage, acks)
	router := s.WireAnswerRouter(answerRouterDeps(w, acks))
	src.Routes = router
	delegates := ActionDelegates{
		Approvals: NewApprovalResolver(s.approvalSvc, s.approvalStore),
		Questions: router,
		Resolver:  NewNotificationResolver(s.GetNotificationStore()),
		Cancels:   w.Cancels,
	}
	if w.Storage != nil {
		src.Backlog = NewBacklogDecisionLookup(w.Storage)
		src.Guidance = NewGuidanceLookup(w.Storage)
	}
	if w.Guidance != nil {
		delegates.Guidance = NewGuidanceAnswerer(w.Guidance)
		w.Guidance.SetDeliveryProbes(s.deliveryProbes(acks))
	}
	if w.Backlog != nil && w.Storage != nil {
		delegates.Backlog = NewBacklogDecider(BacklogDeciderDeps{
			Status:      NewItemStatusReader(w.Storage),
			Transitions: w.Backlog,
			Sessions:    NewWorkSessionFinder(w.Storage),
			Merger:      s,
		})
	}
	s.notificationSvc.SetDescriptorBuilder(notify.NewDescriptorBuilder(src, s.InjectedFlagRegistry()))
	s.actionDispatcher = NewNotificationActionDispatcher(delegates, func() actionAudit {
		if sink := s.auditSink(); sink != nil {
			return sink
		}
		return nil
	}, s.InjectedFlagRegistry())
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
		flags: configActionFlags{ev: flagsvc.CompiledFlagEvidence, fr: s.flagReader()}, rows: d.Rows, acks: d.Acks,
	}
	r.ensure = s.ensureRegistered(d.Mirror)
	store := s.replyState().store
	r.reopen = func(q questionkit.PendingQuestion) {
		store.Forget(questionkit.QuestionKey{SessionUUID: q.SessionUUID, QuestionID: q.QuestionID})
		store.Register(q)
	}
	return r
}
