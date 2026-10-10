package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

func newCountingGateService(t *testing.T) (*SessionService, *events.EventBus, *deliverygate.Gate) {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	gate := deliverygate.NewGate()
	svc.deliveryGate = gate
	svc.autonomousSvc.SetLegacyHiddenCounter(gate.CountLegacySuppressedType)
	return svc, bus, gate
}

func assertNoLegacyNotification(t *testing.T, ch <-chan *events.Event) {
	t.Helper()
	for {
		select {
		case ev := <-ch:
			require.NotEqual(t, events.EventNotification, ev.Type, "legacy check must still swallow the hidden event")
		default:
			return
		}
	}
}

// T-LG-01..T-LG-04 and T-LG-09 (session_service_events.go sites): behavior is
// unchanged (nothing published for a hidden session) and each swallow is counted
// with the closed site label, the type and the class the policy would assign.
func TestLegacyCounters_ShouldLabelSiteTypeAndClass_WhenSessionEventSitesFireForHiddenSession(t *testing.T) {
	t.Parallel()
	svc, bus, gate := newCountingGateService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	inst := &session.Instance{Title: "hidden-legacy", UUID: "hidden-legacy-uuid", Hidden: true}
	svc.onColdRestoreLostHistory(inst)
	svc.onRateLimitDetected(inst, inst.UUID, time.Time{})
	svc.onRateLimitRecoverySucceeded(inst, inst.UUID)
	svc.onRateLimitRecoveryFailed(inst, inst.UUID, "boom")

	// The session-state sync events still fire; only notifications must be absent.
	assertNoLegacyNotification(t, ch)

	m := gate.Metrics()
	for _, c := range []struct{ site, typ, class string }{
		{"session_events_cold_restore", "NOTIFICATION_TYPE_WARNING", "routine"},
		{"session_events_rate_limit_detected", "NOTIFICATION_TYPE_WARNING", "routine"},
		{"session_events_recovery_succeeded", "NOTIFICATION_TYPE_INFO", "routine"},
		// The class column shows what PR 2b would newly deliver: a swallowed FAILURE.
		{"session_events_recovery_failed", "NOTIFICATION_TYPE_FAILURE", "failure"},
	} {
		assert.EqualValues(t, 1, m.Value(deliverygate.CounterLegacySuppressed, c.site, c.typ, c.class), c.site)
	}
}

// T-LG-06: the autonomous generic notifier is still skipped for hidden sessions
// and counted (site autonomous_generic, type FAILURE for a stuck run).
func TestAutonomous_ShouldCountSkippedHiddenGenericNotify_WhenFlagOff(t *testing.T) {
	t.Parallel()
	svc, bus, gate := newCountingGateService(t)

	const title = "hidden-generic-counter"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test", Status: session.Paused,
		Program: "claude", AutonomousMode: true, Hidden: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, svc.storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	svc.autonomousSvc.onAutonomousDriverComplete(title, session.AutonomousDriverOutcome{Done: false, Reason: "stuck", Turns: 3})
	assertNoLegacyNotification(t, ch)
	assert.EqualValues(t, 1, gate.Metrics().Total(deliverygate.CounterLegacySuppressed),
		"exactly one legacy swallow must be counted: %v", gate.Metrics().Snapshot())
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterLegacySuppressed,
		"autonomous_generic", "NOTIFICATION_TYPE_FAILURE", "failure"))
}
