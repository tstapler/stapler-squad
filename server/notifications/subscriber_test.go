package notifications

import (
	"claude-squad/server/events"
	"context"
	"sync"
	"testing"
	"time"
)

// mockAppender records all calls to Append for test assertions.
type mockAppender struct {
	mu      sync.Mutex
	records []*NotificationRecord
}

func (m *mockAppender) Append(record *NotificationRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
	return nil
}

func (m *mockAppender) getRecords() []*NotificationRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*NotificationRecord, len(m.records))
	copy(result, m.records)
	return result
}

// publishNotification publishes a notification event to the bus.
func publishNotification(bus *events.EventBus, sessionID string, notifType int32, id string) {
	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            sessionID,
		Context:              sessionID,
		NotificationID:       id,
		NotificationType:     notifType,
		NotificationPriority: 1,
		NotificationTitle:    "Test " + id,
		NotificationMessage:  "Message " + id,
		NotificationMetadata: map[string]string{"id": id},
	})
}

// TestCoalescing_SameKeyWithinWindow verifies that 10 events for the same
// (sessionID, notificationType) within the coalescing window result in
// exactly 1 Append() call.
func TestCoalescing_SameKeyWithinWindow(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	// Use a short coalescing interval for testing
	StartSubscriberWithInterval(ctx, bus, appender, 50*time.Millisecond)

	// Allow subscriber goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Publish 10 events for the same key rapidly
	for i := 0; i < 10; i++ {
		publishNotification(bus, "session-A", 1, "notif-"+string(rune('a'+i)))
	}

	// Wait for flush (coalescing interval + buffer)
	time.Sleep(120 * time.Millisecond)

	// Cancel to trigger final flush
	cancel()
	time.Sleep(30 * time.Millisecond)

	records := appender.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 Append call (coalesced), got %d", len(records))
		for i, r := range records {
			t.Logf("  record[%d]: sessionID=%s, type=%d, id=%s", i, r.SessionID, r.NotificationType, r.ID)
		}
	}
}

// TestCoalescing_DifferentKeysFlushIndependently verifies that events for
// different (sessionID, notificationType) keys are flushed as separate records.
func TestCoalescing_DifferentKeysFlushIndependently(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	StartSubscriberWithInterval(ctx, bus, appender, 50*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	// Publish events for 3 different keys
	publishNotification(bus, "session-A", 1, "notif-a1")
	publishNotification(bus, "session-A", 1, "notif-a2") // same key as above, should coalesce
	publishNotification(bus, "session-B", 1, "notif-b1")
	publishNotification(bus, "session-A", 2, "notif-a3") // different type

	// Wait for flush
	time.Sleep(120 * time.Millisecond)

	cancel()
	time.Sleep(30 * time.Millisecond)

	records := appender.getRecords()
	if len(records) != 3 {
		t.Errorf("expected 3 Append calls (3 distinct keys), got %d", len(records))
		for i, r := range records {
			t.Logf("  record[%d]: sessionID=%s, type=%d, id=%s", i, r.SessionID, r.NotificationType, r.ID)
		}
	}
}

// TestCoalescing_ContextCancellationFlushes verifies that canceling the context
// triggers an immediate flush of remaining buffered records.
func TestCoalescing_ContextCancellationFlushes(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	// Use a very long coalescing interval so it won't fire naturally during the test
	StartSubscriberWithInterval(ctx, bus, appender, 10*time.Second)

	time.Sleep(10 * time.Millisecond)

	// Publish an event
	publishNotification(bus, "session-A", 1, "notif-1")

	// Give time for the event to be buffered
	time.Sleep(20 * time.Millisecond)

	// Verify nothing has been flushed yet (interval is 10s)
	records := appender.getRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 Append calls before flush, got %d", len(records))
	}

	// Cancel context -- should trigger deferred flush
	cancel()
	time.Sleep(50 * time.Millisecond)

	records = appender.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 Append call after context cancellation, got %d", len(records))
	}
}

// TestCoalescing_LatestEventWins verifies that when multiple events arrive for
// the same key, the latest one's data is used when flushing.
func TestCoalescing_LatestEventWins(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	StartSubscriberWithInterval(ctx, bus, appender, 50*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	// Publish multiple events with different metadata for the same key
	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "session-A",
		Context:              "session-A",
		NotificationID:       "first",
		NotificationType:     1,
		NotificationPriority: 1,
		NotificationTitle:    "First",
		NotificationMessage:  "First message",
		NotificationMetadata: map[string]string{"approval_id": "old"},
	})

	time.Sleep(2 * time.Millisecond)

	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "session-A",
		Context:              "session-A",
		NotificationID:       "latest",
		NotificationType:     1,
		NotificationPriority: 1,
		NotificationTitle:    "Latest",
		NotificationMessage:  "Latest message",
		NotificationMetadata: map[string]string{"approval_id": "new"},
	})

	// Wait for flush
	time.Sleep(120 * time.Millisecond)

	cancel()
	time.Sleep(30 * time.Millisecond)

	records := appender.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.ID != "latest" {
		t.Errorf("expected latest event ID, got %s", rec.ID)
	}
	if rec.Metadata["approval_id"] != "new" {
		t.Errorf("expected metadata approval_id='new', got '%s'", rec.Metadata["approval_id"])
	}
	if rec.Title != "Latest" {
		t.Errorf("expected Title='Latest', got '%s'", rec.Title)
	}
}

// TestCoalescing_NonNotificationEventsIgnored verifies that events of other
// types are ignored by the subscriber.
func TestCoalescing_NonNotificationEventsIgnored(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	StartSubscriberWithInterval(ctx, bus, appender, 50*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	// Publish a non-notification event
	bus.Publish(&events.Event{
		Type:      events.EventSessionCreated,
		Timestamp: time.Now(),
		SessionID: "session-A",
	})

	time.Sleep(120 * time.Millisecond)

	cancel()
	time.Sleep(30 * time.Millisecond)

	records := appender.getRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 records for non-notification events, got %d", len(records))
	}
}
