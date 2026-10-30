package server

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

type countingRecorder struct {
	session.SessionEventRecorder
	mu    sync.Mutex
	turns []session.TurnEnded
}

func (c *countingRecorder) RecordTurnEnded(_ string, t session.TurnEnded) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turns = append(c.turns, t)
}

func (c *countingRecorder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.turns)
}

func wiredPollerAndStorage(t *testing.T, rec session.SessionEventRecorder, enabled func() bool) (*session.ReviewQueuePoller, *session.Storage) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), storage)
	wireAttentionCollaborators(poller, storage, rec, enabled)
	return poller, storage
}

func persistedInstance(t *testing.T, storage *session.Storage, title string) *session.Instance {
	t.Helper()
	inst := &session.Instance{Title: title, Path: "/tmp/test", Status: session.Paused, Program: "claude"}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}

func storedTurn(t *testing.T, storage *session.Storage, title string) session.TurnEnded {
	t.Helper()
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, r := range rows {
		if r.Title == title {
			return r.TurnEnded
		}
	}
	t.Fatalf("session %q not stored", title)
	return session.TurnEnded{}
}

func TestAttentionWiring_AddedInstance_Should_ReachRealSinkAndRecorder_When_EventLogOn(t *testing.T) {
	t.Parallel()
	rec := &countingRecorder{SessionEventRecorder: session.NewNoopSessionEventRecorder()}
	poller, storage := wiredPollerAndStorage(t, rec, func() bool { return true })
	inst := persistedInstance(t, storage, "wired-add")

	poller.AddInstance(inst)
	inst.RecordTurnEnd("Which approach, A or B?", session.TurnEndSourceHook)

	assert.Equal(t, inst.Snapshot().TurnEnded.Episode, storedTurn(t, storage, "wired-add").Episode, "real sink stored the fact")
	assert.Equal(t, 1, rec.count())
}

func TestAttentionWiring_SetInstances_Should_WireEveryInstance(t *testing.T) {
	t.Parallel()
	rec := &countingRecorder{SessionEventRecorder: session.NewNoopSessionEventRecorder()}
	poller, storage := wiredPollerAndStorage(t, rec, func() bool { return true })
	a, b := persistedInstance(t, storage, "wired-a"), persistedInstance(t, storage, "wired-b")

	poller.SetInstances([]*session.Instance{a, b})
	a.RecordTurnEnd("done", session.TurnEndSourceHook)
	b.RecordTurnEnd("done too", session.TurnEndSourceHook)

	assert.False(t, storedTurn(t, storage, "wired-a").IsZero())
	assert.False(t, storedTurn(t, storage, "wired-b").IsZero())
	assert.Equal(t, 2, rec.count())
}

func TestAttentionWiring_EventLogOff_Should_KeepRecorderNoopButStillPersistFact(t *testing.T) {
	t.Parallel()
	rec := &countingRecorder{SessionEventRecorder: session.NewNoopSessionEventRecorder()}
	poller, storage := wiredPollerAndStorage(t, rec, func() bool { return false })
	inst := persistedInstance(t, storage, "wired-off")
	poller.AddInstance(inst)

	inst.RecordTurnEnd("Which approach, A or B?", session.TurnEndSourceHook)

	assert.Zero(t, rec.count(), "session_event_log off: recorder must not be called")
	assert.False(t, storedTurn(t, storage, "wired-off").IsZero(), "the turn-end fact persists unflagged")
}

func TestAttentionWiring_FlagFlip_Should_TakeEffectWithoutRewiring(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	on := false
	rec := &countingRecorder{SessionEventRecorder: session.NewNoopSessionEventRecorder()}
	poller, storage := wiredPollerAndStorage(t, rec, func() bool { mu.Lock(); defer mu.Unlock(); return on })
	inst := persistedInstance(t, storage, "wired-flip")
	poller.AddInstance(inst)

	inst.RecordTurnEnd("one", session.TurnEndSourceHook)
	mu.Lock()
	on = true
	mu.Unlock()
	inst.RecordTurnEnd("two", session.TurnEndSourceHook)

	assert.Equal(t, 1, rec.count())
}
