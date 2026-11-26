package backlog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type fakeSnapshotSource map[string]time.Time

func (f fakeSnapshotSource) LiveSnapshot(id string) (*domain.InstanceSnapshot, bool) {
	t, ok := f[id]
	if !ok {
		return nil, false
	}
	snap := &domain.InstanceSnapshot{}
	snap.LastMeaningfulOutput = t
	return snap, true
}

func staleWorkOpen(t *testing.T, l *BacklogLifecycleListener, itemID string) bool {
	t.Helper()
	ctx := context.Background()
	l.reconcileStaleWorkSessions(ctx, l.storage.Repo)
	open, err := l.storage.Repo.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := backlogdto.FindOpenStuckStateFor(open, itemID, domain.StuckReasonStaleWork)
	return ok
}

func workSessionOf(t *testing.T, l *BacklogLifecycleListener, itemID string) backlogdto.ItemSessionSummary {
	t.Helper()
	ss, err := l.storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	return ss[0]
}

// Root cause under test: last_progress_at moves only on commits/notes, so a
// session with live output but no commits is marked stale_work.
func TestHeartbeat_ActiveSession_NotMarkedStale(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	item := newStaleWorkTestItem(t, storage, storage.Repo) // last_progress_at = 3h ago
	l := NewBacklogLifecycleListener(storage)
	sess := workSessionOf(t, l, item.ID)

	l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: time.Now()})
	// Without the heartbeat step the sweep marks it stale (the bug).
	require.True(t, staleWorkOpen(t, l, item.ID), "precondition: sweep alone marks the output-only session stale")

	l.refreshWorkSessionHeartbeat(context.Background())
	assert.WithinDuration(t, time.Now(), *workSessionOf(t, l, item.ID).LastProgressAt, 5*time.Second)
	assert.False(t, staleWorkOpen(t, l, item.ID), "stale_work must resolve once output advances the heartbeat")
}

func TestHeartbeat_SilentSession_StillMarkedStale(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	item := newStaleWorkTestItem(t, storage, storage.Repo)
	l := NewBacklogLifecycleListener(storage)
	sess := workSessionOf(t, l, item.ID)
	l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: time.Now().Add(-4 * time.Hour)})

	l.refreshWorkSessionHeartbeat(context.Background())
	assert.True(t, staleWorkOpen(t, l, item.ID))
}

func TestHeartbeat_NoWriteWhenAdvanceUnderThresholdOrNotLive(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	item := newStaleWorkTestItem(t, storage, storage.Repo)
	l := NewBacklogLifecycleListener(storage)
	sess := workSessionOf(t, l, item.ID)
	before := *sess.LastProgressAt

	l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: before.Add(10 * time.Second)})
	l.refreshWorkSessionHeartbeat(context.Background())
	assert.True(t, workSessionOf(t, l, item.ID).LastProgressAt.Equal(before), "advance < 30s must not write")

	l.SetHeartbeatSource(fakeSnapshotSource{})
	l.refreshWorkSessionHeartbeat(context.Background())
	assert.True(t, workSessionOf(t, l, item.ID).LastProgressAt.Equal(before), "not-live session must not write")
}

func TestHeartbeat_ThresholdBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		advance time.Duration
		written bool
	}{
		{"exactly_threshold_writes", heartbeatMinAdvance, true},
		{"just_under_threshold_skips", heartbeatMinAdvance - time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage, cleanup := createTestStorage(t)
			t.Cleanup(cleanup)
			item := newStaleWorkTestItem(t, storage, storage.Repo)
			l := NewBacklogLifecycleListener(storage)
			sess := workSessionOf(t, l, item.ID)
			before := *sess.LastProgressAt
			out := before.Add(tc.advance)
			l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: out})

			l.refreshWorkSessionHeartbeat(context.Background())

			got := *workSessionOf(t, l, item.ID).LastProgressAt
			if tc.written {
				assert.WithinDuration(t, out, got, time.Second)
			} else {
				assert.True(t, got.Equal(before))
			}
		})
	}
}

func TestHeartbeat_NeverMovesBackwardsOrIntoFuture(t *testing.T) {
	t.Parallel()
	for name, offset := range map[string]time.Duration{"older_than_clock": -4 * time.Hour, "future_output": time.Hour} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			storage, cleanup := createTestStorage(t)
			t.Cleanup(cleanup)
			item := newStaleWorkTestItem(t, storage, storage.Repo)
			l := NewBacklogLifecycleListener(storage)
			sess := workSessionOf(t, l, item.ID)
			before := *sess.LastProgressAt
			l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: time.Now().Add(offset)})

			l.refreshWorkSessionHeartbeat(context.Background())

			assert.True(t, workSessionOf(t, l, item.ID).LastProgressAt.Equal(before))
		})
	}
}

func TestHeartbeat_NilSourceAndEndedSessionDoNotWrite(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	item := newStaleWorkTestItem(t, storage, storage.Repo)
	l := NewBacklogLifecycleListener(storage)
	sess := workSessionOf(t, l, item.ID)
	before := *sess.LastProgressAt

	l.refreshWorkSessionHeartbeat(context.Background()) // no source wired
	assert.True(t, workSessionOf(t, l, item.ID).LastProgressAt.Equal(before), "nil source must not write")

	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), sess.ID, time.Now()))
	l.SetHeartbeatSource(fakeSnapshotSource{sess.SessionUUID: time.Now()})
	l.refreshWorkSessionHeartbeat(context.Background())
	assert.True(t, workSessionOf(t, l, item.ID).LastProgressAt.Equal(before), "ended session must not write")
}
