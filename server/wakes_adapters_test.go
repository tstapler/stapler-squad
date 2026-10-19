package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/wakes"
)

func notifyOnce(t *testing.T, n wakes.Notification) *events.Event {
	t.Helper()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	wakeNotifier{bus: bus}.Notify(n)
	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no notification published")
		return nil
	}
}

// Gate-class rows for the wake notifications (Task 4.3e): a failure for a
// hidden-session owner is delivered; a reminder is delivered only because it
// carries no session binding.
func TestWakeNotifier_should_ReachOperator_When_OwnerSessionIsHidden(t *testing.T) {
	t.Parallel()

	failure := notifyOnce(t, wakes.Notification{Kind: wakes.NotifyFailure, OwnerSessionUUID: "hidden-1", Title: "Scheduled prompt missed", Message: "m"})
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), failure.NotificationType)
	assert.Equal(t, "hidden-1", failure.SessionID)
	hint, ok := deliverygate.ParseClassHint(failure.NotificationMetadata)
	require.True(t, ok)
	d := deliverygate.ShouldDeliver(deliverygate.VisibilityHidden, deliverygate.Facts{Type: sessionv1.NotificationType(failure.NotificationType), Hint: hint})
	assert.Equal(t, deliverygate.OutcomeDeliver, d.Outcome)
	assert.Equal(t, deliverygate.ClassFailure, d.Class)

	reminder := notifyOnce(t, wakes.Notification{Kind: wakes.NotifyReminder, OwnerSessionUUID: "hidden-1", Title: "Reminder", Message: "m"})
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), reminder.NotificationType)
	assert.Empty(t, reminder.SessionID, "no session binding")
	assert.Equal(t, "hidden-1", reminder.NotificationMetadata["target_session_uuid"])
	hint, ok = deliverygate.ParseClassHint(reminder.NotificationMetadata)
	require.True(t, ok)
	facts := deliverygate.Facts{Type: sessionv1.NotificationType(reminder.NotificationType), Hint: hint}
	assert.Equal(t, deliverygate.OutcomeDeliver, deliverygate.ShouldDeliver(deliverygate.VisibilityNotASession, facts).Outcome)
	assert.Equal(t, deliverygate.OutcomeSuppress, deliverygate.ShouldDeliver(deliverygate.VisibilityHidden, facts).Outcome,
		"the same INFO bound to a hidden session would be suppressed, which is why a reminder is unbound")
}
