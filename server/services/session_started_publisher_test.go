package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func newStartedInstanceWithStatus(t *testing.T, status domain.Status) *session.Instance {
	t.Helper()
	inst := session.NewStartedInstanceForTest(t, "started-pub", namedPM{replyPM: &replyPM{}, name: "started-pub"})
	session.SetStatusForTest(t, inst, status, nil)
	return inst
}

func newPublisherServiceWithBus(t *testing.T) *SessionService {
	t.Helper()
	svc := NewSessionService(svctest.CreateTestStorage(t), events.NewEventBus(100))
	t.Cleanup(svc.Shutdown)
	return svc
}

func drainEvents(ch <-chan *events.Event) (n int) {
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}

func TestSessionStartedPublisher_should_PublishExactlyOncePerStart_When_InstanceIsActive(t *testing.T) {
	t.Parallel()
	svc := newPublisherServiceWithBus(t)
	inst := newStartedInstanceWithStatus(t, domain.Active)
	ch, _ := svc.GetEventBus().Subscribe(context.Background())
	l := &sessionStartedPublisher{svc: svc, inst: inst}

	require.True(t, l.publishIfActive())
	ev := <-ch
	require.Equal(t, events.EventSessionUpdated, ev.Type)
	require.Equal(t, inst.UUID, ev.Session.UUID)
	require.Zero(t, drainEvents(ch), "one start publishes one event")

	require.True(t, l.publishIfActive())
	<-ch
	require.Zero(t, drainEvents(ch))
}

func TestSessionStartedPublisher_should_PublishNothing_When_TransitionFailedOrArchived(t *testing.T) {
	t.Parallel()
	svc := newPublisherServiceWithBus(t)
	ch, _ := svc.GetEventBus().Subscribe(context.Background())

	failed := newStartedInstanceWithStatus(t, domain.Stopped) // reconcile fired EventStarted, transition failed
	require.False(t, (&sessionStartedPublisher{svc: svc, inst: failed}).publishIfActive())

	archivedAt := time.Now()
	archived := newStartedInstanceWithStatus(t, domain.Active)
	session.SetStatusForTest(t, archived, domain.Active, &archivedAt)
	require.False(t, (&sessionStartedPublisher{svc: svc, inst: archived}).publishIfActive())

	require.Zero(t, drainEvents(ch))
}

func TestSessionStartedPublisher_should_PublishViaLifecycleListener_When_InstanceStarts(t *testing.T) {
	t.Parallel()
	svc := newPublisherServiceWithBus(t)
	inst := newStartedInstanceWithStatus(t, domain.Active)
	ch, _ := svc.GetEventBus().Subscribe(context.Background())

	(&sessionStartedPublisher{svc: svc, inst: inst}).OnLifecycleEvent(session.EventStarted, "")

	select {
	case ev := <-ch:
		require.Equal(t, events.EventSessionUpdated, ev.Type)
	case <-time.After(10 * time.Second):
		t.Fatal("no session-updated event published for EventStarted")
	}
}
