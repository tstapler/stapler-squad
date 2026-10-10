package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session"
)

// ErrSteerPaneOwnership marks a SteerFailed caused by the pre-write pane
// ownership check (the tmux target no longer belongs to the session), as
// opposed to a failed write. Nothing was written.
var ErrSteerPaneOwnership = errors.New("pane ownership verification failed")

// notReadyReason says why an instance cannot take an unattended PTY write.
type notReadyReason int

const (
	notReadyNone notReadyReason = iota
	// notReadyNoStatusSource: no status manager or no active controller, so idleness cannot be confirmed.
	notReadyNoStatusSource
	// notReadyBusy: a command is queued or running, or the pane is not at a safe idle prompt.
	notReadyBusy
)

// SteerOutcome is the typed result of a guarded steer.
type SteerOutcome int

const (
	// SteerUnspecified is the zero value so an unset outcome never reads as delivered.
	SteerUnspecified SteerOutcome = iota
	SteerDelivered
	// SteerGuardBusy: another delivery to the session is in flight.
	SteerGuardBusy
	// SteerDuplicate: the same reason signature was delivered moments ago.
	SteerDuplicate
	// SteerCoolingDown: the same reason signature just failed to deliver; retry shortly.
	SteerCoolingDown
	// SteerBusy: the session is not idle.
	SteerBusy
	// SteerNoStatusSource: no active controller/status source to confirm idleness.
	SteerNoStatusSource
	// SteerNotTracked: no live instance for the session.
	SteerNotTracked
	// SteerFailed: the write or the pre-write pane-ownership check failed; see the error.
	SteerFailed
)

func (o SteerOutcome) String() string {
	switch o {
	case SteerUnspecified:
		return "unspecified"
	case SteerDelivered:
		return "delivered"
	case SteerGuardBusy:
		return "guard_busy"
	case SteerDuplicate:
		return "duplicate"
	case SteerCoolingDown:
		return "cooling_down"
	case SteerBusy:
		return "busy"
	case SteerNoStatusSource:
		return "no_status_source"
	case SteerNotTracked:
		return "not_tracked"
	case SteerFailed:
		return "failed"
	default:
		return fmt.Sprintf("SteerOutcome(%d)", int(o))
	}
}

// guardedSteerState is SessionService's single field for guarded steering.
// The nil hooks are production behavior; tests inject fakes and a clock.
type guardedSteerState struct {
	guard      sessionNudgeGuard
	ready      func(*session.Instance) notReadyReason
	verifyPane func(context.Context, *session.Instance) error
	write      func(context.Context, *session.Instance, string) error
}

// instanceReadyForSteer is the single idle gate shared by IsReadyForSteer and
// SteerInstanceGuarded. Anything that cannot be confirmed idle is not ready.
func (s *SessionService) instanceReadyForSteer(inst *session.Instance) notReadyReason {
	if h := s.guardedSteer.ready; h != nil {
		return h(inst)
	}
	if s.statusManager == nil {
		return notReadyNoStatusSource
	}
	info := s.statusManager.GetStatus(inst)
	if !info.IsControllerActive {
		return notReadyNoStatusSource
	}
	if info.QueuedCommands > 0 {
		return notReadyBusy
	}
	if ctrl, ok := s.statusManager.GetController(inst.Snapshot().Title); ok && ctrl != nil && ctrl.GetCurrentCommand() != nil {
		return notReadyBusy
	}
	if !isSafeSteerStatus(info.ClaudeStatus, info.StatusContext) {
		return notReadyBusy
	}
	return notReadyNone
}

// InstanceReadyForSteer implements PRNudger: true only when the shared idle
// gate confirms inst is safe to write to.
func (s *SessionService) InstanceReadyForSteer(inst *session.Instance) bool {
	return s.instanceReadyForSteer(inst) == notReadyNone
}

// SteerInstanceGuarded delivers msg to inst under the per-session nudge guard
// shared by the manual nudge RPC and PR-fix auto-steer. Order: guard ->
// idle gate -> pane-ownership verification -> write. Pane ownership is
// verified unconditionally and immediately before the write because
// steerInstance does not do it itself (tmux-name collision, ce71ad1a).
// Not-delivered outcomes other than SteerFailed return a nil error.
func (s *SessionService) SteerInstanceGuarded(ctx context.Context, inst *session.Instance, sig, msg string) (SteerOutcome, error) {
	if inst == nil {
		return SteerNotTracked, nil
	}
	id := inst.GetStableID()
	release, outcome := s.guardedSteer.guard.TryBegin(id, sig)
	switch outcome {
	case GuardBusy:
		return SteerGuardBusy, nil
	case GuardDuplicate:
		return SteerDuplicate, nil
	case GuardCoolingDown:
		return SteerCoolingDown, nil
	}
	// A panic in ready/verify/write must not leave the session GuardBusy
	// forever. release is once-guarded, so this is a no-op after a normal
	// release; the not-ready paths abandon instead and set abandoned so no
	// failure cooldown is recorded for a claim that never reached the PTY.
	abandoned := false
	defer func() {
		if !abandoned {
			release(false)
		}
	}()

	switch s.instanceReadyForSteer(inst) {
	case notReadyNoStatusSource:
		s.guardedSteer.guard.abandon(id)
		abandoned = true
		return SteerNoStatusSource, nil
	case notReadyBusy:
		s.guardedSteer.guard.abandon(id)
		abandoned = true
		return SteerBusy, nil
	}

	verify := s.guardedSteer.verifyPane
	if verify == nil {
		verify = session.VerifyPaneOwnershipBeforeWrite
	}
	if err := verify(ctx, inst); err != nil {
		release(false)
		return SteerFailed, fmt.Errorf("verify pane ownership before steering session %q: %w: %w", id, ErrSteerPaneOwnership, err)
	}

	write := s.guardedSteer.write
	if write == nil {
		write = s.steerInstance
	}
	if err := write(ctx, inst, msg); err != nil {
		if errors.Is(err, session.ErrLeaseBusy) {
			// Another writer holds the pane: nothing reached it, so no failure
			// cooldown (same as the not-ready paths above).
			s.guardedSteer.guard.abandon(id)
			abandoned = true
			return SteerBusy, nil
		}
		release(false)
		return SteerFailed, err
	}
	release(true)
	return SteerDelivered, nil
}

// SteerSessionGuarded implements SessionSteerer for the UUID-only backlog
// auto-steer path, resolving the live instance once.
func (s *SessionService) SteerSessionGuarded(ctx context.Context, sessionUUID, sig, msg string) (SteerOutcome, error) {
	return s.SteerInstanceGuarded(ctx, s.FindLiveInstance(sessionUUID), sig, msg)
}
