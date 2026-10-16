package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// newCountingGateService is a SessionService whose delivery gate is attached
// but not installed on the bus, for tests that only read the visibility index.
func newCountingGateService(t *testing.T) (*SessionService, *pkgevents.EventBus, *deliverygate.Gate) {
	t.Helper()
	storage := createTestStorage(t)
	bus := pkgevents.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	gate := deliverygate.NewGate()
	svc.deliveryGate = gate
	return svc, bus, gate
}

// newHiddenSiteEnv builds a gated service and seeds one hidden session. flagOn
// pins the global flag explicitly so each test states the state it asserts.
func newHiddenSiteEnv(t *testing.T, flagOn bool, inst *session.Instance) *crashEnv {
	t.Helper()
	e := newCrashEnv(t, true)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.HiddenSessionGateFeatureFlag, flagOn))
	gate := e.svc.DeliveryGate()
	gate.Flags().Reload()
	gate.SeedFromInstances([]*session.Instance{inst})
	return e
}

// The four session_events.go producers no longer check Instance.Hidden: with the
// gate on (the default) only the FAILURE-class recovery failure reaches the bus
// for a hidden session, and the routine WARNING/INFO events are dropped by the
// gate. The session-state sync events still fire.
func TestHiddenSessionEvents_ShouldDeliverOnlyRecoveryFailure_WhenGateOn(t *testing.T) {
	inst := &session.Instance{Title: "hidden-sites", UUID: "hidden-sites-uuid", Hidden: true}
	e := newHiddenSiteEnv(t, true, inst)

	e.svc.onColdRestoreLostHistory(inst)
	e.svc.onRateLimitDetected(inst, inst.UUID, time.Time{})
	e.svc.onRateLimitRecoverySucceeded(inst, inst.UUID)
	e.svc.onRateLimitRecoveryFailed(inst, inst.UUID, "boom")

	got := e.collect(t)
	var notifs int
	for _, ev := range got {
		if ev.NotificationID != "" {
			notifs++
		}
	}
	require.Equal(t, 1, notifs, "only the failure reaches the bus: %v", got)
	require.Len(t, notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE)), 1)
	m := e.svc.DeliveryGate().Metrics()
	assert.EqualValues(t, 3, m.Total(deliverygate.CounterSuppressed), "cold restore, rate limit, recovery success")
	assert.EqualValues(t, 1, m.Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "other"))
}

// Rollback: with the gate off every site delivers for a hidden session and the
// shadow counters record what would have been suppressed.
func TestHiddenSessionEvents_ShouldDeliverEverything_WhenGateOff(t *testing.T) {
	inst := &session.Instance{Title: "hidden-sites-off", UUID: "hidden-sites-off-uuid", Hidden: true}
	e := newHiddenSiteEnv(t, false, inst)

	e.svc.onColdRestoreLostHistory(inst)
	e.svc.onRateLimitDetected(inst, inst.UUID, time.Time{})
	e.svc.onRateLimitRecoverySucceeded(inst, inst.UUID)
	e.svc.onRateLimitRecoveryFailed(inst, inst.UUID, "boom")

	got := e.collect(t)
	var notifs int
	for _, ev := range got {
		if ev.NotificationID != "" {
			notifs++
		}
	}
	assert.Equal(t, 4, notifs, "gate off: nothing is dropped")
	m := e.svc.DeliveryGate().Metrics()
	assert.EqualValues(t, 0, m.Total(deliverygate.CounterSuppressed))
	assert.EqualValues(t, 3, m.Total(deliverygate.CounterWouldSuppress))
}

// A hidden non-review autonomous run: "Autonomous fix stuck" is a FAILURE the
// operator must see (previously swallowed by a legacy !inst.Hidden check);
// "Autonomous fix complete" is routine INFO and is dropped by the gate.
func TestAutonomous_ShouldDeliverStuckFailureAndDropCompleteInfo_WhenHiddenAndGateOn(t *testing.T) {
	const title = "hidden-generic-gated"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test", Status: session.Paused,
		Program: "claude", AutonomousMode: true, Hidden: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	e := newHiddenSiteEnv(t, true, inst)
	require.NoError(t, e.svc.storage.AddInstance(inst))
	e.svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	e.svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: false, Reason: "stuck", Turns: 3})
	e.svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: true, Reason: "all good"})

	got := e.collect(t)
	failures := notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE))
	require.Len(t, failures, 1, "stuck run delivers one FAILURE for a hidden session")
	assert.Equal(t, "Autonomous fix stuck", failures[0].NotificationTitle)
	assert.Empty(t, notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO)),
		"routine completion INFO is dropped by the gate")
}

// The capacity monitor's guardrail stop is a hard stop: its WARNING carries the
// failure stamp so a hidden session's termination still delivers.
func TestCapacityGuardrailStop_ShouldDeliverForHiddenSession_WhenGateOn(t *testing.T) {
	inst := &session.Instance{Title: "hidden-capacity", UUID: "hidden-capacity-uuid", Hidden: true}
	e := newHiddenSiteEnv(t, true, inst)
	m := NewCapacityMonitor(CapacityMonitorParams{EventBus: e.bus})

	m.stopForGuardrail(t.Context(), inst, inst.Snapshot(), "cost_budget_exceeded", "Budget exceeded", "stopped")

	got := notificationsOf(e.collect(t), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING))
	require.Len(t, got, 1, "stamped hard-stop WARNING must pass the gate for a hidden session")
	assert.EqualValues(t, 1, e.svc.DeliveryGate().Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "other"))
}
