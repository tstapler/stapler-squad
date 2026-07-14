package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// ReviewGateTrigger is implemented by BacklogLifecycleListener to fire an
// immediate headless review when an autonomous work session completes.
type ReviewGateTrigger interface {
	TriggerReviewForSession(workSessionUUID string)
}

// AutonomousOrchestrationService manages the lifecycle of AutonomousDriver instances:
// registering them on session creation, stopping them on deletion/hibernate, and
// handling their completion callbacks.
type AutonomousOrchestrationService struct {
	pool *headless.Pool
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
}

// SetReviewGateTrigger wires the review gate trigger (typically BacklogLifecycleListener).
func (a *AutonomousOrchestrationService) SetReviewGateTrigger(t ReviewGateTrigger) {
	a.reviewGateTrigger = t
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
	return &AutonomousOrchestrationService{
		pool:    pool,
		bus:     bus,
		drivers: make(map[string]*session.AutonomousDriver),
	}
}

// SetLifecycleContext binds the server's root context.
// Must be called once during server startup, before any sessions are created.
func (a *AutonomousOrchestrationService) SetLifecycleContext(ctx context.Context) {
	a.lifecycleCtx = ctx
}

// SetPool updates the headless pool after construction.
// Called from SessionService.SetHeadlessPool so the two stay in sync.
func (a *AutonomousOrchestrationService) SetPool(pool *headless.Pool) {
	a.pool = pool
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
	a.drivers[sessionTitle] = d
	a.mu.Unlock()
}

// stopAndDeregisterDriver stops the driver for sessionTitle (if any) and removes it from the registry.
func (a *AutonomousOrchestrationService) stopAndDeregisterDriver(sessionTitle string) {
	a.mu.Lock()
	d, ok := a.drivers[sessionTitle]
	if ok {
		delete(a.drivers, sessionTitle)
	}
	a.mu.Unlock()
	if ok && d != nil {
		d.Stop()
	}
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
				liveInst.AutonomousTurn = int32(turn)
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
			int32(10), // NotificationType_INFO
			int32(1),  // NotificationPriority_LOW
			fmt.Sprintf("Autonomous turn %d/%d", turn, maxTurns),
			fmt.Sprintf("%s: %s", inst.Title, truncated),
			nil,
		))
	}
}

// StartAutonomousDriverForInstance starts an AutonomousDriver on inst if the pool is available.
// Satisfies the AutonomousDriverStarter interface (via SessionService delegate).
func (a *AutonomousOrchestrationService) StartAutonomousDriverForInstance(inst *session.Instance) {
	if a.pool == nil {
		log.Warn("[AutonomousOrchestrationService] StartAutonomousDriverForInstance: pool is nil", "session", inst.Title)
		return
	}
	driver := session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0)
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
	driver := session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0, session.WithStartupTimeout(startupTimeout))
	driver.RegisterCompletionCallback(a.onAutonomousDriverComplete)
	driver.RegisterTurnCallback(a.buildTurnCallback(inst))
	if err := driver.Start(a.driverCtx()); err != nil {
		log.Warn("[AutonomousOrchestrationService] failed to start autonomous driver", "session", inst.Title, "err", err)
		return
	}
	a.registerDriver(inst.Title, driver)
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
	// Deregister the completed driver so it does not leak in the registry.
	// The goroutine has already exited at this point; Stop() is a no-op but cleans the map.
	a.stopAndDeregisterDriver(instanceName)

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

	// Clear autonomous_mode flag and set outcome on the instance so the badge updates.
	// NOTE: These direct field writes are unguarded pending instance-actor-concurrency's
	// Epic 5 (Story 5.3: "Convert buildTurnCallback/onAutonomousDriverComplete to commands").
	// Do not add actor routing here without coordinating with that project.
	inst.AutonomousMode = false
	inst.AutonomousTurn = 0
	inst.AutonomousMaxTurns = 0
	if outcome.Done {
		inst.AutonomousOutcome = "done"
	} else {
		inst.AutonomousOutcome = "stuck"
	}
	a.bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"autonomous_mode", "autonomous_outcome"}))

	// Look up the backlog item linked to this session.
	if a.storageGetter != nil {
		concreteStorage := a.storageGetter()
		if concreteStorage != nil {
			is, err := concreteStorage.GetItemSessionBySessionUUID(ctx, sessionUUID)
			if err == nil {
				item, itemErr := concreteStorage.GetBacklogItem(ctx, is.BacklogItemID)
				if itemErr == nil && item != nil {
					var toStatus session.BacklogStatus
					var expectedStatus string
					switch is.Role {
					case session.SessionRoleTriage:
						if !outcome.Done {
							// Triage was interrupted/stuck — notify operator but do not advance the item.
							// The item stays at 'idea' so the operator can re-trigger triage.
							a.bus.Publish(events.NewNotificationEvent(
								item.ID,
								"Triage stuck",
								fmt.Sprintf("stuck-triage-%s", item.ID),
								int32(9), // NotificationType_FAILURE (warning)
								int32(2), // NotificationPriority_MEDIUM
								"Triage did not complete",
								fmt.Sprintf("%s: autonomous triage session got stuck", item.Title),
								nil,
							))
							log.Info("[AutonomousDriver] triage stuck, notified operator", "item", item.ID, "reason", outcome.Reason)
							return
						}
						toStatus = session.BacklogStatusReady
						expectedStatus = string(session.BacklogStatusIdea)
					case session.SessionRoleWork:
						toStatus = session.BacklogStatusReview
						expectedStatus = string(session.BacklogStatusInProgress)
					default:
						// SessionRoleReview and unknown roles: no transition from AutonomousDriver.
						// Review outcomes are managed by submit_review_verdict.
						log.Info("[AutonomousDriver] skipping status transition for role", "role", is.Role, "item", item.ID)
						return
					}
					precondition := &session.BacklogItemPrecondition{ExpectedStatus: expectedStatus}
					if _, transErr := concreteStorage.TransitionBacklogItemStatus(ctx, item.ID, toStatus, precondition, session.TriggeredBySystem); transErr != nil {
						log.Warn("[AutonomousDriver] failed to transition backlog item", "item", item.ID, "to", toStatus, "err", transErr)
					} else {
						log.Info("[AutonomousDriver] backlog item transitioned", "item", item.ID, "to", toStatus, "done", outcome.Done)
						// Immediately kick off headless review for completed work sessions.
						if toStatus == session.BacklogStatusReview && a.reviewGateTrigger != nil {
							a.reviewGateTrigger.TriggerReviewForSession(sessionUUID)
						}
					}
				}
			}
		}
	}

	// Fire push notification via event bus.
	var title, body string
	notifType := int32(10) // NotificationType_INFO
	if outcome.Done {
		title = "Autonomous fix complete"
		body = fmt.Sprintf("%s: %s", instanceName, outcome.Reason)
		if outcome.PRUrl != "" {
			body += " — " + outcome.PRUrl
		}
	} else {
		title = "Autonomous fix stuck"
		body = fmt.Sprintf("Session '%s' stopped after %d turns without completing. Open the session to review what was accomplished and give the next instruction.", instanceName, outcome.Turns)
		notifType = int32(9) // NotificationType_FAILURE
	}
	a.bus.Publish(events.NewNotificationEvent(
		sessionUUID, instanceName, fmt.Sprintf("autonomous-complete-%s", sessionUUID),
		notifType,
		int32(2), // NotificationPriority_MEDIUM
		title, body, nil,
	))
}
