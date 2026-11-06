package analytics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// recordingProvider captures all Record calls for assertion.
type recordingProvider struct {
	mu     sync.Mutex
	events []Event
}

func (r *recordingProvider) Record(_ context.Context, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *recordingProvider) recorded() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// waitForCount blocks until the provider has recorded at least n events or the
// deadline is exceeded.
func (r *recordingProvider) waitForCount(n int, deadline time.Duration) bool {
	timeout := time.After(deadline)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if len(r.recorded()) >= n {
			return true
		}
		select {
		case <-timeout:
			return false
		case <-tick.C:
		}
	}
}

func TestSubscriber_SessionCreated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewEventBus(10)
	provider := &recordingProvider{}

	StartAnalyticsSubscriber(ctx, bus, provider)

	inst := &session.Instance{Title: "test-session"}
	bus.Publish(events.NewSessionCreatedEvent(inst))

	if !provider.waitForCount(1, 500*time.Millisecond) {
		t.Fatal("timed out waiting for Record call")
	}

	evts := provider.recorded()
	if len(evts) != 1 {
		t.Fatalf("want 1 event, got %d", len(evts))
	}
	ev := evts[0]
	if ev.EventName != "session.created" {
		t.Errorf("want EventName=session.created, got %q", ev.EventName)
	}
	if ev.EventCategory != "user_action" {
		t.Errorf("want EventCategory=user_action, got %q", ev.EventCategory)
	}
}

func TestSubscriber_SessionDeleted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewEventBus(10)
	provider := &recordingProvider{}

	StartAnalyticsSubscriber(ctx, bus, provider)

	bus.Publish(events.NewSessionDeletedEvent("sess-xyz"))

	if !provider.waitForCount(1, 500*time.Millisecond) {
		t.Fatal("timed out waiting for Record call")
	}

	ev := provider.recorded()[0]
	if ev.EventName != "session.deleted" {
		t.Errorf("want EventName=session.deleted, got %q", ev.EventName)
	}
	if ev.SessionID != "sess-xyz" {
		t.Errorf("want SessionID=sess-xyz, got %q", ev.SessionID)
	}
}

func TestSubscriber_StatusChanged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewEventBus(10)
	provider := &recordingProvider{}

	StartAnalyticsSubscriber(ctx, bus, provider)

	inst := &session.Instance{ID: "sess-status", Title: "status-session", Status: session.Active}

	// First event: seeds lastStatusByID — no transition recorded.
	bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))

	// The subscriber drains a single FIFO channel, so the second event is
	// always processed after the first; no wait needed in between.
	// Second event: status changes Active → Stopped — transition should be recorded.
	inst2 := &session.Instance{ID: "sess-status", Title: "status-session", Status: session.Stopped}
	bus.Publish(events.NewSessionUpdatedEvent(inst2, []string{"status"}))

	if !provider.waitForCount(1, 500*time.Millisecond) {
		t.Fatal("timed out waiting for Record call")
	}

	ev := provider.recorded()[0]
	if ev.EventName != "session.status_changed" {
		t.Errorf("want EventName=session.status_changed, got %q", ev.EventName)
	}
	if ev.Labels["old_status"] != session.Active.String() {
		t.Errorf("want old_status=%q, got %q", session.Active.String(), ev.Labels["old_status"])
	}
	if ev.Labels["new_status"] != session.Stopped.String() {
		t.Errorf("want new_status=%q, got %q", session.Stopped.String(), ev.Labels["new_status"])
	}
}

func TestSubscriber_UnknownEventSkipped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewEventBus(10)
	provider := &recordingProvider{}

	StartAnalyticsSubscriber(ctx, bus, provider)

	// Publish a notification event (not in the handled set).
	bus.Publish(events.NewNotificationEvent(
		"sess-1", "Session", "notif-id", 0, 0, "title", "body", nil,
	))

	// Sentinel: events are processed in order, so once the sentinel is recorded
	// the notification has already been handled (and skipped).
	bus.Publish(events.NewSessionDeletedEvent("sentinel"))
	if !provider.waitForCount(1, 500*time.Millisecond) {
		t.Fatal("timed out waiting for sentinel Record call")
	}

	got := provider.recorded()
	if len(got) != 1 || got[0].SessionID != "sentinel" {
		t.Errorf("want only the sentinel event recorded, got %+v", got)
	}
}
