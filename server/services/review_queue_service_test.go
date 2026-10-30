package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// setupRQSFixture creates a ReviewQueueService wired with real SQLite storage and a
// ReviewQueuePoller. It mirrors the setupForkTestFixture pattern.
func setupRQSFixture(t *testing.T) (rqs *ReviewQueueService, poller *session.ReviewQueuePoller, storage *session.Storage, cleanup func()) {
	t.Helper()

	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller = session.NewReviewQueuePoller(queue, statusMgr, nil)

	rqs = NewReviewQueueService(queue, storage, bus)
	rqs.SetReviewQueuePoller(poller)

	cleanup = func() {
		bus.Close()
	}
	return
}

// TestAcknowledgeSession_UpdatesLiveInstance verifies that AcknowledgeSession modifies
// the live in-memory instance held by the poller rather than replacing it with a fresh
// copy from LoadInstances. Replacing poller instances discards live state (PTY handles,
// controllers, etc.) on every instance, not just the one being acknowledged.
func TestAcknowledgeSession_UpdatesLiveInstance(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func storedAckRow(t *testing.T, storage *session.Storage, title string) session.InstanceData {
	t.Helper()
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, r := range rows {
		if r.Title == title {
			return r
		}
	}
	t.Fatalf("session %q not stored", title)
	return session.InstanceData{}
}

func openQuestionInstance(t *testing.T, storage *session.Storage, title string) *session.Instance {
	t.Helper()
	inst := &session.Instance{Title: title, Path: "/tmp/test", Status: session.Paused, Program: "claude"}
	require.NoError(t, storage.AddInstance(inst))
	inst.SetTurnEndSink(session.NewTurnEndSink(storage))
	inst.RecordTurnEnd("Which approach, A or B?", session.TurnEndSourceHook)
	require.False(t, storedAckRow(t, storage, title).TurnEnded.IsZero(), "turn end persisted by the narrow writer")
	return inst
}

func TestAcknowledgeSession_LiveBranch_Should_PersistAckedEpisode(t *testing.T) {
	t.Parallel()
	rqs, poller, storage, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)
	inst := openQuestionInstance(t, storage, "ack-live")
	poller.SetInstances([]*session.Instance{inst})
	episode := inst.Snapshot().TurnEnded.Episode

	_, err := rqs.AcknowledgeSession(context.Background(), connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{Id: "ack-live"}))
	require.NoError(t, err)

	assert.Equal(t, episode, inst.Snapshot().AckedEpisodeID)
	row := storedAckRow(t, storage, "ack-live")
	assert.Equal(t, episode, row.AckedEpisodeID)
	assert.False(t, row.LastAcknowledged.IsZero())
	restored, err := session.FromInstanceDataDeferred(row)
	require.NoError(t, err)
	assert.Equal(t, episode, restored.Snapshot().AckedEpisodeID, "restart sees the ack")
}

func TestAcknowledgeSession_NotInPollerBranch_Should_PersistAckedEpisodeWithoutFullSave(t *testing.T) {
	t.Parallel()
	rqs, _, storage, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)
	inst := openQuestionInstance(t, storage, "ack-stored")
	episode := inst.Snapshot().TurnEnded.Episode

	_, err := rqs.AcknowledgeSession(context.Background(), connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{Id: "ack-stored"}))
	require.NoError(t, err)

	row := storedAckRow(t, storage, "ack-stored")
	assert.Equal(t, episode, row.AckedEpisodeID)
	assert.False(t, row.LastAcknowledged.IsZero())
	assert.Equal(t, episode, row.TurnEnded.Episode, "the ack did not disturb the turn-end columns")
}
