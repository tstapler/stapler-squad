package services

import (
	"context"
	"errors"

	"github.com/tstapler/stapler-squad/session"
)

// guidance_ack_wiring.go binds the acknowledgement detector to this service's
// live sessions and gives it a goroutine that Shutdown joins.

var errAckSessionGone = errors.New("session is not live")
var errAckLeaseBusy = errors.New("terminal write lease is busy")

// wireGuidanceAcks builds the detector over storage and starts its loop; nil
// storage leaves keystroke answers untracked.
func (s *SessionService) wireGuidanceAcks(storage *session.Storage) *GuidanceAckDetector {
	if storage == nil {
		return nil
	}
	s.stopGuidanceAcks()
	d := NewGuidanceAckDetector(storage, s.ackCapture, func(uuid string) bool {
		return s.answerSessions()(uuid).phase != phaseLive
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.ackCancel, s.ackDone, s.guidanceAcks = cancel, done, d
	go func() {
		defer close(done)
		d.Run(ctx)
	}()
	return d
}

func (s *SessionService) stopGuidanceAcks() {
	if s.ackCancel == nil {
		return
	}
	s.ackCancel()
	<-s.ackDone
	s.ackCancel, s.ackDone, s.guidanceAcks = nil, nil, nil
}

// GuidanceAcks is the detector, or nil before notification actions are wired.
func (s *SessionService) GuidanceAcks() *GuidanceAckDetector { return s.guidanceAcks }

func (s *SessionService) ackCapture(_ context.Context, sessionUUID string, underLease bool) (string, error) {
	inst := s.findInstance(sessionUUID)
	if inst == nil {
		return "", errAckSessionGone
	}
	if underLease {
		lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
		if !ok {
			return "", errAckLeaseBusy
		}
		defer lease.Release()
	}
	return inst.CapturePaneContent()
}

// answerRouterDeps maps the wiring inputs onto the router's. A nil storage or
// detector stays a nil interface so the router sees "not wired".
func answerRouterDeps(w NotificationActionWiring, acks *GuidanceAckDetector) AnswerRouterDeps {
	d := AnswerRouterDeps{Mirror: w.Mirror}
	if w.Storage != nil {
		d.Rows = w.Storage
	}
	if acks != nil {
		d.Acks = acks
	}
	return d
}
