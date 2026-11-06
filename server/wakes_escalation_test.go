package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type escalationOwners struct{}

func (escalationOwners) Owner(context.Context, string) (wakes.OwnerFacts, error) {
	return wakes.OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"}, nil
}

func escalationDispatcher(t *testing.T, storage *session.Storage, bus *events.EventBus, clock *wakes.FakeClock) *wakes.Dispatcher {
	t.Helper()
	d := newWakeDispatcher(wakeWiring{
		Store: storage, Steerer: wakeTestSteerer{wrote: make(chan string, 1)}, Owners: escalationOwners{},
		Admission: wakeTestAdmission{}, Clock: clock, Notifier: wakeNotifier{bus: bus}, Events: bus, Leader: wakes.NoLeader{},
		Peers: wakePeerRequests{storage: storage, titles: func(id string) string { return "title-" + id }},
	}, storage.SetWakeChangeHook)
	require.NoError(t, d.Start(context.Background()))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	return d
}

func TestPeerHelpEscalation_should_RaiseOneInputRequiredDeliveredForAHiddenAsker_And_NotRepeatAfterRestart(t *testing.T) {
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: "asker", QuestionText: "Which approach?", QuestionType: domain.QuestionTypeShortAnswer,
		Source: domain.QuestionSourcePeer, AskerSessionUUID: "asker", AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: "peer",
	})
	require.NoError(t, err)
	row, err := storage.CreateMirroredGuidanceRequest(context.Background(), req, 10)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	// The request is 11 minutes old by the clock the dispatcher uses.
	clock := wakes.NewFakeClock(time.Now().Add(11 * time.Minute))
	first := escalationDispatcher(t, storage, bus, clock)

	var got *events.Event
	select {
	case got = <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("no escalation notification")
	}
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED), got.NotificationType)
	assert.Equal(t, "peer-help-stalled:"+row.ID, got.NotificationID)
	assert.Equal(t, row.ID, got.NotificationMetadata[services.MetaGuidanceRequestID])
	assert.Equal(t, "title-peer", got.NotificationMetadata[services.MetaPeerTitle])
	assert.Equal(t, "asker", got.SessionID)
	hint, _ := deliverygate.ParseClassHint(got.NotificationMetadata)
	d := deliverygate.ShouldDeliver(deliverygate.VisibilityHidden, deliverygate.Facts{Type: sessionv1.NotificationType(got.NotificationType), Hint: hint})
	assert.Equal(t, deliverygate.OutcomeDeliver, d.Outcome)
	assert.Equal(t, deliverygate.ClassNeedsHuman, d.Class)

	require.NoError(t, first.Stop(context.Background()))
	escalationDispatcher(t, storage, bus, clock)
	select {
	case again := <-ch:
		t.Fatalf("a restart raised a second notification: %s", again.NotificationID)
	case <-time.After(500 * time.Millisecond):
	}
}
