package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// StaleWorkDisposition is what the stale-work remediator does after inspection.
type StaleWorkDisposition int

const (
	// StaleWorkRespawn: end the ItemSession, kill the pane and respawn (the pre-nudge behaviour).
	StaleWorkRespawn StaleWorkDisposition = iota
	// StaleWorkNudged: the idle session was prompted to continue; restart nothing.
	StaleWorkNudged
	// StaleWorkSkipped: leave the session alone (stall recovery live, or nudge delivery uncertain).
	StaleWorkSkipped
)

// StaleWorkInspector looks at a stale work session before it is restarted.
// Implemented by *AutonomousOrchestrationService.
type StaleWorkInspector interface {
	InspectStaleWork(ctx context.Context, sessionUUID string, item *session.BacklogItemData) StaleWorkDisposition
}

var _ StaleWorkInspector = (*AutonomousOrchestrationService)(nil)

// recoveryLive reports whether a stall recovery is in flight for the session title.
func (a *AutonomousOrchestrationService) recoveryLive(title string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, live := a.recoveries[title]
	return live
}

// InspectStaleWork captures a StopSnapshot of the stale session, logs it, and
// either nudges an idle-at-prompt session (ADR-001 row 7) or tells the caller
// to restart it. It runs before UpdateItemSessionEnded, so the capture is
// deadline-bound (stopCaptureTimeout) and never delays BUG-064's ordering.
func (a *AutonomousOrchestrationService) InspectStaleWork(ctx context.Context, sessionUUID string, item *session.BacklogItemData) StaleWorkDisposition {
	var inst *session.Instance
	if a.instanceFinder != nil {
		inst = a.instanceFinder(sessionUUID)
	}
	storage := a.concreteStorage()
	if inst == nil || storage == nil {
		return StaleWorkRespawn // no live pane to inspect
	}
	title := inst.Snapshot().Title
	if a.recoveryLive(title) {
		log.Info("[StaleWork] skipping — stall recovery live", "session", title, "item", item.ID)
		return StaleWorkSkipped
	}

	snap := a.captureStaleSnapshot(inst)
	rec := stopRecord{
		Session:  title,
		ItemID:   item.ID,
		Role:     session.SessionRoleWork,
		Outcome:  session.AutonomousDriverOutcome{StopReason: domain.StopReasonStaleWork, Snapshot: snap},
		State:    domain.ClassifyObserved(snap),
		Tail:     sanitizeStopTail(snap.PaneTail),
		Recovery: stopRecovery{Action: stopActionNone},
	}
	log.Warn("[StaleWork] stop snapshot", "session", title, "item", item.ID,
		"stop_reason", stopReasonCode(rec.reason()), "state", string(rec.State),
		"capture_err", snap.CaptureErr, "tail_bytes", len(rec.Tail))

	plan, _, _ := a.planStall(ctx, storage, inst, item, rec)
	// A nudge needs a driver to re-attach; an interactive session keeps today's respawn.
	if plan == domain.StallActionNudge && !inst.Snapshot().Autonomous.AutonomousMode {
		plan = domain.StallActionRespawn
	}
	if plan != domain.StallActionNudge {
		rec.Recovery.Action = stopActionRespawned
		a.refreshStuckRow(ctx, storage, item, rec)
		return StaleWorkRespawn
	}

	return a.nudgeStaleWork(ctx, storage, inst, item, rec)
}

func (a *AutonomousOrchestrationService) captureStaleSnapshot(inst *session.Instance) domain.StopSnapshot {
	if a.staleSnapshot != nil {
		return a.staleSnapshot(inst)
	}
	return session.CaptureInstanceStopSnapshot(inst, 0)
}

// concreteStorage returns the wired backing store, or nil.
func (a *AutonomousOrchestrationService) concreteStorage() *session.Storage {
	if a.storageGetter == nil {
		return nil
	}
	return a.storageGetter()
}

// nudgeStaleWork delivers the nudge under a registered recovery (so a user stop
// cancels it) and falls back to a restart when the nudge cannot be delivered.
func (a *AutonomousOrchestrationService) nudgeStaleWork(ctx context.Context, storage *session.Storage, inst *session.Instance, item *session.BacklogItemData, rec stopRecord) StaleWorkDisposition {
	title := rec.Session
	// Register before touching the row: a driver recovery that registered since the
	// early recoveryLive check must not be overwritten (its cancel would be lost).
	recoveryCtx, release, ok := a.tryBeginRecovery(title)
	if !ok {
		log.Info("[StaleWork] skipping — stall recovery started during inspection", "session", title, "item", item.ID)
		return StaleWorkSkipped
	}
	defer release()
	rec.Recovery.Action = stopActionNudging
	a.refreshStuckRow(ctx, storage, item, rec)
	respawn, _ := a.nudgeStalledWorkSession(ctx, recoveryCtx, storage, inst, item, &rec)
	if respawn {
		rec.Recovery.Action = stopActionRespawned
	}
	a.refreshStuckRow(ctx, storage, item, rec)
	switch {
	case respawn:
		return StaleWorkRespawn
	case rec.Recovery.Action == stopActionNudged:
		a.publishNudged(inst.UUID, title, item.ID, rec)
		return StaleWorkNudged
	default:
		return StaleWorkSkipped
	}
}

// staleWorkInspectTimeout bounds snapshot + planning + nudge delivery; generous so a
// nudge that is delivering is not cut short and then answered with a restart.
const staleWorkInspectTimeout = 2 * time.Minute

// RecordDeadPane writes the dead-pane stop snapshot to the stuck row of the
// session's backlog item, so the operator sees why the health checker respawned
// it. Only a live work ItemSession qualifies; anything else is a no-op. The
// health checker still respawns regardless — this is record-only.
func (a *AutonomousOrchestrationService) RecordDeadPane(sessionUUID, title string, snap domain.StopSnapshot, state domain.ObservedState) {
	storage := a.concreteStorage()
	if storage == nil {
		return
	}
	ctx, cancel := context.WithTimeout(a.driverCtx(), deadPaneRecordTimeout)
	defer cancel()
	is, err := storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil || is.Role != session.SessionRoleWork || is.EndedAt != nil {
		return
	}
	item, err := storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil || item == nil {
		return
	}
	a.refreshStuckRow(ctx, storage, item, stopRecord{
		Session:  title,
		ItemID:   item.ID,
		Role:     session.SessionRoleWork,
		Outcome:  session.AutonomousDriverOutcome{StopReason: domain.StopReasonDeadPane, Snapshot: snap},
		State:    state,
		Tail:     sanitizeStopTail(snap.PaneTail),
		Recovery: stopRecovery{Action: stopActionRespawned},
	})
}

const deadPaneRecordTimeout = 5 * time.Second
