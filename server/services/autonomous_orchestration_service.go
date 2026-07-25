package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
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

	// autonomousStuckRespawner gives an item a fresh autonomous work-session
	// turn budget when a work session hits its turn cap without a DONE signal.
	// Optional — if nil, the item is simply left in_progress (marked
	// autonomous_stuck) until a human reopens it manually.
	autonomousStuckRespawner AutonomousStuckRespawner
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
	// statusTransitionErr is surfaced in the notification below so the operator never sees
	// "complete" while the backlog item silently failed to advance (e.g. a concurrent status
	// change broke the optimistic-concurrency precondition).
	var statusTransitionErr error
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
					// Write a durable autonomous_stuck row so a turn-cap stop is visible in
					// the Unfinished tab, not just the ephemeral "Autonomous fix stuck"
					// notification published below — previously invisible to the whole
					// stuck-reason/recovery system every other detector in
					// session/backlog_lifecycle.go participates in. Additive, never a gate:
					// a MarkStuck/MarkStuckNotified failure is logged but must not block the
					// role-specific status handling below.
					if !outcome.Done {
						if _, markErr := concreteStorage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck,
							session.BacklogStatus(item.Status),
							fmt.Sprintf("autonomous driver stopped after %d turns without a DONE signal (%s)", outcome.Turns, outcome.Reason)); markErr != nil {
							log.Warn("[AutonomousDriver] onAutonomousDriverComplete: MarkStuck(autonomous_stuck) failed", "item", item.ID, "err", markErr)
						} else if _, notifyErr := concreteStorage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonAutonomousStuck); notifyErr != nil {
							log.Warn("[AutonomousDriver] onAutonomousDriverComplete: MarkStuckNotified(autonomous_stuck) failed", "item", item.ID, "err", notifyErr)
						}
					}

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
						if !outcome.Done {
							// Turn-cap stop with no DONE signal: don't force the item into
							// review — that reviews known-incomplete work every time, and
							// (before this fix) a doomed review that itself exits with no
							// verdict fed straight back into another turn-cap-doomed work
							// session, with no working circuit breaker to stop it (a live
							// item bounced 78 times in 24h this way — see
							// docs/tasks/backlog-feature-improvement.md, 2026-07-19 update).
							// Leave toStatus unset so the item stays in_progress; the
							// autonomous_stuck row written above already makes this visible
							// in the Unfinished tab, and the generic "Autonomous fix stuck"
							// notification below still fires. Give the item a fresh turn
							// budget directly instead, gated by the same rework cap the
							// review-side auto-reopen loop uses.
							log.Info("[AutonomousDriver] work session hit turn cap without DONE; leaving in_progress instead of forcing review", "item", item.ID, "reason", outcome.Reason)
							if a.autonomousStuckRespawner != nil {
								respawner := a.autonomousStuckRespawner
								itemID := item.ID
								itemTitle := item.Title
								// Backoff-gated (session/backlog_remediation.go, Phase A of
								// docs/tasks/backlog-stuck-item-auto-remediation.md): the
								// MarkStuck(autonomous_stuck) call just above always
								// refreshes/opens this item's stuck row on every
								// turn-cap-without-DONE occurrence, so without this gate the
								// respawn below would fire on every single occurrence too —
								// exactly the "burns through the rework cap in minutes"
								// shape the design doc's backoff schedule exists to stop
								// (count-capped only by the rework cap otherwise, which can
								// be raised well past what a genuinely-looping item should
								// get). Checked synchronously (not inside the goroutine) so
								// the attempt/restart-grace accounting write happens exactly
								// once per eligible occurrence, before the async dispatch.
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
										int32(8), // NotificationType_WARNING
										int32(3), // NotificationPriority_HIGH
										"Automated retry paused",
										fmt.Sprintf("%s — automated turn-budget respawns have been retried %d times over an extended period without finishing. It now needs manual attention; use Reset to try again automatically.", itemTitle, session.MaxRemediationAttempts),
										nil,
									))
								}
								if due {
									go func() {
										if respawnErr := respawner.AutoRespawnAutonomousWork(a.lifecycleCtx, itemID); respawnErr != nil {
											log.Warn("[AutonomousDriver] AutoRespawnAutonomousWork failed", "item", itemID, "err", respawnErr)
										}
									}()
								} else {
									log.Info("[AutonomousDriver] autonomous_stuck remediation backoff not yet due, skipping respawn", "item", itemID)
								}
							}
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
							// BUG-048: the item already moved on from "review" by the time this
							// stuck exit was processed — most likely session/backlog_lifecycle.go's
							// bouncing gate already reopened it via a different, earlier
							// review-session-exit event. The condition this autonomous_stuck row
							// represents no longer describes the item's current state; resolve it
							// now instead of leaving it open and drifting arbitrarily overdue
							// forever (the original bug: nothing else was ever going to revisit
							// this row once opened).
							a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
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
					}
					if toStatus != "" {
						precondition := &session.BacklogItemPrecondition{ExpectedStatus: expectedStatus}
						if _, transErr := concreteStorage.TransitionBacklogItemStatus(ctx, item.ID, toStatus, precondition, session.TriggeredBySystem); transErr != nil {
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
	if statusTransitionErr != nil {
		// The driver finished (or got stuck), but the backlog item's status update failed.
		// Never let the operator see "complete" while the item is silently stuck in its
		// previous status — override the notification to say so explicitly.
		title += " — status update failed"
		body += fmt.Sprintf(" The backlog item status could not be updated (%v); it may be stuck in its previous status — check manually.", statusTransitionErr)
		notifType = int32(9) // NotificationType_FAILURE
	}
	a.bus.Publish(events.NewNotificationEvent(
		sessionUUID, instanceName, fmt.Sprintf("autonomous-complete-%s", sessionUUID),
		notifType,
		int32(2), // NotificationPriority_MEDIUM
		title, body, nil,
	))
}

// resolveAutonomousStuck best-effort closes any open autonomous_stuck row for
// itemID. Mirrors the resolve-at-point-of-success pattern resolveToPRPending
// uses for push_failed/abandoned_review (session/backlog_lifecycle.go:1729-1738).
// autonomous_stuck has no non-terminal anchor in selfHealStuck's per-reason
// sweep (session/backlog_lifecycle.go's selfHealStuck) — it relies on that
// sweep's blanket terminal-status rule (any open row on a done/archived item
// is resolved regardless of reason) to eventually catch it, or on a
// human-initiated TransitionBacklogItemStatus RPC via
// resolveStuckOnManualTransition (server/services/backlog_service_lifecycle.go).
// This function exists to resolve the row immediately, at the moment of
// success, rather than waiting for the next sweep tick: without it, items
// that are marked autonomous_stuck and then later complete purely through the
// automated pipeline (no human ever clicks a manual transition) would sit
// with a stuck row open until the next selfHealStuck run. A failure here is
// logged, not returned: this is bookkeeping cleanup and must never block the
// caller's own notification/transition handling.
func (a *AutonomousOrchestrationService) resolveAutonomousStuck(ctx context.Context, storage *session.Storage, itemID string) {
	if _, err := storage.ResolveStuck(ctx, itemID, domain.StuckReasonAutonomousStuck); err != nil {
		log.Warn("[AutonomousDriver] resolveAutonomousStuck ResolveStuck(autonomous_stuck) failed", "item", itemID, "err", err)
	}
}
