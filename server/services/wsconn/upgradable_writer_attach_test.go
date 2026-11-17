package wsconn

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

func attachFixture(t *testing.T, h *ConnectRPCWebSocketHandler, inst *instance.Instance) (*streamhub.StreamHub, *upgradableWriter, <-chan struct{}, context.CancelFunc) {
	t.Helper()
	hub := streamhub.NewStreamHub("attach", nil, streamhub.WithTeardownGrace(time.Hour))
	t.Cleanup(func() { _ = hub.ForceTeardown() })
	stream, _, cleanup := createTestWebSocketPair(t)
	t.Cleanup(cleanup)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w, _, watchDone := h.attachUpgradableSubscriber(ctx, hub, hubSubscriberAttachment{
		stream: stream, instance: inst, sessionID: "attach", tmuxSessionName: "attach", currentPaneReq: handshakeReq(100, 30),
	})
	return hub, w, watchDone, cancel
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

func newServiceWithBus(t *testing.T) *services.SessionService {
	t.Helper()
	svc := services.NewSessionService(svctest.CreateTestStorage(t), events.NewEventBus(100))
	t.Cleanup(svc.Shutdown)
	return svc
}

func TestAttachUpgradableSubscriber_should_UpgradeViaServiceEventBus_When_StatusFlipsWithNoClientFrame(t *testing.T) {
	t.Parallel()
	svc := newServiceWithBus(t)
	h := NewConnectRPCWebSocketHandler(svc, nil, nil)
	inst := newTymuxInstanceWithStatus(t, domain.PermanentlyFailed)
	hub, w, watchDone, _ := attachFixture(t, h, inst)
	require.Equal(t, streamhub.TerminalSize{}, hub.NegotiatedSize(), "read-only attach casts no vote")

	instance.SetStatusForTest(t, inst, domain.Active, nil)
	svc.GetEventBus().Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))

	waitClosed(t, w.Upgraded(), "upgrade")
	w.WaitVotes()
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{100, 30}, [2]int{got.Cols(), got.Rows()})
	waitClosed(t, watchDone, "bus subscription release after upgrade")
	require.Zero(t, svc.GetEventBus().SubscriberCount(), "the bus subscription is released after the upgrade")
}

func TestAttachUpgradableSubscriber_should_UpgradeOnUnarchiveEvent(t *testing.T) {
	t.Parallel()
	svc := newServiceWithBus(t)
	h := NewConnectRPCWebSocketHandler(svc, nil, nil)
	inst := newTymuxInstanceWithStatus(t, domain.Active)
	archivedAt := time.Now()
	instance.SetStatusForTest(t, inst, domain.Active, &archivedAt)
	_, w, _, _ := attachFixture(t, h, inst)

	instance.SetStatusForTest(t, inst, domain.Active, nil)
	svc.GetEventBus().Publish(events.NewSessionUpdatedEvent(inst, []string{"archived_at"}))

	waitClosed(t, w.Upgraded(), "upgrade after unarchive")
}

func TestAttachUpgradableSubscriber_should_ReleaseBusSubscription_When_ConnectionContextIsCancelled(t *testing.T) {
	t.Parallel()
	svc := newServiceWithBus(t)
	h := NewConnectRPCWebSocketHandler(svc, nil, nil)
	inst := newTymuxInstanceWithStatus(t, domain.PermanentlyFailed)
	_, _, watchDone, cancel := attachFixture(t, h, inst)

	cancel()
	waitClosed(t, watchDone, "bus subscription release after cancel")
	require.Zero(t, svc.GetEventBus().SubscriberCount())
}

func TestAttachUpgradableSubscriber_should_UpgradeOnNextFrame_When_NoServiceBus(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	inst := newTymuxInstanceWithStatus(t, domain.PermanentlyFailed)
	hub, w, _, _ := attachFixture(t, h, inst)

	instance.SetStatusForTest(t, inst, domain.Active, nil)
	require.NotNil(t, w.Writer(), "the next frame's writer lookup upgrades")

	waitClosed(t, w.Upgraded(), "upgrade")
	w.WaitVotes()
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{100, 30}, [2]int{got.Cols(), got.Rows()})
}
