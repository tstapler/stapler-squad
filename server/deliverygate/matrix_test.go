package deliverygate_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/push"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

const (
	hiddenTitle  = "review:abc"
	visibleTitle = "my-work"
	urgentPrio   = int32(4)
)

// matrixSinks are the three real bus consumers: history (fake appender), push
// (recording Notifier) and a WatchSessions-style direct subscriber. Each closes
// its sentinel channel when the visible-session sentinel event reaches it, so
// completion is signalled by data, not by sleeping.
type matrixSinks struct {
	mu       sync.Mutex
	history  map[string]int // notification type name -> hidden-session records
	bySess   map[string]int // session id + "|" + type name -> records, every session
	pushed   map[string]int // notification id -> deliveries
	watched  map[string]int // notification type name -> events
	sentinel [3]chan struct{}
	once     [3]sync.Once
}

const sentinelID = "sentinel"

type appender struct{ m *matrixSinks }

func (a appender) Append(r *notifications.NotificationRecord) error {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	a.m.bySess[r.SessionID+"|"+sessionv1.NotificationType(r.NotificationType).String()]++
	if r.SessionID == hiddenTitle {
		a.m.history[sessionv1.NotificationType(r.NotificationType).String()]++
	}
	if r.SessionID == visibleTitle {
		a.m.once[0].Do(func() { close(a.m.sentinel[0]) })
	}
	return nil
}

type pushRecorder struct{ m *matrixSinks }

func (p pushRecorder) Name() string { return "rec" }
func (p pushRecorder) Send(_ context.Context, n push.DeliveryNotification) error {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()
	p.m.pushed[n.Tag]++
	if n.Tag == "notification-"+sentinelID {
		p.m.once[1].Do(func() { close(p.m.sentinel[1]) })
	}
	return nil
}

// matrixEnv builds bus + gate + the three sinks and returns a publish helper.
type matrixEnv struct {
	bus   *events.EventBus
	gate  *deliverygate.Gate
	sinks *matrixSinks
	done  func()
}

func newMatrixEnv(t *testing.T, flagOn bool) *matrixEnv {
	t.Helper()
	return newMatrixEnvWith(t, deliverygate.FlagSettings{Global: flagOn},
		deliverygate.Entry{UUID: "u-h", Title: hiddenTitle, Hidden: true, Kind: deliverygate.KindReview})
}

// newMatrixEnvWith builds the matrix over arbitrary flag settings and hidden
// entries; the visible sentinel session is always added.
func newMatrixEnvWith(t *testing.T, settings deliverygate.FlagSettings, hidden ...deliverygate.Entry) *matrixEnv {
	t.Helper()
	bus := events.NewEventBus(256)
	t.Cleanup(bus.Close)
	gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return settings, nil
	}))
	gate.Flags().Reload()
	gate.Index().Replace(append(hidden, deliverygate.Entry{UUID: "u-v", Title: visibleTitle}))
	bus.SetPublishFilter(gate.PublishFilter())

	s := &matrixSinks{history: map[string]int{}, bySess: map[string]int{}, pushed: map[string]int{}, watched: map[string]int{}}
	for i := range s.sentinel {
		s.sentinel[i] = make(chan struct{})
	}
	ctx, cancel := context.WithCancel(context.Background())

	notifications.StartSubscriberWithInterval(ctx, bus, appender{s}, 5*time.Millisecond)
	pushDone := push.StartDeliverySubscriber(ctx, bus, []push.Notifier{pushRecorder{s}}, push.WithSessionDeliveryGate(gate))

	watchCh, _ := bus.Subscribe(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		for ev := range watchCh {
			if ev.Type != events.EventNotification {
				continue
			}
			s.mu.Lock()
			s.watched[sessionv1.NotificationType(ev.NotificationType).String()]++
			if ev.SessionID == visibleTitle {
				s.once[2].Do(func() { close(s.sentinel[2]) })
			}
			s.mu.Unlock()
		}
	}()

	return &matrixEnv{bus: bus, gate: gate, sinks: s, done: func() {
		cancel()
		<-pushDone
		<-watchDone
	}}
}

func (e *matrixEnv) waitSentinel(t *testing.T) {
	t.Helper()
	for i, ch := range e.sinks.sentinel {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("sink %d never saw the sentinel event", i)
		}
	}
}

func notifEvent(session string, typ sessionv1.NotificationType, id string) *events.Event {
	return events.NewNotificationEvent(session, session, id, int32(typ), urgentPrio,
		"title "+typ.String(), "message", nil)
}

// publishEveryType publishes one urgent event per NotificationType for the
// hidden session, a Stopped session.updated for it, then the visible sentinel.
func (e *matrixEnv) publishEveryType(t *testing.T) {
	t.Helper()
	for _, typ := range deliverygate.AllNotificationTypes() {
		e.bus.Publish(notifEvent(hiddenTitle, typ, "n-"+typ.String()))
	}
	e.bus.Publish(&events.Event{
		Type:          events.EventSessionUpdated,
		Session:       &session.Instance{ID: "id-h", UUID: "u-h", Title: hiddenTitle, Hidden: true, Status: session.Stopped},
		UpdatedFields: []string{events.FieldStatus},
	})
	e.bus.Publish(notifEvent(visibleTitle, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, sentinelID))
	e.waitSentinel(t)
	e.done()
}

// T-MX-01: with the gate on, every notification type for a hidden session yields
// zero routine rows and exactly one failure/needs-human row in each sink, and the
// hidden Stopped status-change push is dropped.
func TestMatrix_ShouldYieldZeroRoutineAndOneFailureNeedsHumanInEverySink_WhenHiddenSessionPublishesEveryNotificationType(t *testing.T) {
	t.Parallel()
	env := newMatrixEnv(t, true)
	env.publishEveryType(t)

	env.sinks.mu.Lock()
	defer env.sinks.mu.Unlock()
	for _, typ := range deliverygate.AllNotificationTypes() {
		want := 0
		if deliverygate.ClassOf(typ) != deliverygate.ClassRoutine {
			want = 1
		}
		name := typ.String()
		assert.Equal(t, want, env.sinks.history[name], "history rows for %s", name)
		assert.Equal(t, want, env.sinks.watched[name]-sentinelInfo(name), "watch events for %s", name)
		assert.Equal(t, want, env.sinks.pushed["notification-n-"+name], "push for %s", name)
	}
	_, statusPushed := env.sinks.pushed["session-completed-id-h"]
	assert.False(t, statusPushed, "hidden Stopped status-change push must be dropped")
}

// sentinelInfo accounts for the visible sentinel (an INFO event) seen by the watcher.
func sentinelInfo(typeName string) int {
	if typeName == sessionv1.NotificationType_NOTIFICATION_TYPE_INFO.String() {
		return 1
	}
	return 0
}

// T-MX-03: with the flag off nothing is dropped and every routine type is only
// counted as would-suppress.
func TestMatrix_ShouldDeliverEverythingAndCountWouldSuppress_WhenFlagOff(t *testing.T) {
	t.Parallel()
	env := newMatrixEnv(t, false)
	env.publishEveryType(t)

	routine := 0
	env.sinks.mu.Lock()
	defer env.sinks.mu.Unlock()
	for _, typ := range deliverygate.AllNotificationTypes() {
		if deliverygate.ClassOf(typ) == deliverygate.ClassRoutine {
			routine++
		}
		assert.Equal(t, 1, env.sinks.history[typ.String()], "flag off: history row for %s", typ)
	}
	_, statusPushed := env.sinks.pushed["session-completed-id-h"]
	assert.True(t, statusPushed, "flag off: Stopped push still delivered")
	// bus: one would-suppress per routine type; push_status: one for the Stopped push.
	assert.EqualValues(t, routine+1, env.gate.Metrics().Total(deliverygate.CounterWouldSuppress))
	assert.EqualValues(t, 0, env.gate.Metrics().Total(deliverygate.CounterSuppressed))
}

// T-MX-02 (auto-approved cell): the real approval handler, real classifier and
// real history store behind the gate: a hidden allow is not recorded, a hidden
// deny still is.
func TestMatrix_ShouldYieldZeroRoutineOnAutoApprovedSink_WhenHiddenReviewSession(t *testing.T) {
	t.Parallel()
	store, err := notifications.NewNotificationHistoryStore(t.TempDir() + "/notifications.json")
	require.NoError(t, err)
	gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: true}, nil
	}))
	gate.Flags().Reload()
	gate.Index().Replace([]deliverygate.Entry{{UUID: "u-h", Title: hiddenTitle, Hidden: true, Kind: deliverygate.KindReview}})

	h := services.NewApprovalHandler(services.NewApprovalStore(""), nil, events.NewEventBus(8))
	h.SetClassifier(classifier.NewRuleBasedClassifier())
	h.SetAutoApprovalLogger(store)
	h.SetAutoApprovedGate(gate)

	postPermission(t, h, hiddenTitle, "Bash", map[string]interface{}{"command": "ls -la && pwd"})                  // auto-allow
	postPermission(t, h, hiddenTitle, "Write", map[string]interface{}{"file_path": "/tmp/p/.env", "content": "x"}) // auto-deny

	recs, _, err := store.List(notifications.ListOptions{Limit: 50})
	require.NoError(t, err)
	var allow, deny int
	for _, r := range recs {
		switch r.Metadata["approval_decision"] {
		case "allow":
			allow++
		case "deny":
			deny++
		}
	}
	assert.Equal(t, 0, allow, "hidden auto-allow row must not be recorded")
	assert.Equal(t, 1, deny, "hidden auto-deny row is the audit trail and is kept")
}

// T-MX-04: a review override on with global off suppresses routine events of
// review sessions only; the inheriting hidden kinds (triage, diagnose) are
// delivered and counted as would-suppress. A needs-human event is delivered in
// every sink for every kind.
func TestMatrix_ShouldSuppressOnlyOverriddenKind_WhenReviewOverrideOnAndTriageInherits(t *testing.T) {
	t.Parallel()
	env := newMatrixEnvWith(t,
		deliverygate.FlagSettings{KindOverrides: map[deliverygate.HiddenKind]bool{deliverygate.KindReview: true}},
		deliverygate.Entry{UUID: "u-r", Title: "review:r", Hidden: true, Kind: deliverygate.KindReview},
		deliverygate.Entry{UUID: "u-t", Title: "triage:t", Hidden: true, Kind: deliverygate.KindTriage},
		deliverygate.Entry{UUID: "u-d", Title: "diagnose:d", Hidden: true, Kind: deliverygate.KindDiagnose},
	)
	done := sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE
	ask := sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED
	for _, title := range []string{"review:r", "triage:t", "diagnose:d"} {
		env.bus.Publish(notifEvent(title, done, "done-"+title))
		env.bus.Publish(notifEvent(title, ask, "ask-"+title))
	}
	env.bus.Publish(notifEvent(visibleTitle, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, sentinelID))
	env.waitSentinel(t)
	env.done()

	env.sinks.mu.Lock()
	defer env.sinks.mu.Unlock()
	assert.Equal(t, 0, env.sinks.bySess["review:r|"+done.String()], "review routine suppressed")
	assert.Equal(t, 1, env.sinks.bySess["triage:t|"+done.String()], "inheriting triage delivered")
	assert.Equal(t, 1, env.sinks.bySess["diagnose:d|"+done.String()], "inheriting diagnose delivered")
	for _, title := range []string{"review:r", "triage:t", "diagnose:d"} {
		assert.Equal(t, 1, env.sinks.bySess[title+"|"+ask.String()], "needs-human delivered for %s", title)
	}
	assert.EqualValues(t, 1, env.gate.Metrics().Total(deliverygate.CounterSuppressed))
	assert.EqualValues(t, 2, env.gate.Metrics().Total(deliverygate.CounterWouldSuppress))
}
