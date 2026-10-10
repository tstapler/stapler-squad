package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// This file is the second UI exception to "no UI path writes to a hidden
// session" (operator decision O7): the backlog composer's Steer of a live
// review session. It is typed (BacklogReviewLink), audited (pre-write line,
// outcome line) and decided on durable state, never on a tag.

const steerPreviewRunes = 80

var (
	// errSteerAuditUnavailable maps to Internal: nothing was written.
	errSteerAuditUnavailable = errors.New("audit log unavailable, steer not sent")
	errSteerInvalid          = errors.New("invalid steer message")
	errSteerNoLink           = errors.New("this background session has no live backlog review link")
	errSteerNotAlone         = errors.New("steer_must_be_alone: a background session accepts a steer message with no other field")
)

// BacklogLinkResolver decides whether a hidden session is a live backlog
// review session. ok is false (with a nil error) when it is not.
type BacklogLinkResolver interface {
	ResolveLiveReviewLink(ctx context.Context, inst *session.Instance) (link BacklogReviewLink, ok bool, err error)
}

// BacklogReviewLink proves the durable predicate held when it was built: the
// newest item_session row of the session is an un-ended review row and the
// instance is live. It has unexported fields and one constructor, in this file
// (guard check (a)).
type BacklogReviewLink struct {
	inst   *session.Instance
	itemID string
}

type itemSessionLookup interface {
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (session.ItemSessionSummary, error)
}

// storageLinkResolver implements the predicate over the item_session table. A
// nil lookup (a service built without storage) never links.
type storageLinkResolver struct{ lookup itemSessionLookup }

func (r storageLinkResolver) ResolveLiveReviewLink(ctx context.Context, inst *session.Instance) (BacklogReviewLink, bool, error) {
	if r.lookup == nil || inst == nil || !inst.Started() || inst.Snapshot().Status.IsSuspended() {
		return BacklogReviewLink{}, false, nil
	}
	row, err := r.lookup.GetItemSessionBySessionUUID(ctx, inst.GetStableID())
	switch {
	case errors.Is(err, session.ErrNotFound):
		return BacklogReviewLink{}, false, nil
	case err != nil:
		return BacklogReviewLink{}, false, err
	}
	if row.Role != session.SessionRoleReview || row.EndedAt != nil {
		return BacklogReviewLink{}, false, nil
	}
	if err := inst.VerifyPaneOwner(ctx); err != nil {
		return BacklogReviewLink{}, false, nil
	}
	return BacklogReviewLink{inst: inst, itemID: row.BacklogItemID}, true, nil
}

// steerRequestFacts are the per-request facts an audit line records, carried
// to the writer through the context.
type steerRequestFacts struct {
	refusal   LocalWriteRefusal
	userAgent string
}

type steerFactsKey struct{}

func withSteerRequestFacts(ctx context.Context, f steerRequestFacts) context.Context {
	return context.WithValue(ctx, steerFactsKey{}, f)
}

func steerRequestFactsFrom(ctx context.Context) steerRequestFacts {
	f, _ := ctx.Value(steerFactsKey{}).(steerRequestFacts)
	return f
}

// backlogSteerRunner is what BacklogSteerWriter calls; *SessionService.
type backlogSteerRunner interface {
	steerHiddenReviewViaBacklogLink(ctx context.Context, link BacklogReviewLink, msg string) error
}

// BacklogSteerWriter is the capability of the O7 steer: one method, only
// obtainable with a BacklogReviewLink.
type BacklogSteerWriter struct {
	link BacklogReviewLink
	run  backlogSteerRunner
}

// BacklogSteerWriter returns the writer for link, or ErrReadOnly for a zero
// link (a link nobody resolved).
func (a TerminalAccess) BacklogSteerWriter(link BacklogReviewLink, run backlogSteerRunner) (*BacklogSteerWriter, error) {
	if link.inst == nil || run == nil {
		return nil, ErrReadOnly
	}
	return &BacklogSteerWriter{link: link, run: run}, nil
}

// Steer validates text and runs the audited chain.
func (w *BacklogSteerWriter) Steer(ctx context.Context, text string) error {
	if err := validateBacklogSteerText(text); err != nil {
		return err
	}
	return w.run.steerHiddenReviewViaBacklogLink(ctx, w.link, text)
}

// validateBacklogSteerText applies the shared single-line rules minus LF and
// TAB (ADR-010 decision 5): no C0 or C1 control character, no U+2028/9, no
// format character except the two joiners.
func validateBacklogSteerText(text string) error {
	if len(text) > session.MaxSteerMessageLength {
		return fmt.Errorf("%w: exceeds maximum length of %d bytes", errSteerInvalid, session.MaxSteerMessageLength)
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("%w: not valid UTF-8", errSteerInvalid)
	}
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
		case unicode.IsControl(r), r == 0x2028, r == 0x2029:
			return fmt.Errorf("%w: contains a control character (U+%04X)", errSteerInvalid, r)
		case unicode.Is(unicode.Cf, r) && r != 0x200C && r != 0x200D:
			return fmt.Errorf("%w: contains a format character (U+%04X)", errSteerInvalid, r)
		}
	}
	return nil
}

func (s *SessionService) countBacklogSteer(outcome string) {
	if s.deliveryGate != nil {
		s.deliveryGate.Metrics().Add(deliverygate.CounterBacklogSteer, outcome)
	}
}

func steerPreview(msg string) string {
	if utf8.RuneCountInString(msg) <= steerPreviewRunes {
		return msg
	}
	return string([]rune(msg)[:steerPreviewRunes])
}

func (s *SessionService) steerAuditLine(ctx context.Context, inst *session.Instance, msg string) AuditLine {
	f := steerRequestFactsFrom(ctx)
	sum := sha256.Sum256([]byte(msg))
	snap := inst.Snapshot()
	return AuditLine{
		ChangeID:    uuid.NewString(),
		SessionUUID: inst.GetStableID(), SessionTitle: snap.Title,
		MessageLen: len(msg), MessageSHA256: hex.EncodeToString(sum[:]), MessagePreview: steerPreview(msg),
		Listener: f.refusal.Listener, PeerAddr: f.refusal.Peer, Host: f.refusal.Host, Origin: f.refusal.Origin,
		UserAgent: f.userAgent, AuthMode: f.refusal.AuthMode,
		PeerLoopback: boolRef(f.refusal.PeerLoopback), Proxied: boolRef(f.refusal.Proxied),
	}
}

func (s *SessionService) auditSink() *AuditSink {
	if s.featureFlagSvc == nil {
		return nil
	}
	return s.featureFlagSvc.audit
}

// appendSteerRequest writes the durable pre-write line. With the guards on a
// sink fault refuses the steer; with them off the qualifying review steer goes
// through on the degraded WARN record instead (the flag exists to restore it).
func (s *SessionService) appendSteerRequest(ctx context.Context, line AuditLine) error {
	line.Phase = auditPhaseRequest
	sink := s.auditSink()
	var err error
	if sink == nil {
		err = ErrAuditFailed
	} else {
		err = sink.AppendBounded(ctx, line)
	}
	if err == nil {
		return nil
	}
	if s.guardsEnabled() {
		s.countBacklogSteer(steerOutcomeAuditFailed)
		log.Error("[BacklogSteer] audit append failed; steer not sent", "session", line.SessionUUID, "err", err)
		return errSteerAuditUnavailable
	}
	if sink != nil {
		sink.fallback(line, err)
	}
	return nil
}

// appendSteerResult records the outcome after the write. It runs after the
// write, so a fault here cannot undo anything: it falls back to the WARN record.
func (s *SessionService) appendSteerResult(line AuditLine, outcome string) {
	line.Phase, line.Outcome = auditPhaseResult, outcome
	sink := s.auditSink()
	if sink == nil {
		return
	}
	if err := sink.Append(line); err != nil {
		sink.fallback(line, err)
	}
}

func (s *SessionService) guardsEnabled() bool {
	return s.guards == nil || s.guards.GuardsEnabled()
}

// steerHiddenReviewViaBacklogLink is the O7 chain's acquirer: the audit line is
// durable before the lease is taken, the link is re-checked once the lease is
// held (EndedAt is written asynchronously), and steerAuthorized only verifies
// the token.
func (s *SessionService) steerHiddenReviewViaBacklogLink(ctx context.Context, link BacklogReviewLink, msg string) error {
	inst := link.inst
	line := s.steerAuditLine(ctx, inst, msg)
	line.Kind, line.ItemID = auditKindBacklogSteer, link.itemID
	if err := s.appendSteerRequest(ctx, line); err != nil {
		return err
	}
	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterSteer)
	if !ok {
		s.appendSteerResult(line, steerOutcomeAborted)
		return fmt.Errorf("steer session %q: %w", inst.Title, session.ErrLeaseBusy)
	}
	if _, stillLinked, err := s.backlogLinks.ResolveLiveReviewLink(ctx, inst); err != nil || !stillLinked {
		lease.Release()
		s.countBacklogSteer(steerOutcomeNoLink)
		s.appendSteerResult(line, steerOutcomeAborted)
		return errSteerNoLink
	}
	if err := s.steerBacklogLinked(ctx, link, lease, msg); err != nil {
		s.countBacklogSteer(steerOutcomeFailed)
		s.appendSteerResult(line, steerOutcomeFailed)
		return err
	}
	s.countBacklogSteer(steerOutcomeSent)
	s.appendSteerResult(line, steerOutcomeSent)
	return nil
}
