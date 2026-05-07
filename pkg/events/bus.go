package events

import (
	"context"
	"sync"
)

// EventBus provides a thread-safe pub/sub event bus for session events.
// It uses Go channels for event distribution and supports multiple concurrent subscribers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan *Event
	bufferSize  int
}

// NewEventBus creates a new event bus with the specified buffer size.
// Buffer size determines how many events can be queued per subscriber before dropping.
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer size
	}
	return &EventBus{
		subscribers: make(map[string]chan *Event),
		bufferSize:  bufferSize,
	}
}

// Subscribe creates a new subscription to the event bus.
// Returns a read-only channel for receiving events and a subscription ID for cleanup.
// The subscription is automatically cleaned up when the context is canceled.
func (eb *EventBus) Subscribe(ctx context.Context) (<-chan *Event, string) {
	ch := make(chan *Event, eb.bufferSize)
	id := generateSubscriberID()

	eb.mu.Lock()
	eb.subscribers[id] = ch
	eb.mu.Unlock()

	// Cleanup subscription on context cancellation
	go func() {
		<-ctx.Done()
		eb.Unsubscribe(id)
	}()

	return ch, id
}

// Publish broadcasts an event to all active subscribers.
// Events are sent asynchronously and non-blocking. If a subscriber's buffer is full,
// the event is dropped for that subscriber to prevent blocking other subscribers.
func (eb *EventBus) Publish(event *Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Subscriber is slow, drop event to prevent blocking
			// In production, this could be logged for monitoring
		}
	}
}

// Unsubscribe removes a subscriber and closes their channel.
// This is idempotent - calling it multiple times with the same ID is safe.
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if ch, exists := eb.subscribers[id]; exists {
		close(ch)
		delete(eb.subscribers, id)
	}
}

// SubscriberCount returns the current number of active subscribers.
// Useful for monitoring and testing.
func (eb *EventBus) SubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers)
}

// Close unsubscribes all subscribers and closes their channels.
// Should be called during graceful shutdown.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for id, ch := range eb.subscribers {
		close(ch)
		delete(eb.subscribers, id)
	}
}
