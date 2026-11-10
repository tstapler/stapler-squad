package services

import (
	"github.com/tstapler/stapler-squad/session"
)

// NotificationActionWiring is what WireNotificationActions needs beyond the
// SessionService itself. Nil members leave their action kinds unavailable.
type NotificationActionWiring struct {
	Storage  *session.Storage
	Guidance *GuidanceRequestService
	// Backlog is the TransitionBacklogItemStatus delegate; leave it a nil interface (not a boxed nil
	// pointer) to disable backlog actions.
	Backlog BacklogTransitioner
	// Cancels backs CANCEL_WAKE and CANCEL_PEER_HELP (optional).
	Cancels CancelDelegate
	// Mirror re-registers an expired hook question from its durable row (optional).
	Mirror *GuidanceHookMirror
}

// WireNotificationActions installs the descriptor builder on notification
// history and the dispatcher behind InvokeNotificationAction. Call it once the
// notification store is wired.
func (s *SessionService) WireNotificationActions(w NotificationActionWiring) {
	src := DescriptorSources{
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
	s.notificationSvc.SetDescriptorBuilder(NewDescriptorBuilder(src))
	s.actionDispatcher = NewNotificationActionDispatcher(delegates, func() actionAudit {
		if sink := s.auditSink(); sink != nil {
			return sink
		}
		return nil
	})
}
