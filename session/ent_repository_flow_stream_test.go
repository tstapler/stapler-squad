package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/flow"
)

var _ flow.EventSource = (*EntFlowReader)(nil)

func TestStreamEvents_should_DeliverItemBatchesEqualToEvents_When_ChunkItemsTwo(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	want := map[flow.ItemID]int{}
	for i := 0; i < 5; i++ {
		item := seedFlowItem(t, c, "stream")
		for k := 0; k < i+1; k++ {
			seedFlowEvent(t, c, item, "ready", "in_progress", base.Add(time.Duration(i*10+k)*time.Minute), "n")
		}
		want[flow.ItemID(item.String())] = i + 1
	}
	r := NewEntFlowReader(c)
	rng := flow.EventRange{From: base.Add(-time.Hour), To: base.Add(time.Hour), ChunkItems: 2}

	var batches [][]flow.Event
	err := r.StreamEvents(context.Background(), rng, func(b []flow.Event) error {
		batches = append(batches, b)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, batches, 3, "5 items at 2 per batch")
	got := map[flow.ItemID]int{}
	for _, b := range batches {
		items := map[flow.ItemID]struct{}{}
		for _, e := range b {
			got[e.ItemID]++
			items[e.ItemID] = struct{}{}
		}
		require.LessOrEqual(t, len(items), 2)
	}
	require.Equal(t, want, got, "a batch holds every event of each of its items")
}

func TestStreamEvents_should_StopAndReturnCallbackError_When_CallbackFails(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedFlowEvent(t, c, seedFlowItem(t, c, "cb"), "idea", "ready", at, "")
	}
	boom := errors.New("boom")
	calls := 0
	err := NewEntFlowReader(c).StreamEvents(context.Background(),
		flow.EventRange{From: at.Add(-time.Hour), ChunkItems: 1},
		func([]flow.Event) error { calls++; return boom })
	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, calls)
}

func TestStreamEvents_should_CancelStatementAndReleaseConnection_When_BatchExceedsTimeout(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	item := seedFlowItem(t, c, "timeout")
	seedFlowEvent(t, c, item, "idea", "ready", at, "")
	r := NewEntFlowReader(c)

	// A 1ns budget has expired before the driver runs the batch statement.
	err := r.StreamEvents(context.Background(),
		flow.EventRange{From: at.Add(-time.Hour), ChunkItems: 1, BatchTimeout: time.Nanosecond},
		func([]flow.Event) error { return nil })
	var be *flow.BatchError
	require.ErrorAs(t, err, &be, "the typed error names the batch")
	require.Equal(t, 1, be.Batch)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// No connection leaked: a read and a write both still complete.
	_, err = r.Events(context.Background(), flow.EventRange{From: at.Add(-time.Hour)})
	require.NoError(t, err)
	seedFlowEvent(t, c, item, "ready", "in_progress", at.Add(time.Minute), "")
}

func TestStreamEvents_should_ReturnNothing_When_RangeHasNoEvents(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	called := false
	err := NewEntFlowReader(repo.GetEntClient()).StreamEvents(context.Background(),
		flow.EventRange{From: time.Now().Add(-time.Hour)},
		func([]flow.Event) error { called = true; return nil })
	require.NoError(t, err)
	require.False(t, called)
}
