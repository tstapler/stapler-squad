package services

// Unit tests for AutonomousOrchestrationService: driver registry, lifecycle context
// binding, and completion callback deregistration behavior.
// These tests do not require tmux or a real headless pool.

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
)

// instantDonePool is a HeadlessPoolClient that returns DONE on the first call,
// allowing an AutonomousDriver to complete without needing a real LLM backend.
type instantDonePool struct{}

func (p *instantDonePool) CallBlocking(
	_ context.Context,
	_ headless.FeatureKey,
	_, _ string,
	_ headless.CallOptions,
) (string, float64, error) {
	return "DONE: test complete", 0, nil
}

// addPausedAutonomousInstance inserts a paused session with AutonomousMode=true into storage.
func addPausedAutonomousInstance(t *testing.T, storage *session.Storage, title string) *session.Instance {
	t.Helper()
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid-1234",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}

// TestNewAutonomousOrchestrationService verifies basic construction invariants.
func TestNewAutonomousOrchestrationService(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(100)
	svc := NewAutonomousOrchestrationService(nil, bus)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.drivers)
	// pool nil is valid — callers degrade gracefully.
	assert.Nil(t, svc.pool)
}

// TestAutonomousOrchestrationService_SetLifecycleContext verifies that SetLifecycleContext
// stores the provided context and that driverCtx() returns it (observable via cancellation).
func TestAutonomousOrchestrationService_SetLifecycleContext(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	// Without SetLifecycleContext, driverCtx() should fall back to Background.
	assert.NoError(t, svc.autonomousSvc.driverCtx().Err(), "driverCtx() should return non-cancelled ctx before SetLifecycleContext")

	// After wiring a cancelled context, driverCtx() should reflect it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	svc.SetLifecycleContext(ctx)

	assert.Error(t, svc.autonomousSvc.driverCtx().Err(), "driverCtx() should return the cancelled lifecycle context after SetLifecycleContext")
}

// TestAutonomousOrchestrationService_DriverRegistry_RegisterAndDeregister verifies that
// registerDriver stores a driver and stopAndDeregisterDriver removes it and calls Stop.
func TestAutonomousOrchestrationService_DriverRegistry_RegisterAndDeregister(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	inst := &session.Instance{
		Title: "reg-test",
		UUID:  "reg-uuid-5678",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)

	// Register the driver without starting it (Start() would require a controller).
	svc.autonomousSvc.registerDriver("reg-test", driver)

	// Deregister and stop — should not panic.
	svc.autonomousSvc.stopAndDeregisterDriver("reg-test")

	// Second deregister is a no-op — should also not panic.
	svc.autonomousSvc.stopAndDeregisterDriver("reg-test")
}

// TestAutonomousOrchestrationService_DeleteSession_StopsRegisteredDriver verifies that
// DeleteSession calls stopAndDeregisterDriver for the deleted session's title.
func TestAutonomousOrchestrationService_DeleteSession_StopsRegisteredDriver(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "delete-driver-test"
	addPausedAutonomousInstance(t, storage, title)

	inst := &session.Instance{
		Title: title,
		UUID:  title + "-uuid-1234",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)
	svc.autonomousSvc.registerDriver(title, driver)

	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: title})
	resp, err := svc.DeleteSession(context.Background(), req)
	require.NoError(t, err, "DeleteSession should succeed")
	assert.True(t, resp.Msg.Success)

	// The registry entry must have been removed — subsequent stop is a no-op.
	svc.autonomousSvc.stopAndDeregisterDriver(title) // no panic = driver already deregistered
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DeregistersDriver verifies
// that the completion callback removes the driver from the registry so it does not leak.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DeregistersDriver(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "complete-driver-test"

	inst := &session.Instance{
		Title: title,
		UUID:  title + "-uuid-9999",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)
	svc.autonomousSvc.registerDriver(title, driver)

	// Fire the completion callback (simulates driver goroutine finishing).
	// instanceFinder returns nil here — expected when the session exited before callback fires.
	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	// The driver must have been removed from the registry.
	svc.autonomousSvc.stopAndDeregisterDriver(title) // no panic = already deregistered
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NotifiesStatusUpdateFailure
// verifies the fix for the notification/transition-divergence bug: when the driver reports
// Done but the backlog item's status transition fails its optimistic-concurrency precondition
// (e.g. a concurrent status change), the published notification must say so explicitly rather
// than announcing a plain "complete" while the item is silently stuck.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NotifiesStatusUpdateFailure(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "notif-race-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	// Bypass the real ReviewQueuePoller (not wired in this unit test) and resolve the
	// instance directly, matching what FindLiveInstance would return once wired.
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	// Item is already at 'review' — the triage-role transition below expects 'idea', so its
	// precondition will fail and TransitionBacklogItemStatus must return an error.
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Notification race test item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleTriage,
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	// onAutonomousDriverComplete also publishes a session.updated event before the
	// notification (badge update for autonomous_mode/autonomous_outcome) — skip past it.
	var notif *events.Event
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 5
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "expected a notification event")
	assert.Contains(t, notif.NotificationTitle, "status update failed", "operator must be told the status update failed, not just 'complete'")
	assert.Equal(t, int32(9), notif.NotificationType, "a divergence between driver-done and status-update-failed must surface as a FAILURE notification")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone
// is the regression test for closing the "conversions limit" visibility gap: a
// stuck (outcome.Done=false) autonomous run on a backlog-linked session must
// write a durable autonomous_stuck BacklogStuckState row — previously only an
// ephemeral "Autonomous fix stuck" notification, invisible to the Unfinished
// tab's stuck-reason system every other detector participates in.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "autonomous-stuck-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous stuck test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonAutonomousStuck, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "20 turns")
	assert.NotNil(t, open[0].NotifiedAt, "dedup must be pre-set since the notification already fired")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NoStuckRow_When_Done verifies
// a successful (Done=true) completion never writes an autonomous_stuck row.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NoStuckRow_When_Done(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "autonomous-done-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous done test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "task complete", Turns: 5}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a successful completion must never write an autonomous_stuck row")
}

// fakeReviewGateTrigger records TriggerReviewForSession calls so tests can assert
// whether (and how many times) a review was spawned.
type fakeReviewGateTrigger struct {
	calls []string
}

func (f *fakeReviewGateTrigger) TriggerReviewForSession(workSessionUUID string) {
	f.calls = append(f.calls, workSessionUUID)
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DoesNotForceReview_When_OrchestratorClaimsDoneWithoutRequestReview
// is the regression test for the live 2026-07-24/25 premature-review-trigger bug: two
// backlog items each had a review session spawned within minutes of their work session
// starting — while the work session was still actively running and had committed nothing
// but a requirements.md doc — because the AutonomousDriver's orchestrator LLM judged
// "DONE" from a raw terminal-tail snapshot and onAutonomousDriverComplete trusted that
// signal to force the in_progress→review transition, racing the legitimate completion
// path (the request_review MCP tool, which requires the work session's own agent to
// decide the goal is met and rejects uncommitted changes).
//
// This reproduces the exact failure shape: a SessionRoleWork driver reports Done=true
// (simulating the orchestrator's premature DONE verdict) while the backlog item is still
// "in_progress" (simulating a work session mid-SDD-workflow that never called
// request_review). The item must NOT transition to review and no review session may be
// spawned — request_review remains the only path to review for backlog work sessions.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DoesNotForceReview_When_OrchestratorClaimsDoneWithoutRequestReview(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	trigger := &fakeReviewGateTrigger{}
	svc.SetReviewGateTrigger(trigger)

	const title = "premature-review-trigger-test"
	inst := addPausedAutonomousInstance(t, storage, title)
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Mid-SDD-workflow work session, never called request_review",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// Simulate the orchestrator's premature DONE verdict — the exact signal that forced
	// the live bug's in_progress→review transition.
	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "looks complete from the terminal tail", Turns: 1}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	reloaded, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), reloaded.Status,
		"the orchestrator's inferred DONE must never force in_progress→review; only request_review may do that")
	assert.Empty(t, trigger.calls, "no review session may be spawned off the orchestrator's inferred DONE signal")
}

// captureLogs swaps the default slog logger for one that writes to a buffer at Debug level,
// restoring the previous logger via t.Cleanup. Returns the buffer to inspect after the call.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug verifies
// the fix for the swallowed-error bug: when no item session is linked to the completing
// session (the common, expected case — most autonomous sessions are not backlog-linked), the
// lookup "failure" must log at Debug, not escalate to Warn.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "not-linked-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	buf := captureLogs(t)
	svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1})

	assert.Contains(t, buf.String(), "no linked backlog item session")
	assert.NotContains(t, buf.String(), "level=WARN", "the expected not-linked case must not escalate to Warn")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsRealLookupFailureAtWarn
// verifies the other half of the fix: when an item session IS found but its linked backlog
// item cannot be loaded (a genuine data-integrity problem, not "not linked"), the failure
// must log at Warn so it's diagnosable — previously this took the identical silent path as
// the expected not-linked case above.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsRealLookupFailureAtWarn(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "dangling-item-session-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	// Create a real backlog item + item session (FK-valid), then delete the backlog item —
	// simulating an operator deleting an item while its autonomous session is still running.
	// If the item session row also disappears via cascade, GetItemSessionBySessionUUID will
	// return ErrNotFound and this test will report that below instead of asserting blindly.
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Deleted-out-from-under-us item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.DeleteBacklogItem(ctx, item.ID))

	if _, lookupErr := storage.GetItemSessionBySessionUUID(ctx, inst.UUID); lookupErr != nil {
		t.Skip("DeleteBacklogItem cascades to the item session too — this scenario isn't reachable via the public Storage API")
	}

	buf := captureLogs(t)
	svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1})

	assert.Contains(t, buf.String(), "failed to load linked backlog item")
	assert.Contains(t, buf.String(), "level=WARN", "a genuine lookup failure must be diagnosable, not silent")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_UnrecognizedRoleStillNotifies
// verifies the fix for the silent-default-role bug: an item session with a role the switch
// doesn't recognize (e.g. a new pipeline stage added elsewhere without updating this switch)
// must still log at Warn AND publish the generic done/stuck notification — previously it fell
// into the same silent early-return as the expected SessionRoleReview case, leaving the
// operator with zero signal that anything happened at all.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_UnrecognizedRoleStillNotifies(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "unrecognized-role-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Unrecognized role test item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: "future-pipeline-stage", // not triage/work/review
	})
	require.NoError(t, err)

	buf := captureLogs(t)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1})

	assert.Contains(t, buf.String(), "unrecognized session role")
	assert.Contains(t, buf.String(), "level=WARN", "an unrecognized role must be diagnosable, not silent")

	var notif *events.Event
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 5
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "the operator must still get the generic complete/stuck notification, not silence")
	assert.Equal(t, "Autonomous fix complete", notif.NotificationTitle)
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ResolvesAutonomousStuck_When_WorkSucceeds
// is the regression test for the "notify-only, not resolved" defect: an item
// previously marked autonomous_stuck (e.g. a prior turn-cap stop) that later
// completes successfully through the automated pipeline (SessionRoleWork,
// outcome.Done=true) must have its open autonomous_stuck row resolved here,
// rather than staying permanently open until a human happens to trigger a
// manual TransitionBacklogItemStatus RPC (server/services/
// backlog_service_lifecycle.go's resolveStuckOnManualTransition — the only
// other resolve path, and one the automated pipeline never exercises).
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ResolvesAutonomousStuck_When_WorkSucceeds(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "autonomous-stuck-resolves-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous stuck resolves test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// Simulate a prior turn-cap stop that left an open autonomous_stuck row.
	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck,
		session.BacklogStatusInProgress, "autonomous driver stopped after 20 turns without a DONE signal")
	require.NoError(t, err)
	require.True(t, applied)
	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "precondition: the stuck row must be open before the success run")

	// The item resumes and this time the work driver actually finishes.
	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "task complete", Turns: 3}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err = storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a successful automated completion must resolve the previously-open autonomous_stuck row")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ResolvesAutonomousStuck_When_ReviewSucceeds
// covers the SessionRoleReview half of the same fix: onAutonomousDriverComplete
// intentionally skips the backlog item's status transition for review sessions
// (that is submit_review_verdict's job), but a successful review driver run is
// still proof the driver itself is no longer stuck, so the open autonomous_stuck
// row must still be resolved here.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ResolvesAutonomousStuck_When_ReviewSucceeds(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "autonomous-stuck-review-resolves-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous stuck review resolves test item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck,
		session.BacklogStatusReview, "autonomous review driver stopped after 20 turns without a DONE signal")
	require.NoError(t, err)
	require.True(t, applied)
	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "precondition: the stuck row must be open before the success run")

	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "review complete", Turns: 2}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err = storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a successful review driver run must resolve the previously-open autonomous_stuck row even though the status transition itself happens in submit_review_verdict")
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_KeepsAutonomousStuck_When_WorkStillStuck
// guards against a naive "resolve whenever the transition succeeds" fix: the
// SessionRoleWork case still transitions in_progress->review even when the
// driver itself got stuck (outcome.Done=false; see selfHealStuck's doc
// comment in session/backlog_lifecycle.go for why that transition happens
// regardless of outcome), so the row MarkStuck just (re)opened a few lines
// above must NOT be immediately resolved by that same transition succeeding.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_KeepsAutonomousStuck_When_WorkStillStuck(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "autonomous-stuck-still-stuck-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous still-stuck test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "the MarkStuck row written moments earlier in the same call must survive — the item is still stuck")
	assert.Equal(t, domain.StuckReasonAutonomousStuck, open[0].Reason)
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_ResolvesRow_When_ItemAlreadyMovedOn
// is BUG-048's regression test for the "bouncing already handled it" half of the
// fix: if the item's status has already moved off "review" by the time a
// review-role driver's stuck (outcome.Done=false) exit is processed — e.g.
// session/backlog_lifecycle.go's bouncing gate already reopened it via a
// different, earlier review-session-exit event — the autonomous_stuck row this
// exit's own MarkStuck call just (re)opened must be resolved immediately
// instead of being left open and drifting arbitrarily overdue forever.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_ResolvesRow_When_ItemAlreadyMovedOn(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "review-stuck-already-moved-on-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	// Item has already been reopened to in_progress by the time this stuck
	// review exit is processed (simulates bouncing's autoReopenWithBackoffGate
	// having already acted via a different, earlier session exit).
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Review stuck, item already moved on",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "autonomous_stuck must be resolved immediately once the item's status shows the underlying condition was already handled elsewhere")

	// The ItemSession row is untouched by this branch — only the resolve path runs.
	refreshed, err := storage.GetItemSessionBySessionUUID(ctx, inst.UUID)
	require.NoError(t, err)
	assert.Equal(t, is.ID, refreshed.ID)
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_EndsSession_NoCompetingRespawn
// is BUG-048's regression test for the "still genuinely stuck" half of the fix:
// when a review-role autonomous driver exits stuck (outcome.Done=false) and the
// item is still in "review", onAutonomousDriverComplete must NOT spawn a
// competing review session (that responsibility belongs solely to
// session/backlog_lifecycle.go's abandoned_review detector) — it must instead
// end the ItemSession row so the item becomes visible to that existing
// machinery on the next reconcile tick. Asserts the row's EndedAt is now
// non-nil and that the still-open autonomous_stuck row is left as an honest
// signal rather than resolved.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_EndsSession_NoCompetingRespawn(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "review-stuck-still-stuck-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Review stuck, still stuck test item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)
	require.Nil(t, is.EndedAt, "precondition: the review ItemSession is still active (autonomous driver never ended it)")

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	// The autonomous_stuck row must survive — the item is genuinely still stuck.
	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "autonomous_stuck must stay open — the item is still genuinely stuck in review")
	assert.Equal(t, domain.StuckReasonAutonomousStuck, open[0].Reason)

	// The review ItemSession must now be ended so the abandoned_review detector's
	// FindStuckReviewItems query (which excludes any EndedAt-nil review/work
	// session) can see this item on the next tick, instead of the item being
	// permanently invisible to every existing reconciler.
	refreshed, err := storage.GetItemSessionBySessionUUID(ctx, inst.UUID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.EndedAt, "the review ItemSession must be marked ended so the item becomes visible to the existing abandoned_review/bouncing machinery")

	// No new review (or any) session was spawned as a competing responder.
	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1, "onAutonomousDriverComplete must not spawn a competing review session — that is abandoned_review's job")
}

// TestOnAutonomousDriverComplete_StampsItemID_When_TriageStuck is the regression test for
// Epic 3 Story 3.1: a stuck (outcome.Done=false) triage-role driver run previously published
// the "Triage stuck" notification with nil metadata, giving downstream consumers (e.g.
// server/notifications) no way to correlate the notification back to its backlog item. The
// notification's metadata must now carry {"item_id": <item.ID>}.
func TestOnAutonomousDriverComplete_StampsItemID_When_TriageStuck(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "triage-stuck-metadata-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Triage stuck metadata test item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleTriage,
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 5, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	var notif *events.Event
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification && ev.NotificationTitle == "Triage did not complete" {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 5
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "expected the 'Triage did not complete' notification")
	require.NotNil(t, notif.NotificationMetadata, "metadata must not be nil so downstream consumers can correlate the notification to its item")
	assert.Equal(t, item.ID, notif.NotificationMetadata["item_id"])
}

// TestOnAutonomousDriverComplete_SuppressesGenericNotification_When_InstanceHidden verifies
// the Epic 3 Story 3.2 Hidden gate: the generic done/stuck notification at the end of
// onAutonomousDriverComplete must never fire for a Hidden instance (e.g. a review-gate driver
// run the operator never surfaces in the UI) — publishing it anyway would functionally
// duplicate AC1's intent of suppressing notifications for sessions hidden from the UI.
func TestOnAutonomousDriverComplete_SuppressesGenericNotification_When_InstanceHidden(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "hidden-generic-notif-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		Hidden:         true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	// The session.updated badge event still fires (unrelated to the Hidden gate); only the
	// notification event must be absent. Drain briefly and assert no notification arrives.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			require.NotEqual(t, events.EventNotification, ev.Type, "a Hidden instance must never receive the generic done/stuck notification")
		case <-deadline:
			return
		}
	}
}

// TestOnAutonomousDriverComplete_StampsSessionScopedMetadata_When_NotHiddenAndBacklogLinked
// covers the positive case for Epic 3 Story 3.2: a non-Hidden instance whose completing
// session is linked to a backlog item must have the generic done/stuck notification's
// metadata built via events.SessionScopedMetadata — {"item_id": ..., "session_scoped": "true"}
// — not the nil it carried before this fix.
func TestOnAutonomousDriverComplete_StampsSessionScopedMetadata_When_NotHiddenAndBacklogLinked(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "generic-notif-metadata-test"
	inst := &session.Instance{
		Title:          title,
		UUID:           title + "-uuid",
		Path:           "/tmp/test",
		Status:         session.Paused,
		Program:        "claude",
		AutonomousMode: true,
		Hidden:         false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Generic notif metadata test item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: "future-pipeline-stage", // reaches the generic notifier, not a status transition
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	var notif *events.Event
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 5
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "expected the generic done/stuck notification")
	require.NotNil(t, notif.NotificationMetadata)
	assert.Equal(t, item.ID, notif.NotificationMetadata["item_id"])
	assert.Equal(t, "true", notif.NotificationMetadata["session_scoped"])
}

// TestNotifyStuckReviewBookkeepingFailed_should_publishFailureNotification_When_Called
// is the regression test for one of the four instances of the recurring
// "silent status-transition failure" bug shape found by the 2026-07-27
// backlog-feature-improvement audit (BUG-030/040/041/046/048's shape):
// onAutonomousDriverComplete's "still genuinely stuck in review" branch (see
// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_EndsSession_NoCompetingRespawn)
// calls UpdateItemSessionEnded — the exact mechanism BUG-048's own fix added
// to make a stuck review session visible to abandoned_review/bouncing.
// Previously, if that write itself failed, it was only log.Warn'd — and the
// caller returns immediately afterward without ever reaching any other
// notification path — reproducing BUG-048's original gap one layer
// underneath its own fix. notifyStuckReviewBookkeepingFailed (extracted from
// that call site so it's directly testable, at the same fidelity as
// TestNotifySpawnAndRollbackFailed_should_markStuckAndNotify_When_Called
// covers BUG-030's equivalent fix) is the closure of that gap.
func TestNotifyStuckReviewBookkeepingFailed_should_publishFailureNotification_When_Called(t *testing.T) {
	t.Parallel()
	eventBus := events.NewEventBus(4)
	svc := &AutonomousOrchestrationService{bus: eventBus}

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	svc.notifyStuckReviewBookkeepingFailed("item-456", "Stuck review item", "item-session-789",
		fmt.Errorf("failed to set ended_at on item session item-session-789: item_session not found"))

	var notif *events.Event
	for i := 0; i < 3; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 3
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "expected an operator-facing notification when the stuck-review bookkeeping write fails, instead of only being logged")
	assert.Equal(t, "Stuck-review bookkeeping failed", notif.NotificationTitle)
	assert.Equal(t, int32(9), notif.NotificationType, "must surface as a FAILURE notification")
	assert.Contains(t, notif.NotificationMessage, "Stuck review item")
	assert.Contains(t, notif.NotificationMessage, "could not mark the stalled review session ended")
}

// fakeFailingAutonomousStuckRespawner always returns err from
// AutoRespawnAutonomousWork, simulating a headless respawn attempt that
// fails (timeout, pool exhaustion, etc.) rather than a wiring/no-op gap.
type fakeFailingAutonomousStuckRespawner struct {
	err error
}

func (f *fakeFailingAutonomousStuckRespawner) AutoRespawnAutonomousWork(_ context.Context, _ string) error {
	return f.err
}

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NotifiesOperator_When_RespawnAttemptFails
// is the regression test for the MAJOR bug flagged in
// docs/tasks/backlog-feature-improvement.md's 2026-07-28 entry:
// AutoRespawnAutonomousWork errors were only log.Warn'd, with no operator
// notification until RemediationDue's justParked branch finally fired once
// the full attempt budget was exhausted (up to the ~4.5-day backoff
// schedule) — unlike the justParked branch a few lines above it in the same
// block, which already notifies immediately. This reproduces a single failed
// respawn attempt (first occurrence, so RemediationDue's fresh-row default
// grants it immediately) and asserts a non-terminal, WARNING-level
// notification is published right away, distinct from the "Autonomous fix
// stuck" generic notification onAutonomousDriverComplete always fires and
// from the terminal "Auto-rework paused" justParked notification.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NotifiesOperator_When_RespawnAttemptFails(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	// A cancellable context (not context.Background()) so SetLifecycleContext's
	// capacityMonitor.Start goroutine actually exits when the test ends, instead
	// of leaking for the life of the test binary.
	ctx, lifecycleCancel := context.WithCancel(context.Background())
	t.Cleanup(lifecycleCancel)
	eventBus := events.NewEventBus(4)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })
	svc.SetLifecycleContext(ctx)

	respawnErr := fmt.Errorf("headless pool exhausted")
	svc.SetAutonomousStuckRespawner(&fakeFailingAutonomousStuckRespawner{err: respawnErr})

	const title = "autonomous-respawn-failure-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Autonomous respawn failure test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.autonomousSvc.onAutonomousDriverComplete(title, outcome)

	var notif *events.Event
	for i := 0; i < 10; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification && ev.NotificationTitle == "Automated retry failed" {
				notif = ev
			}
		case <-time.After(2 * time.Second):
			i = 10
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "a failed respawn attempt must publish an operator-facing notification, not just a log line")
	assert.Equal(t, int32(8), notif.NotificationType, "must surface as a WARNING, not a terminal FAILURE")
	assert.Equal(t, int32(2), notif.NotificationPriority, "non-terminal — must not demand acknowledgment like the justParked notification does")
	assert.Contains(t, notif.NotificationMessage, "headless pool exhausted")
	assert.Contains(t, notif.NotificationMessage, "will retry automatically")
}
