package notifications

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/testutil"
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
	StartSubscriberWithInterval(ctx, bus, appender, 5*time.Millisecond)

	// Publish 10 events for the same key rapidly
	for i := 0; i < 10; i++ {
		publishNotification(bus, "session-A", 1, "notif-"+string(rune('a'+i)))
	}

	// Wait for flush (coalescing interval + buffer)
	if err := testutil.WaitForCondition(func() bool {
		return len(appender.getRecords()) >= 1
	}, testutil.FastWaitConfig()); err != nil {
		// Cancel to trigger final flush and wait again
		cancel()
		_ = testutil.WaitForCondition(func() bool {
			return len(appender.getRecords()) >= 1
		}, testutil.FastWaitConfig())
	} else {
		cancel()
	}

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

	StartSubscriberWithInterval(ctx, bus, appender, 5*time.Millisecond)

	// Publish events for 3 different keys
	publishNotification(bus, "session-A", 1, "notif-a1")
	publishNotification(bus, "session-A", 1, "notif-a2") // same key as above, should coalesce
	publishNotification(bus, "session-B", 1, "notif-b1")
	publishNotification(bus, "session-A", 2, "notif-a3") // different type

	// Wait for flush (all 3 distinct keys)
	if err := testutil.WaitForCondition(func() bool {
		return len(appender.getRecords()) >= 3
	}, testutil.FastWaitConfig()); err != nil {
		cancel()
		_ = testutil.WaitForCondition(func() bool {
			return len(appender.getRecords()) >= 3
		}, testutil.FastWaitConfig())
	} else {
		cancel()
	}

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

	// Publish an event
	publishNotification(bus, "session-A", 1, "notif-1")

	// Verify nothing has been flushed yet (interval is 10s, so flushing only happens on cancel)
	records := appender.getRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 Append calls before flush, got %d", len(records))
	}

	// Cancel context -- should trigger deferred flush
	cancel()
	if err := testutil.WaitForCondition(func() bool {
		return len(appender.getRecords()) >= 1
	}, testutil.FastWaitConfig()); err != nil {
		t.Errorf("timed out waiting for flush after context cancellation: %v", err)
	}

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

	// Use a long flush interval so both events land before any flush fires.
	StartSubscriberWithInterval(ctx, bus, appender, 100*time.Millisecond)

	// Publish both events back-to-back before the first flush window closes.
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

	// Wait for the flush window to fire.
	if err := testutil.WaitForCondition(func() bool {
		return len(appender.getRecords()) >= 1
	}, testutil.FastWaitConfig()); err != nil {
		cancel()
		_ = testutil.WaitForCondition(func() bool {
			return len(appender.getRecords()) >= 1
		}, testutil.FastWaitConfig())
	} else {
		cancel()
	}

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

// TestCoalescing_DifferentBacklogItemsSurviveWithinWindow reproduces the bug reported
// 2026-07-19/20 ("I keep getting a PR Creation failed notification, it doesn't show up
// on the notification page"): every backlog-item notification call site
// (server/services/backlog_notifier.go's EventBusNotifier.Notify and the three
// notify* helpers in backlog_service_triage.go) used to publish with an EMPTY
// sessionID, even though the real backlog item ID was available and only went into
// metadata["item_id"]. Since coalesceKey is "sessionID:notificationType", two
// DIFFERENT items' same-type notifications landing in the same coalescing window
// collapsed to the same key and only the last one survived to the persisted store —
// even though the live toast for the dropped one still fired.
//
// The fix threads the real item ID through as sessionID (see EventBusNotifier.Notify),
// which this test verifies: two distinct "items" (simulated here directly at the
// subscriber level, which is agnostic to whether the sessionID came from a real
// session or a backlog item) publishing the same notification type within the
// coalescing window must both survive to the store, not clobber each other.
func TestCoalescing_DifferentBacklogItemsSurviveWithinWindow(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	StartSubscriberWithInterval(ctx, bus, appender, 5*time.Millisecond)

	const warningType = int32(8) // NOTIFICATION_TYPE_WARNING

	// Two different backlog items, both hitting the same WARNING-type notification
	// (e.g. "rework cap reached") within the same coalescing window. Prior to the fix,
	// both call sites published with sessionID="", so these would share the coalesce
	// key ":8" and only one would survive.
	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "item-aaaa-1111",
		NotificationID:       "notif-item-a",
		NotificationType:     warningType,
		NotificationPriority: 2,
		NotificationTitle:    "Auto-rework cap reached",
		NotificationMessage:  "Item A — hit the rework cap.",
		NotificationMetadata: map[string]string{"item_id": "item-aaaa-1111"},
	})
	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "item-bbbb-2222",
		NotificationID:       "notif-item-b",
		NotificationType:     warningType,
		NotificationPriority: 2,
		NotificationTitle:    "Auto-rework cap reached",
		NotificationMessage:  "Item B — hit the rework cap.",
		NotificationMetadata: map[string]string{"item_id": "item-bbbb-2222"},
	})

	if err := testutil.WaitForCondition(func() bool {
		return len(appender.getRecords()) >= 2
	}, testutil.FastWaitConfig()); err != nil {
		cancel()
		_ = testutil.WaitForCondition(func() bool {
			return len(appender.getRecords()) >= 2
		}, testutil.FastWaitConfig())
	} else {
		cancel()
	}

	records := appender.getRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 Append calls (one per distinct backlog item), got %d — different items' same-type notifications clobbered each other", len(records))
	}

	seenIDs := map[string]bool{}
	for _, r := range records {
		seenIDs[r.ID] = true
	}
	if !seenIDs["notif-item-a"] || !seenIDs["notif-item-b"] {
		t.Errorf("expected both notif-item-a and notif-item-b to survive, got records: %+v", records)
	}
}

// TestEventToRecord_BacklogItemSessionNameStaysEmpty verifies that when a
// notification's SessionID has been set to a backlog item ID (metadata["item_id"]
// present, per the coalescing-key fix in EventBusNotifier.Notify) and Context
// (sessionName) is empty, eventToRecord does NOT fall back SessionName to the raw
// item ID. That fallback exists for genuine session notifications (so a nameless
// session still gets a displayable identifier) but would leak a raw item UUID into
// SessionName for backlog items — which wins the frontend's title fallback chain
// (sessionName || title || sessionId, see NotificationPanel.tsx) and would clobber
// the real, descriptive notification title (e.g. "Auto-rework cap reached").
func TestEventToRecord_BacklogItemSessionNameStaysEmpty(t *testing.T) {
	event := &events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "item-aaaa-1111",
		Context:              "", // no friendly session name — this is a backlog item, not a session
		NotificationID:       "notif-1",
		NotificationType:     8,
		NotificationPriority: 2,
		NotificationTitle:    "Auto-rework cap reached",
		NotificationMessage:  "Item A — hit the rework cap.",
		NotificationMetadata: map[string]string{"item_id": "item-aaaa-1111"},
	}

	record := eventToRecord(event)
	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if record.SessionName != "" {
		t.Errorf("expected SessionName to stay empty for a backlog-item notification, got %q", record.SessionName)
	}
	if record.SessionID != "item-aaaa-1111" {
		t.Errorf("expected SessionID to be the item ID, got %q", record.SessionID)
	}
}

// TestEventToRecord_RealSessionFallsBackToSessionID verifies the pre-existing
// fallback (SessionName = SessionID when Context is empty and there's no
// metadata["item_id"]) still holds for genuine session notifications.
func TestEventToRecord_RealSessionFallsBackToSessionID(t *testing.T) {
	event := &events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		SessionID:            "real-session-uuid",
		Context:              "",
		NotificationID:       "notif-2",
		NotificationType:     1,
		NotificationPriority: 1,
		NotificationTitle:    "Test",
		NotificationMessage:  "Message",
	}

	record := eventToRecord(event)
	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if record.SessionName != "real-session-uuid" {
		t.Errorf("expected SessionName to fall back to SessionID for a real session notification, got %q", record.SessionName)
	}
}

// TestCoalescing_NonNotificationEventsIgnored verifies that events of other
// types are ignored by the subscriber.
func TestCoalescing_NonNotificationEventsIgnored(t *testing.T) {
	bus := events.NewEventBus(100)
	defer bus.Close()
	appender := &mockAppender{}

	ctx, cancel := context.WithCancel(context.Background())

	StartSubscriberWithInterval(ctx, bus, appender, 5*time.Millisecond)

	// Publish a non-notification event
	bus.Publish(&events.Event{
		Type:      events.EventSessionCreated,
		Timestamp: time.Now(),
		SessionID: "session-A",
	})

	// Cancel context; non-notification events produce no records even after flush
	cancel()
	// Wait briefly to ensure the subscriber has had a chance to process and exit
	_ = testutil.WaitForCondition(func() bool {
		// Confirm no records have appeared (appender remains empty)
		return len(appender.getRecords()) == 0
	}, testutil.FastWaitConfig())

	records := appender.getRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 records for non-notification events, got %d", len(records))
	}
}
