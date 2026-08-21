package events

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// generateSubscriberID creates a unique identifier for a subscriber.
// Uses cryptographically secure random bytes to ensure uniqueness.
func generateSubscriberID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simpler ID if random fails (should never happen)
		return fmt.Sprintf("sub-%d", len(bytes))
	}
	return hex.EncodeToString(bytes)
}

// Subscriber represents an active event bus subscription.
// Provides a convenient wrapper around the channel and subscription ID.
type Subscriber struct {
	ID      string
	Events  <-chan *Event
	cleanup func()
}

// NewSubscriber creates a Subscriber wrapper.
// This is primarily for convenience and testing.
func NewSubscriber(id string, events <-chan *Event, cleanup func()) *Subscriber {
	return &Subscriber{
		ID:      id,
		Events:  events,
		cleanup: cleanup,
	}
}

// Close unsubscribes and cleans up the subscriber.
func (s *Subscriber) Close() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
