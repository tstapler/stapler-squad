package services

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// newReadyItemThatCannotSpawn returns a service plus a ready item whose spawn always
// fails (no repo_path), the shape of the live dequeue loop on item 0791f5b4.
func newReadyItemThatCannotSpawn(t *testing.T) (*BacklogService, *session.Storage, string) {
	t.Helper()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "cannot spawn",
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true,
		SkipPlanning:       true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: itemID, TargetStatus: "ready",
	}))
	require.NoError(t, err)
	return svc, storage, itemID
}

func countEventsWithNote(t *testing.T, storage *session.Storage, itemID, note string) int {
	t.Helper()
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	n := 0
	for _, e := range item.StatusEvents {
		if e.Note != nil && *e.Note == note {
			n++
		}
	}
	return n
}

func openSpawnFailedRow(t *testing.T, storage *session.Storage, itemID string) (session.OpenStuckStateData, bool) {
	t.Helper()
	rows, err := storage.FindOpenStuckStates(t.Context())
	require.NoError(t, err)
	for _, r := range rows {
		if r.ItemID == itemID && r.Reason == domain.StuckReasonSpawnFailed {
			return r, true
		}
	}
	return session.OpenStuckStateData{}, false
}

// Regression for the live ready<->in_progress loop: with no backoff every sweep re-claimed
// the item, failed, and rolled back (one flip pair per minute). After the first failure the
// item must sit out the backoff window, however many sweeps run inside it.
func TestDequeueNextQueuedItems_BacksOffAfterSpawnFailure(t *testing.T) {
	t.Parallel()
	svc, storage, itemID := newReadyItemThatCannotSpawn(t)

	for range 10 {
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	}

	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeue spawn failed"),
		"ten sweeps inside the backoff window may roll back only once")
	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeued: WIP slot freed"),
		"ten sweeps inside the backoff window may claim only once")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "ready", item.Status)

	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok, "the failure must surface as an open spawn_failed stuck row")
	assert.Equal(t, int32(1), row.RemediationAttempts)
	require.NotNil(t, row.NextRemediationAt, "the backoff deadline must be durable in next_remediation_at")
}

// A later successful dequeue must not inherit the old failure count.
func TestClearDequeueSpawnFailure_ResetsAttemptsAndResolvesRow(t *testing.T) {
	t.Parallel()
	svc, storage, itemID := newReadyItemThatCannotSpawn(t)
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	_, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok)

	svc.clearDequeueSpawnFailure(t.Context(), itemID)

	_, ok = openSpawnFailedRow(t, storage, itemID)
	assert.False(t, ok, "row must be resolved")
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok)
	assert.Equal(t, int32(1), row.RemediationAttempts, "reopened row must start from a fresh count")
}
