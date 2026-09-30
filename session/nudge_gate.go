package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ownershipCheckTimeout bounds the tmux show-environment call
// VerifyPaneOwnershipBeforeWrite makes, mirroring the timeout used by the
// existing STAPLER_SESSION_UUID readers (LiveTmuxSessionUUIDs,
// ReconcileOrphanedTmuxSessions) this function's technique is modeled on.
const ownershipCheckTimeout = 5 * time.Second

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
		time.Sleep(pollInterval)
	}
}

// VerifyPaneOwnershipBeforeWrite re-reads the target tmux pane's
// STAPLER_SESSION_UUID marker via `tmux show-environment` and confirms it
// still matches inst's own UUID, immediately before a nudge write is sent.
//
// This exists because CheckNudgeEligible's idle check (and any earlier
// identity check made when the diagnostic agent selected its nudge target)
// can go stale: a tmux session-name collision can cause a completely
// different Instance's pane to be reattached under the name this Instance
// expects (the stapler-squad-backlog-devbug incident, backlog item
// ce71ad1a) between the eligibility check and the write itself. Re-reading
// the marker right before the write — rather than trusting a value cached
// at attach or eligibility-check time — is the only way to catch that
// window. Deliberately independent of the tmux pane-ownership fix landed on
// the (as of this writing, unmerged) backlog/stapler-squad-verify-tmux-session-ownership
// branch (commit 6c9026e81), which hardens reattach/kill call sites, not
// this one — this check reads the same STAPLER_SESSION_UUID marker via the
// same technique already used on main (LiveTmuxSessionUUIDs,
// session/workspace_peers.go), so it does not depend on that branch merging.
func VerifyPaneOwnershipBeforeWrite(ctx context.Context, inst *Instance) error {
	if inst == nil {
		return fmt.Errorf("nudge target session not found")
	}
	expected := inst.Snapshot().UUID
	name := inst.GetTmuxSessionName()
	if expected == "" || name == "" {
		return fmt.Errorf("nudge target session %s has no tmux identity to verify", inst.UUID)
	}

	envCtx, cancel := context.WithTimeout(ctx, ownershipCheckTimeout)
	defer cancel()
	socketArgs := tmux.ResolveSocket("").Args
	out, err := safeexec.CommandContext(envCtx, tmux.Binary(), socketArgs("show-environment", "-t", name, "STAPLER_SESSION_UUID")...).Output()
	if err != nil {
		return fmt.Errorf("could not re-verify pane ownership for session %s (tmux target %s): %w", inst.UUID, name, err)
	}
	actual := strings.TrimPrefix(strings.TrimSpace(string(out)), "STAPLER_SESSION_UUID=")
	if actual == "" || actual != expected {
		return fmt.Errorf("pane ownership mismatch: tmux target %q now reports owner %q, expected %q — refusing to write", name, actual, expected)
	}
	return nil
}

// VerifyNudgeSafeToWrite runs both AC2 checks in the order the nudge path
// must apply them: idle-status eligibility first (cheap, in-memory), then the
// write-time pane-ownership re-verification (a tmux round trip) immediately
// before the caller actually writes to the pane. Returns the first error
// encountered.
func VerifyNudgeSafeToWrite(ctx context.Context, inst *Instance) error {
	if err := CheckNudgeEligible(inst); err != nil {
		return err
	}
	return VerifyPaneOwnershipBeforeWrite(ctx, inst)
}
