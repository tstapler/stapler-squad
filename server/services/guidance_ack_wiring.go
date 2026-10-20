package services

import (
	"context"
	"errors"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
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
	d.SetNotes(historyItemResolver{store: s.GetNotificationStore()})
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

// ObservePostToolUse routes a PostToolUse receipt to the detector. sessionID is
// the hook header value, a session title or UUID.
func (s *SessionService) ObservePostToolUse(ctx context.Context, sessionID, toolName string) {
	if toolName == scheduleWakeupTool {
		s.observeScheduleWakeup(ctx, sessionID)
	}
	if toolName != askUserQuestion {
		return
	}
	inst := s.findInstance(sessionID)
	if inst == nil {
		return
	}
	if s.guidanceAcks != nil {
		s.guidanceAcks.Receipt(ctx, inst.GetStableID(), toolName)
	}
	// Whatever the receipt did not acknowledge was answered in the terminal.
	if m := s.guidanceMirror.Load(); m != nil {
		if err := m.ClearOpenForSession(ctx, inst.GetStableID(), domain.CancelReasonAnsweredInTerminal); err != nil {
			log.Warn("[GuidanceMirror] terminal answer not cleared", "session", inst.GetStableID(), "err", err)
		}
	}
}

// observeScheduleWakeup resolves the hook header (title or UUID) to the stable
// session id before counting, so the log line and the role lookup agree.
func (s *SessionService) observeScheduleWakeup(ctx context.Context, sessionID string) {
	roles := scheduleWakeupRoles{}
	if s.concStorage != nil {
		roles.lookup = s.concStorage
	}
	if inst := s.findInstance(sessionID); inst != nil {
		sessionID = inst.GetStableID()
	}
	observeScheduleWakeup(ctx, roles, sessionID)
}

// wireHookReply installs the pre-answered-question resolver. It stays inert
// while stage:hook_reply is closed (spikeS1HookReplyPassed).
func (s *SessionService) wireHookReply(storage *session.Storage, acks *GuidanceAckDetector) {
	if storage == nil {
		return
	}
	r := &hookReplyResolver{rows: storage, flags: configActionFlags{ev: compiledFlagEvidence}}
	if acks != nil {
		r.acks = acks
	}
	s.hookReply.Store(r)
}

// HookReplier is the AskUserQuestion hook answerer for the hook IPC server. It
// reads the resolver at call time, so it may be installed before actions are wired.
func (s *SessionService) HookReplier() HookReplier { return lazyHookReplier{s} }

type lazyHookReplier struct{ s *SessionService }

func (l lazyHookReplier) Reply(ctx context.Context, sessionUUID string, toolInput map[string]interface{}) *hookipc.HookOutput {
	if r := l.s.hookReply.Load(); r != nil {
		return r.Reply(ctx, sessionUUID, toolInput)
	}
	return nil
}
