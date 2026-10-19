package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// pane_reply_core.go is the claim, audit, write-lease, DialogMatch and
// one-write core of the Reply path (ADR-010), separated from the hidden-only
// policy that stays in hidden_question_reply.go so visible-session answers
// reuse it. The order is: limiters, claim, audit, lease, before-first-byte
// checks, one write, closed check.

// Operator rails, in one place: 1 reply per 5 s per session, 10 per minute overall.
const (
	replyPerSessionInterval = 5 * time.Second // burst 1
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
	replyStageTargetMismatch  = "target_mismatch"
	replyStageClaimDeleted    = "claim_deleted"
	replyStageLeaseBusy       = "lease_busy"
	replyStageNotSent         = "not_sent"
	replyStageIndeterminate   = "write_indeterminate"
)

// AnswerTarget is the kind of session a pane answer may be written to. The
// zero value admits nothing, so an unset target fails closed.
type AnswerTarget uint8

const (
	// AnswerTargetHidden: a background (read-only) session, the ADR-010 Reply path.
	AnswerTargetHidden AnswerTarget = iota + 1
	// AnswerTargetVisible: a visible (read-write) session.
	AnswerTargetVisible
)

func (t AnswerTarget) admits(a TerminalAccess) bool {
	switch t {
	case AnswerTargetHidden:
		return a == TerminalReadOnly
	case AnswerTargetVisible:
		return a == TerminalReadWrite
	}
	return false
}

// mismatchOutcome is what a session of the wrong kind for t returns. Nothing was written.
func (t AnswerTarget) mismatchOutcome() (sessionv1.ReplyOutcome, string) {
	if t == AnswerTargetHidden {
		return sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN, replyStageNotHidden
	}
	return sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT, replyStageTargetMismatch
}

// errAnswerTargetMismatch is returned when the session is not the kind the target admits.
var errAnswerTargetMismatch = errors.New("session is not the kind of answer target requested")

// replyOutcomeResult is what a reply attempt returns and what the idempotency
// table caches.
type replyOutcomeResult struct {
	Outcome    sessionv1.ReplyOutcome
	RetryAfter int32
	// Err is a Connect error the leader returned; followers return it too.
	Err error
}

// replyLimiters are the operator rails. They are checked with Reserve so a NOT_SENT can cancel.
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

// ---- capability ----

// QuestionReplyWriter is the capability of the Reply path: one method, only
// obtainable with a PendingQuestionClaim, and only for a session of the kind
// its AnswerTarget admits.
type QuestionReplyWriter struct {
	claim PendingQuestionClaim
	inst  *session.Instance
	after func(time.Duration) <-chan time.Time
}

// QuestionReplyWriter returns the writer for claim on inst, or ErrReadOnly for
// a zero claim (never claimed) and errAnswerTargetMismatch when this access is
// not the kind target admits.
func (a TerminalAccess) QuestionReplyWriter(target AnswerTarget, claim PendingQuestionClaim, inst *session.Instance, after func(time.Duration) <-chan time.Time) (*QuestionReplyWriter, error) {
	if !claim.Valid() || inst == nil {
		return nil, ErrReadOnly
	}
	if !target.admits(a) {
		return nil, errAnswerTargetMismatch
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

// ---- core ----

// PaneReplyCore runs one audited, rate-limited digit into a dialog. It holds
// no policy about which sessions may be answered: the caller passes an
// AnswerTarget and the core refuses a session of the other kind with zero writes.
type PaneReplyCore struct {
	store            *PendingQuestionStore
	limiters         *replyLimiters
	auditSink        func() *AuditSink
	count            func(label string)
	after            func(time.Duration) <-chan time.Time
	controllerActive func(*session.Instance) bool
}

// paneReplyCore builds the core from the service's current seams.
func (s *SessionService) paneReplyCore() *PaneReplyCore {
	st := s.replyState()
	return &PaneReplyCore{
		store: st.store, limiters: st.limiters,
		auditSink: s.auditSink, count: s.countReply,
		after: s.replyAfter, controllerActive: s.controllerActiveFor,
	}
}

// replyInput is one validated reply request.
type replyInput struct {
	sessionID   string
	sessionUUID string
	questionID  string
	replyID     string
	digit       byte
}

// Run is the leader's path: limiters, claim, audit, lease, write. cache is
// true when the result must be returned to a Retry instead of attempted again.
func (c *PaneReplyCore) Run(ctx context.Context, target AnswerTarget, in replyInput, inst *session.Instance, facts steerRequestFacts) (res replyOutcomeResult, cache bool) {
	key := QuestionKey{SessionUUID: in.sessionUUID, QuestionID: in.questionID}
	done := func(o sessionv1.ReplyOutcome) replyOutcomeResult {
		c.count(replyOutcomeLabel(o))
		return replyOutcomeResult{Outcome: o}
	}
	reservation, retryAfter, ok := c.limiters.reserve(in.sessionUUID)
	if !ok {
		secs := int32((retryAfter + time.Second - 1) / time.Second)
		c.count(replyOutcomeLabel(sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED))
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED, RetryAfter: max(secs, 1)}, false
	}
	claim, cr := c.store.Claim(key)
	if cr != ClaimOK {
		reservation.cancel()
		return done(sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING), false
	}
	line := c.auditLine(in, claim, inst, facts)
	if aerr := c.appendRequest(ctx, line); aerr != nil {
		c.store.Release(claim)
		reservation.cancel()
		c.count(replyLabelAuditFailed)
		log.Error("[Reply] audit append failed; nothing written", "session", in.sessionUUID, "err", aerr)
		return replyOutcomeResult{Err: connect.NewError(connect.CodeInternal, errors.New("audit log unavailable, reply not sent"))}, false
	}
	res, stage, consumed := c.writeUnderLease(ctx, target, in, claim, inst)
	if consumed {
		c.store.Consume(claim)
	} else {
		c.store.Release(claim)
		reservation.cancel()
	}
	c.appendResult(line, res, stage)
	c.count(replyOutcomeLabel(res.Outcome))
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

// writeUnderLease is the Reply chain's acquirer: it takes the lease, runs the
// before-first-byte checks and the one write. consumed is true when the digit
// may have reached the pane.
func (c *PaneReplyCore) writeUnderLease(ctx context.Context, target AnswerTarget, in replyInput, claim PendingQuestionClaim, inst *session.Instance) (res replyOutcomeResult, stage string, consumed bool) {
	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
	if !ok {
		return replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT}, replyStageLeaseBusy, false
	}
	writer, werr := AccessFor(inst).QuestionReplyWriter(target, claim, inst, c.after)
	if werr != nil {
		lease.Release()
		out := sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT
		stage = replyStageNotSent
		if errors.Is(werr, errAnswerTargetMismatch) {
			out, stage = target.mismatchOutcome()
		}
		return replyOutcomeResult{Outcome: out}, stage, false
	}
	rs, err := writer.SubmitReply(ctx, lease, in.digit, c.beforeFirstByteChecks(target, claim, inst))
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
// still the target kind with the claim's UUID and not claimed-deleted; a status
// controller is active (its value is never read: it is blank for 6-9 s after any
// key and cannot tell a question from a permission dialog); and the structural
// match of a fresh live capture, which is authoritative.
func (c *PaneReplyCore) beforeFirstByteChecks(target AnswerTarget, claim PendingQuestionClaim, inst *session.Instance) func() error {
	return func() error {
		if !target.admits(AccessFor(inst)) {
			out, stage := target.mismatchOutcome()
			return &replyWriteError{out, stage}
		}
		if inst.GetStableID() != claim.SessionUUID() || claim.Deleted() {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, replyStageClaimDeleted}
		}
		if !c.controllerActive(inst) {
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

// ---- audit and metrics ----

func (c *PaneReplyCore) auditLine(in replyInput, claim PendingQuestionClaim, inst *session.Instance, facts steerRequestFacts) AuditLine {
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

// appendRequest is the durable pre-write line: no audit, no write.
func (c *PaneReplyCore) appendRequest(ctx context.Context, line AuditLine) error {
	line.Phase = auditPhaseRequest
	sink := c.auditSink()
	if sink == nil {
		return ErrAuditFailed
	}
	return sink.AppendBounded(ctx, line)
}

// appendResult records the outcome after the write; a fault here cannot
// undo anything, so it logs.
func (c *PaneReplyCore) appendResult(line AuditLine, res replyOutcomeResult, stage string) {
	line.Phase, line.Outcome, line.Stage = auditPhaseResult, replyOutcomeLabel(res.Outcome), stage
	sink := c.auditSink()
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
