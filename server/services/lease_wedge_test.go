package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/session/instance"
)

func wedgeTarget(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{Title: "review:" + uuid, UUID: uuid, Hidden: true}
	inst.SetLeaseFlag(&instance.LeaseFlag{})
	return inst
}

func only(inst *instance.Instance) func(*instance.Instance) bool {
	return func(i *instance.Instance) bool { return i == inst }
}

// T-WL-08: exactly one tray warning per wedge episode, never pushed, and
// delivered for a hidden session through the gate (delivery_class=failure).
func TestWedgedLease_ShouldRaiseExactlyOneTrayWarningPerEpisodeNotPushedAndDeliveredForAHiddenSession_WhenAWriteOutlivesLeaseWedgeWarnAfter(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	inst := wedgeTarget(t, "uuid-wedge-1")
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: inst.UUID, Title: inst.Title, TmuxName: "ssq_review_wedge", Hidden: true, Kind: deliverygate.KindReview},
	})
	warning := int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING)

	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	require.True(t, ok)
	t0 := time.Now()
	scan := func(now time.Time) int {
		return autonomysvc.ScanLeaseWedges(now, only(inst), e.svc.leaseWedgePorts())
	}

	assert.Zero(t, scan(t0.Add(instance.LeaseWedgeWarnAfter-time.Second)), "not wedged yet")
	assert.Equal(t, 1, scan(t0.Add(instance.LeaseWedgeWarnAfter+time.Second)))
	assert.Zero(t, scan(t0.Add(5*time.Minute)), "the same episode never raises a second one")

	got := notificationsOf(e.collect(t), warning)
	require.Len(t, got, 1)
	n := got[0]
	assert.Equal(t, "A write to this session is stuck", n.NotificationTitle)
	assert.Equal(t, inst.UUID, n.SessionID)
	assert.Equal(t, notify.DerivePriority(true, false), n.NotificationPriority, "medium: below the urgent push threshold")
	assert.Equal(t, pkgevents.DeliveryClassFailure, n.NotificationMetadata[pkgevents.MetadataKeyDeliveryClass])
	assert.Equal(t, "true", n.NotificationMetadata[pkgevents.MetadataKeySessionScoped])
	assert.NotEqual(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), n.NotificationPriority)
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "review"),
		"delivered for a hidden session through the gate")
	assert.EqualValues(t, 1, gate.Metrics().Total(deliverygate.CounterLeaseWedgeNotified))

	// A second episode (a new acquisition) raises a second warning.
	lease.Release()
	lease2, ok := inst.TryTerminalWriteLease(instance.LeaseWriterDriver)
	require.True(t, ok)
	t1 := time.Now()
	assert.Equal(t, 1, scan(t1.Add(instance.LeaseWedgeWarnAfter+time.Second)))
	assert.Len(t, notificationsOf(e.collect(t), warning), 1)
	lease2.Release()
}

// The hub's watcher scans through the hub's own ports on each tick. A sweep sees
// every lease in the process, so only this instance's events are asserted.
func TestLeaseWedgeWatcher_ShouldRaiseThroughTheHubPorts_When_ATickArrives(t *testing.T) {
	e := newCrashEnv(t, false)
	inst := wedgeTarget(t, "uuid-wedge-watch")
	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	require.True(t, ok)
	defer lease.Release()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	tick := make(chan time.Time)
	e.svc.StartLeaseWedgeWatcher(ctx, tick, &wg)

	select {
	case tick <- time.Now().Add(instance.LeaseWedgeWarnAfter + time.Second):
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher never took the tick")
	}
	cancel()
	wg.Wait() // joined: the goroutine exits on cancel
	var got int
	for _, n := range notificationsOf(e.collect(t), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING)) {
		if n.SessionID == inst.UUID {
			got++
		}
	}
	assert.Equal(t, 1, got)
}
