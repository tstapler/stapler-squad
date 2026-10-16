package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

func TestPinSession_should_PersistPinnedTrue_When_RPCSucceeds(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addPausedSession(t, fix, "pin-me")
	ctx := context.Background()

	for range 2 { // idempotent
		_, err := fix.svc.PinSession(ctx, connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "pin-me"}))
		require.NoError(t, err)
	}
	assert.True(t, fix.poller.FindInstance("pin-me").Snapshot().Pinned)

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := false
	for _, li := range loaded {
		if li.Title == "pin-me" {
			found = true
			assert.True(t, li.Snapshot().Pinned, "pin must be persisted to storage")
		}
	}
	require.True(t, found)

	for range 2 {
		_, err = fix.svc.UnpinSession(ctx, connect.NewRequest(&sessionv1.UnpinSessionRequest{SessionId: "pin-me"}))
		require.NoError(t, err)
	}
	assert.False(t, fix.poller.FindInstance("pin-me").Snapshot().Pinned)
}

func TestPinSession_should_ReturnFailedPrecondition_When_SessionArchived(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addPausedSession(t, fix, "archived-one")
	ctx := context.Background()
	_, err := fix.svc.ArchiveSession(ctx, connect.NewRequest(&sessionv1.ArchiveSessionRequest{SessionId: "archived-one"}))
	require.NoError(t, err)

	_, err = fix.svc.PinSession(ctx, connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "archived-one"}))

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestPinSession_should_ReturnNotFoundOrInvalid_When_IDBad(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	ctx := context.Background()

	_, err := fix.svc.PinSession(ctx, connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "nope"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = fix.svc.UnpinSession(ctx, connect.NewRequest(&sessionv1.UnpinSessionRequest{SessionId: ""}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestPinSession_should_PublishSessionUpdatedEvent_When_Pinned(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addPausedSession(t, fix, "pin-event")
	ch, subID := fix.svc.eventBus.Subscribe(context.Background())
	defer fix.svc.eventBus.Unsubscribe(subID)

	_, err := fix.svc.PinSession(context.Background(), connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "pin-event"}))
	require.NoError(t, err)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventSessionUpdated, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a session_updated event after pinning")
	}
}
