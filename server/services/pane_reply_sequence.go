package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// pane_reply_sequence.go is the multi-key sibling of PaneReplyCore.Run for the
// tabbed multi-question AskUserQuestion (Story 3.5). Same rails and audit
// order (limiters, audit line, lease, before-first-byte checks, write, closed
// check); the write is session.SubmitReplySequence with a frame check before
// every key. It serves visible sessions only; a hidden session keeps the
// single-digit Reply path.

// replyStageAbortedPartial is the audit stage of an answer typed in part.
const replyStageAbortedPartial = "aborted_partial"

// sequenceInput is one validated multi-key answer.
type sequenceInput struct {
	sessionID   string
	sessionUUID string
	questionID  string
	replyID     string
	steps       []session.ReplyStep
	// summary is the audited, truncated text of the answer.
	summary  string
	question string
}

// sequenceOutcome is what a sequence attempt returns.
type sequenceOutcome struct {
	replyOutcomeResult
	// Partial is true when some keys were typed and the answer was not finished.
	Partial bool
	Written int
	Total   int
}

// RunSequence is the leader's path for a multi-key answer. consumed is true
// when a key may have reached the pane.
func (c *PaneReplyCore) RunSequence(ctx context.Context, target AnswerTarget, in sequenceInput, inst *session.Instance, facts steerRequestFacts) sequenceOutcome {
	reservation, retryAfter, ok := c.limiters.reserve(in.sessionUUID)
	if !ok {
		secs := int32((retryAfter + time.Second - 1) / time.Second)
		c.count(replyOutcomeLabel(sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED))
		return sequenceOutcome{replyOutcomeResult: replyOutcomeResult{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED, RetryAfter: max(secs, 1)}}
	}
	line := c.sequenceAuditLine(in, inst, facts)
	if aerr := c.appendRequest(ctx, line); aerr != nil {
		reservation.cancel()
		c.count(replyLabelAuditFailed)
		log.Error("[Reply] audit append failed; nothing written", "session", in.sessionUUID, "err", aerr)
		return sequenceOutcome{replyOutcomeResult: replyOutcomeResult{Err: connect.NewError(connect.CodeInternal, errors.New("audit log unavailable, reply not sent"))}}
	}
	out, stage, consumed := c.writeSequenceUnderLease(ctx, target, in, inst)
	if !consumed {
		reservation.cancel()
	}
	c.appendResult(line, out.replyOutcomeResult, stage)
	c.count(replyOutcomeLabel(out.Outcome))
	log.Info("hidden_session_reply", "outcome", replyOutcomeLabel(out.Outcome), "stage", stage,
		"steps_written", out.Written, "steps_total", out.Total, "session", in.sessionUUID)
	return out
}

func (c *PaneReplyCore) sequenceAuditLine(in sequenceInput, inst *session.Instance, facts steerRequestFacts) AuditLine {
	return AuditLine{
		Kind: auditKindReply, ChangeID: uuid.NewString(),
		SessionUUID: in.sessionUUID, SessionTitle: inst.Snapshot().Title,
		QuestionID: in.questionID, ReplyID: in.replyID, OptionLabel: truncateRunes(in.summary, 80),
		QuestionToken: QuestionToken(in.question),
		Listener:      facts.refusal.Listener, PeerAddr: facts.refusal.Peer, Host: facts.refusal.Host, Origin: facts.refusal.Origin,
		UserAgent: facts.userAgent, AuthMode: facts.refusal.AuthMode,
		PeerLoopback: boolRef(facts.refusal.PeerLoopback), Proxied: boolRef(facts.refusal.Proxied),
	}
}

// writeSequenceUnderLease is the sequence chain's acquirer.
func (c *PaneReplyCore) writeSequenceUnderLease(ctx context.Context, target AnswerTarget, in sequenceInput, inst *session.Instance) (sequenceOutcome, string, bool) {
	notSent := func(o sessionv1.ReplyOutcome, stage string) (sequenceOutcome, string, bool) {
		return sequenceOutcome{replyOutcomeResult: replyOutcomeResult{Outcome: o}}, stage, false
	}
	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
	if !ok {
		return notSent(sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT, replyStageLeaseBusy)
	}
	if !target.admits(AccessFor(inst)) {
		lease.Release()
		o, stage := target.mismatchOutcome()
		return notSent(o, stage)
	}
	res := session.SubmitReplySequence(ctx, inst, lease, in.steps, session.SequenceOptions{
		StillOpen:       session.TabbedStillOpen,
		BeforeFirstByte: c.sequenceChecks(target, in, inst),
		After:           c.after,
	})
	return mapSequenceResult(res)
}

// sequenceChecks run under the lease before the first key: still the target
// kind, the session is the question's, and a status controller is active. The
// first step's frame check is the structural match.
func (c *PaneReplyCore) sequenceChecks(target AnswerTarget, in sequenceInput, inst *session.Instance) func() error {
	return func() error {
		if !target.admits(AccessFor(inst)) {
			out, stage := target.mismatchOutcome()
			return &replyWriteError{out, stage}
		}
		if inst.GetStableID() != in.sessionUUID {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, replyStageClaimDeleted}
		}
		if !c.controllerActive(inst) {
			return &replyWriteError{sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING, replyStageNotWaiting}
		}
		return nil
	}
}

// mapSequenceResult turns a driver result into the reply outcome vocabulary:
// NOT_SENT only when no key was handed over, SEND_INDETERMINATE when a key may
// have landed, STALE_PROMPT when a frame check failed (Partial when keys were
// already typed).
func mapSequenceResult(r session.SequenceResult) (sequenceOutcome, string, bool) {
	out := sequenceOutcome{Written: r.Written, Total: r.Total, Partial: r.Partial()}
	set := func(o sessionv1.ReplyOutcome) { out.Outcome = o }
	var refused *replyWriteError
	switch {
	case r.Stage == session.StageDialogClosed:
		set(sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT)
		return out, replyStageDialogClosed, true
	case r.Stage == session.StageDialogStillOpen:
		set(sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE)
		return out, replyStageDialogStillOpen, true
	case r.Stale && r.Written > 0:
		set(sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT)
		return out, replyStageAbortedPartial, true
	case r.Stage == session.StageWriteEntered:
		set(sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE)
		return out, replyStageIndeterminate, true
	case r.Stale:
		set(sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT)
		return out, replyStageStalePrompt, false
	case errors.As(r.Err, &refused):
		set(refused.outcome)
		return out, refused.stage, false
	}
	set(sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT)
	return out, replyStageNotSent, false
}

// sequenceSummary is the audited text of picks, one question per segment.
func sequenceSummary(picks [][]string) string {
	parts := make([]string, len(picks))
	for i, p := range picks {
		parts[i] = strings.Join(p, ", ")
	}
	return strings.Join(parts, " | ")
}
