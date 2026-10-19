package services

import (
	"github.com/tstapler/stapler-squad/session"
)

// NotificationActionWiring is what WireNotificationActions needs beyond the
// SessionService itself. Nil members leave their action kinds unavailable.
type NotificationActionWiring struct {
	Storage  *session.Storage
	Guidance *GuidanceRequestService
	Backlog  *BacklogService
}

// WireNotificationActions installs the descriptor builder on notification
// history and the dispatcher behind InvokeNotificationAction. Call it once the
// notification store is wired.
func (s *SessionService) WireNotificationActions(w NotificationActionWiring) {
	src := DescriptorSources{
		Approvals: NewApprovalLookup(s.approvalStore),
		Questions: s.replyState().store,
	}
	delegates := ActionDelegates{
		Approvals: NewApprovalResolver(s.approvalSvc, s.approvalStore),
		Questions: s.WireHiddenReplyAdapter(),
		Resolver:  NewNotificationResolver(s.GetNotificationStore()),
	}
	if w.Storage != nil {
		src.Backlog = NewBacklogDecisionLookup(w.Storage)
		src.Guidance = NewGuidanceLookup(w.Storage)
	}
	if w.Guidance != nil {
		delegates.Guidance = NewGuidanceAnswerer(w.Guidance)
	}
	// A nil *BacklogService boxed into the interface would defeat the nil check.
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
