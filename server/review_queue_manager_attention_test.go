package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

type fakeEpisodeHistory struct{ seen map[string]bool }

func (f fakeEpisodeHistory) HasEpisode(sessionID, episodeID string) bool {
	return f.seen[sessionID+"|"+episodeID]
}

type fakeSuppressionRecorder struct {
	session.SessionEventRecorder
	mu   sync.Mutex
	rows [][2]string
}

func (f *fakeSuppressionRecorder) RecordNotificationSuppressed(title, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, [2]string{title, reason})
}

func attentionItem(title string, reason session.AttentionReason, episode string) *session.ReviewItem {
	p := session.PriorityLow
	if reason == session.ReasonProseQuestion {
		p = session.PriorityMedium
	}
	return &session.ReviewItem{
		SessionID: title, SessionName: title, Reason: reason, Priority: p,
		DetectedAt: time.Now(), Context: "excerpt",
		Metadata: map[string]string{"episode_id": episode},
	}
}

func collectNotifications(t *testing.T, ch <-chan *events.Event, wait time.Duration) []*events.Event {
	t.Helper()
	var out []*events.Event
	deadline := time.After(wait)
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				out = append(out, ev)
			}
		case <-deadline:
			return out
		}
	}
}

func TestOnItemAdded_ShouldMapAttentionReasonsToNotificationTypes(t *testing.T) {
	t.Parallel()
	mgr, _, bus := newReactiveQueueTestSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	mgr.OnItemAdded(attentionItem("q", session.ReasonProseQuestion, "E1"))
	mgr.OnItemAdded(attentionItem("a", session.ReasonAwaitingDirection, "E1"))

	got := collectNotifications(t, ch, 200*time.Millisecond)
	require.Len(t, got, 2)
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED), got[0].NotificationType)
	assert.Equal(t, events.AttentionReasonProseQuestion, got[0].NotificationMetadata[events.MetadataKeyAttentionReason])
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), got[1].NotificationType)
	assert.Equal(t, events.AttentionReasonAwaitingDirection, got[1].NotificationMetadata[events.MetadataKeyAttentionReason])
	assert.Equal(t, "E1", got[1].NotificationMetadata[events.MetadataKeyEpisodeID])
}

func TestOnItemAdded_ShouldPublishOncePerEpisode_WhenSameEpisodeReAdded(t *testing.T) {
	t.Parallel()
	mgr, _, bus := newReactiveQueueTestSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	mgr.OnItemAdded(attentionItem("demo", session.ReasonAwaitingDirection, "E1"))
	mgr.OnItemAdded(attentionItem("demo", session.ReasonAwaitingDirection, "E1")) // poller re-add
	mgr.OnItemAdded(attentionItem("demo", session.ReasonAwaitingDirection, "E2"))

	got := collectNotifications(t, ch, 200*time.Millisecond)
	require.Len(t, got, 2, "E1 once, E2 its own")
	assert.Equal(t, "E2", got[1].NotificationMetadata[events.MetadataKeyEpisodeID])
}

func TestOnItemAdded_ShouldNotRepublish_WhenEpisodeAlreadyInPersistedHistory(t *testing.T) {
	t.Parallel()
	mgr, _, bus := newReactiveQueueTestSetup(t)
	mgr.SetEpisodeHistory(fakeEpisodeHistory{seen: map[string]bool{"demo|E1": true}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	mgr.OnItemAdded(attentionItem("demo", session.ReasonProseQuestion, "E1")) // server restart rebuilt the queue

	assert.Empty(t, collectNotifications(t, ch, 200*time.Millisecond))
}

func TestOnItemAdded_ShouldSkipSlackForAwaitingDirectionOnly(t *testing.T) {
	setTestSlackConfig(t, true, 0)
	mgr, _, _ := newReactiveQueueTestSetup(t)
	fake := &fakeSlackNotifierWiring{}
	mgr.SetSlackNotifier(fake)

	mgr.OnItemAdded(attentionItem("a", session.ReasonAwaitingDirection, "E1"))
	notify, threshold := fake.counts()
	assert.Zero(t, notify)
	assert.Zero(t, threshold)

	mgr.OnItemAdded(attentionItem("q", session.ReasonProseQuestion, "E1"))
	notify, _ = fake.counts()
	assert.Equal(t, 1, notify, "prose_question reaches Slack as INPUT_REQUIRED does")
}

func TestOnItemAdded_ShouldRecordSuppressionEvent_WhenGateSuppressesHiddenAwaitingDirection(t *testing.T) {
	mgr, poller, _ := newReactiveQueueTestSetup(t)
	rec := &fakeSuppressionRecorder{}
	mgr.SetEventRecorder(rec)

	inst := &session.Instance{Title: "hidden-demo", UUID: "uuid-hidden-1", Hidden: true}
	poller.SetInstances([]*session.Instance{inst})
	gate := gateBusOn(t, mgr, true, inst)
	gate.SetSuppressionObserver(mgr.OnDeliverySuppressed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := mgr.eventBus.Subscribe(ctx)

	mgr.OnItemAdded(attentionItem("hidden-demo", session.ReasonAwaitingDirection, "E1"))

	assert.Empty(t, collectNotifications(t, ch, 200*time.Millisecond), "gate drops it")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Equal(t, [][2]string{{"hidden-demo", "routine_for_hidden"}}, rec.rows)
}
