package services

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// warnCollector is an injected slog handler that records messages (never the
// global default; BUG-087).
type warnCollector struct {
	mu   *sync.Mutex
	msgs *[]string
}

func newWarnCollector() (*slog.Logger, func() []string) {
	h := warnCollector{mu: &sync.Mutex{}, msgs: &[]string{}}
	return slog.New(h), func() []string {
		h.mu.Lock()
		defer h.mu.Unlock()
		return append([]string(nil), (*h.msgs)...)
	}
}
func (warnCollector) Enabled(context.Context, slog.Level) bool { return true }
func (h warnCollector) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	*h.msgs = append(*h.msgs, r.Message)
	h.mu.Unlock()
	return nil
}
func (h warnCollector) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h warnCollector) WithGroup(string) slog.Handler      { return h }

type gatedNotifFixture struct {
	gate   *deliverygate.Gate
	events <-chan *events.Event
	client sessionv1connect.SessionServiceClient
	logs   func() []string
}

// newGatedNotifFixture wires a real bus + gate (filter installed) + a poller
// holding one hidden and one visible session.
func newGatedNotifFixture(t *testing.T, flagOn bool) *gatedNotifFixture {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(64)
	t.Cleanup(bus.Close)
	lg, logs := newWarnCollector()
	gate := deliverygate.NewGate(
		deliverygate.WithLogger(lg),
		deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
			return deliverygate.FlagSettings{Global: flagOn}, nil
		}),
	)
	gate.Flags().Reload()
	bus.SetPublishFilter(gate.PublishFilter())

	svc := NewSessionService(storage, bus)
	svc.notificationSvc.SetDeliveryGate(gate)
	t.Cleanup(func() { svc.Shutdown() })

	hidden := &session.Instance{Title: "review-h", UUID: "uuid-hidden", Hidden: true}
	visible := &session.Instance{Title: "work-v", UUID: "uuid-visible"}
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*session.Instance{hidden, visible})
	svc.SetReviewQueuePoller(poller)
	gate.SeedFromInstances([]*session.Instance{hidden, visible})

	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	return &gatedNotifFixture{gate: gate, events: ch, client: newTestClient(srv), logs: logs}
}

func (f *gatedNotifFixture) send(t *testing.T, sessionID string, typ sessionv1.NotificationType, prio sessionv1.NotificationPriority, md map[string]string) {
	t.Helper()
	_, err := f.client.SendNotification(context.Background(), connect.NewRequest(&sessionv1.SendNotificationRequest{
		SessionId: sessionID, Title: "t", Message: "m", NotificationType: typ, Priority: prio, Metadata: md,
	}))
	require.NoError(t, err)
}

// next returns the next notification event or nil if none arrives shortly.
// Publish is synchronous, so absence is known as soon as send returns; the short
// wait only covers channel hand-off.
func (f *gatedNotifFixture) next() *events.Event {
	select {
	case e := <-f.events:
		return e
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}

const (
	typeProcessFinished = sessionv1.NotificationType_NOTIFICATION_TYPE_PROCESS_FINISHED // an old script's task_failed (6)
	prioMedium          = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	prioLow             = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
)

// T-SN-04: an unversioned request is counted and WARNs at most once per hour.
func TestSendNotification_ShouldCountUnversionedAndWarnOncePerHour_WhenSchemaMissing(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, false)
	for i := 0; i < 3; i++ {
		f.send(t, "work-v", sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, prioMedium, nil)
	}
	f.send(t, "work-v", sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, prioMedium,
		map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion})

	assert.EqualValues(t, 3, f.gate.Metrics().Value(deliverygate.CounterRPCUnversioned))
	warns := 0
	for _, m := range f.logs() {
		if m == "legacy_ssq_notify_detected" {
			warns++
		}
	}
	assert.Equal(t, 1, warns, "WARN must be rate limited to once per hour")
}

// T-SN-05: with the gate on, an unversioned request for a hidden session fails
// open (delivered, skew visible in the counter); a versioned one of the same
// number is suppressed as routine.
func TestSendNotification_ShouldDeliverHiddenUnversionedType6ButSuppressVersioned_WhenGateOn(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, true)

	f.send(t, "review-h", typeProcessFinished, prioMedium, nil)
	got := f.next()
	require.NotNil(t, got, "unversioned hidden event must be delivered (fail open)")
	assert.Equal(t, "true", got.NotificationMetadata[events.MetadataKeyUntrustedType])
	assert.EqualValues(t, 1, f.gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "routine", "other"))

	f.send(t, "review-h", typeProcessFinished, prioMedium,
		map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion})
	assert.Nil(t, f.next(), "versioned routine event for a hidden session must be suppressed")

	f.send(t, "work-v", typeProcessFinished, prioMedium, nil)
	assert.NotNil(t, f.next(), "visible session is unchanged")
}

// A client cannot forge the untrusted stamp to push a versioned routine event
// through, nor strip it from an unversioned one.
func TestSendNotification_ShouldStripClientSuppliedUntrustedStamp_WhenRequestVersioned(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, true)
	f.send(t, "review-h", typeProcessFinished, prioMedium, map[string]string{
		events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion,
		events.MetadataKeyUntrustedType:   "true",
	})
	assert.Nil(t, f.next(), "a forged untrusted stamp must not bypass the gate")
}

// T-SN-07: with the flag off the event for a hidden and a visible session
// resolves name/UUID exactly as before and only gains the server-side stamp.
func TestSendNotification_ShouldBeByteIdenticalToMain_WhenFlagOffAndIndexUnseeded(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, false)
	f.gate.Index().Replace(nil) // empty index: the legacy poller path alone resolves

	md := map[string]string{"k": "v", events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion}
	for _, c := range []struct{ id, wantUUID, wantName string }{
		{"review-h", "uuid-hidden", "review-h"},
		{"work-v", "uuid-visible", "work-v"},
	} {
		f.send(t, c.id, sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE, prioMedium, md)
		e := f.next()
		require.NotNil(t, e, c.id)
		assert.Equal(t, c.wantUUID, e.SessionID)
		assert.Equal(t, c.wantName, e.Context)
		assert.Equal(t, md, e.NotificationMetadata, "versioned request metadata must pass through unchanged")
	}
}

// T-LG-05: the legacy hidden+LOW swallow still happens and is counted.
func TestSendNotification_ShouldSwallowHiddenAndLowAsOnMain_WhenFlagOff(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, false)
	f.send(t, "review-h", sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE, prioLow,
		map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion})
	assert.Nil(t, f.next(), "legacy check must still swallow hidden LOW")
	assert.EqualValues(t, 1, f.gate.Metrics().Value(deliverygate.CounterLegacySuppressed,
		"send_notification_low", "NOTIFICATION_TYPE_TASK_COMPLETE", "routine"))

	f.send(t, "work-v", sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE, prioLow,
		map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion})
	assert.NotNil(t, f.next(), "visible LOW is delivered")
}

// T-OB-29 (ADV-N30): the operator's synthetic probe traverses the real gate
// end to end with the legacy hidden checks present. A `-p low` probe never gets
// that far (the legacy hidden+LOW swallow drops it first, so it would prove
// nothing about the gate); a `-p medium` ERROR probe reaches the bus with its
// label, is counted as a probe delivery, and carries no metadata that could
// steer a channel. That medium priority does not push is T-OB-29's push half,
// in server/push (TestShouldNotify_ShouldBeFalse_WhenMediumPriorityErrorProbe).
func TestSyntheticProbe_ShouldTraverseTheRealGateNotBePushedAndKeepItsLabel_WhenSsqNotifySendsAMediumPriorityErrorForAHiddenSession(t *testing.T) {
	t.Parallel()
	f := newGatedNotifFixture(t, true)
	title := deliverygate.ProbeTitlePrefix + " gate check"
	send := func(prio sessionv1.NotificationPriority) {
		_, err := f.client.SendNotification(context.Background(), connect.NewRequest(&sessionv1.SendNotificationRequest{
			SessionId: "review-h", Title: title, Message: "probe",
			NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR, Priority: prio,
			Metadata: map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion},
		}))
		require.NoError(t, err)
	}

	send(prioLow)
	assert.Nil(t, f.next(), "a low-priority probe is dropped by the legacy check before the gate")
	assert.EqualValues(t, 0, f.gate.Metrics().Total(deliverygate.CounterHiddenDelivered), "a low probe never reaches the gate")

	send(prioMedium)
	got := f.next()
	require.NotNil(t, got, "a medium-priority probe must traverse the gate and be delivered")
	assert.Equal(t, title, got.NotificationTitle, "the probe keeps its label")
	assert.EqualValues(t, 2, got.NotificationPriority)
	assert.EqualValues(t, 1, f.gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "other"))
	assert.EqualValues(t, 1, f.gate.StatsSnapshot().Soak.ProbeDeliveredWhileOn)
	for k := range got.NotificationMetadata {
		assert.NotContains(t, []string{events.MetadataKeyDeliveryClass, "channel", "push"}, k,
			"the probe must not carry a metadata hint that steers a channel")
	}
}
