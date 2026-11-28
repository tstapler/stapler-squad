package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/githubrpc"
	"github.com/tstapler/stapler-squad/session/instance"
)

var _ githubrpc.PRNudger = (*SessionService)(nil)

// notReadyReason says why an instance cannot take an unattended PTY write.
type notReadyReason int

const (
	notReadyNone notReadyReason = iota
	// notReadyNoStatusSource: no status manager or no active controller, so idleness cannot be confirmed.
	notReadyNoStatusSource
	// notReadyBusy: a command is queued or running, or the pane is not at a safe idle prompt.
	notReadyBusy
)

// guardedSteerState is SessionService's single field for guarded steering.
// The nil hooks are production behavior; tests inject fakes and a clock.
type guardedSteerState struct {
	guard      access.SessionNudgeGuard
	ready      func(*instance.Instance) notReadyReason
	verifyPane func(context.Context, *instance.Instance) error
	write      func(context.Context, *instance.Instance, string) error
}

// instanceReadyForSteer is the single idle gate shared by IsReadyForSteer and
// SteerInstanceGuarded. Anything that cannot be confirmed idle is not ready.
func (s *SessionService) instanceReadyForSteer(inst *instance.Instance) notReadyReason {
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

// InstanceReadyForSteer implements githubrpc.PRNudger: true only when the shared idle
// gate confirms inst is safe to write to.
func (s *SessionService) InstanceReadyForSteer(inst *instance.Instance) bool {
	return s.instanceReadyForSteer(inst) == notReadyNone
}

// SteerInstanceGuarded delivers msg to inst under the per-session nudge guard
// shared by the manual nudge RPC and PR-fix auto-steer. Order: guard ->
// idle gate -> pane-ownership verification -> write. Pane ownership is
// verified unconditionally and immediately before the write because
// steerInternal does not do it itself (tmux-name collision, ce71ad1a).
// Not-delivered outcomes other than SteerFailed return a nil error.
func (s *SessionService) SteerInstanceGuarded(ctx context.Context, inst *instance.Instance, sig, msg string) (access.SteerOutcome, error) {
	if inst == nil {
		return access.SteerNotTracked, nil
	}
	id := inst.GetStableID()
	release, outcome := s.guardedSteer.guard.TryBegin(id, sig)
	switch outcome {
	case access.GuardBusy:
		return access.SteerGuardBusy, nil
	case access.GuardDuplicate:
		return access.SteerDuplicate, nil
	case access.GuardCoolingDown:
		return access.SteerCoolingDown, nil
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
		s.guardedSteer.guard.Abandon(id)
		abandoned = true
		return access.SteerNoStatusSource, nil
	case notReadyBusy:
		s.guardedSteer.guard.Abandon(id)
		abandoned = true
		return access.SteerBusy, nil
	}

	verify := s.guardedSteer.verifyPane
	if verify == nil {
		verify = instance.VerifyPaneOwnershipBeforeWrite
	}
	if err := verify(ctx, inst); err != nil {
		release(false)
		return access.SteerFailed, fmt.Errorf("verify pane ownership before steering session %q: %w: %w", id, access.ErrSteerPaneOwnership, err)
	}

	write := s.guardedSteer.write
	if write == nil {
		write = s.steerInternal
	}
	if err := write(ctx, inst, msg); err != nil {
		if errors.Is(err, instance.ErrLeaseBusy) {
			// Another writer holds the pane: nothing reached it, so no failure
			// cooldown (same as the not-ready paths above).
			s.guardedSteer.guard.Abandon(id)
			abandoned = true
			return access.SteerBusy, nil
		}
		release(false)
		return access.SteerFailed, err
	}
	release(true)
	return access.SteerDelivered, nil
}

// SteerSessionGuarded implements SessionSteerer for the UUID-only backlog
// auto-steer path, resolving the live instance once.
func (s *SessionService) SteerSessionGuarded(ctx context.Context, sessionUUID, sig, msg string) (access.SteerOutcome, error) {
	return s.SteerInstanceGuarded(ctx, s.FindLiveInstance(sessionUUID), sig, msg)
}
