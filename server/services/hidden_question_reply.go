package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// hidden_question_reply.go is the Reply path (ADR-010): one audited, rate-limited
// digit into the AskUserQuestion dialog of a hidden session. The order of the
// request path is: verdict, flag, validate, idempotency lookup, limiters, claim,
// audit, lease, before-first-byte checks, one write, closed check.

const (
	hiddenSessionReplyFlagName = "hidden_session_reply"

	replyPerSessionInterval = 5 * time.Second // 1 reply per 5s, burst 1
	replyGlobalPerMinute    = 10
	replyLimiterMapMax      = 1024
)

// Metric label values: the lower-cased ReplyOutcome names plus three non-enum ones.
const (
	replyLabelInvalid     = "invalid"
	replyLabelAuditFailed = "audit_failed"
	replyLabelRefused     = "refused"
)

// Audit stages of the result line.
const (
	replyStageDialogClosed    = "dialog_closed"
	replyStageDialogStillOpen = "dialog_still_open"
	replyStageStalePrompt     = "stale_prompt"
	replyStageNotWaiting      = "not_waiting"
	replyStageNotHidden       = "not_hidden"
	replyStageClaimDeleted    = "claim_deleted"
	replyStageLeaseBusy       = "lease_busy"
	replyStageNotSent         = "not_sent"
	replyStageIndeterminate   = "write_indeterminate"
)

// replyOutcomeResult is what a reply attempt returns and what the idempotency
// table caches.
type replyOutcomeResult struct {
	Outcome    sessionv1.ReplyOutcome
	RetryAfter int32
	// Err is a Connect error the leader returned; followers return it too.
	Err error
}

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

// replyLimiters are the operator rails: 1 reply per 5s per session and 10 per
// minute overall. They are checked with Reserve so a NOT_SENT can cancel.
type replyLimiters struct {
	mu      sync.Mutex
	now     func() time.Time
	global  *rate.Limiter
	session map[string]*sessionLimiter
	tick    uint64
}

type sessionLimiter struct {
	l    *rate.Limiter
	used uint64
}

func newReplyLimiters(now func() time.Time) *replyLimiters {
	return &replyLimiters{
		now:     now,
		global:  rate.NewLimiter(rate.Every(time.Minute/replyGlobalPerMinute), replyGlobalPerMinute),
		session: map[string]*sessionLimiter{},
	}
}

// replyReservation is cancellable until the write is attempted.
type replyReservation struct {
	rs  []*rate.Reservation
	now time.Time
}

func (r *replyReservation) cancel() {
	if r == nil {
		return
	}
	for _, rs := range r.rs {
		rs.CancelAt(r.now)
	}
}

// reserve takes one token from the session and the global limiter. ok is false
// (with nothing held) when either would delay; retryAfter is the longer wait.
func (l *replyLimiters) reserve(sessionUUID string) (res *replyReservation, retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.tick++
	sl := l.session[sessionUUID]
	if sl == nil {
		sl = &sessionLimiter{l: rate.NewLimiter(rate.Every(replyPerSessionInterval), 1)}
		l.session[sessionUUID] = sl
		l.evictLocked()
	}
	sl.used = l.tick
	res = &replyReservation{now: now}
	for _, lim := range []*rate.Limiter{sl.l, l.global} {
		r := lim.ReserveN(now, 1)
		res.rs = append(res.rs, r)
		if d := r.DelayFrom(now); !r.OK() || d > 0 {
			if d > retryAfter {
				retryAfter = d
			}
			res.cancel()
			return nil, retryAfter, false
		}
	}
	return res, 0, true
}

func (l *replyLimiters) evictLocked() {
	for len(l.session) > replyLimiterMapMax {
		var oldest string
		var oldestUsed uint64
		for id, sl := range l.session {
			if oldest == "" || sl.used < oldestUsed {
				oldest, oldestUsed = id, sl.used
			}
		}
		delete(l.session, oldest)
	}
}

func (l *replyLimiters) forget(sessionUUID string) {
	l.mu.Lock()
	delete(l.session, sessionUUID)
	l.mu.Unlock()
}

// onSessionDeleted drops everything Reply keeps for a deleted session.
func (s *SessionService) onSessionDeletedForReply(sessionUUID string) {
	st := s.replyState()
	st.store.SessionDeleted(sessionUUID)
	st.limiters.forget(sessionUUID)
}

// ---- capability ----

// errReplyNotHidden is returned when a writer is requested for a visible session.
var errReplyNotHidden = errors.New("reply is only for background sessions: answer in the terminal")

// QuestionReplyWriter is the capability of the Reply path: one method, only
// obtainable with a PendingQuestionClaim, and only for a ReadOnly (hidden) target.
type QuestionReplyWriter struct {
	claim PendingQuestionClaim
	inst  *session.Instance
	after func(time.Duration) <-chan time.Time
}

// QuestionReplyWriter returns the writer for claim on inst, or ErrReadOnly for
// a zero claim (never claimed) and errReplyNotHidden for a visible session.
func (a TerminalAccess) QuestionReplyWriter(claim PendingQuestionClaim, inst *session.Instance, after func(time.Duration) <-chan time.Time) (*QuestionReplyWriter, error) {
	if !claim.Valid() || inst == nil {
		return nil, ErrReadOnly
	}
	if a != TerminalReadOnly {
		return nil, errReplyNotHidden
	}
	return &QuestionReplyWriter{claim: claim, inst: inst, after: after}, nil
}

// SubmitReply writes the digit through session.SubmitReplyOnce under lease. It
// never calls SubmitContentWithEnter: that primitive writes a blind second Enter.
func (w *QuestionReplyWriter) SubmitReply(ctx context.Context, lease *session.HeldLease, digit byte, beforeFirstByte func() error) (session.ReplyStage, error) {
	return session.SubmitReplyOnce(ctx, w.inst, lease, digit, session.ReplyOptions{
		Dialog:          w.claim.Question().Dialog,
		BeforeFirstByte: beforeFirstByte,
		After:           w.after,
	})
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
	res, cache := s.runReply(ctx, in, key, inst, facts)
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

type replyInput struct {
	sessionID   string
	sessionUUID string
	questionID  string
	replyID     string
	digit       byte
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

// runReply is the leader's path: limiters, claim, audit, lease, write. cache is
// true when the result must be returned to a Retry instead of attempted again.
func (s *SessionService) runReply(ctx context.Context, in replyInput, key QuestionKey, inst *session.Instance, facts steerRequestFacts) (res replyOutcomeResult, cache bool) {
	st := s.replyState()
	done := func(o sessionv1.ReplyOutcome) replyOutcomeResult {
		s.countReply(replyOutcomeLabel(o))
		return replyOutcomeResult{Outcome: o}
	}
	reservation, retryAfter, ok := st.limiters.reserve(in.sessionUUID)
	if !ok {
		secs := int32((retryAfter + time.Second - 1) / time.Second)
		s.countReply(replyOutcomeLabel(sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED))
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED, RetryAfter: max(secs, 1)}, false
	}
	claim, cr := st.store.Claim(key)
	if cr != ClaimOK {
		reservation.cancel()
		return done(sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING), false
	}
	line := s.replyAuditLine(in, claim, inst, facts)
	if aerr := s.appendReplyRequest(ctx, line); aerr != nil {
		st.store.Release(claim)
		reservation.cancel()
		s.countReply(replyLabelAuditFailed)
		log.Error("[Reply] audit append failed; nothing written", "session", in.sessionUUID, "err", aerr)
		return replyOutcomeResult{Err: connect.NewError(connect.CodeInternal, errors.New("audit log unavailable, reply not sent"))}, false
	}
	res, stage, consumed := s.replyToPendingQuestion(ctx, in, claim, inst)
	if consumed {
		st.store.Consume(claim)
	} else {
		st.store.Release(claim)
		reservation.cancel()
	}
	s.appendReplyResult(line, res, stage)
	s.countReply(replyOutcomeLabel(res.Outcome))
	log.Info("hidden_session_reply", "outcome", replyOutcomeLabel(res.Outcome), "stage", stage,
		"option_index", int(in.digit-'0'), "question_token", claim.Question().Token, "session", in.sessionUUID)
	return res, consumed
}

// replyWriteError carries the specific outcome of a refused before-first-byte check.
type replyWriteError struct {
	outcome sessionv1.ReplyOutcome
	stage   string
}

func (e *replyWriteError) Error() string { return "reply refused before the first byte: " + e.stage }

// replyToPendingQuestion is the Reply chain's acquirer: it takes the lease, runs the before-first-byte checks and the one
// write. consumed is true when the digit may have reached the pane.
func (s *SessionService) replyToPendingQuestion(ctx context.Context, in replyInput, claim PendingQuestionClaim, inst *session.Instance) (res replyOutcomeResult, stage string, consumed bool) {
	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
	if !ok {
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT}, replyStageLeaseBusy, false
	}
	writer, werr := AccessFor(inst).QuestionReplyWriter(claim, inst, s.replyAfter)
	if werr != nil {
		lease.Release()
		out := sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT
		stage = replyStageNotSent
		if errors.Is(werr, errReplyNotHidden) {
			out, stage = sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN, replyStageNotHidden
		}
		return replyOutcomeResult{Outcome: out}, stage, false
	}
	check := s.beforeFirstByteChecks(claim, inst)
	rs, err := writer.SubmitReply(ctx, lease, in.digit, check)
	return mapReplyStage(rs, err)
}

func mapReplyStage(rs session.ReplyStage, err error) (replyOutcomeResult, string, bool) {
	var refused *replyWriteError
	switch rs {
	case session.StageDialogClosed:
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT}, replyStageDialogClosed, true
	case session.StageDialogStillOpen:
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE}, replyStageDialogStillOpen, true
	case session.StageWriteEntered:
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE}, replyStageIndeterminate, true
	}
	if errors.As(err, &refused) {
		return replyOutcomeResult{Outcome: refused.outcome}, refused.stage, false
	}
	return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT}, replyStageNotSent, false
}

// beforeFirstByteChecks run under the lease, after the audit append, in order:
// still hidden with the claim's UUID and not claimed-deleted; a status
// controller is active (its value is never read: it is blank for 6-9 s after any
// key and cannot tell a question from a permission dialog); and the structural
// match of a fresh live capture, which is authoritative.
func (s *SessionService) beforeFirstByteChecks(claim PendingQuestionClaim, inst *session.Instance) func() error {
	return func() error {
		if AccessFor(inst) != TerminalReadOnly {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN, replyStageNotHidden}
		}
		if inst.GetStableID() != claim.SessionUUID() || claim.Deleted() {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, replyStageClaimDeleted}
		}
		if !s.controllerActiveFor(inst) {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING, replyStageNotWaiting}
		}
		capture, err := inst.CapturePaneContent()
		if err != nil {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING, replyStageNotWaiting}
		}
		if !session.DialogMatch(capture, claim.Question().Dialog) {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT, replyStageStalePrompt}
		}
		return nil
	}
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

// ---- audit and metrics ----

func (s *SessionService) replyAuditLine(in replyInput, claim PendingQuestionClaim, inst *session.Instance, facts steerRequestFacts) AuditLine {
	q := claim.Question()
	idx := int(in.digit - '0')
	label := ""
	if idx >= 1 && idx <= len(q.Dialog.Labels) {
		label = truncateRunes(q.Dialog.Labels[idx-1], 80)
	}
	return AuditLine{
		Kind: auditKindReply, ChangeID: uuid.NewString(),
		SessionUUID: q.SessionUUID, SessionTitle: inst.Snapshot().Title,
		QuestionID: q.QuestionID, ReplyID: in.replyID, OptionIndex: idx, OptionLabel: label, QuestionToken: q.Token,
		Listener: facts.refusal.Listener, PeerAddr: facts.refusal.Peer, Host: facts.refusal.Host, Origin: facts.refusal.Origin,
		UserAgent: facts.userAgent, AuthMode: facts.refusal.AuthMode,
		PeerLoopback: boolRef(facts.refusal.PeerLoopback), Proxied: boolRef(facts.refusal.Proxied),
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// appendReplyRequest is the durable pre-write line: no audit, no write.
func (s *SessionService) appendReplyRequest(ctx context.Context, line AuditLine) error {
	line.Phase = auditPhaseRequest
	sink := s.auditSink()
	if sink == nil {
		return ErrAuditFailed
	}
	return sink.AppendBounded(ctx, line)
}

// appendReplyResult records the outcome after the write; a fault here cannot
// undo anything, so it logs.
func (s *SessionService) appendReplyResult(line AuditLine, res replyOutcomeResult, stage string) {
	line.Phase, line.Outcome, line.Stage = auditPhaseResult, replyOutcomeLabel(res.Outcome), stage
	sink := s.auditSink()
	if sink == nil {
		return
	}
	if err := sink.Append(line); err != nil {
		log.Warn("[Reply] result audit line not written", "change_id", line.ChangeID, "outcome", line.Outcome, "err", err)
	}
}

func replyOutcomeLabel(o sessionv1.ReplyOutcome) string {
	return strings.ToLower(strings.TrimPrefix(o.String(), "REPLY_OUTCOME_"))
}

func (s *SessionService) countReply(label string) {
	if s.deliveryGate != nil {
		s.deliveryGate.Metrics().Add(deliverygate.CounterReply, label)
	}
}
