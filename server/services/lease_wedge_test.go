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
	"github.com/tstapler/stapler-squad/session"
)

func wedgeTarget(t *testing.T, uuid string) *session.Instance {
	t.Helper()
	inst := &session.Instance{Title: "review:" + uuid, UUID: uuid, Hidden: true}
	inst.SetLeaseFlag(&session.LeaseFlag{})
	return inst
}

func only(inst *session.Instance) func(*session.Instance) bool {
	return func(i *session.Instance) bool { return i == inst }
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

	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
	require.True(t, ok)
	t0 := time.Now()

	assert.Zero(t, e.svc.scanLeaseWedges(t0.Add(session.LeaseWedgeWarnAfter-time.Second), only(inst)), "not wedged yet")
	assert.Equal(t, 1, e.svc.scanLeaseWedges(t0.Add(session.LeaseWedgeWarnAfter+time.Second), only(inst)))
	assert.Zero(t, e.svc.scanLeaseWedges(t0.Add(5*time.Minute), only(inst)), "the same episode never raises a second one")

	got := notificationsOf(e.collect(t), warning)
	require.Len(t, got, 1)
	n := got[0]
	assert.Equal(t, leaseWedgeTitle, n.NotificationTitle)
	assert.Equal(t, inst.UUID, n.SessionID)
	assert.Equal(t, derivePriority(true, false), n.NotificationPriority, "medium: below the urgent push threshold")
	assert.Equal(t, pkgevents.DeliveryClassFailure, n.NotificationMetadata[pkgevents.MetadataKeyDeliveryClass])
	assert.Equal(t, "true", n.NotificationMetadata[pkgevents.MetadataKeySessionScoped])
	assert.NotEqual(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), n.NotificationPriority)
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "review"),
		"delivered for a hidden session through the gate")
	assert.EqualValues(t, 1, gate.Metrics().Total(deliverygate.CounterLeaseWedgeNotified))

	// A second episode (a new acquisition) raises a second warning.
	lease.Release()
	lease2, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)
	t1 := time.Now()
	assert.Equal(t, 1, e.svc.scanLeaseWedges(t1.Add(session.LeaseWedgeWarnAfter+time.Second), only(inst)))
	assert.Len(t, notificationsOf(e.collect(t), warning), 1)
	lease2.Release()
}

func TestLeaseWedgeWatcher_ShouldScanOnEveryTickAndJoinOnCancel(t *testing.T) {
	e := newCrashEnv(t, false)
	inst := wedgeTarget(t, "uuid-wedge-watch")
	lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterReply)
	require.True(t, ok)
	defer lease.Release()

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	var wg sync.WaitGroup
	e.svc.startLeaseWedgeWatcher(ctx, tick, &wg, func(now time.Time) { e.svc.scanLeaseWedges(now, only(inst)) })

	tick <- time.Now().Add(session.LeaseWedgeWarnAfter + time.Second)
	cancel()
	wg.Wait() // joined: the goroutine exits on cancel
	got := notificationsOf(e.collect(t), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING))
	assert.Len(t, got, 1)
}
