package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// hidden_question_reply.go is the Reply path (ADR-010): one audited, rate-limited
// digit into the AskUserQuestion dialog of a hidden session. The order of the
// request path is: verdict, flag, validate, idempotency lookup, limiters, claim,
// audit, lease, before-first-byte checks, one write, closed check. The limiters,
// claim, audit, lease and write are PaneReplyCore (pane_reply_core.go); this
// file keeps the hidden-only policy: the flag, the NOT_HIDDEN verdict and idempotency.

const hiddenSessionReplyFlagName = "hidden_session_reply"

// ReplyFlag is the live hidden_session_reply value. The zero value is OFF: a
// flag nobody wired, or a config that could not be read, never lets a reply
// through (fail closed).
type ReplyFlag struct{ on atomic.Bool }

// Enabled reports whether Reply is allowed.
func (f *ReplyFlag) Enabled() bool { return f != nil && f.on.Load() }

// SetEnabled flips the flag.
func (f *ReplyFlag) SetEnabled(on bool) { f.on.Store(on) }

// replyFlagAuditPolicy: on loosens (a Reply path opens), so it is audited and
// fsynced before it is persisted and refused if the sink is down; off is a
// tightening flip that is persisted first and queued.
var replyFlagAuditPolicy = FlagAuditPolicy{
	Loosening: func(enabled bool) bool { return enabled },
}

type replyFlagController struct{ flag *ReplyFlag }

func (c replyFlagController) Enable(context.Context) error { c.flag.SetEnabled(true); return nil }
func (c replyFlagController) Disable() error               { c.flag.SetEnabled(false); return nil }
func (c replyFlagController) IsEnabled() bool              { return c.flag.Enabled() }

// startupReplyFlag reads the persisted value strictly: no config file yet is the
// default (on), an explicit value is that value, and a config that exists but
// cannot be read or parsed is OFF.
func startupReplyFlag() bool {
	dir, err := config.GetConfigDir()
	if err != nil {
		return false
	}
	cfg, err := config.LoadConfigFromPath(filepath.Join(dir, config.ConfigFileName))
	switch {
	case err == nil:
		return cfg.GetFeatureFlagWithDefault(hiddenSessionReplyFlagName, true)
	case errors.Is(err, os.ErrNotExist):
		return true
	default:
		return false
	}
}

// wireReplyFlag connects hidden_session_reply to the service's flag atomic.
func (s *SessionService) wireReplyFlag() {
	st := s.replyState()
	st.flag.SetEnabled(startupReplyFlag())
	s.featureFlagSvc.SetFeatureController(hiddenSessionReplyFlagName, replyFlagController{flag: st.flag})
	s.featureFlagSvc.AddStatusDetailSource(hiddenSessionReplyFlagName, func() string {
		if st.flag.Enabled() {
			return ""
		}
		return "Hidden-session Reply is OFF"
	})
}

// replyState is the Reply machinery of one SessionService, built on first use.
type replyState struct {
	store    *PendingQuestionStore
	flights  *replyFlightTable
	flag     *ReplyFlag
	limiters *replyLimiters
	now      func() time.Time
}

func (s *SessionService) replyState() *replyState {
	s.replyOnce.Do(func() {
		now := time.Now
		if s.replyNow != nil {
			now = s.replyNow
		}
		s.reply = &replyState{
			store:    NewPendingQuestionStore(now),
			flights:  newReplyFlightTable(now),
			flag:     &ReplyFlag{},
			limiters: newReplyLimiters(now),
			now:      now,
		}
	})
	return s.reply
}

// PendingQuestions is the registry the approval handler feeds.
func (s *SessionService) PendingQuestions() *PendingQuestionStore { return s.replyState().store }

// onSessionDeleted drops everything Reply keeps for a deleted session.
func (s *SessionService) onSessionDeletedForReply(sessionUUID string) {
	st := s.replyState()
	st.store.SessionDeleted(sessionUUID)
	st.limiters.forget(sessionUUID)
}

// ---- handler ----

// ReplyToPendingQuestion answers one outstanding single-select AskUserQuestion
// in a hidden session with one option digit (ADR-010).
// +api: session:reply-to-question
func (s *SessionService) ReplyToPendingQuestion(
	ctx context.Context,
	req *connect.Request[sessionv1.ReplyToPendingQuestionRequest],
) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	st := s.replyState()
	peer := req.Peer().Addr
	if s.replyPeerAddr != nil {
		peer = s.replyPeerAddr(req)
	}
	refusal, err := EvaluateLocalWrite(ctx, peer, LocalWriteLocalCaller)
	if err != nil {
		s.countReply(replyLabelRefused)
		return nil, err
	}
	facts := steerRequestFacts{refusal: refusal, userAgent: req.Header().Get("User-Agent")}
	if !st.flag.Enabled() {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_DISABLED), nil
	}
	in, err := s.validateReplyRequest(req.Msg)
	if err != nil {
		s.countReply(replyLabelInvalid)
		return nil, err
	}
	inst := s.findInstance(in.sessionID)
	if inst == nil {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING), nil
	}
	in.sessionUUID = inst.GetStableID()
	if AccessFor(inst) != TerminalReadOnly {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN), nil
	}
	key := QuestionKey{SessionUUID: in.sessionUUID, QuestionID: in.questionID}
	if q, ok := st.store.Peek(key); ok && int(in.digit-'0') > len(q.Dialog.Labels) {
		s.countReply(replyLabelInvalid)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("option %c is not available for this question", in.digit))
	}

	flight, role := st.flights.begin(in.replyID, hashReplyPayload(in.sessionUUID, in.questionID, in.digit))
	switch role {
	case replyMismatch:
		s.countReply(replyLabelInvalid)
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("reply_id was already used for a different reply"))
	case replyFollower, replyCached:
		res, werr := flight.wait(ctx)
		if werr != nil {
			return nil, connect.NewError(connect.CodeCanceled, werr)
		}
		return s.replyResult(res)
	}
	res, cache := s.paneReplyCore().Run(ctx, AnswerTargetHidden, in, inst, facts)
	st.flights.finish(in.replyID, flight, res, cache && res.Err == nil)
	return s.replyResult(res)
}

func (s *SessionService) replyResult(res replyOutcomeResult) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	if res.Err != nil {
		return nil, res.Err
	}
	return connect.NewResponse(&sessionv1.ReplyToPendingQuestionResponse{Outcome: res.Outcome, RetryAfterSeconds: res.RetryAfter}), nil
}

// replyEarly answers an outcome decided before any attempt starts.
func (s *SessionService) replyEarly(o sessionv1.ReplyOutcome) *connect.Response[sessionv1.ReplyToPendingQuestionResponse] {
	s.countReply(replyOutcomeLabel(o))
	return connect.NewResponse(&sessionv1.ReplyToPendingQuestionResponse{Outcome: o})
}

func (s *SessionService) validateReplyRequest(m *sessionv1.ReplyToPendingQuestionRequest) (replyInput, error) {
	bad := func(msg string) (replyInput, error) {
		return replyInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
	}
	switch {
	case m.GetSessionId() == "":
		return bad("session_id is required")
	case m.GetQuestionId() == "":
		return bad("question_id is required")
	case m.GetReplyId() == "" || len(m.GetReplyId()) > 128:
		return bad("reply_id is required (at most 128 bytes)")
	case len(m.GetReplyText()) != 1 || m.GetReplyText()[0] < '1' || m.GetReplyText()[0] > '9':
		return bad("reply_text must be exactly one ASCII digit, the option number")
	}
	return replyInput{
		sessionID: m.GetSessionId(), questionID: m.GetQuestionId(), replyID: m.GetReplyId(), digit: m.GetReplyText()[0],
	}, nil
}

// controllerActiveFor reports whether a status controller runs for inst. The
// status value is never consulted: only that the pane is being watched.
func (s *SessionService) controllerActiveFor(inst *session.Instance) bool {
	if s.replyControllerActive != nil {
		return s.replyControllerActive(inst)
	}
	c := inst.GetController()
	return c != nil && c.IsStarted()
}

func (s *SessionService) countReply(label string) {
	if s.deliveryGate != nil {
		s.deliveryGate.Metrics().Add(deliverygate.CounterReply, label)
	}
}
