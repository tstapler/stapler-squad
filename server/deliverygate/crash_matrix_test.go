package deliverygate_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
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
		Type:          events.EventSessionUpdated,
		Session:       &session.Instance{ID: "id-h", UUID: "u-h", Title: hiddenTitle, Hidden: true, Status: session.Stopped},
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
