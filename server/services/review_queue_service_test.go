package services

import (
	"context"
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// setupRQSFixture creates a ReviewQueueService wired with real SQLite storage and a
// ReviewQueuePoller. It mirrors the setupForkTestFixture pattern.
func setupRQSFixture(t *testing.T) (rqs *ReviewQueueService, poller *session.ReviewQueuePoller, storage *session.Storage, cleanup func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "rqs-test-*")
	require.NoError(t, err)

	dbPath := fmt.Sprintf("%s/sessions.db", tmpDir)
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	storage, err = session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller = session.NewReviewQueuePoller(queue, statusMgr, nil)

	rqs = NewReviewQueueService(queue, storage, bus)
	rqs.SetReviewQueuePoller(poller)

	cleanup = func() {
		bus.Close()
		repo.Close()
		os.RemoveAll(tmpDir)
	}
	return
}

// TestAcknowledgeSession_UpdatesLiveInstance verifies that AcknowledgeSession modifies
// the live in-memory instance held by the poller rather than replacing it with a fresh
// copy from LoadInstances. Replacing poller instances discards live state (PTY handles,
// controllers, etc.) on every instance, not just the one being acknowledged.
func TestAcknowledgeSession_UpdatesLiveInstance(t *testing.T) {
	rqs, poller, storage, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)

	// Persist a paused instance so AcknowledgeSession can find it in storage.
	persisted := &session.Instance{
		Title:   "sess-1",
		Path:    "/tmp/test",
		Status:  session.Paused,
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(persisted))

	// Register the sentinel with the poller — this represents the live in-memory state.
	sentinel := persisted
	poller.SetInstances([]*session.Instance{sentinel})

	_, err := rqs.AcknowledgeSession(context.Background(), connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{
		Id: "sess-1",
	}))
	require.NoError(t, err)

	// The poller must still hold the exact same pointer — not a fresh copy from
	// LoadInstances. Replacing it discards live PTY/controller state on ALL instances.
	found := poller.FindInstance("sess-1")
	require.NotNil(t, found, "instance should still be in poller after acknowledge")
	if found != sentinel {
		t.Error("AcknowledgeSession replaced the live poller instance with a fresh copy; " +
			"the sentinel pointer must remain unchanged so live state is preserved",
		)
	}

	// The live sentinel itself must have been acknowledged.
	if sentinel.LastAcknowledged.IsZero() {
		t.Error("AcknowledgeSession did not call MarkAcknowledged on the live sentinel instance")
	}
}

// TestAcknowledgeSession_UnknownSession verifies that AcknowledgeSession succeeds
// gracefully when the session ID is not in storage (external session / corrupt ID).
func TestAcknowledgeSession_UnknownSession(t *testing.T) {
	rqs, _, _, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)

	resp, err := rqs.AcknowledgeSession(context.Background(), connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{
		Id: "nonexistent-session",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)
}

// TestAcknowledgeSession_EmptyID verifies that a missing session ID returns InvalidArgument.
func TestAcknowledgeSession_EmptyID(t *testing.T) {
	rqs, _, _, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)

	_, err := rqs.AcknowledgeSession(context.Background(), connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{
		Id: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
