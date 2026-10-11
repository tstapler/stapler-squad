package deliverygate_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// T-CR-02 and T-CR-03: the crash FAILURE reaches history, push and the watcher
// for a hidden session with the gate on (failure class), and for a visible one;
// the hidden session's Stopped status-change push is still dropped.
func TestCrashNotification_ShouldDeliverOneHistoryRowAndOnePush_WhenHiddenGateOnAndStoppedPushDropped(t *testing.T) {
	t.Parallel()
	env := newMatrixEnv(t, true)
	hidden := services.NewSessionCrashEvent(hiddenTitle, hiddenTitle, "pane exited 137", time.Unix(1_700_000_000, 0))
	visible := services.NewSessionCrashEvent(visibleTitle, visibleTitle, "pane exited 1", time.Unix(1_700_000_001, 0))
	env.bus.Publish(hidden)
	env.bus.Publish(visible)
	env.bus.Publish(&events.Event{
		Type: events.EventSessionUpdated,
		// The instance is Hidden: the gate (not a check in the push builder) drops it.
		Session:       &session.Instance{ID: "id-h", UUID: "u-h", Title: hiddenTitle, Status: session.Stopped, Hidden: true},
		UpdatedFields: []string{events.FieldStatus},
	})
	env.bus.Publish(notifEvent(visibleTitle, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, sentinelID))
	env.waitSentinel(t)
	env.done()

	env.sinks.mu.Lock()
	defer env.sinks.mu.Unlock()
	failure := sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE.String()
	assert.Equal(t, 1, env.sinks.history[failure], "hidden crash leaves one FAILURE history row (Story 5.4's join reads it)")
	assert.Equal(t, 1, env.sinks.pushed["notification-"+hidden.NotificationID], "URGENT crash pushes once for a hidden session")
	assert.Equal(t, 1, env.sinks.pushed["notification-"+visible.NotificationID], "a visible crash notifies too")
	_, stoppedPushed := env.sinks.pushed["session-completed-id-h"]
	assert.False(t, stoppedPushed, "hidden Stopped never pushes")
	m := env.gate.Metrics()
	require.EqualValues(t, 1, m.Value("notification_hidden_delivered_total", "bus", "failure", "review"),
		"the crash is delivered as failure class, not suppressed")
	require.EqualValues(t, 1, m.Total("notification_delivery_suppressed_total"),
		"the only suppression is the hidden Stopped status-change push")
}

type recordCapture struct {
	mu       sync.Mutex
	records  []notifications.NotificationRecord
	sentinel chan struct{}
}

func (c *recordCapture) Append(r *notifications.NotificationRecord) error {
	c.mu.Lock()
	c.records = append(c.records, *r)
	c.mu.Unlock()
	if r.SessionID == visibleTitle {
		select {
		case c.sentinel <- struct{}{}:
		default:
		}
	}
	return nil
}

// T-CR-08: a hidden session's crash leaves exactly one unread, session-scoped
// FAILURE history record keyed by the hidden session's id and title, so the
// Background activity join (history x hidden sessions) can find it, and the
// key still resolves to the hidden session in the index.
func TestHiddenCrash_ShouldLeaveFailureHistoryRecord_WhenBackgroundJoinReads(t *testing.T) {
	t.Parallel()
	const hiddenUUID = "u-h"
	bus := events.NewEventBus(64)
	t.Cleanup(bus.Close)
	gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: true}, nil
	}))
	gate.Flags().Reload()
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: hiddenUUID, Title: hiddenTitle, Hidden: true, Kind: deliverygate.KindReview},
		{UUID: "u-v", Title: visibleTitle},
	})
	bus.SetPublishFilter(gate.PublishFilter())
	capture := &recordCapture{sentinel: make(chan struct{}, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	notifications.StartSubscriberWithInterval(ctx, bus, capture, 5*time.Millisecond)

	bus.Publish(services.NewSessionCrashEvent(hiddenUUID, hiddenTitle, "pane exited 137", time.Unix(1_700_000_000, 0)))
	// The subscriber flushes a coalescing map in random order, so one sentinel can land before
	// the crash record of the same flush. A second sentinel published after the first was seen
	// can only arrive in a later flush, by which time the first flush has fully completed.
	for _, typ := range []sessionv1.NotificationType{
		sessionv1.NotificationType_NOTIFICATION_TYPE_INFO,
		sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR,
	} {
		bus.Publish(notifEvent(visibleTitle, typ, sentinelID))
		select {
		case <-capture.sentinel:
		case <-time.After(10 * time.Second):
			t.Fatal("history sink never saw the sentinel")
		}
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	var hidden []notifications.NotificationRecord
	for _, r := range capture.records {
		if r.SessionID == hiddenUUID {
			hidden = append(hidden, r)
		}
	}
	require.Len(t, hidden, 1, "one history record for the hidden crash")
	r := hidden[0]
	assert.EqualValues(t, sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE, r.NotificationType)
	assert.Equal(t, hiddenTitle, r.SessionName)
	assert.False(t, r.IsRead)
	assert.True(t, r.SessionScoped)
	for _, key := range []string{r.SessionID, r.SessionName} {
		assert.Equal(t, deliverygate.VisibilityHidden, gate.Resolver().Resolve(key, nil).Visibility, "join key %q", key)
	}
}
