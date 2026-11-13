package session

import (
	"context"
	"fmt"
	"time"
)

// ErrNudgeTargetNotIdle is returned by CheckNudgeEligible when inst is not
// currently in an idle-shaped detection.DetectedStatus.
var ErrNudgeTargetNotIdle = fmt.Errorf("nudge target session is not idle")

// nudgeIdleSettleWindow is how long inst's status must continuously read as a
// genuinely-idle Claude Code prompt (IsSafeSteerStatus) before
// CheckNudgeEligible passes — guards against a single noisy poll (e.g. a
// status transiting through Idle for a moment) being treated as "safe to
// nudge". Short relative to AutonomousDriver's own idleSettleWindow (60s): a
// nudge is a one-shot, externally-triggered write, not a long-running
// orchestration loop, so the cost of a several-second poll here is easily
// affordable.
const nudgeIdleSettleWindow = 3 * time.Second

// nudgeIdleSettlePollInterval is how often CheckNudgeEligible re-polls the
// controller's cached status while confirming nudgeIdleSettleWindow.
const nudgeIdleSettlePollInterval = 500 * time.Millisecond

// CheckNudgeEligible reports whether inst is currently safe to nudge, per
// Diagnose & Nudge's AC2. Delegates to CheckNudgeEligibleWithSettleWindow
// using the package's real settle window/poll interval; see that function's
// doc comment for the two things this tightens over a naive isIdleStatus
// point-in-time check.
func CheckNudgeEligible(inst *Instance) error {
	return CheckNudgeEligibleWithSettleWindow(inst, nudgeIdleSettleWindow, nudgeIdleSettlePollInterval)
}

// CheckNudgeEligibleWithSettleWindow is CheckNudgeEligible's real
// implementation, parameterized on the settle window/poll interval so tests
// can shrink both rather than waiting out the real window — mirrors
// AutonomousDriver's own interval-parameterized-implementation seam
// (WithIdleSettleWindow).
//
// Two things distinguish this from the isIdleStatus(StatusIdle||StatusReady||
// StatusSuccess) point-in-time check this replaced: (1) it requires
// IsSafeSteerStatus specifically — StatusIdle alone is ambiguous with a raw
// shell/vim/editor prompt reporting the same enum value, and StatusReady is
// documented dead code (never produced by MatchLines) while StatusSuccess is
// a terminal-turn signal, not "idle right now"; (2) it requires the safe
// status to hold continuously for settleWindow, not just on a single poll,
// mirroring autonomous_driver.go's waitForIdle debounce rationale. The
// write-time pane-ownership re-verification (VerifyPaneOwnershipBeforeWrite)
// remains the guard against a status/identity change between this check
// returning and the write itself — settleWindow only widens what "checked" means.
func CheckNudgeEligibleWithSettleWindow(inst *Instance, settleWindow, pollInterval time.Duration) error {
	return CheckNudgeEligibleWithSettleWindowCtx(context.Background(), inst, settleWindow, pollInterval)
}

// CheckNudgeEligibleWithSettleWindowCtx is CheckNudgeEligibleWithSettleWindow
// with a cancellable wait: a cancelled ctx returns promptly (wrapping
// ctx.Err()) instead of sleeping out the rest of the settle window, so a
// caller that was told to stop is never left blocked here.
func CheckNudgeEligibleWithSettleWindowCtx(ctx context.Context, inst *Instance, settleWindow, pollInterval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("nudge eligibility check cancelled: %w", err)
	}
	if inst == nil {
		return fmt.Errorf("nudge target session not found")
	}
	ctrl := inst.GetController()
	if ctrl == nil {
		return fmt.Errorf("nudge target session %s has no running controller", inst.UUID)
	}

	start := time.Now()
	for {
		status, statusCtx := ctrl.GetCurrentStatus()
		if !IsSafeSteerStatus(status, statusCtx) {
			return fmt.Errorf("%w: session %s is %s (context %q)", ErrNudgeTargetNotIdle, inst.UUID, status, statusCtx)
		}
		if time.Since(start) >= settleWindow {
			return nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("nudge eligibility check cancelled: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// VerifyNudgeSafeToWrite runs both AC2 checks in the order the nudge path
// must apply them: idle-status eligibility first (cheap, in-memory), then the
// write-time pane-ownership re-verification (a tmux round trip) immediately
// before the caller actually writes to the pane. Returns the first error
// encountered.
func VerifyNudgeSafeToWrite(ctx context.Context, inst *Instance) error {
	if err := CheckNudgeEligibleWithSettleWindowCtx(ctx, inst, nudgeIdleSettleWindow, nudgeIdleSettlePollInterval); err != nil {
		return err
	}
	return VerifyPaneOwnershipBeforeWrite(ctx, inst)
}
