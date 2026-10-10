package services

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// The access-decision block of UpdateSession (plan Story 5.2). It runs right
// after the instance is found and before any field is applied, because several
// of them publish in memory at once.

// otherMutatingFields lists the request fields, besides steer_message, that
// would change the session.
func otherMutatingFields(msg *sessionv1.UpdateSessionRequest) []string {
	var out []string
	add := func(present bool, name string) {
		if present {
			out = append(out, name)
		}
	}
	add(msg.Status != nil, "status")
	add(msg.Category != nil, "category")
	add(msg.Title != nil, "title")
	add(msg.Program != nil, "program")
	add(len(msg.Tags) > 0, "tags")
	add(msg.WorkingDir != nil, "working_dir")
	add(msg.RateLimitEnabled != nil, "rate_limit_enabled")
	add(msg.PauseReason != nil, "pause_reason")
	add(msg.AutonomousMode != nil, "autonomous_mode")
	add(msg.Note != nil, "note")
	add(msg.AutoApprove != nil, "auto_approve")
	return out
}

// restartsPane reports whether applying program or auto_approve would restart
// the agent (and type a marker into the new pane), the second route into a
// hidden session's terminal (ADV-N28). A value equal to the current one is a
// no-op in the handler and is not refused here.
func restartsPane(msg *sessionv1.UpdateSessionRequest, inst *session.Instance) bool {
	snap := inst.Snapshot()
	if msg.Program != nil && (*msg.Program == "" || *msg.Program != snap.Program) {
		return true
	}
	return msg.AutoApprove != nil && *msg.AutoApprove != snap.AutoApprove
}

// decideUpdateAccess returns the steer decision for req (zero when there is no
// steer) or the connect error that refuses the whole request.
func (s *SessionService) decideUpdateAccess(ctx context.Context, req *connect.Request[sessionv1.UpdateSessionRequest], inst *session.Instance) (steerDecision, error) {
	msg := req.Msg
	steering := msg.SteerMessage != nil && *msg.SteerMessage != ""
	hidden := inst.Snapshot().Hidden
	if hidden && steering {
		if others := otherMutatingFields(msg); len(others) > 0 {
			return steerDecision{}, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("%w (also set: %v)", errSteerNotAlone, others))
		}
	}
	if hidden && AccessForUnary(inst, s.guards) == TerminalReadOnly && restartsPane(msg, inst) {
		return steerDecision{}, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("this background session is read-only: program and auto_approve changes restart its terminal"))
	}
	if !steering {
		return steerDecision{}, nil
	}
	return s.decideSteerAccess(ctx, req, inst)
}

// runSteer sends msg by the path decideSteerAccess chose: the audited backlog
// writer for the O7 path, the lease-taking visible acquirer otherwise.
func (s *SessionService) runSteer(ctx context.Context, d steerDecision, inst *session.Instance, msg string) error {
	if d.bypass {
		return s.runBypassSteer(ctx, d, inst, msg)
	}
	if !d.typed {
		return s.steerUnderLease(ctx, d.auth, inst, msg)
	}
	w, err := AccessForUnary(inst, s.guards).BacklogSteerWriter(d.link, s)
	if err != nil {
		return fmt.Errorf("backlog steer writer: %w", err)
	}
	return w.Steer(withSteerRequestFacts(ctx, d.facts), msg)
}

// steerErrorToConnect maps a steer failure to its connect code.
func steerErrorToConnect(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, errSteerAuditUnavailable):
		return connect.NewError(connect.CodeInternal, err)
	case errors.Is(err, errSteerInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
}

// runBypassSteer is the guards-off steer of a non-qualifying hidden target: the
// guard_bypass line is durable before the write, and a result line follows.
func (s *SessionService) runBypassSteer(ctx context.Context, d steerDecision, inst *session.Instance, msg string) error {
	line, err := s.auditGuardBypass(ctx, d, inst, msg)
	if err != nil {
		return err
	}
	outcome := steerOutcomeSent
	err = s.steerUnderLease(ctx, d.auth, inst, msg)
	if err != nil {
		outcome = steerOutcomeFailed
	}
	s.appendSteerResult(line, outcome)
	return err
}
