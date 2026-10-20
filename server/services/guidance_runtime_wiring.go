package services

import (
	"context"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// guidance_runtime_wiring.go binds the hook-question mirror, the dialog-gone
// sampler and the layout-drift canary to the live service (Story 3.3).

// canaryNotice is the one operator notification the canary raises when it trips.
const (
	canaryNoticeTitle   = "Question answers paused"
	canaryNoticeMessage = "Most recent attempts to answer a question did not match the terminal. Answer questions in the session until this clears."
	canaryNoticeSession = "question-dialog-canary"
)

// wireDialogCanary installs the canary once and returns it.
func (s *SessionService) wireDialogCanary() *DialogMatchCanary {
	if c := s.dialogCanary.Load(); c != nil {
		return c
	}
	count := func(result DialogMatchResult, version string) {
		if s.deliveryGate != nil {
			s.deliveryGate.Metrics().Add(deliverygate.CounterQuestionDialogMatch, string(result), version)
		}
	}
	notify := func() {
		(&EventBusNotifier{Bus: s.eventBus}).NotifySession(canaryNoticeSession, canaryNoticeTitle, canaryNoticeMessage,
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), false, true)
	}
	c := NewDialogMatchCanary(nil, count, notify)
	if !s.dialogCanary.CompareAndSwap(nil, c) {
		return s.dialogCanary.Load()
	}
	return c
}

// SetGuidanceMirror installs the durable hook-question mirror: the router
// re-registers an expired question through it, a terminal-answer receipt clears
// through it, and a hook create that hits the cap re-verifies through it.
func (s *SessionService) SetGuidanceMirror(m *GuidanceHookMirror) {
	if m != nil {
		m.SetPaneCapture(s.PaneCapture)
		m.SetCardRetirer(historyItemResolver{store: s.GetNotificationStore()}.ResolveGuidanceNotifications)
	}
	s.guidanceMirror.Store(m)
}

// PaneCapture takes one fresh capture of a live session's pane, without the write lease.
func (s *SessionService) PaneCapture(ctx context.Context, sessionUUID string) (string, error) {
	return s.ackCapture(ctx, sessionUUID, false)
}

// ensureRegistered re-registers an expired hook question from its durable row,
// through the explicit mirror when given and otherwise the installed one.
func (s *SessionService) ensureRegistered(explicit *GuidanceHookMirror) func(context.Context, QuestionKey) bool {
	return func(ctx context.Context, key QuestionKey) bool {
		m := explicit
		if m == nil {
			m = s.guidanceMirror.Load()
		}
		return m != nil && m.EnsureRegistered(ctx, key)
	}
}

// deliveryProbes reads unconfirmed answers from the detector and session
// liveness from the live service. acks may be nil.
func (s *SessionService) deliveryProbes(acks *GuidanceAckDetector) DeliveryProbes {
	phase := func(sessionUUID string) answerPhase { return s.answerSessions()(sessionUUID).phase }
	p := DeliveryProbes{
		SessionEnded:    func(sessionUUID string) bool { return phase(sessionUUID) != phaseLive },
		SessionArchived: func(sessionUUID string) bool { return phase(sessionUUID) == phaseArchived },
	}
	if acks != nil {
		p.Unconfirmed = acks.Unconfirmed
	}
	return p
}
