package push

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil"
)

// UT-2.x — shouldNotify table [R4, R5]. Tightened from the old ">= HIGH" threshold to
// "== URGENT only" (2026-09, notification push-gate redesign): HIGH now means
// "important but not urgent" and must not interrupt the user. Also age-gates URGENT
// against urgentTTL — see shouldNotify's doc comment for why (urgency decays, importance
// doesn't).
func TestShouldNotifyTable(t *testing.T) {
	tests := []struct {
		name             string
		eventType        events.EventType
		priority         int32
		notificationType int32
		age              time.Duration
		wantNotify       bool
	}{
		// EventNotification cases
		{"low priority generic → no push", events.EventNotification, priorityLow, typeUnspecified, 0, false},
		{"medium priority generic → no push", events.EventNotification, priorityMedium, typeUnspecified, 0, false},
		{"high priority generic → no push", events.EventNotification, priorityHigh, typeUnspecified, 0, false},                                          // UT-2.1a: HIGH = important-only, must not push
		{"urgent priority generic, fresh → push", events.EventNotification, priorityUrgent, typeUnspecified, 5 * time.Minute, true},                     // UT-2.1b [BUG-2 fix]
		{"urgent priority generic, aged past TTL → no push", events.EventNotification, priorityUrgent, typeUnspecified, urgentTTL + time.Minute, false}, // urgency decay
		{"urgent priority generic, exactly at TTL → no push", events.EventNotification, priorityUrgent, typeUnspecified, urgentTTL, false},              // boundary is exclusive
		{"low priority APPROVAL → push", events.EventNotification, priorityLow, typeApproval, 0, true},                                                  // UT-2.2 [R5]
		{"high priority APPROVAL → push", events.EventNotification, priorityHigh, typeApproval, 0, true},                                                // UT-2.3
		{"urgent priority APPROVAL, aged past TTL → still push", events.EventNotification, priorityUrgent, typeApproval, urgentTTL + time.Minute, true}, // approval override ignores age
		// Other event types — EventSessionUpdated is covered separately by
		// TestSessionUpdatedPushGateTable, since shouldNotify never actually
		// sees it in production (buildDeliveryNotification routes it to
		// buildStatusChangeNotification instead).
		{"unrelated event → no push", events.EventSessionCreated, 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldNotify(tt.eventType, tt.priority, tt.notificationType, tt.age)
			assert.Equal(t, tt.wantNotify, got, "shouldNotify mismatch for: %s", tt.name)
		})
	}
}

// UT-2.4 — URL uses session.ID not session.Title [R6]
func TestNotificationURLUsesSessionID(t *testing.T) {
	notification := buildNotificationForSession(&session.Instance{
		ID:    "session-abc-123",
		Title: "My Session (renamed)",
	}, events.EventSessionUpdated)

	url, ok := notification.Data["url"].(string)
	assert.True(t, ok, "url must be a string in Data")
	assert.Contains(t, url, "session-abc-123", "URL must contain session ID")
	assert.NotContains(t, url, "My Session (renamed)", "URL must not contain mutable title")
}

// UT-2.5 — Tag uses session.ID not session.Title [R6]
func TestNotificationTagUsesSessionID(t *testing.T) {
	notification := buildNotificationForSession(&session.Instance{
		ID:    "session-abc-123",
		Title: "Renamed Title",
	}, events.EventSessionUpdated)

	assert.Contains(t, notification.Tag, "session-abc-123")
	assert.NotContains(t, notification.Tag, "Renamed Title")
}

// UT-5.1 — Payload data map includes notificationType and timestamp [R12]
func TestPayloadDataMapFields(t *testing.T) {
	notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})

	assert.Equal(t, "APPROVAL_NEEDED", notif.Data["notificationType"])
	ts, ok := notif.Data["timestamp"].(int64)
	assert.True(t, ok, "timestamp must be int64")
	assert.Greater(t, ts, int64(0))
}

// UT-5.2 — Payload data map includes sessionId using session.ID [R12, R6]
func TestPayloadDataSessionID(t *testing.T) {
	notif := buildApprovalNotification(&session.Instance{ID: "abc-123", Title: "Changed Title"})
	assert.Equal(t, "abc-123", notif.Data["sessionId"])
}

// "actions" (the review/later buttons) belongs only on approval notifications,
// not on plain completion ones.
func TestActionsOnlyOnApprovalDataMap(t *testing.T) {
	inst := &session.Instance{ID: "s1", Title: "S1"}
	_, hasActions := buildDataMap(inst, "SESSION_COMPLETE")["actions"]
	assert.False(t, hasActions, "buildDataMap must not include actions")

	_, hasActions = buildApprovalDataMap(inst)["actions"]
	assert.True(t, hasActions, "buildApprovalDataMap must include actions")
}

// UT-5.3 — RequireInteraction=true on approval_needed [R13]
func TestRequireInteractionApproval(t *testing.T) {
	notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})
	assert.True(t, notif.RequireInteraction)
}

func TestRequireInteractionSessionComplete(t *testing.T) {
	notif := buildCompletedNotification(&session.Instance{ID: "s1", Title: "S1"})
	assert.False(t, notif.RequireInteraction)
}

// UT-5.4 — Renotify=true on approval_needed, false on session-complete [R14]
func TestRenotifyApproval(t *testing.T) {
	notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})
	assert.True(t, notif.Renotify)
}

func TestRenotifyComplete(t *testing.T) {
	notif := buildCompletedNotification(&session.Instance{ID: "s1", Title: "S1"})
	assert.False(t, notif.Renotify)
}

// BV-3 — Empty notifier slice does not panic
func TestStartDeliverySubscriberEmptyNotifiers(t *testing.T) {
	bus := events.NewEventBus(10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	assert.NotPanics(t, func() {
		StartDeliverySubscriber(ctx, bus, []Notifier{})
		bus.Publish(&events.Event{
			Type:                 events.EventNotification,
			NotificationPriority: priorityHigh,
		})
		<-ctx.Done()
	})
}

// BV-4 — Deduplication window: same tag within 2s suppressed
func TestDeduplicationWindow(t *testing.T) {
	bus := events.NewEventBus(10)
	n := &mockNotifier{name: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	StartDeliverySubscriber(ctx, bus, []Notifier{n})

	event := &events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		NotificationPriority: priorityUrgent,
		NotificationTitle:    "Test",
		NotificationMessage:  "Body",
		NotificationID:       "same-id",
	}
	bus.Publish(event)
	bus.Publish(event) // same tag, within 2s
	// Wait for the first delivery, then confirm no second delivery follows
	require.NoError(t, testutil.WaitForCondition(func() bool {
		return n.CallCount() >= 1
	}, testutil.FastWaitConfig()))
	assert.Equal(t, 1, n.CallCount(), "duplicate within 2s window must be suppressed")
}

// UT-3.1 — StartDeliverySubscriber calls all Notifiers in slice [R7]
func TestStartDeliverySubscriberCallsAllNotifiers(t *testing.T) {
	bus := events.NewEventBus(10)
	n1, n2 := &mockNotifier{name: "n1"}, &mockNotifier{name: "n2"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartDeliverySubscriber(ctx, bus, []Notifier{n1, n2})

	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		NotificationPriority: priorityUrgent,
		NotificationTitle:    "Test",
		NotificationMessage:  "Body",
	})

	require.NoError(t, testutil.WaitForCondition(func() bool {
		return n1.CallCount() >= 1 && n2.CallCount() >= 1
	}, testutil.FastWaitConfig()))
	assert.Equal(t, 1, n1.CallCount(), "n1 must receive notification")
	assert.Equal(t, 1, n2.CallCount(), "n2 must receive notification")
}

// UT-3.2 — One failing Notifier does not prevent delivery to others [R7]
func TestDeliverySubscriberContinuesOnNotifierError(t *testing.T) {
	bus := events.NewEventBus(10)
	failing := &errorNotifier{name: "failing"}
	success := &mockNotifier{name: "success"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartDeliverySubscriber(ctx, bus, []Notifier{failing, success})

	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		NotificationPriority: priorityUrgent,
		NotificationTitle:    "Test",
		NotificationMessage:  "Body",
	})

	require.NoError(t, testutil.WaitForCondition(func() bool {
		return success.CallCount() >= 1
	}, testutil.FastWaitConfig()))
	assert.Equal(t, 1, success.CallCount(), "success notifier must still receive despite first notifier error")
}

// IT-1.2 — Subscriber goroutine exits when context is cancelled [R20]
func TestSubscriberExitsOnContextCancel(t *testing.T) {
	bus := events.NewEventBus(10)
	n := &mockNotifier{name: "test"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := StartDeliverySubscriber(ctx, bus, []Notifier{n})

	// Cancel the context and wait for the done channel to close — this is an exact
	// signal from the goroutine itself, not a flaky goroutine-count heuristic.
	cancel()
	select {
	case <-done:
		// goroutine exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber goroutine should exit after context cancel")
	}
}

// IT-2.1 — APPROVAL_NEEDED at LOW priority triggers delivery [R5]
func TestApprovalAtLowPriorityTriggersPush(t *testing.T) {
	bus := events.NewEventBus(10)
	n := &mockNotifier{name: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	StartDeliverySubscriber(ctx, bus, []Notifier{n})

	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		NotificationPriority: priorityLow,
		NotificationType:     typeApproval,
		NotificationTitle:    "Approval needed",
		NotificationMessage:  "Please review",
	})

	require.NoError(t, testutil.WaitForCondition(func() bool {
		return n.CallCount() >= 1
	}, testutil.FastWaitConfig()))
	assert.Equal(t, 1, n.CallCount(), "APPROVAL_NEEDED must trigger push at LOW priority")
}

// IT-2.2 — URGENT priority notification triggers delivery [R4]
func TestUrgentPriorityTriggersPush(t *testing.T) {
	bus := events.NewEventBus(10)
	n := &mockNotifier{name: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	StartDeliverySubscriber(ctx, bus, []Notifier{n})

	bus.Publish(&events.Event{
		Type:                 events.EventNotification,
		Timestamp:            time.Now(),
		NotificationPriority: priorityUrgent,
		NotificationType:     typeUnspecified,
		NotificationTitle:    "Urgent!",
		NotificationMessage:  "Critical issue",
	})

	require.NoError(t, testutil.WaitForCondition(func() bool {
		return n.CallCount() >= 1
	}, testutil.FastWaitConfig()))
	assert.Equal(t, 1, n.CallCount(), "URGENT priority must trigger push")
}

// IT-4.x — EventSessionUpdated push gating table: a push fires only when
// UpdatedFields names "status" AND the session's current status is Stopped.
// Regression coverage for the notification-flood bug (an unrelated update on
// an already-Stopped session used to re-fire the push) and for the
// resume-path shape where "status" is present but the transition is to
// Active, not Stopped (session_service.go's resume path appends "status" to
// UpdatedFields on every Active-transition resume).
func TestSessionUpdatedPushGateTable(t *testing.T) {
	tests := []struct {
		name          string
		status        session.Status
		updatedFields []string
		wantCalls     int
	}{
		{"Stopped + status field → push", session.Stopped, []string{"status"}, 1},
		{"Stopped + unrelated field → suppressed", session.Stopped, []string{"goal"}, 0},
		{"Active + status field → suppressed", session.Active, []string{"status"}, 0},
		{"Active + no UpdatedFields → suppressed", session.Active, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPushGateCase(t, tt.status, tt.updatedFields, tt.wantCalls)
		})
	}
}

// assertPushGateCase publishes a single EventSessionUpdated event and asserts
// the delivered push count matches wantCalls, waiting for delivery when a
// push is expected and for the context deadline otherwise.
func assertPushGateCase(t *testing.T, status session.Status, updatedFields []string, wantCalls int) {
	t.Helper()
	bus := events.NewEventBus(10)
	n := &mockNotifier{name: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	StartDeliverySubscriber(ctx, bus, []Notifier{n})

	inst := &session.Instance{ID: "sess-1", Title: "Session", Status: status}
	bus.Publish(&events.Event{
		Type:          events.EventSessionUpdated,
		Session:       inst,
		UpdatedFields: updatedFields,
	})

	if wantCalls > 0 {
		require.NoError(t, testutil.WaitForCondition(func() bool {
			return n.CallCount() >= wantCalls
		}, testutil.FastWaitConfig()))
	} else {
		<-ctx.Done()
	}
	assert.Equal(t, wantCalls, n.CallCount(), "unexpected push count")
}

// ─── test helpers ─────────────────────────────────────────────────────────────

type mockNotifier struct {
	name  string
	calls []DeliveryNotification
	mu    sync.Mutex
}

func (m *mockNotifier) Send(_ context.Context, n DeliveryNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, n)
	return nil
}
func (m *mockNotifier) Name() string { return m.name }
func (m *mockNotifier) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type errorNotifier struct{ name string }

func (e *errorNotifier) Send(_ context.Context, _ DeliveryNotification) error {
	return fmt.Errorf("notifier %q always fails", e.name)
}
func (e *errorNotifier) Name() string { return e.name }
