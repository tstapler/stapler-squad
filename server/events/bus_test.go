package events

import (
	"claude-squad/session"
	"context"
	"sync"
	"testing"
	"time"
)

// TestEventBusBasicSubscribePublish tests basic subscription and event delivery.
func TestEventBusBasicSubscribePublish(t *testing.T) {
	bus := NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to events
	events, subID := bus.Subscribe(ctx)

	// Create a test event
	testEvent := NewSessionCreatedEvent(&session.Instance{Title: "test-session"})

	// Publish event
	bus.Publish(testEvent)

	// Verify event received
	select {
	case received := <-events:
		if received.Type != EventSessionCreated {
			t.Errorf("Expected event type %s, got %s", EventSessionCreated, received.Type)
		}
		if received.Session.Title != "test-session" {
			t.Errorf("Expected session title 'test-session', got '%s'", received.Session.Title)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	// Verify subscriber count
	if count := bus.SubscriberCount(); count != 1 {
		t.Errorf("Expected 1 subscriber, got %d", count)
	}

	// Unsubscribe
	bus.Unsubscribe(subID)

	// Verify subscriber removed
	if count := bus.SubscriberCount(); count != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", count)
	}
}

// TestEventBusMultipleSubscribers tests broadcasting to multiple subscribers.
func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple subscribers
	numSubscribers := 5
	subscribers := make([]<-chan *Event, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		events, _ := bus.Subscribe(ctx)
		subscribers[i] = events
	}

	// Verify subscriber count
	if count := bus.SubscriberCount(); count != numSubscribers {
		t.Errorf("Expected %d subscribers, got %d", numSubscribers, count)
	}

	// Publish event
	testEvent := NewSessionDeletedEvent("test-id")
	bus.Publish(testEvent)

	// Verify all subscribers receive the event
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	for i, sub := range subscribers {
		go func(idx int, events <-chan *Event) {
			defer wg.Done()
			select {
			case received := <-events:
				if received.Type != EventSessionDeleted {
					t.Errorf("Subscriber %d: Expected event type %s, got %s", idx, EventSessionDeleted, received.Type)
				}
				if received.SessionID != "test-id" {
					t.Errorf("Subscriber %d: Expected session ID 'test-id', got '%s'", idx, received.SessionID)
				}
			case <-time.After(time.Second):
				t.Errorf("Subscriber %d: Timeout waiting for event", idx)
			}
		}(i, sub)
	}

	wg.Wait()
}

// TestEventBusConcurrentPublish tests thread safety with concurrent publishing.
func TestEventBusConcurrentPublish(t *testing.T) {
	bus := NewEventBus(100)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create subscriber
	events, _ := bus.Subscribe(ctx)

	// Track received events
	received := make(map[string]bool)
	var mu sync.Mutex

	// Start receiving events
	done := make(chan bool)
	go func() {
		for event := range events {
			mu.Lock()
			received[event.SessionID] = true
			mu.Unlock()
		}
		done <- true
	}()

	// Publish events concurrently
	numEvents := 100
	var wg sync.WaitGroup
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func(id int) {
			defer wg.Done()
			event := NewSessionDeletedEvent(string(rune(id)))
			bus.Publish(event)
		}(i)
	}

	wg.Wait()

	// Wait for all events to be received
	time.Sleep(100 * time.Millisecond)
	cancel() // Close subscriber

	<-done

	// Verify we received events (exact count may vary due to timing)
	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count == 0 {
		t.Error("Expected to receive some events, got none")
	}
}

// TestEventBusContextCancellation tests automatic cleanup on context cancel.
func TestEventBusContextCancellation(t *testing.T) {
	bus := NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Subscribe
	events, subID := bus.Subscribe(ctx)

	// Verify subscriber exists
	if count := bus.SubscriberCount(); count != 1 {
		t.Errorf("Expected 1 subscriber, got %d", count)
	}

	// Cancel context
	cancel()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify subscriber was removed
	if count := bus.SubscriberCount(); count != 0 {
		t.Errorf("Expected 0 subscribers after context cancel, got %d", count)
	}

	// Verify channel is closed
	select {
	case _, ok := <-events:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("Channel should have been closed immediately")
	}

	// Verify double unsubscribe is safe
	bus.Unsubscribe(subID)
}

// TestEventBusBufferOverflow tests behavior when subscriber buffer is full.
func TestEventBusBufferOverflow(t *testing.T) {
	bufferSize := 5
	bus := NewEventBus(bufferSize)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create slow subscriber (doesn't consume events)
	events, _ := bus.Subscribe(ctx)

	// Publish more events than buffer can hold
	for i := 0; i < bufferSize*2; i++ {
		event := NewSessionDeletedEvent(string(rune(i)))
		bus.Publish(event)
	}

	// Verify buffer has events but didn't block
	received := 0
	timeout := time.After(100 * time.Millisecond)

drainLoop:
	for {
		select {
		case <-events:
			received++
		case <-timeout:
			break drainLoop
		}
	}

	// Should have received exactly buffer size (overflow events dropped)
	if received > bufferSize+1 { // +1 for race condition tolerance
		t.Errorf("Expected at most %d events (buffer size), got %d", bufferSize, received)
	}
}

// TestEventBusClose tests graceful shutdown.
func TestEventBusClose(t *testing.T) {
	bus := NewEventBus(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple subscribers
	numSubscribers := 3
	channels := make([]<-chan *Event, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		events, _ := bus.Subscribe(ctx)
		channels[i] = events
	}

	// Close the bus
	bus.Close()

	// Verify all channels are closed
	for i, ch := range channels {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("Subscriber %d: Expected channel to be closed", i)
			}
		case <-time.After(time.Second):
			t.Errorf("Subscriber %d: Timeout waiting for channel close", i)
		}
	}

	// Verify subscriber count is zero
	if count := bus.SubscriberCount(); count != 0 {
		t.Errorf("Expected 0 subscribers after close, got %d", count)
	}
}

// TestEventBusEventTypes tests all event type constructors.
func TestEventBusEventTypes(t *testing.T) {
	testSession := &session.Instance{Title: "test"}

	tests := []struct {
		name      string
		event     *Event
		eventType EventType
	}{
		{
			name:      "SessionCreated",
			event:     NewSessionCreatedEvent(testSession),
			eventType: EventSessionCreated,
		},
		{
			name:      "SessionUpdated",
			event:     NewSessionUpdatedEvent(testSession, []string{"title", "category"}),
			eventType: EventSessionUpdated,
		},
		{
			name:      "SessionDeleted",
			event:     NewSessionDeletedEvent("test-id"),
			eventType: EventSessionDeleted,
		},
		{
			name:      "SessionStatusChanged",
			event:     NewSessionStatusChangedEvent(testSession, session.Running, session.Paused),
			eventType: EventSessionStatusChanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type != tt.eventType {
				t.Errorf("Expected event type %s, got %s", tt.eventType, tt.event.Type)
			}
			if tt.event.Timestamp.IsZero() {
				t.Error("Expected timestamp to be set")
			}
		})
	}
}
