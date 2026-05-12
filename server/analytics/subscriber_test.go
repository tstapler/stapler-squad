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
	for {
		select {
		case <-timeout:
			return false
		default:
			if len(r.recorded()) >= n {
				return true
			}
			time.Sleep(5 * time.Millisecond)
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

	inst := &session.Instance{Title: "status-session"}
	bus.Publish(events.NewSessionStatusChangedEvent(inst, session.Running, session.Stopped))

	if !provider.waitForCount(1, 500*time.Millisecond) {
		t.Fatal("timed out waiting for Record call")
	}

	ev := provider.recorded()[0]
	if ev.EventName != "session.status_changed" {
		t.Errorf("want EventName=session.status_changed, got %q", ev.EventName)
	}
	if ev.Labels["old_status"] != session.Running.String() {
		t.Errorf("want old_status=%q, got %q", session.Running.String(), ev.Labels["old_status"])
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

	// Give the goroutine time to process the event.
	time.Sleep(50 * time.Millisecond)

	if n := len(provider.recorded()); n != 0 {
		t.Errorf("want 0 events recorded, got %d", n)
	}
}
