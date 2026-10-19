package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
)

// ReviewGateTrigger is implemented by BacklogLifecycleListener to fire an
// immediate headless review when an autonomous work session completes.
type ReviewGateTrigger interface {
	TriggerReviewForSession(workSessionUUID string)
}

// AutonomousStuckRespawner is implemented by BacklogService to give an
// in_progress backlog item a fresh work-session turn budget after an
// autonomous work session hits its turn cap without a DONE signal, instead of
// forcing the item into review against work the driver itself flagged
// incomplete (see onAutonomousDriverComplete's SessionRoleWork case). Gated
// by the same rework cap AutoReopenAfterFailedReview uses, so this respawn
// loop can't run forever either.
type AutonomousStuckRespawner interface {
	AutoRespawnAutonomousWork(ctx context.Context, itemID string) error
}

// AutonomousOrchestrationService manages the lifecycle of AutonomousDriver instances:
// registering them on session creation, stopping them on deletion/hibernate, and
// handling their completion callbacks.
type AutonomousOrchestrationService struct {
	pool headless.PoolClient
	bus  *events.EventBus

	// mu guards drivers (registry membership only — not Instance state).
	mu      sync.Mutex
	drivers map[string]*session.AutonomousDriver

	// lifecycleCtx is the server's root context. Set via SetLifecycleContext.
	// Autonomous driver goroutines are bound to this so they exit with the server.
	lifecycleCtx context.Context

	// instanceFinder resolves a session title to a live in-memory instance.
	// Wired via SessionService.SetReviewQueuePoller once the poller is available.
	instanceFinder func(string) *session.Instance

	// storageGetter returns the concrete backing store for backlog item transitions.
	// Wired at construction time via a closure over the InstanceStore.
	storageGetter func() *session.Storage

	// reviewGateTrigger fires an immediate headless review when a work session
	// completes under autonomous mode. Optional — if nil, review runs on next ReconcileStuck tick.
	reviewGateTrigger ReviewGateTrigger

	// autonomousStuckRespawner gives an item a fresh autonomous work-session
	// turn budget when a work session hits its turn cap without a DONE signal.
	// Optional — if nil, the item is simply left in_progress (marked
	// autonomous_stuck) until a human reopens it manually.
	autonomousStuckRespawner AutonomousStuckRespawner

	// readoptionEnabled reads the autonomous_driver_readoption flag at the instant
	// of a shutdown stop. Nil means OFF, the shipped default.
	readoptionEnabled func() bool

	// Stall nudge seams; nil values fall back to the config-backed / real ones.
	stallNudger          StallNudger
	diagnoseNudgeEnabled func() bool
	stallNudgeEnabled    func() bool
	restartDriver        func(*session.Instance, time.Duration) error
	// staleSnapshot replaces the real pane capture of InspectStaleWork (tests).
	staleSnapshot func(*session.Instance) domain.StopSnapshot
	// recoveries holds the cancel of each in-flight stall recovery, by session title (guarded by mu).
	recoveries map[string]*stallRecovery
}

// SetReadoptionEnabled wires the live read of the autonomous_driver_readoption flag.
func (a *AutonomousOrchestrationService) SetReadoptionEnabled(fn func() bool) {
	a.readoptionEnabled = fn
}

func (a *AutonomousOrchestrationService) readoptionOn() bool {
	return a.readoptionEnabled != nil && a.readoptionEnabled()
}

// SetReviewGateTrigger wires the review gate trigger (typically BacklogLifecycleListener).
func (a *AutonomousOrchestrationService) SetReviewGateTrigger(t ReviewGateTrigger) {
	a.reviewGateTrigger = t
}

// SetAutonomousStuckRespawner wires the respawner (typically BacklogService) used to
// retry a turn-cap-stopped autonomous work session instead of forcing it to review.
func (a *AutonomousOrchestrationService) SetAutonomousStuckRespawner(r AutonomousStuckRespawner) {
	a.autonomousStuckRespawner = r
}

// TriggerReviewForSession is a public passthrough to the wired ReviewGateTrigger, used
// by the request_review MCP tool to spawn a review gate immediately instead of waiting
// for the next ReconcileStuck tick. No-op if no trigger is wired.
func (a *AutonomousOrchestrationService) TriggerReviewForSession(sessionUUID string) {
	if a.reviewGateTrigger != nil {
		a.reviewGateTrigger.TriggerReviewForSession(sessionUUID)
	}
}

// NewAutonomousOrchestrationService creates a new service.
// pool may be nil when the claude binary is not found; methods degrade gracefully.
func NewAutonomousOrchestrationService(pool *headless.Pool, bus *events.EventBus) *AutonomousOrchestrationService {
	a := &AutonomousOrchestrationService{
		bus:     bus,
		drivers: make(map[string]*session.AutonomousDriver),
	}
	a.SetPool(pool) // nil *Pool must stay an untyped-nil interface
	return a
}

// SetLifecycleContext binds the server's root context.
// Must be called once during server startup, before any sessions are created.
func (a *AutonomousOrchestrationService) SetLifecycleContext(ctx context.Context) {
	a.lifecycleCtx = ctx
}

// SetPool updates the headless pool after construction.
// Called from SessionService.SetHeadlessPool so the two stay in sync.
func (a *AutonomousOrchestrationService) SetPool(pool *headless.Pool) {
	if pool == nil {
		a.pool = nil
		return
	}
	a.pool = pool
}

// SetClient wires a backend-selecting client in place of the raw claude pool.
func (a *AutonomousOrchestrationService) SetClient(c headless.PoolClient) {
	a.pool = c
}

// SetInstanceFinder wires a function for resolving live instances by title.
func (a *AutonomousOrchestrationService) SetInstanceFinder(fn func(string) *session.Instance) {
	a.instanceFinder = fn
}

// SetStorageGetter wires a function for getting the concrete storage.
func (a *AutonomousOrchestrationService) SetStorageGetter(fn func() *session.Storage) {
	a.storageGetter = fn
}

// driverCtx returns the lifecycle context, falling back to Background() if not set.
func (a *AutonomousOrchestrationService) driverCtx() context.Context {
	if a.lifecycleCtx != nil {
		return a.lifecycleCtx
	}
	return context.Background()
}

// registerDriver records a running AutonomousDriver in the registry so it can be stopped later.
func (a *AutonomousOrchestrationService) registerDriver(sessionTitle string, d *session.AutonomousDriver) {
	a.mu.Lock()
	old := a.drivers[sessionTitle]
	a.drivers[sessionTitle] = d
	a.mu.Unlock()
	if old != nil && old != d {
		old.Stop() // never leave two drivers typing into one pane
	}
}

// stopAndDeregisterDriver stops the driver for sessionTitle (if any) and removes it from the registry.
func (a *AutonomousOrchestrationService) stopAndDeregisterDriver(sessionTitle string) {
	a.cancelRecovery(sessionTitle)
	a.deregisterDriver(sessionTitle)
}

// deregisterDriver is stopAndDeregisterDriver without cancelling an in-flight stall
// recovery: the completion callback owns that recovery.
func (a *AutonomousOrchestrationService) deregisterDriver(sessionTitle string) {
	a.deregisterDriverRun(sessionTitle, nil)
}

func (a *AutonomousOrchestrationService) driverReplaced(sessionTitle string, runID uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.drivers[sessionTitle]
	return ok && d.RunID() != runID
}

// deregisterDriverRun removes the driver for sessionTitle only when it is the run
// that completed; it reports false when a newer driver has replaced that run, which
// must be left running.
func (a *AutonomousOrchestrationService) deregisterDriverRun(sessionTitle string, runID *uint64) (owned bool) {
	a.mu.Lock()
	d, ok := a.drivers[sessionTitle]
	if ok && runID != nil && d.RunID() != *runID {
		a.mu.Unlock()
		return false
	}
	if ok {
		delete(a.drivers, sessionTitle)
	}
	a.mu.Unlock()
	if ok && d != nil {
		d.Stop()
	}
	return true
}

// StopDriverForSession stops the AutonomousDriver registered under sessionTitle.
// Used by MCP handlers as a belt-and-suspenders stop after task completion.
// Satisfies mcp.ReviewCompletionSignaler.
func (a *AutonomousOrchestrationService) StopDriverForSession(sessionTitle string) {
	a.stopAndDeregisterDriver(sessionTitle)
}

// buildTurnCallback returns the TurnCallback wired for inst.
// Shared by StartAutonomousDriverForInstance and StartAutonomousDriverWithTimeout
// to prevent divergence over time.
func (a *AutonomousOrchestrationService) buildTurnCallback(inst *session.Instance) session.TurnCallback {
	return func(turn, maxTurns int, prompt string) {
		if a.instanceFinder != nil {
			if liveInst := a.instanceFinder(inst.Title); liveInst != nil {
				// #nosec G115 -- autonomous-driver loop iteration counter, far below int32 range.
				liveInst.AutonomousTurn = int32(turn)
				// #nosec G115 -- see AutonomousTurn above.
				liveInst.AutonomousMaxTurns = int32(maxTurns)
				a.bus.Publish(events.NewSessionUpdatedEvent(liveInst, []string{"autonomous_turn"}))
			}
		}
		truncated := prompt
		if len(truncated) > 120 {
			truncated = truncated[:120] + "…"
		}
		a.bus.Publish(events.NewNotificationEvent(
			inst.UUID, inst.Title, fmt.Sprintf("autonomous-turn-%s-%d", inst.UUID, turn),
			int32(10),                    // NotificationType_INFO
			derivePriority(false, false), // urgent, important — per-turn progress telemetry, no action needed
			fmt.Sprintf("Autonomous turn %d/%d", turn, maxTurns),
			fmt.Sprintf("%s: %s", inst.Title, truncated),
			nil,
		))
	}
}

// withCostSink builds the DriverOption that accumulates a driver's per-turn LLM
// cost onto inst's linked ItemSession, if any. Falls back to session.NoopDriverOption
// when storageGetter isn't wired — same nil-guard shape as onAutonomousDriverComplete's
// storageGetter lookup above.
func (a *AutonomousOrchestrationService) withCostSink(inst *session.Instance) session.DriverOption {
	if a.storageGetter == nil {
		return session.NoopDriverOption
	}
	concreteStorage := a.storageGetter()
	if concreteStorage == nil {
		return session.NoopDriverOption
	}
	return session.WithCostSink(session.CostSinkForSessionUUID(concreteStorage, inst.UUID))
}

// StartAutonomousDriverForInstance starts an AutonomousDriver on inst if the pool is available.
// Satisfies the AutonomousDriverStarter interface (via SessionService delegate).
func (a *AutonomousOrchestrationService) StartAutonomousDriverForInstance(inst *session.Instance) {
	if a.pool == nil {
		log.Warn("[AutonomousOrchestrationService] StartAutonomousDriverForInstance: pool is nil", "session", inst.Title)
		return
	}
	driver := session.NewAutonomousDriver(inst, a.pool, inst.Prompt, config.LoadConfig().AutonomousMaxTurnsOrDefault(), a.withCostSink(inst))
	driver.RegisterCompletionCallback(a.onAutonomousDriverComplete)
	driver.RegisterTurnCallback(a.buildTurnCallback(inst))
	if err := driver.Start(a.driverCtx()); err != nil {
		log.Warn("[AutonomousOrchestrationService] failed to start autonomous driver for backlog session", "session", inst.Title, "err", err)
		return
	}
	a.registerDriver(inst.Title, driver)
}

// StartAutonomousDriverWithTimeout is like StartAutonomousDriverForInstance but uses a
// configurable startup timeout for sessions that need a longer warm-up
// (e.g. triage sessions that spawn parallel subagents).
func (a *AutonomousOrchestrationService) StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration) {
	if a.pool == nil {
		log.Warn("[AutonomousOrchestrationService] StartAutonomousDriverWithTimeout: pool is nil", "session", inst.Title)
		return
	}
	if err := a.startDriver(inst, startupTimeout); err != nil {
		log.Warn("[AutonomousOrchestrationService] failed to start autonomous driver", "session", inst.Title, "err", err)
	}
}

// startDriver builds, starts and registers a driver with the given warm-up.
func (a *AutonomousOrchestrationService) startDriver(inst *session.Instance, startupTimeout time.Duration) error {
	if a.pool == nil {
		return errors.New("headless pool unavailable")
	}
	driver := session.NewAutonomousDriver(inst, a.pool, inst.Prompt, config.LoadConfig().AutonomousMaxTurnsOrDefault(), session.WithStartupTimeout(startupTimeout), a.withCostSink(inst))
	driver.RegisterCompletionCallback(a.onAutonomousDriverComplete)
	driver.RegisterTurnCallback(a.buildTurnCallback(inst))
	if err := driver.Start(a.driverCtx()); err != nil {
		return err
	}
	a.registerDriver(inst.Title, driver)
	return nil
}

// Compile-time assertion: AutonomousOrchestrationService satisfies the public surface of
// AutonomousDriverStarter. SessionService remains the registered AutonomousDriverStarter
// (delegates to this service); this assertion keeps the two signature-compatible.
var _ interface {
	StartAutonomousDriverForInstance(*session.Instance)
	StartAutonomousDriverWithTimeout(*session.Instance, time.Duration)
	StopDriverForSession(string)
} = (*AutonomousOrchestrationService)(nil)

// onAutonomousDriverComplete handles the outcome of an AutonomousDriver run.
// Updates the linked backlog item status and fires a push notification.
func (a *AutonomousOrchestrationService) onAutonomousDriverComplete(instanceName string, outcome session.AutonomousDriverOutcome) {
	// One "stopped" line per stop, on every return path, with the final recovery.
	rec := stopRecord{
		Session:  instanceName,
		Outcome:  outcome,
		State:    domain.ClassifyObserved(outcome.Snapshot),
		Tail:     sanitizeStopTail(outcome.Snapshot.PaneTail),
		Recovery: stopRecovery{Action: stopActionNone},
	}
	defer func() { logStop(rec) }()

	// Deregister the completed driver so it does not leak in the registry.
	// The goroutine has already exited at this point; Stop() is a no-op but cleans the map.
	// Register the recovery before deregistering, so a stop that lands during the
	// bookkeeping below cancels it instead of being lost.
	// A stopped, replaced driver's callback must not touch the new driver or the instance,
	// nor clobber the new run's recovery registration.
	if a.driverReplaced(instanceName, outcome.RunID) {
		return
	}
	recoveryCtx, releaseRecovery := a.beginRecovery(instanceName)
	defer releaseRecovery()
	if !a.deregisterDriverRun(instanceName, &outcome.RunID) {
		rec.Recovery.Action = stopActionNone
		return
	}

	// Use Background() intentionally: we want this bookkeeping to complete even if the
	// server is shutting down concurrently (the driver just finished; its result must persist).
	ctx := context.Background()

	// Resolve the session UUID from the instance name using the live poller.
	if a.instanceFinder == nil {
		log.Warn("[AutonomousDriver] onAutonomousDriverComplete: instanceFinder not wired", "session", instanceName)
		return
	}
	inst := a.instanceFinder(instanceName)
	if inst == nil {
		log.Warn("[AutonomousDriver] onAutonomousDriverComplete: instance not found", "session", instanceName)
		return
	}
	sessionUUID := inst.UUID
	// Read before the stop write zeroes it: the budget this run actually had.
	rec.MaxTurns = int(inst.Snapshot().Autonomous.AutonomousMaxTurns)
	cancelled := outcome.Class() == domain.OutcomeClassCancelled

	// The instance write happens once: at plan time for a work stall (so a nudge keeps
	// AutonomousMode on), otherwise as a plain stop on exit. A failed write is logged
	// inside applyInstanceStop; bookkeeping carries on.
	stopApplied := false
	applyStop := func(action domain.StallAction) {
		if stopApplied {
			return
		}
		stopApplied = true
		_ = a.applyInstanceStop(inst, action, outcome)
	}
	defer applyStop(domain.StallActionNone)

	// Look up the backlog item linked to this session.
	// statusTransitionErr is surfaced in the notification below so the operator never sees
	// "complete" while the backlog item silently failed to advance (e.g. a concurrent status
	// change broke the optimistic-concurrency precondition).
	var statusTransitionErr error
	// stall is the work-role stall handler's result; its Notify override is applied
	// at the notification site below.
	var stall stallResult
	// linkedItemID carries the backlog item ID (when this session is backlog-linked) out
	// of the nested lookup block below, so the generic done/stuck notification at the end
	// of this function can stamp it via events.SessionScopedMetadata.
	var linkedItemID string
	if a.storageGetter != nil {
		concreteStorage := a.storageGetter()
		if concreteStorage != nil {
			is, err := concreteStorage.GetItemSessionBySessionUUID(ctx, sessionUUID)
			if err != nil {
				if errors.Is(err, session.ErrNotFound) {
					// Expected: most autonomous sessions are not backlog-linked.
					log.Debug("[AutonomousDriver] onAutonomousDriverComplete: no linked backlog item session", "session", instanceName)
				} else {
					// A real lookup failure would otherwise take the identical silent path as
					// "not backlog-linked" above, making it undiagnosable in production.
					log.Warn("[AutonomousDriver] onAutonomousDriverComplete: item session lookup failed", "session", instanceName, "err", err)
				}
			} else {
				item, itemErr := concreteStorage.GetBacklogItem(ctx, is.BacklogItemID)
				if itemErr != nil || item == nil {
					log.Warn("[AutonomousDriver] onAutonomousDriverComplete: failed to load linked backlog item", "itemSession", is.ID, "item", is.BacklogItemID, "err", itemErr)
				} else {
					linkedItemID = item.ID

					// Write a durable autonomous_stuck row so a turn-cap stop is visible in
					// the Unfinished tab, not just the ephemeral "Autonomous fix stuck"
					// notification published below — previously invisible to the whole
					// stuck-reason/recovery system every other detector in
					// session/backlog_lifecycle.go participates in. Additive, never a gate:
					// a MarkStuck/MarkStuckNotified failure is logged but must not block the
					// role-specific status handling below.
					rec.ItemID, rec.Role = item.ID, is.Role
					// The work role marks inside handleWorkStall, once its plan is known.
					if outcome.Class() == domain.OutcomeClassStall && is.Role != session.SessionRoleWork {
						if _, markErr := concreteStorage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck,
							session.BacklogStatus(item.Status), formatStopDetail(rec)); markErr != nil {
							log.Warn("[AutonomousDriver] onAutonomousDriverComplete: MarkStuck(autonomous_stuck) failed", "item", item.ID, "err", markErr)
						} else if _, notifyErr := concreteStorage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonAutonomousStuck); notifyErr != nil {
							log.Warn("[AutonomousDriver] onAutonomousDriverComplete: MarkStuckNotified(autonomous_stuck) failed", "item", item.ID, "err", notifyErr)
						}
					}

					var toStatus session.BacklogStatus
					var expectedStatus string
					switch is.Role {
					case session.SessionRoleTriage:
						if cancelled {
							log.Info("[AutonomousDriver] triage run cancelled, leaving item untouched", "item", item.ID, "stop_reason", outcome.NormalizedStopReason())
							return
						}
						if !outcome.Done {
							// Triage was interrupted/stuck — notify operator but do not advance the item.
							// The item stays at 'idea' so the operator can re-trigger triage.
							a.bus.Publish(events.NewNotificationEvent(
								item.ID,
								"Triage stuck",
								fmt.Sprintf("stuck-triage-%s", item.ID),
								int32(9),                     // NotificationType_FAILURE (warning)
								derivePriority(false, false), // urgent, important — interrupted, awaiting operator re-trigger; matches "Triage may be stuck"
								"Triage did not complete",
								fmt.Sprintf("%s: autonomous triage session got stuck", item.Title),
								map[string]string{"item_id": item.ID},
							))
							log.Info("[AutonomousDriver] triage stuck, notified operator", "item", item.ID, "reason", outcome.Reason)
							return
						}
						toStatus = session.BacklogStatusReady
						expectedStatus = string(session.BacklogStatusIdea)
					case session.SessionRoleWork:
						if cancelled {
							log.Info("[AutonomousDriver] work run cancelled, not a stall", "item", item.ID, "stop_reason", outcome.NormalizedStopReason())
							return
						}
						if !outcome.Done {
							stall = a.handleWorkStall(ctx, recoveryCtx, concreteStorage, inst, item, is, &rec, outcome, applyStop)
						} else {
							// The orchestrator LLM (session/autonomous_driver.go's
							// autonomousSystemPrompt) judges "done" purely from a raw
							// terminal-tail snapshot (buildOrchestrationPrompt) — it has no
							// visibility into acceptance criteria, committed diff state, or
							// whether request_review was ever called. That is a much weaker
							// signal than backlog work's own explicit completion protocol:
							// the request_review MCP tool (server/mcp/tools_backlog.go),
							// which rejects uncommitted changes and requires the work
							// session's own agent to decide the goal is met.
							//
							// Confirmed live (2026-07-24/25): this orchestrator hallucinated
							// DONE ~10 minutes into a still-running SDD workflow, right after
							// nothing but a requirements.md commit, forcing a premature
							// in_progress→review transition and spawning a review against an
							// empty diff — while the underlying session kept working under
							// its own steam and landed the real fix commits 40+ minutes
							// later, unaware a (now-stale) FAIL verdict had already been
							// recorded. Two items hit this independently in the same session.
							//
							// Do NOT let this signal drive the transition. If request_review
							// already fired, item.Status is no longer in_progress and there
							// is nothing to do here — the legitimate mechanism already
							// handled it. If it hasn't, the orchestrator's DONE claim is
							// unconfirmed: leave the item in_progress (request_review remains
							// the sole path to review) and just log it so a repeat is
							// diagnosable, without forcing a status change or spawning a
							// second, competing driver on top of a session that (per the live
							// evidence above) may still be legitimately working.
							if session.BacklogStatus(item.Status) == session.BacklogStatusInProgress {
								log.Warn("[AutonomousDriver] orchestrator reported DONE but request_review was never called; leaving item in_progress",
									"item", item.ID, "session", sessionUUID, "reason", outcome.Reason)
							}
							// A well-formed DONE reply from the orchestrator is still real
							// evidence the driver itself isn't malfunctioning (looping on
							// malformed responses, hitting the turn cap) — that claim is
							// independent of whether we trust "DONE" to mean "ready for
							// review" above, so still resolve any open autonomous_stuck row,
							// mirroring the SessionRoleReview case below. Called directly
							// here (not via the toStatus-gated block further down) since
							// toStatus is deliberately left unset for this branch.
							a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
						}
					case session.SessionRoleReview:
						// Review outcomes are managed by submit_review_verdict — no transition,
						// and no generic notification (that would duplicate the review-specific one).
						// The review driver completing without hitting the turn cap is still
						// evidence the "driver run stuck" condition no longer applies, so resolve
						// any open autonomous_stuck row here even though the item's own status
						// transition (if any) happens elsewhere. Only when outcome.Done — a stuck
						// review run must not immediately undo the MarkStuck call a few lines up.
						switch {
						case outcome.Done:
							a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
						case session.BacklogStatus(item.Status) != session.BacklogStatusReview:
							// Also correctly covers a custom stage: "moved on from review" is
							// true regardless of which built-in or custom status it moved to,
							// so resolving the stuck row here needs no per-stage knowledge
							// (Epic 2.1, Story 2.1.3e's open-stage-slug-type decision).
							// BUG-048: the item already moved on from "review" by the time this
							// stuck exit was processed — most likely session/backlog_lifecycle.go's
							// bouncing gate already reopened it via a different, earlier
							// review-session-exit event. The condition this autonomous_stuck row
							// represents no longer describes the item's current state; resolve it
							// now instead of leaving it open and drifting arbitrarily overdue
							// forever (the original bug: nothing else was ever going to revisit
							// this row once opened).
							a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
						case cancelled:
							// A requested stop of a review still in review is not "stuck": leave the
							// ItemSession and any open row untouched.
						default:
							// BUG-048: still genuinely stuck in review. A review-role autonomous
							// driver that hits its turn cap without a DONE signal does NOT kill
							// the underlying tmux/CLI session — AutonomousDriver.run
							// (session/autonomous_driver.go) simply stops injecting turns — so the
							// ItemSession row stays EndedAt == nil ("active") indefinitely. That
							// single fact hides this item from both existing subsystems that could
							// otherwise recover it:
							//   - session/backlog_lifecycle.go's bouncing gate only ever fires from
							//     handleReviewSessionExited, itself only invoked on a genuine
							//     Instance EventExited/EventStopped — which will never arrive here,
							//     since nothing is left driving this session toward exit.
							//   - The abandoned_review detector's FindStuckReviewItems query
							//     explicitly excludes items with any EndedAt-nil review/work
							//     session (session/storage_backlog.go).
							// Deliberately do NOT spawn a competing review session here —
							// session/backlog_lifecycle.go's abandoned_review detector (itself
							// already bouncing-gate-aware, see BUG-043's RemediationBlocked fix)
							// is the sole intended owner of that responsibility; a second
							// independent respawner racing it here would repeat the exact
							// "two subsystems chase the same condition" shape BUG-041/043/046
							// already found and fixed. Instead, close the visibility gap: end
							// this ItemSession row so the item becomes visible to that existing,
							// already-correct machinery on the very next ReconcileStuck tick
							// (~60s) — bookkeeping only, no new actor, no session spawn.
							if endErr := concreteStorage.UpdateItemSessionEnded(ctx, is.ID, time.Now()); endErr != nil {
								log.Warn("[AutonomousDriver] onAutonomousDriverComplete: UpdateItemSessionEnded(review, stuck) failed", "item", item.ID, "itemSession", is.ID, "err", endErr)
								// This bookkeeping write is the exact mechanism BUG-048's
								// fix added to make a stuck review session visible to
								// abandoned_review/bouncing — a silent failure here
								// reproduces BUG-048's original gap one layer underneath
								// its own fix, and this function returns right below
								// without ever reaching the generic "Autonomous fix
								// stuck" notification further down (SessionRoleReview
								// deliberately skips it, see comment above).
								a.notifyStuckBookkeepingFailed(item.ID, item.Title, is.ID, stuckSessionRoleReview, endErr)
							}
						}
						log.Info("[AutonomousDriver] skipping status transition for role", "role", is.Role, "item", item.ID)
						return
					default:
						// Unrecognized role — a new pipeline stage was likely added elsewhere
						// without updating this switch. Previously this fell into the same silent
						// return as SessionRoleReview, leaving the operator with zero signal. Log
						// at Warn (diagnosable) and fall through to the generic done/stuck
						// notification below instead of returning early.
						log.Warn("[AutonomousDriver] onAutonomousDriverComplete: unrecognized session role, skipping status transition", "role", is.Role, "item", item.ID)
						if cancelled {
							return
						}
					}
					if toStatus != "" {
						precondition := &session.BacklogItemPrecondition{ExpectedStatus: expectedStatus}
						if _, transErr := concreteStorage.TransitionBacklogItemStatus(ctx, item.ID, toStatus, precondition, session.TriggeredBySystem); transErr != nil { //nolint:silenttransition surfaced below via statusTransitionErr, which overrides the operator notification title/body ("status update failed") rather than a silent success
							statusTransitionErr = transErr
							log.Warn("[AutonomousDriver] failed to transition backlog item", "item", item.ID, "to", toStatus, "err", transErr)
						} else {
							log.Info("[AutonomousDriver] backlog item transitioned", "item", item.ID, "to", toStatus, "done", outcome.Done)
							// The item just advanced via the automated pipeline. Close any open
							// autonomous_stuck row now rather than leaving it for a human to clear
							// via resolveStuckOnManualTransition (server/services/
							// backlog_service_lifecycle.go), which only fires on a manual
							// TransitionBacklogItemStatus RPC — items that complete entirely through
							// the automated pipeline never hit that path. Only resolve on an actually
							// successful (outcome.Done) run: the SessionRoleWork case above still
							// transitions in_progress->review even when the driver got stuck, and
							// MarkStuck may have just (re)opened this exact row a few lines up.
							if outcome.Done {
								a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
							}
							// Immediately kick off headless review for completed work sessions.
							if toStatus == session.BacklogStatusReview && a.reviewGateTrigger != nil {
								a.reviewGateTrigger.TriggerReviewForSession(sessionUUID)
							}
						}
					}
				}
			}
		}
	}

	// Fire push notification via event bus.
	var title, body string
	notifType := int32(10) // NotificationType_INFO
	// urgent/important default to the "complete" case: informational, no action needed.
	urgent, important := false, false
	switch outcome.Class() {
	case domain.OutcomeClassCancelled:
		// A requested stop (external or shutdown) is neither complete nor stuck.
		return
	case domain.OutcomeClassDone:
		title = "Autonomous fix complete"
		body = fmt.Sprintf("%s: %s", instanceName, outcome.Reason)
		if outcome.PRUrl != "" {
			body += " — " + outcome.PRUrl
		}
	default:
		title = stopTitle(rec.Recovery)
		body = fmt.Sprintf("Session '%s': %s", instanceName, plainSummary(rec.reason(), rec.State, rec.Recovery))
		notifType = int32(9) // NotificationType_FAILURE
		important = true     // matters, but not yet a confirmed dead end (backoff/respawn still handles it)
		if rec.Recovery.Action == stopActionNudged {
			notifType, important = int32(10), false // NotificationType_INFO: recovered, no action needed
		}
	}
	if statusTransitionErr != nil {
		// The driver finished (or got stuck), but the backlog item's status update failed.
		// Never let the operator see "complete" while the item is silently stuck in its
		// previous status — override the notification to say so explicitly.
		title += " — status update failed"
		body += fmt.Sprintf(" The backlog item status could not be updated (%v); it may be stuck in its previous status — check manually.", statusTransitionErr)
		notifType = int32(9)           // NotificationType_FAILURE
		urgent, important = true, true // a silent status/reality mismatch is a genuine correctness bug
	}
	if stall.Notify.Suppress {
		return
	}
	if stall.Notify.Title != "" {
		title = stall.Notify.Title
	}
	if stall.Notify.Body != "" {
		body = stall.Notify.Body
	}
	// Hidden sessions: the delivery gate drops the routine "Autonomous fix
	// complete" (INFO) and delivers the FAILURE-class "stuck" notification.
	// SessionRoleReview returns before this point, so a hidden review run
	// never reaches this generic notifier.
	a.bus.Publish(events.NewNotificationEvent(
		sessionUUID, instanceName, fmt.Sprintf("autonomous-complete-%s", sessionUUID),
		notifType,
		derivePriority(urgent, important),
		title, body, events.SessionScopedMetadata(nil, linkedItemID),
	))
}

// stallResult is what handleWorkStall tells its caller: the recovery taken and any
// change to the generic "Autonomous fix stuck" notification (zero value = publish it
// unchanged).
type stallResult struct {
	Action domain.StallAction
	Notify notifyOverride
}

// notifyOverride suppresses or rewords the generic stuck notification.
type notifyOverride struct {
	Suppress bool
	Title    string
	Body     string
}

// handleWorkStall handles a work-role driver stop that did not finish. It plans the
// recovery (ADR-001), writes the stuck row, applies the instance stop once, then
// nudges the idle session or ends the ItemSession and dispatches a respawn.
// ctx is the outer Background ctx; every storage write here uses it.
func (a *AutonomousOrchestrationService) handleWorkStall(ctx, recoveryCtx context.Context, concreteStorage *session.Storage, inst *session.Instance, item *session.BacklogItemData, is session.ItemSessionSummary, rec *stopRecord, outcome session.AutonomousDriverOutcome, applyStop func(domain.StallAction)) stallResult {
	if recoveryCtx.Err() != nil {
		// A stop was requested while this stop was being recorded: the stopper owns the outcome.
		rec.Recovery = stopRecovery{Action: stopActionNone, NudgeOutcome: nudgeOutcomeCtxCancelled}
		a.stopObservedAsPlain(inst, outcome, true)
		return stallResult{Action: domain.StallActionNone, Notify: notifyOverride{Suppress: true}}
	}
	plan, wouldNudge, noneWhy := a.planStall(ctx, concreteStorage, inst, item, *rec)
	if plan == domain.StallActionResupervise {
		plan = domain.StallActionRespawn // re-supervision is not wired; never chosen today
	}
	if plan == domain.StallActionNone {
		rec.Recovery = stopRecovery{Action: stopActionNone, NoneBecause: noneWhy}
		log.Info("[AutonomousDriver] stall not recovered", "item", item.ID, "session", inst.Snapshot().Title, "because", string(noneWhy))
		return stallResult{Action: domain.StallActionNone}
	}
	if plan == domain.StallActionRespawn && wouldNudge {
		rec.Recovery.NudgeOutcome = nudgeOutcomeDisabled
	}

	// Plan-time row: annotate what is about to happen.
	switch {
	case plan == domain.StallActionNudge:
		rec.Recovery.Action = stopActionNudging
	case a.autonomousStuckRespawner != nil:
		rec.Recovery.Action = stopActionRespawned
	default:
		rec.Recovery.Action = stopActionNone
	}
	applied, markErr := concreteStorage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, session.BacklogStatus(item.Status), formatStopDetail(*rec))
	switch {
	case markErr != nil:
		// Additive, never a gate: fail open to today's respawn body.
		log.Warn("[AutonomousDriver] MarkStuck(autonomous_stuck) failed", "item", item.ID, "err", markErr)
		plan = domain.StallActionRespawn
		rec.Recovery.Action = stopActionNone
		if a.autonomousStuckRespawner != nil {
			rec.Recovery.Action = stopActionRespawned
		}
	case !applied && plan == domain.StallActionNudge:
		log.Warn("[AutonomousDriver] stall nudge skipped: item status moved on", "item", item.ID)
		plan = domain.StallActionNone
		rec.Recovery.Action = stopActionNone
	default:
		if _, notifyErr := concreteStorage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonAutonomousStuck); notifyErr != nil {
			log.Warn("[AutonomousDriver] MarkStuckNotified(autonomous_stuck) failed", "item", item.ID, "err", notifyErr)
		}
	}
	if plan == domain.StallActionNone {
		applyStop(domain.StallActionNone)
		return stallResult{Action: domain.StallActionNone}
	}

	// A nudge keeps AutonomousMode on; the write precedes any effect.
	applyStop(plan)
	if plan == domain.StallActionNudge {
		if respawn, detached := a.nudgeStalledWorkSession(ctx, recoveryCtx, concreteStorage, inst, item, rec); !respawn {
			a.refreshStuckRow(ctx, concreteStorage, item, *rec)
			if rec.Recovery.Action == stopActionNudged {
				if detached {
					a.stopObservedAsPlain(inst, outcome, true) // delivered, but a stop landed before re-attach
				}
				a.publishNudged(inst.UUID, inst.Snapshot().Title, item.ID, *rec)
				return stallResult{Action: domain.StallActionNudge, Notify: notifyOverride{Suppress: true}}
			}
			// Pane left alone (stop requested, session no longer live, or delivery
			// uncertain). A requested stop is the stopper's outcome: no alarm.
			stopped := rec.Recovery.NudgeOutcome == nudgeOutcomeCtxCancelled || rec.Recovery.NudgeOutcome == nudgeOutcomeNotRestartable
			a.stopObservedAsPlain(inst, outcome, stopped)
			return stallResult{Action: domain.StallActionNone, Notify: notifyOverride{Suppress: stopped}}
		}
		// Fall back to today's respawn; mode must read off again.
		a.stopObservedAsPlain(inst, outcome, false)
		rec.Recovery.Action = stopActionRespawned
		if a.autonomousStuckRespawner == nil {
			rec.Recovery.Action = stopActionNone
		}
		a.refreshStuckRow(ctx, concreteStorage, item, *rec)
	}
	if !a.respawnStalledWork(ctx, concreteStorage, item, is, outcome) && rec.Recovery.Action == stopActionRespawned {
		// Backoff deferred it (or it is parked): never record a respawn that did not start.
		rec.Recovery.Action = stopActionNone
		a.refreshStuckRow(ctx, concreteStorage, item, *rec)
	}
	return stallResult{Action: domain.StallActionRespawn}
}

// stopObservedAsPlain turns autonomy off after a nudge that did not end in a
// re-attached driver (no driver is left running for the session).
func (a *AutonomousOrchestrationService) stopObservedAsPlain(inst *session.Instance, outcome session.AutonomousDriverOutcome, stopRequested bool) {
	a.mu.Lock()
	_, running := a.drivers[inst.Snapshot().Title]
	a.mu.Unlock()
	if running {
		return
	}
	if stopRequested {
		outcome.StopReason = domain.StopReasonCancelledExternal // keep the stopper's own badge state
	}
	_ = a.applyInstanceStop(inst, domain.StallActionRespawn, outcome)
}

// respawnStalledWork ends the stalled ItemSession and, subject to the
// remediation backoff, dispatches a respawn (the pre-nudge behaviour, unchanged).
// It reports whether a respawn was actually dispatched.
func (a *AutonomousOrchestrationService) respawnStalledWork(ctx context.Context, concreteStorage *session.Storage, item *session.BacklogItemData, is session.ItemSessionSummary, outcome session.AutonomousDriverOutcome) (dispatched bool) {
	log.Info("[AutonomousDriver] work session hit turn cap without DONE; leaving in_progress instead of forcing review", "item", item.ID, "reason", outcome.Reason)
	// A stopped driver leaves the pane live; end the ItemSession now so the respawn's
	// liveness check does not self-block on it (BUG-048).
	if endErr := concreteStorage.UpdateItemSessionEnded(ctx, is.ID, time.Now()); endErr != nil { //nolint:silenttransition // AutoRespawnAutonomousWork -> tombstoneOrphanWorkSessions/findActiveWorkSession blocks respawn with notifyRespawnBlockedByActiveSession
		log.Warn("[AutonomousDriver] onAutonomousDriverComplete: UpdateItemSessionEnded(work, stuck) failed", "item", item.ID, "itemSession", is.ID, "err", endErr)
		a.notifyStuckBookkeepingFailed(item.ID, item.Title, is.ID, stuckSessionRoleWork, endErr)
	}
	if a.autonomousStuckRespawner != nil {
		respawner := a.autonomousStuckRespawner
		itemID := item.ID
		itemTitle := item.Title
		// Backoff-gated, checked synchronously so the attempt accounting happens once per
		// occurrence (session/backlog_remediation.go).
		due, justParked, gateErr := concreteStorage.RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)
		if gateErr != nil {
			log.Warn("[AutonomousDriver] RemediationDue(autonomous_stuck) failed", "item", itemID, "err", gateErr)
			due = true // fail open — see session.autoReopenWithBackoffGate's identical rationale
		}
		if justParked {
			a.bus.Publish(events.NewNotificationEvent(
				itemID,
				"Auto-rework paused",
				fmt.Sprintf("stuck-autonomous-parked-%s", itemID),
				int32(8),                   // NotificationType_WARNING
				derivePriority(true, true), // urgent, important — automated retry gave up; a genuine dead end
				"Automated retry paused",
				fmt.Sprintf("%s — automated turn-budget respawns have been retried %d times over an extended period without finishing. It now needs manual attention; use Reset to try again automatically.", itemTitle, session.MaxRemediationAttempts),
				nil,
			))
		}
		if due {
			dispatched = true
			go func() {
				if respawnErr := respawner.AutoRespawnAutonomousWork(a.lifecycleCtx, itemID); respawnErr != nil {
					log.Warn("[AutonomousDriver] AutoRespawnAutonomousWork failed", "item", itemID, "err", respawnErr)
					a.notifyAutonomousRespawnAttemptFailed(itemID, itemTitle, respawnErr)
				}
			}()
		} else {
			log.Info("[AutonomousDriver] autonomous_stuck remediation backoff not yet due, skipping respawn", "item", itemID)
		}
	}
	return dispatched
}

// applyInstanceStop records the stop on inst through the actor and publishes the
// badge update, as one unit: the event carries the post-write instance, and no
// event is published when the write did not apply. Nudge and Resupervise keep
// AutonomousMode on (the approval gate reads it), as does a shutdown stop while
// re-adoption is enabled; every other action ends autonomy.
// The returned error is non-nil only when the instance actor is gone.
func (a *AutonomousOrchestrationService) applyInstanceStop(inst *session.Instance, action domain.StallAction, outcome session.AutonomousDriverOutcome) error {
	reason := outcome.NormalizedStopReason()
	var err error
	switch {
	case action == domain.StallActionNudge || action == domain.StallActionResupervise:
		err = inst.SetAutonomousStopObserved(reason)
	case reason == domain.StopReasonCancelledShutdown && a.readoptionOn():
		// Persisted mode is the re-adoption signal; keep it on.
		err = inst.SetAutonomousStopObserved(reason)
	default:
		err = inst.SetAutonomousStopped(reason)
	}
	if err != nil {
		log.Warn("[AutonomousDriver] applyInstanceStop: instance write did not apply", "session", inst.Snapshot().Title, "stop_reason", reason, "err", err)
		return err
	}
	a.bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"autonomous_mode", "autonomous_outcome"}))
	return nil
}

// stuckSessionRole distinguishes which onAutonomousDriverComplete branch a
// stuck-bookkeeping notification came from, matching the call site's session
// role rather than passing an untyped string.
type stuckSessionRole string

const (
	stuckSessionRoleReview stuckSessionRole = "review"
	stuckSessionRoleWork   stuckSessionRole = "work"
)

// notifyStuckBookkeepingFailed publishes an operator-facing notification when
// onAutonomousDriverComplete's UpdateItemSessionEnded call — the mechanism
// BUG-048's fix added to make a stuck session visible to recovery — itself
// fails for either the SessionRoleReview or SessionRoleWork branch. Previously
// this was only log.Warn'd, reproducing BUG-048's original silent-stranding
// gap one layer underneath its own fix. See each call site's own comment for
// why that branch needs this (SessionRoleReview skips the generic "Autonomous
// fix stuck" notification below and relies on this as its only signal;
// SessionRoleWork falls through to it as an additional one).
func (a *AutonomousOrchestrationService) notifyStuckBookkeepingFailed(itemID, itemTitle, itemSessionID string, role stuckSessionRole, endErr error) {
	a.bus.Publish(events.NewNotificationEvent(
		itemID, "", fmt.Sprintf("stuck-%s-bookkeeping-failed-%s", role, itemSessionID),
		int32(9),                   // NotificationType_FAILURE
		derivePriority(true, true), // urgent, important — the anti-silent-stranding mechanism itself is failing
		fmt.Sprintf("Stuck-%s bookkeeping failed", role),
		fmt.Sprintf("%s: could not mark the stalled %s session ended (%v) — it may stay invisible to automatic recovery until this is fixed manually.", itemTitle, role, endErr),
		nil,
	))
}

// notifyAutonomousRespawnAttemptFailed publishes an operator-facing
// notification when a single AutoRespawnAutonomousWork attempt fails. Before
// this fix, a failed respawn attempt was only log.Warn'd — invisible to the
// operator for up to the full ~4.5-day backoff schedule, until either a
// later attempt silently succeeded or RemediationDue's justParked branch
// finally surfaced something once attempts were exhausted (see
// docs/tasks/backlog-feature-improvement.md's 2026-07-28 entry: "unlike
// BUG-030's fix which surfaces every failure immediately"). Unlike that
// justParked notification (terminal: "give up automatically, use Reset"),
// this fires on every individual failed attempt and is deliberately
// non-terminal — WARNING/MEDIUM rather than justParked's WARNING/HIGH, since
// no operator action is required yet: the item will still retry
// automatically on the next backoff-eligible tick. Mirrors
// notifyStuckBookkeepingFailed's direct a.bus.Publish shape.
func (a *AutonomousOrchestrationService) notifyAutonomousRespawnAttemptFailed(itemID, itemTitle string, respawnErr error) {
	a.bus.Publish(events.NewNotificationEvent(
		itemID, "", fmt.Sprintf("stuck-autonomous-respawn-failed-%s", itemID),
		int32(8),                     // NotificationType_WARNING
		derivePriority(false, false), // urgent, important — no operator action needed yet, will retry automatically
		"Automated retry failed",
		fmt.Sprintf("%s — an automated turn-budget respawn attempt failed (%v). It will retry automatically per the standard backoff schedule.", itemTitle, respawnErr),
		map[string]string{"item_id": itemID, pkgevents.MetadataKeyAutoRemediating: "true"},
	))
}

// resolveAutonomousStuck best-effort closes any open autonomous_stuck row for
// itemID. Mirrors the resolve-at-point-of-success pattern resolveToPRPending
// uses for push_failed/abandoned_review (session/backlog_lifecycle.go:1729-1738).
// autonomous_stuck's non-terminal anchor in selfHealStuck's per-reason sweep
// (session/backlog_lifecycle.go's selfHealStuck) only catches a row once the
// item leaves BOTH in_progress and review (mirroring StuckReasonBouncing's
// identical two-status anchor, for the same false-resolve-risk reason) — this
// function exists to resolve the row immediately, at the moment of success,
// rather than waiting for that sweep's next tick, AND to cover an item that
// completes purely through the automated pipeline (no human ever clicks a
// manual transition via resolveStuckOnManualTransition,
// server/services/backlog_service_lifecycle.go) before the anchor's own status
// check would otherwise catch it. A failure here is logged, not returned: this
// is bookkeeping cleanup and must never block the caller's own
// notification/transition handling.
func (a *AutonomousOrchestrationService) resolveAutonomousStuck(ctx context.Context, storage *session.Storage, itemID string) {
	if _, err := storage.ResolveStuck(ctx, itemID, domain.StuckReasonAutonomousStuck); err != nil {
		log.Warn("[AutonomousDriver] resolveAutonomousStuck ResolveStuck(autonomous_stuck) failed", "item", itemID, "err", err)
	}
}
