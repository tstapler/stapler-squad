package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// T-CR-05: PermanentlyFailed already has a bus producer (EventBusNotifier.Notify
// with type ERROR and item_id = the session UUID). With the gate on it is
// delivered exactly once, and the resolver still classifies the session as
// hidden by UUID although the item_id stamp is present.
func TestPermanentlyFailed_ShouldDeliverOneErrorAndResolveHidden_WhenItemIDEqualsUUID(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: "uuid-hidden-1", Title: "review:abc", TmuxName: "ssq_review_abc", Hidden: true, Kind: deliverygate.KindReview},
	})

	n := &EventBusNotifier{Bus: e.bus}
	n.Notify("uuid-hidden-1", "Session gave up after repeated failures", "failed 3 attempts",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), true, true)
	// A routine event from the same hidden session is dropped, proving the
	// session was resolved as hidden (not delivered through fail-open).
	n.Notify("uuid-hidden-1", "done", "finished",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false, false)

	got := e.collect(t)
	errs := notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR))
	require.Len(t, errs, 1)
	assert.Empty(t, notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE)))
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "review"))
	assert.EqualValues(t, 0, gate.Metrics().Total(deliverygate.CounterUnresolved))
}

// A hidden crash is delivered through the gate (failure class); a hidden
// Stopped exit publishes no notification at all.
func TestHiddenCrash_ShouldDeliverFailureThroughTheGate_WhenGateOn(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: "uuid-hidden-2", Title: "diag:xyz", TmuxName: "ssq_diag_xyz", Hidden: true, Kind: deliverygate.KindDiagnose},
	})

	e.svc.publishCrash(crashedSnap("uuid-hidden-2", "diag:xyz", "pane exited 137", testNow))
	got := notificationsOf(e.collect(t), failureType)
	require.Len(t, got, 1)
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "diagnose"))
}
