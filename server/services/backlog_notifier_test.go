package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// TestEventBusNotifier_NotifySession_should_PublishEventWithoutItemIdMetadata_When_Called
// covers validation.md's noted coverage gap (Gap #2): no existing test exercised
// EventBusNotifier.NotifySession directly — Tasks 1.2.3d/e only proved that
// session.resolveFinding *calls* the right Notifier method (via fakeNotifier), not that
// the real EventBusNotifier implementation behaves correctly end-to-end. This is
// Architecture-A1's regression guard applied to the concrete implementation: a session
// with no linked BacklogItem must never get "item_id" written into the published event's
// metadata.
func TestEventBusNotifier_NotifySession_should_PublishEventWithoutItemIdMetadata_When_Called(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := &EventBusNotifier{Bus: bus}
	notifier.NotifySession("sess-b8ccca59", "Worktree row auto-repaired", "auto-repaired: derived from live git worktree", 10, false, true)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, "sess-b8ccca59", ev.SessionID)
		assert.Equal(t, "Worktree row auto-repaired", ev.NotificationTitle)
		assert.NotContains(t, ev.NotificationMetadata, "item_id",
			"NotifySession must never write a session UUID into metadata[\"item_id\"] — that key means \"this notification is about backlog item <value>\"")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event after NotifySession")
	}
}

// TestEventBusNotifier_Notify_should_PublishEventWithItemIdMetadata_When_Called is the
// contrasting happy path, confirming Notify's existing behavior is unchanged by this
// addition — a real BacklogItem ID still lands in metadata["item_id"].
func TestEventBusNotifier_Notify_should_PublishEventWithItemIdMetadata_When_Called(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := &EventBusNotifier{Bus: bus}
	notifier.Notify("item-123", "title", "message", 8, true, true)

	select {
	case ev := <-ch:
		assert.Equal(t, "item-123", ev.NotificationMetadata["item_id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event after Notify")
	}
}

// TestEventBusNotifier_NotifySession_should_NoOp_When_BusNil confirms the nil-safety
// guard shared with Notify.
func TestEventBusNotifier_NotifySession_should_NoOp_When_BusNil(t *testing.T) {
	t.Parallel()
	var notifier *EventBusNotifier
	require.NotPanics(t, func() {
		notifier.NotifySession("sess-1", "title", "message", 10, false, false)
	})

	notifier = &EventBusNotifier{}
	require.NotPanics(t, func() {
		notifier.NotifySession("sess-1", "title", "message", 10, false, false)
	})
}

// The auto-remediating WARNING carries the stamp that makes it informational.
func TestEventBusNotifier_NotifyAutoRemediating_should_StampAutoRemediatingAndKeepItemId_When_Called(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	(&EventBusNotifier{Bus: bus}).NotifyAutoRemediating("item-1", "PR needs attention", "retrying", 8, false, true)

	select {
	case ev := <-ch:
		assert.Equal(t, "true", ev.NotificationMetadata["auto_remediating"])
		assert.Equal(t, "item-1", ev.NotificationMetadata["item_id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event after NotifyAutoRemediating")
	}
}

// A decision notification carries the stable decision_schema and the needs-human type.
func TestEventBusNotifier_NotifyDecision_should_StampDecisionSchemaWithChoices_When_ItemAwaitsDecision(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	(&EventBusNotifier{Bus: bus}).NotifyDecision("b608ab1e", "Review passed", "accept, rework or merge", DefaultDecisionSchema())

	select {
	case ev := <-ch:
		assert.Equal(t, "b608ab1e", ev.NotificationMetadata["item_id"])
		var schema DecisionSchema
		require.NoError(t, json.Unmarshal([]byte(ev.NotificationMetadata["decision_schema"]), &schema))
		var values []string
		for _, c := range schema.Choices {
			values = append(values, c.Value)
		}
		assert.Equal(t, []string{"accept", "rework", "merge"}, values)
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_CONFIRMATION_NEEDED), ev.NotificationType)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event after NotifyDecision")
	}
}
