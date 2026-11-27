package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/server/services/notifyactions"
	"github.com/tstapler/stapler-squad/server/services/qareply"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
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
	src.Questions = s.PendingQuestions()
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

// WireAnswerRouter returns the QuestionReplier over this service's live sessions.
func (s *SessionService) WireAnswerRouter(d qareply.AnswerRouterDeps) *qareply.AnswerRouter {
	d.Flags = notifyactions.NewConfigFlags(flagsvc.CompiledFlagEvidence, s.flagReader())
	return s.replyService().NewAnswerRouter(d)
}
