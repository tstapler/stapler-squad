package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session"
)

// steerAuthKind names which decision produced a steerAuthorization.
type steerAuthKind uint8

const (
	// steerAuthNone is the zero value: a token nobody built, refused at runtime.
	steerAuthNone steerAuthKind = iota
	// steerAuthVisible: the target is writable per AccessForUnary (a visible
	// session, or a hidden one while the guards flag is off).
	steerAuthVisible
	// steerAuthBacklogLink: a hidden review session with a live BacklogReviewLink (O7).
	steerAuthBacklogLink
	// steerAuthInternal: SteerActiveSession and the guarded-steer family,
	// whose callers are pinned by the guard set rather than gated by access.
	steerAuthInternal
)

// steerAuthorization is the token steerAuthorized requires. It carries the
// instance identity it was built for, so a token for one session cannot steer
// another. Every construction is in this file (guard check (a)).
type steerAuthorization struct {
	kind         steerAuthKind
	instanceUUID string
}

var (
	errSteerNoAuthorization  = errors.New("steer is not authorized")
	errSteerAuthTargetDiffer = errors.New("steer authorization was issued for another session")
)

// verify refuses a zero token, a token for another instance and a lease bound
// to another instance. It has no side effects.
func (a steerAuthorization) verify(inst *session.Instance, lease *session.HeldLease) error {
	switch {
	case a.kind == steerAuthNone || a.instanceUUID == "":
		return errSteerNoAuthorization
	case inst == nil || a.instanceUUID != inst.LeaseOwnerUUID():
		return errSteerAuthTargetDiffer
	case lease == nil || lease.OwnerUUID() == "":
		return session.ErrNoLease
	case lease.OwnerUUID() != inst.LeaseOwnerUUID():
		return session.ErrLeaseMismatch
	}
	return nil
}

// internalSteerAuthorization is the one constructor of the internal kind; its
// callers are pinned by the guard set (steerInternal only).
func internalSteerAuthorization(inst *session.Instance) steerAuthorization {
	return steerAuthorization{kind: steerAuthInternal, instanceUUID: inst.LeaseOwnerUUID()}
}

// backlogLinkAuthorization builds the backlog_link kind from a link; the link's
// own constructor already proved the durable predicate.
func backlogLinkAuthorization(link BacklogReviewLink) steerAuthorization {
	return steerAuthorization{kind: steerAuthBacklogLink, instanceUUID: link.inst.LeaseOwnerUUID()}
}

// steerDecision is what the access-decision block hands the steer branch.
type steerDecision struct {
	auth   steerAuthorization
	link   BacklogReviewLink // zero unless the O7 path was chosen
	typed  bool              // the O7 backlog path: audited, through BacklogSteerWriter
	bypass bool              // hidden, non-qualifying, guards flag off: audited guard_bypass
	facts  steerRequestFacts // what the audit lines record about the request
}

// Outcome labels of hidden_session_backlog_steer_total and of the steer audit
// result line.
const (
	steerOutcomeSent        = "sent"
	steerOutcomeRefused     = "refused"
	steerOutcomeNoLink      = "no_link"
	steerOutcomeAuditFailed = "audit_failed"
	steerOutcomeAborted     = "aborted_before_write"
	steerOutcomeGuardBypass = "guard_bypass"
	steerOutcomeFailed      = "failed"
)

// requestFactsOf records the request's raw facts without evaluating a verdict.
func requestFactsOf(ctx context.Context, req connect.AnyRequest) steerRequestFacts {
	peer := req.Peer().Addr
	f := LocalWriteRefusal{Peer: peer, PeerLoopback: middleware.PeerIsLoopback(peer)}
	if rec, ok := RequestRecordFrom(ctx); ok {
		f.Listener, f.AuthMode, f.Host, f.Origin, f.Proxied = rec.Listener, rec.AuthMode, rec.Host, rec.Origin, rec.Proxied
	}
	return steerRequestFacts{refusal: f, userAgent: req.Header().Get("User-Agent")}
}

// decideSteerAccess is the one constructor of the visible and backlog_link
// kinds. It runs in the access-decision block of UpdateSession, before any
// mutation. A hidden target qualifies only through a live BacklogReviewLink
// (never a tag); with the guards flag off a non-qualifying hidden target is
// allowed as on main, marked bypass so the caller audits it.
func (s *SessionService) decideSteerAccess(ctx context.Context, req connect.AnyRequest, inst *session.Instance) (steerDecision, error) {
	if !inst.Snapshot().Hidden {
		return steerDecision{auth: steerAuthorization{kind: steerAuthVisible, instanceUUID: inst.LeaseOwnerUUID()}}, nil
	}
	link, ok, err := s.backlogLinks.ResolveLiveReviewLink(ctx, inst)
	if err != nil {
		return steerDecision{}, connect.NewError(connect.CodeInternal, fmt.Errorf("resolve review link: %w", err))
	}
	if ok {
		refusal, verr := EvaluateLocalWrite(ctx, req.Peer().Addr, LocalWriteRebinding)
		if verr != nil {
			s.countBacklogSteer(steerOutcomeRefused)
			return steerDecision{}, verr
		}
		return steerDecision{
			auth: backlogLinkAuthorization(link), link: link, typed: true,
			facts: steerRequestFacts{refusal: refusal, userAgent: req.Header().Get("User-Agent")},
		}, nil
	}
	if AccessForUnary(inst, s.guards) == TerminalReadWrite {
		// Guards off: allowed as on main, but audited (guard_bypass) before the write.
		return steerDecision{
			auth:   steerAuthorization{kind: steerAuthVisible, instanceUUID: inst.LeaseOwnerUUID()},
			bypass: true,
			facts:  requestFactsOf(ctx, req),
		}, nil
	}
	s.countBacklogSteer(steerOutcomeNoLink)
	return steerDecision{}, connect.NewError(connect.CodeFailedPrecondition,
		errors.New("this background session is read-only; only a live backlog review session can be steered"))
}

// steerUnderLease is the acquirer of the UpdateSession steer chain: it takes
// the write lease once and hands it to steerAuthorized with a token that
// decideSteerAccess built for this instance.
func (s *SessionService) steerUnderLease(ctx context.Context, auth steerAuthorization, instance *session.Instance, message string) error {
	lease, ok := instance.TryTerminalWriteLease(session.LeaseWriterSteer)
	if !ok {
		return fmt.Errorf("steer session %q: %w", instance.Title, session.ErrLeaseBusy)
	}
	return s.steerAuthorized(ctx, auth, lease, instance, message)
}

// steerInternal is the acquirer for in-process callers (SteerActiveSession and
// the guarded-steer family). Their access is pinned by the guard set instead of
// decided per request, so it alone builds the internal kind.
func (s *SessionService) steerInternal(ctx context.Context, instance *session.Instance, message string) error {
	lease, ok := instance.TryTerminalWriteLease(session.LeaseWriterSteer)
	if !ok {
		return fmt.Errorf("steer session %q: %w", instance.Title, session.ErrLeaseBusy)
	}
	return s.steerAuthorized(ctx, internalSteerAuthorization(instance), lease, instance, message)
}

// steerAuthorized injects message into instance's active session. It never
// acquires: it verifies the token (kind set, built for this instance) and the
// lease (held, for this instance), refusing with 0 writes and releasing the
// lease otherwise, then releases it exactly once on every path (the autonomous
// branch's writing goroutine, or SubmitContentWithEnter's). Autonomous sessions
// keep the ClaudeController command-queue path; non-autonomous, Instance-backed
// sessions use the same PTY send primitive the MCP steer_session tool uses.
// Returns only plain fmt.Errorf-wrapped errors, never connect.NewError, since
// SteerActiveSession calls this in-process from BacklogService; UpdateSession is
// the sole caller that translates the error into a connect.Code.
func (s *SessionService) steerAuthorized(ctx context.Context, auth steerAuthorization, lease *session.HeldLease, instance *session.Instance, message string) error {
	if err := auth.verify(instance, lease); err != nil {
		lease.Release()
		return fmt.Errorf("steer session %q: %w", instance.Title, err)
	}
	if instance.AutonomousMode {
		controller := instance.GetController()
		if controller == nil {
			lease.Release()
			return fmt.Errorf("steer autonomous session %q: controller not started", instance.Title)
		}

		// SendCommandImmediate's own ~5min internal timeout doesn't protect
		// against the raw PTY write itself hanging, which would leak
		// steerActiveSessionForPRFix's steerInFlight guard forever.
		errCh := make(chan error, 1)
		go func() {
			defer lease.Release()
			_, sendErr := controller.SendCommandImmediate(message + "\r")
			errCh <- sendErr
		}()

		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		select {
		case sendErr := <-errCh:
			if sendErr != nil {
				return fmt.Errorf("steer autonomous session %q: %w", instance.Title, sendErr)
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("timed out steering autonomous session %q: %w", instance.Title, timeoutCtx.Err())
		}
		s.notifySteerSent(instance, message)
		return nil
	}

	// Non-autonomous sessions get the same PTY send primitive the MCP
	// steer_session tool falls back to (session.SubmitContentWithEnter,
	// bounded with a generous timeout so a browser click against a
	// wedged/dead session can't hang this goroutine forever) — content and
	// the submit keystroke travel as two separate SendKeys writes (BUG-031),
	// never concatenated.
	if err := session.SubmitContentWithEnter(ctx, instance, lease, message); err != nil {
		return fmt.Errorf("steer session %q: %w", instance.Title, err)
	}
	s.notifySteerSent(instance, message)
	return nil
}

// steerBacklogLinked is the O7 chain's hand-off to steerAuthorized: the token
// is built here, in the one file that constructs them, from a resolved link.
func (s *SessionService) steerBacklogLinked(ctx context.Context, link BacklogReviewLink, lease *session.HeldLease, message string) error {
	return s.steerAuthorized(ctx, backlogLinkAuthorization(link), lease, link.inst, message)
}
