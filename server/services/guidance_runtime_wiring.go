package services

import (
	"context"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/server/services/notifyactions"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
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
func (s *SessionService) wireDialogCanary() *questionkit.DialogMatchCanary {
	if c := s.dialogCanary.Load(); c != nil {
		return c
	}
	count := func(result questionkit.DialogMatchResult, version string) {
		if s.deliveryGate != nil {
			s.deliveryGate.Metrics().Add(deliverygate.CounterQuestionDialogMatch, string(result), version)
		}
	}
	notify := func() {
		(&notify.EventBusNotifier{Bus: s.eventBus}).NotifySession(canaryNoticeSession, canaryNoticeTitle, canaryNoticeMessage,
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), false, true)
	}
	c := questionkit.NewDialogMatchCanary(nil, count, notify)
	if !s.dialogCanary.CompareAndSwap(nil, c) {
		return s.dialogCanary.Load()
	}
	return c
}

// SetGuidanceMirror installs the durable hook-question mirror: the router
// re-registers an expired question through it, a terminal-answer receipt clears
// through it, and a hook create that hits the cap re-verifies through it.
func (s *SessionService) SetGuidanceMirror(m *questionkit.GuidanceHookMirror) {
	if m != nil {
		m.SetPaneCapture(s.PaneCapture)
		m.SetCardRetirer(notifyactions.NewNotificationResolver(s.GetNotificationStore()).ResolveGuidanceNotifications)
	}
	s.guidanceMirror.Store(m)
}

// PaneCapture takes one fresh capture of a live session's pane, without the write lease.
func (s *SessionService) PaneCapture(ctx context.Context, sessionUUID string) (string, error) {
	return s.ackCapture(ctx, sessionUUID, false)
}

// GuidanceSeedPorts binds the e2e question seed route to this service's live sessions.
func (s *SessionService) GuidanceSeedPorts() guidance.SeedPorts {
	return guidance.SeedPorts{
		SessionID: func(id string) (string, bool) {
			inst := s.findInstance(id)
			if inst == nil {
				return "", false
			}
			return inst.GetStableID(), true
		},
		Mirror: s.GuidanceMirror,
	}
}

// deliveryProbes reads unconfirmed answers from the detector and session
// liveness from the live service. acks may be nil.
func (s *SessionService) deliveryProbes(acks *guidance.GuidanceAckDetector) guidance.DeliveryProbes {
	replies := s.replyService()
	p := guidance.DeliveryProbes{SessionEnded: replies.SessionEnded, SessionArchived: replies.SessionArchived}
	if acks != nil {
		p.Unconfirmed = acks.Unconfirmed
	}
	return p
}

// GuidanceMirror returns the installed hook mirror, or nil before SetGuidanceMirror.
func (s *SessionService) GuidanceMirror() *questionkit.GuidanceHookMirror {
	return s.guidanceMirror.Load()
}
