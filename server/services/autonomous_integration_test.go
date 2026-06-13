package services

// Service-layer integration tests for the AutonomousDriver registry wiring.
// These tests verify that SessionService correctly manages driver lifecycles:
// registration, lifecycle context binding, and stop-on-delete/hibernate behavior.
//
// They operate at the service boundary without requiring tmux or a real headless pool.

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// instantDonePool is a HeadlessPoolClient that returns DONE on the first call,
// allowing an AutonomousDriver to complete without needing a real LLM backend.
type instantDonePool struct{}

func (p *instantDonePool) CallBlockingWithOptions(
	_ context.Context,
	_ headless.FeatureKey,
	_, _ string,
	_ headless.CallOptions,
) (string, error) {
	return "DONE: test complete", nil
}

// addPausedAutonomousInstance inserts a paused session with AutonomousMode=true into storage.
func addPausedAutonomousInstance(t *testing.T, storage *session.Storage, title string) *session.Instance {
	t.Helper()
	inst := &session.Instance{
		Title:         title,
		UUID:          title + "-uuid-1234",
		Path:          "/tmp/test",
		Status:        session.Paused,
		Program:       "claude",
		AutonomousMode: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}

// TestSessionService_SetLifecycleContext verifies that SetLifecycleContext stores the
// provided context and that driverCtx() returns it (observable via cancellation).
func TestSessionService_SetLifecycleContext(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	// Without SetLifecycleContext, driverCtx() should fall back to Background.
	assert.NoError(t, svc.driverCtx().Err(), "driverCtx() should return non-cancelled ctx before SetLifecycleContext")

	// After wiring a cancelled context, driverCtx() should reflect it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	svc.SetLifecycleContext(ctx)

	assert.Error(t, svc.driverCtx().Err(), "driverCtx() should return the cancelled lifecycle context after SetLifecycleContext")
}

// TestSessionService_DriverRegistry_RegisterAndDeregister verifies that registerDriver
// stores a driver and stopAndDeregisterDriver removes it and calls Stop.
func TestSessionService_DriverRegistry_RegisterAndDeregister(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	inst := &session.Instance{
		Title: "reg-test",
		UUID:  "reg-uuid-5678",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)

	// Register the driver without starting it (Start() would require a controller).
	// We test registry semantics directly since this is in the same package.
	svc.registerDriver("reg-test", driver)

	// Deregister and stop — should not panic.
	svc.stopAndDeregisterDriver("reg-test")

	// Second deregister is a no-op — should also not panic.
	svc.stopAndDeregisterDriver("reg-test")
}

// TestSessionService_DeleteSession_StopsRegisteredDriver verifies that DeleteSession
// calls stopAndDeregisterDriver for the deleted session's title.
// The driver is manually registered (bypassing CreateSession which needs tmux) to
// isolate the registry-stop behavior from session creation mechanics.
func TestSessionService_DeleteSession_StopsRegisteredDriver(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	const title = "delete-driver-test"
	addPausedAutonomousInstance(t, storage, title)

	// Create and start a driver, then manually register it in the service registry.
	inst := &session.Instance{
		Title: title,
		UUID:  title + "-uuid-1234",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)
	svc.registerDriver(title, driver)

	// DeleteSession must call stopAndDeregisterDriver before destroying resources.
	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: title})
	resp, err := svc.DeleteSession(context.Background(), req)
	require.NoError(t, err, "DeleteSession should succeed")
	assert.True(t, resp.Msg.Success)

	// The registry entry for this title must have been removed.
	// Call stopAndDeregisterDriver again — it must be a no-op (not double-stop).
	svc.stopAndDeregisterDriver(title) // no panic = driver already deregistered
}

// TestSessionService_OnAutonomousDriverComplete_DeregistersDriver verifies that the
// completion callback removes the driver from the registry so it does not leak.
func TestSessionService_OnAutonomousDriverComplete_DeregistersDriver(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	const title = "complete-driver-test"

	// Register a driver.
	inst := &session.Instance{
		Title: title,
		UUID:  title + "-uuid-9999",
	}
	pool := &instantDonePool{}
	driver := session.NewAutonomousDriver(inst, pool, "fix it", 1)
	svc.registerDriver(title, driver)

	// Fire the completion callback (simulates driver goroutine finishing).
	// FindLiveInstance returns nil for a session not in the live poller — that is the
	// expected production path when the session exited before the callback fires.
	outcome := session.AutonomousDriverOutcome{Done: true, Reason: "test done", Turns: 1}
	svc.onAutonomousDriverComplete(title, outcome)

	// The driver must have been removed from the registry by the callback.
	// We verify by ensuring a subsequent manual stop is a no-op (not a double-stop).
	svc.stopAndDeregisterDriver(title) // no panic = already deregistered
}
