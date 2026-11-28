package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/markerstore"
	"github.com/tstapler/stapler-squad/session/queue"
)

// Ports are taken before the setters run, so each one has to read the field
// when it is called, not when it is built.
func TestStreamPorts_ShouldReadTheServiceFieldsAtCallTime_When_SettersRunLater(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	ports := svc.streamPorts()

	assert.Nil(t, ports.FindPolled("x"))
	_, polled := ports.Polled()
	assert.False(t, polled)
	assert.Nil(t, ports.StatusManager())
	assert.Nil(t, ports.Federation())

	stored := instance.NewPausedTestInstance("stored")
	require.NoError(t, storage.AddInstance(stored))
	loaded, err := ports.LoadWired()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	data, err := ports.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, data, 1)

	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	live := &instance.Instance{Title: "live"}
	poller.AddInstance(live)
	statusMgr := instance.NewInstanceStatusManager()
	mirror := federation.NewStaticMirror("h", nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetStatusManager(statusMgr)
	svc.SetFederation(mirror)

	assert.Same(t, live, ports.FindPolled("live"))
	assert.Same(t, live, ports.FindInstance("live"))
	insts, polled := ports.Polled()
	assert.True(t, polled)
	assert.Len(t, insts, 1)
	assert.Same(t, statusMgr, ports.StatusManager())
	assert.Same(t, mirror, ports.Federation())
}

func TestWriteToSession_ShouldRecordAManualNudge_When_TheSessionBelongsToABacklogItem(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)
	ctx := context.Background()
	item, err := e.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "nudged"})
	require.NoError(t, err)
	_, err = e.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: p.inst.UUID, SessionRole: "work"})
	require.NoError(t, err)
	start := time.Now().Add(-time.Minute)

	_, err = e.client.WriteToSession(ctx, connect.NewRequest(&sessionv1.WriteToSessionRequest{SessionId: p.inst.Title, Input: uuid.NewString()}))
	require.NoError(t, err)

	got, err := e.svc.concStorage.Repo.ListFlowMarkers(ctx, markerstore.FlowMarkerManualNudge, start, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, item.ID, got[0].ItemID)
	assert.Equal(t, markerstore.ManualNudgeWrite, got[0].Value)
}
