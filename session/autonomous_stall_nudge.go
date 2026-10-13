package session

// autonomous_stall_nudge.go is the one session-level writer the autonomous
// stall path uses to nudge an idle work session. Order is verify -> lease ->
// claim -> write, so a failed gate or a busy lease never consumes budget.

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// autonomousStallNudgeText is fixed: no item title or pane content is ever
// interpolated into text typed into a pane.
const autonomousStallNudgeText = "The autonomous driver noticed this session stopped. If the task is finished, call request_review now. If it is not, continue working on it and call report_progress when you reach a milestone. Do not ask for confirmation."

var (
	// ErrNudgeSlotUnavailable means the claim was refused (cap, cooldown or no
	// open row); nothing was written.
	ErrNudgeSlotUnavailable = errors.New("nudge slot unavailable")
	// ErrNudgeSubmitUncertain wraps every submit error after a successful
	// claim: the text may have landed unsubmitted, so the slot stays consumed
	// and callers must leave the pane alone.
	ErrNudgeSubmitUncertain = errors.New("nudge delivery uncertain")
	// ErrNudgeClaimFailed wraps a storage error while claiming the slot; nothing was written.
	ErrNudgeClaimFailed = errors.New("claim nudge slot")
	// ErrNudgeSessionNotActive means the target is paused, stopped or archived;
	// callers must not treat it as a reason to restart anything.
	ErrNudgeSessionNotActive = errors.New("nudge target session is not an active, unarchived session")
)

// nudgeSeams are the test-only substitutions for the writer's real
// dependencies. The wrapper below is the only place that names the lease and
// write primitives, so the delivery guard pins exactly one unit.
type nudgeSeams struct {
	settleWindow time.Duration
	poll         time.Duration
	tryLease     func(writer string) (*HeldLease, bool)
	submit       func(ctx context.Context, inst paneSubmitter, lease *HeldLease, content string, pollInterval, maxWait time.Duration) error
}

// NudgeStalledAutonomousSession types autonomousStallNudgeText into inst after
// the idle, ownership and lease gates pass and claim() grants a nudge slot.
// claim must only touch storage. Returns ErrNudgeTargetNotIdle, ErrLeaseBusy,
// ErrNudgeSlotUnavailable, ErrNudgeSubmitUncertain or ctx's error; on every
// pre-claim error nothing was written and no slot was spent.
func NudgeStalledAutonomousSession(ctx context.Context, inst *Instance, claim func() (bool, error)) error {
	if inst == nil {
		return fmt.Errorf("nudge target session not found")
	}
	return nudgeStalledWith(ctx, inst, claim, nudgeSeams{
		settleWindow: nudgeIdleSettleWindow,
		poll:         nudgeIdleSettlePollInterval,
		tryLease:     inst.TryTerminalWriteLease,
		submit:       SubmitDriverContent,
	})
}

func nudgeStalledWith(ctx context.Context, inst *Instance, claim func() (bool, error), s nudgeSeams) error {
	snap := inst.Snapshot()
	if snap.Status != Active || snap.ArchivedAt != nil {
		return fmt.Errorf("%w: %s (status %s)", ErrNudgeSessionNotActive, snap.UUID, snap.Status)
	}
	if err := CheckNudgeEligibleWithSettleWindowCtx(ctx, inst, s.settleWindow, s.poll); err != nil {
		return err
	}
	if err := inst.VerifyPaneOwner(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("nudge cancelled before taking the write lease: %w", err)
	}

	lease, ok := s.tryLease(LeaseWriterNudge)
	if !ok {
		return ErrLeaseBusy
	}
	if err := ctx.Err(); err != nil {
		lease.Release()
		return fmt.Errorf("nudge cancelled before claiming a slot: %w", err)
	}
	// Status can change while waiting for the lease; the claim spends budget.
	if snap := inst.Snapshot(); snap.Status != Active || snap.ArchivedAt != nil {
		lease.Release()
		return fmt.Errorf("%w: %s (status %s)", ErrNudgeSessionNotActive, snap.UUID, snap.Status)
	}
	granted, err := claim()
	if err != nil {
		lease.Release()
		return fmt.Errorf("%w: %w", ErrNudgeClaimFailed, err)
	}
	if !granted {
		lease.Release()
		return ErrNudgeSlotUnavailable
	}

	// submit owns the lease from here and releases it on every path.
	if err := s.submit(ctx, inst, lease, autonomousStallNudgeText, DefaultPaneSettlePollInterval, DefaultPaneSettleMaxWait); err != nil {
		return fmt.Errorf("%w: %w", ErrNudgeSubmitUncertain, err)
	}
	return nil
}
