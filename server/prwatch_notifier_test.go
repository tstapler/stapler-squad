package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/prwatch"
)

func prwatchNotifyOnce(t *testing.T, n prwatch.Notification) *events.Event {
	t.Helper()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	require.NoError(t, prwatchNotifier{bus: bus}.Notify(n))
	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no notification published")
		return nil
	}
}

func TestPRWatchNotifier_should_PublishOnBusWithDedupeKeyAsID_When_Notified(t *testing.T) {
	t.Parallel()

	info := prwatchNotifyOnce(t, prwatch.Notification{Kind: prwatch.NotifyCoverageGap, SessionUUID: "s1", DedupeKey: "prwatch-coverage:o/r", Title: "t", Message: "m"})
	assert.Equal(t, "prwatch-coverage:o/r", info.NotificationID)
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), info.NotificationType)
	assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM), info.NotificationPriority)
	assert.Empty(t, info.SessionID, "informational kinds are not bound to a session")
	assert.Equal(t, "t", info.NotificationTitle)
	assert.Equal(t, "m", info.NotificationMessage)

	for _, k := range []prwatch.NotificationKind{prwatch.NotifySteerFailed, prwatch.NotifyHeldDropped} {
		fail := prwatchNotifyOnce(t, prwatch.Notification{Kind: k, SessionUUID: "hidden-1", DedupeKey: "k-" + string(k), Title: "t", Message: "m"})
		assert.Equal(t, "k-"+string(k), fail.NotificationID)
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), fail.NotificationType)
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH), fail.NotificationPriority)
		assert.Equal(t, "hidden-1", fail.SessionID)
		hint, ok := deliverygate.ParseClassHint(fail.NotificationMetadata)
		require.True(t, ok)
		d := deliverygate.ShouldDeliver(deliverygate.VisibilityHidden, deliverygate.Facts{Type: sessionv1.NotificationType(fail.NotificationType), Hint: hint})
		assert.Equal(t, deliverygate.OutcomeDeliver, d.Outcome, "hidden-session owner still reaches the operator")
	}

	state := prwatchNotifyOnce(t, prwatch.Notification{Kind: prwatch.NotifyInboxState, State: prwatch.InboxStateUnreachable, DedupeKey: "prwatch-inbox-state:unreachable", Title: "t", Message: "m"})
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), state.NotificationType)
	assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM), state.NotificationPriority)
	assert.Empty(t, state.SessionID)
	recovered := prwatchNotifyOnce(t, prwatch.Notification{Kind: prwatch.NotifyInboxState, State: prwatch.InboxStateOK, DedupeKey: "prwatch-inbox-state:unreachable", Title: "t", Message: "m"})
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), recovered.NotificationType)
}

func TestPRWatchNotifier_should_RejectMissingBusOrKey_When_Misconfigured(t *testing.T) {
	t.Parallel()
	assert.Error(t, prwatchNotifier{}.Notify(prwatch.Notification{DedupeKey: "k"}))
	assert.Error(t, prwatchNotifier{bus: events.NewEventBus(1)}.Notify(prwatch.Notification{}))
}

func TestPRWatchGuidePath_should_BeAbsoluteOrFixedFallback_When_Resolved(t *testing.T) {
	t.Parallel()
	got := prwatchGuidePath()
	if got != prwatch.GuideFallback {
		assert.True(t, strings.HasPrefix(got, "/"), got)
		assert.True(t, strings.HasSuffix(got, prwatch.GuideRelPath))
	}
}
