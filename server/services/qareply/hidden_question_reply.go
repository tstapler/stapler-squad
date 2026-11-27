package qareply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
)

// hidden_question_reply.go is the Reply path (ADR-010): one audited, rate-limited
// digit into the AskUserQuestion dialog of a hidden session. The order of the
// request path is: verdict, flag, validate, idempotency lookup, limiters, claim,
// audit, lease, before-first-byte checks, one write, closed check. The limiters,
// claim, audit, lease and write are PaneReplyCore (pane_reply_core.go); this
// file keeps the hidden-only policy: the flag, the NOT_HIDDEN verdict and idempotency.

// ReplyFlag is the live hidden_session_reply value. The zero value is OFF: a
// flag nobody wired, or a config that could not be read, never lets a reply
// through (fail closed).
type ReplyFlag struct{ on atomic.Bool }

// Enabled reports whether Reply is allowed.
func (f *ReplyFlag) Enabled() bool { return f != nil && f.on.Load() }

// SetEnabled flips the flag.
func (f *ReplyFlag) SetEnabled(on bool) { f.on.Store(on) }

// FlagAuditPolicy: on loosens (a Reply path opens), so it is audited and
// fsynced before it is persisted and refused if the sink is down; off is a
// tightening flip that is persisted first and queued.
var FlagAuditPolicy = featureflags.FlagAuditPolicy{
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
		return cfg.GetFeatureFlagWithDefault(featureflags.HiddenSessionReplyFlagName, true)
	case errors.Is(err, os.ErrNotExist):
		return true
	default:
		return false
	}
}

// FlagRegistrar is the part of the feature-flag service the Reply flag is wired to.
type FlagRegistrar interface {
	SetFeatureController(name string, c featureflags.FeatureController)
	AddStatusDetailSource(name string, fn func() string)
}

// WireFlag connects hidden_session_reply to the service's flag atomic.
func (s *Service) WireFlag(flags FlagRegistrar) {
	s.flag.SetEnabled(startupReplyFlag())
	flags.SetFeatureController(featureflags.HiddenSessionReplyFlagName, replyFlagController{flag: s.flag})
	flags.AddStatusDetailSource(featureflags.HiddenSessionReplyFlagName, func() string {
		if s.flag.Enabled() {
			return ""
		}
		return "Hidden-session Reply is OFF"
	})
}

// ---- handler ----

// Reply answers one outstanding single-select AskUserQuestion in a hidden
// session with one option digit (ADR-010): the body of ReplyToPendingQuestion.
func (s *Service) Reply(
	ctx context.Context,
	req *connect.Request[sessionv1.ReplyToPendingQuestionRequest],
) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	peer := req.Peer().Addr
	if s.peerAddr != nil {
		peer = s.peerAddr(req)
	}
	refusal, err := access.EvaluateLocalWrite(ctx, peer, access.LocalWriteLocalCaller)
	if err != nil {
		s.ports.Count(replyLabelRefused)
		return nil, err
	}
	facts := access.SteerRequestFacts{Refusal: refusal, UserAgent: req.Header().Get("User-Agent")}
	if !s.flag.Enabled() {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_DISABLED), nil
	}
	in, err := s.validateReplyRequest(req.Msg)
	if err != nil {
		s.ports.Count(replyLabelInvalid)
		return nil, err
	}
	inst := s.ports.FindInstance(in.sessionID)
	if inst == nil {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING), nil
	}
	in.sessionUUID = inst.GetStableID()
	// The same check the answer router uses: a retired session takes no keystroke.
	if inst.IsArchived() {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING), nil
	}
	if access.AccessFor(inst) != access.TerminalReadOnly {
		return s.replyEarly(sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN), nil
	}
	key := questionkit.QuestionKey{SessionUUID: in.sessionUUID, QuestionID: in.questionID}
	if q, ok := s.store.Peek(key); ok && int(in.digit-'0') > len(q.Dialog.Labels) {
		s.ports.Count(replyLabelInvalid)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("option %c is not available for this question", in.digit))
	}

	flight, role := s.flights.Begin(in.replyID, questionkit.HashReplyPayload(in.sessionUUID, in.questionID, in.digit))
	switch role {
	case questionkit.ReplyMismatch:
		s.ports.Count(replyLabelInvalid)
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("reply_id was already used for a different reply"))
	case questionkit.ReplyFollower, questionkit.ReplyCached:
		res, werr := flight.Wait(ctx)
		if werr != nil {
			return nil, connect.NewError(connect.CodeCanceled, werr)
		}
		return s.replyResult(res)
	}
	res, cache := s.paneReplyCore().Run(ctx, AnswerTargetHidden, in, inst, facts)
	s.flights.Finish(in.replyID, flight, res, cache && res.Err == nil)
	return s.replyResult(res)
}

func (s *Service) replyResult(res questionkit.ReplyOutcomeResult) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	if res.Err != nil {
		return nil, res.Err
	}
	return connect.NewResponse(&sessionv1.ReplyToPendingQuestionResponse{Outcome: res.Outcome, RetryAfterSeconds: res.RetryAfter}), nil
}

// replyEarly answers an outcome decided before any attempt starts.
func (s *Service) replyEarly(o sessionv1.ReplyOutcome) *connect.Response[sessionv1.ReplyToPendingQuestionResponse] {
	s.ports.Count(replyOutcomeLabel(o))
	return connect.NewResponse(&sessionv1.ReplyToPendingQuestionResponse{Outcome: o})
}

func (s *Service) validateReplyRequest(m *sessionv1.ReplyToPendingQuestionRequest) (replyInput, error) {
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
