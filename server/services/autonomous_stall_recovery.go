package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// stallNudgeRestartTimeout is the driver warm-up after a nudge, the value triage uses.
const stallNudgeRestartTimeout = 5 * time.Minute

// stallNudgeReserve keeps one slot of the item's nudge cap for the diagnose agent.
const stallNudgeReserve = 1

// StallNudger types the fixed stall reminder into an idle work session.
// claim must only touch storage.
type StallNudger interface {
	NudgeStalledSession(ctx context.Context, inst *session.Instance, claim func() (bool, error)) error
}

type defaultStallNudger struct{}

func (defaultStallNudger) NudgeStalledSession(ctx context.Context, inst *session.Instance, claim func() (bool, error)) error {
	return session.NudgeStalledAutonomousSession(ctx, inst, claim)
}

// SetStallNudger replaces the nudge writer (tests).
func (a *AutonomousOrchestrationService) SetStallNudger(n StallNudger) { a.stallNudger = n }

// SetNudgeFlagReaders replaces the live readers of diagnose_nudge_enabled and
// autonomous_stall_nudge_enabled (tests; nil keeps the config-backed default).
func (a *AutonomousOrchestrationService) SetNudgeFlagReaders(diagnose, stall func() bool) {
	a.diagnoseNudgeEnabled, a.stallNudgeEnabled = diagnose, stall
}

// SetDriverRestarter replaces how a nudged session's driver is re-attached (tests).
func (a *AutonomousOrchestrationService) SetDriverRestarter(fn func(*session.Instance, time.Duration) error) {
	a.restartDriver = fn
}

func (a *AutonomousOrchestrationService) nudger() StallNudger {
	if a.stallNudger != nil {
		return a.stallNudger
	}
	return defaultStallNudger{}
}

// nudgeFlagsOn reports both flags, each and the AND, read at this instant.
func (a *AutonomousOrchestrationService) nudgeFlagsOn() (diagnose, stall bool) {
	if a.diagnoseNudgeEnabled != nil {
		diagnose = a.diagnoseNudgeEnabled()
	} else {
		diagnose = config.EffectiveDiagnoseNudgeEnabled(config.LoadConfig())
	}
	if a.stallNudgeEnabled != nil {
		stall = a.stallNudgeEnabled()
	} else {
		stall = config.EffectiveAutonomousStallNudgeEnabled(config.LoadConfig())
	}
	return diagnose, stall
}

// beginRecovery returns a ctx that StopDriverForSession (a user stop, delete or
// toggle-off) cancels, so an in-flight nudge never outlives a requested stop.
func (a *AutonomousOrchestrationService) beginRecovery(title string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(a.driverCtx())
	a.mu.Lock()
	if a.recoveries == nil {
		a.recoveries = make(map[string]context.CancelFunc)
	}
	a.recoveries[title] = cancel
	a.mu.Unlock()
	return ctx, func() {
		a.mu.Lock()
		delete(a.recoveries, title)
		a.mu.Unlock()
		cancel()
	}
}

func (a *AutonomousOrchestrationService) cancelRecovery(title string) {
	a.mu.Lock()
	cancel := a.recoveries[title]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// canRestart is the last check before touching a pane or starting a driver: the
// session is still live, unarchived, not retrying, autonomy was not turned off,
// and no stop was requested.
func canRestart(ctx context.Context, inst *session.Instance) bool {
	if ctx.Err() != nil {
		return false
	}
	snap := inst.Snapshot()
	return snap.Status == session.Active && snap.ArchivedAt == nil && snap.Autonomous.AutonomousMode && !inst.IsRetryPending()
}

// reattachDriver starts a fresh driver on inst unless one is already registered.
func (a *AutonomousOrchestrationService) reattachDriver(inst *session.Instance, timeout time.Duration) error {
	if a.restartDriver != nil {
		return a.restartDriver(inst, timeout)
	}
	a.mu.Lock()
	_, present := a.drivers[inst.Title]
	a.mu.Unlock()
	if present {
		return nil
	}
	return a.startDriver(inst, timeout)
}

// planStall evaluates the ADR-001 decision table. wouldNudge is the plan with
// both flags forced on, so a disabled nudge is recorded as such.
func (a *AutonomousOrchestrationService) planStall(ctx context.Context, storage *session.Storage, inst *session.Instance, item *session.BacklogItemData, rec stopRecord) (plan domain.StallAction, wouldNudge bool) {
	diagnoseOn, stallOn := a.nudgeFlagsOn()
	in := domain.StallDecisionInput{
		Reason:           rec.reason(),
		State:            rec.State,
		NudgeEnabled:     diagnoseOn && stallOn,
		ItemInWorkStatus: session.BacklogStatus(item.Status) == session.BacklogStatusInProgress,
		Archived:         inst.IsArchived(),
		RetryPending:     inst.IsRetryPending(),
	}
	// Fail closed: an unreadable allowance is "not allowed"; the claim is authoritative.
	allowed, _, err := storage.DiagnoseNudgeAllowedWithReserve(ctx, item.ID, domain.StuckReasonAutonomousStuck, stallNudgeReserve)
	in.NudgeAllowed = err == nil && allowed
	plan = domain.DecideStallAction(in)
	in.NudgeEnabled = true
	wouldNudge = domain.DecideStallAction(in) == domain.StallActionNudge
	return plan, wouldNudge
}

// classifyNudgeError maps the writer's error to the outcome class and whether
// the pane may be left alone (no fallback respawn).
func classifyNudgeError(err error) (outcome nudgeOutcome, leavePane bool) {
	switch {
	case err == nil:
		return nudgeOutcomeSuccess, false
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return nudgeOutcomeCtxCancelled, true
	case errors.Is(err, session.ErrNudgeSubmitUncertain):
		return nudgeOutcomeUncertain, true
	case errors.Is(err, session.ErrNudgeTargetNotIdle):
		return nudgeOutcomeNotIdle, false
	case errors.Is(err, session.ErrLeaseBusy):
		return nudgeOutcomeLeaseBusy, false
	case errors.Is(err, session.ErrNudgeSlotUnavailable):
		return nudgeOutcomeSlotUnavailable, false
	default:
		return nudgeOutcomeOwnership, false
	}
}

// nudgeStalledWorkSession runs the nudge plan end to end and returns the action
// the stop finally took; rec.Recovery carries the outcome for the row and copy.
// A false return from the fallback means the caller must run the respawn body.
func (a *AutonomousOrchestrationService) nudgeStalledWorkSession(ctx context.Context, storage *session.Storage, inst *session.Instance, item *session.BacklogItemData, rec *stopRecord) (respawn bool) {
	rctx, release := a.beginRecovery(inst.Title)
	defer release()

	nudgeCap := config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault() - stallNudgeReserve
	rec.Recovery.NudgeCap = nudgeCap
	claim := func() (bool, error) {
		r, err := storage.ClaimDiagnoseNudgeSlot(ctx, item.ID, domain.StuckReasonAutonomousStuck, stallNudgeReserve)
		return r.Claimed, err
	}

	var err error
	if !canRestart(rctx, inst) {
		err = rctx.Err()
		if err == nil {
			err = fmt.Errorf("session no longer restartable")
		}
	} else {
		err = a.nudger().NudgeStalledSession(rctx, inst, claim)
	}
	outcome, leave := classifyNudgeError(err)
	if err != nil {
		log.Warn("[AutonomousDriver] stall nudge not delivered", "session", inst.Title, "item", item.ID, "nudge_outcome", string(outcome), "err", err)
	}
	switch {
	case outcome == nudgeOutcomeSuccess:
		if n, cErr := storage.DiagnoseNudgeCount(ctx, item.ID, domain.StuckReasonAutonomousStuck); cErr == nil {
			rec.Recovery.NudgeUsed = n
		}
		if !canRestart(rctx, inst) {
			rec.Recovery.NudgeOutcome = nudgeOutcomeCtxCancelled
			rec.Recovery.Action = stopActionNone
			return false
		}
		if rErr := a.reattachDriver(inst, stallNudgeRestartTimeout); rErr != nil {
			log.Warn("[AutonomousDriver] driver re-attach after nudge failed", "session", inst.Title, "err", rErr)
			rec.Recovery.NudgeOutcome = nudgeOutcomeRestartFailed
			return true
		}
		rec.Recovery.NudgeOutcome = nudgeOutcomeSuccess
		rec.Recovery.Action = stopActionNudged
		return false
	case leave:
		rec.Recovery.NudgeOutcome = outcome
		rec.Recovery.Action = stopActionNone
		return false
	default:
		rec.Recovery.NudgeOutcome = outcome
		return true
	}
}

// refreshStuckRow rewrites the open row's context with the final recovery;
// best effort, the outcome is already decided.
func (a *AutonomousOrchestrationService) refreshStuckRow(ctx context.Context, storage *session.Storage, item *session.BacklogItemData, rec stopRecord) {
	if _, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, session.BacklogStatus(item.Status), formatStopDetail(rec)); err != nil {
		log.Warn("[AutonomousDriver] refresh autonomous_stuck row failed", "item", item.ID, "err", err)
	}
}

// publishNudged tells the operator, at INFO, that no action is needed.
func (a *AutonomousOrchestrationService) publishNudged(sessionUUID, title, itemID string, rec stopRecord) {
	a.bus.Publish(events.NewNotificationEvent(
		sessionUUID, title, "autonomous-nudged-"+sessionUUID,
		int32(10), // NotificationType_INFO
		derivePriority(false, false),
		stopTitle(rec.Recovery),
		fmt.Sprintf("Session '%s': %s", title, plainSummary(rec.reason(), rec.State, rec.Recovery)),
		events.SessionScopedMetadata(nil, itemID),
	))
}
